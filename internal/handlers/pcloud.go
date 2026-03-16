package handlers

import (
	"encoding/json"
	"fmt"
	"infokeep/internal/database"
	"io"
	"log"
	"mime/multipart"
	"net/http"
	"net/url"
	"os"
	"strconv"
	"time"
)

// pCloud OAuth2 configuration (set via environment variables)
var (
	pcloudClientID     = os.Getenv("PCLOUD_CLIENT_ID")
	pcloudClientSecret = os.Getenv("PCLOUD_CLIENT_SECRET")
)

// DBPath is set from main.go so the backup scheduler knows which file to upload
var DBPath string

// PCloudLinkHandler redirects the user to pCloud's OAuth2 authorize page
func PCloudLinkHandler(w http.ResponseWriter, r *http.Request) {
	if pcloudClientID == "" {
		http.Error(w, "pCloud integration not configured (PCLOUD_CLIENT_ID not set)", http.StatusInternalServerError)
		return
	}

	// Build the redirect URI from the current request
	scheme := "https"
	if r.TLS == nil {
		if fwd := r.Header.Get("X-Forwarded-Proto"); fwd != "" {
			scheme = fwd
		} else {
			scheme = "http"
		}
	}
	redirectURI := fmt.Sprintf("%s://%s/settings/pcloud/callback", scheme, r.Host)

	authorizeURL := fmt.Sprintf(
		"https://my.pcloud.com/oauth2/authorize?client_id=%s&response_type=code&redirect_uri=%s",
		url.QueryEscape(pcloudClientID),
		url.QueryEscape(redirectURI),
	)

	http.Redirect(w, r, authorizeURL, http.StatusFound)
}

// PCloudCallbackHandler handles the OAuth2 callback from pCloud
func PCloudCallbackHandler(w http.ResponseWriter, r *http.Request) {
	code := r.URL.Query().Get("code")
	hostname := r.URL.Query().Get("hostname")
	locationID := r.URL.Query().Get("locationid")

	if code == "" {
		http.Error(w, "Authorization denied or failed", http.StatusBadRequest)
		return
	}

	// Default hostname based on location
	if hostname == "" {
		if locationID == "2" {
			hostname = "eapi.pcloud.com"
		} else {
			hostname = "api.pcloud.com"
		}
	}

	// Exchange code for access token
	scheme := "https"
	if r.TLS == nil {
		if fwd := r.Header.Get("X-Forwarded-Proto"); fwd != "" {
			scheme = fwd
		} else {
			scheme = "http"
		}
	}
	redirectURI := fmt.Sprintf("%s://%s/settings/pcloud/callback", scheme, r.Host)

	tokenURL := fmt.Sprintf("https://%s/oauth2_token?client_id=%s&client_secret=%s&code=%s&redirect_uri=%s",
		hostname,
		url.QueryEscape(pcloudClientID),
		url.QueryEscape(pcloudClientSecret),
		url.QueryEscape(code),
		url.QueryEscape(redirectURI),
	)

	resp, err := http.Get(tokenURL)
	if err != nil {
		log.Printf("pCloud token exchange failed: %v", err)
		http.Error(w, "Failed to exchange authorization code", http.StatusInternalServerError)
		return
	}
	defer resp.Body.Close()

	var tokenResp struct {
		AccessToken string `json:"access_token"`
		TokenType   string `json:"token_type"`
		UserID      int64  `json:"userid"`
		Error       int    `json:"error"`
		Message     string `json:"message"`
	}

	if err := json.NewDecoder(resp.Body).Decode(&tokenResp); err != nil {
		log.Printf("pCloud token decode failed: %v", err)
		http.Error(w, "Failed to parse token response", http.StatusInternalServerError)
		return
	}

	if tokenResp.Error != 0 {
		log.Printf("pCloud token error: %s", tokenResp.Message)
		http.Error(w, fmt.Sprintf("pCloud error: %s", tokenResp.Message), http.StatusBadRequest)
		return
	}

	// Save credentials
	userID := getUserID(r)
	if err := database.SetPCloudCredentials(userID, tokenResp.AccessToken, hostname); err != nil {
		log.Printf("Failed to save pCloud credentials: %v", err)
		http.Error(w, "Failed to save credentials", http.StatusInternalServerError)
		return
	}

	log.Printf("pCloud account linked successfully for user %d", userID)
	http.Redirect(w, r, "/settings?pcloud=linked", http.StatusFound)
}

// PCloudUnlinkHandler removes pCloud credentials
func PCloudUnlinkHandler(w http.ResponseWriter, r *http.Request) {
	userID := getUserID(r)
	if err := database.ClearPCloudCredentials(userID); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]string{"status": "unlinked"})
}

// PCloudUpdateIntervalHandler saves the backup interval
func PCloudUpdateIntervalHandler(w http.ResponseWriter, r *http.Request) {
	userID := getUserID(r)
	daysStr := r.FormValue("days")
	days, err := strconv.Atoi(daysStr)
	if err != nil || days < 1 {
		days = 7
	}
	if days > 365 {
		days = 365
	}

	if err := database.SetBackupInterval(userID, days); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{"status": "saved", "days": days})
}

// PCloudBackupNowHandler triggers an immediate backup
func PCloudBackupNowHandler(w http.ResponseWriter, r *http.Request) {
	userID := getUserID(r)
	token, hostname, err := database.GetPCloudCredentials(userID)
	if err != nil || token == "" {
		http.Error(w, "pCloud not linked", http.StatusBadRequest)
		return
	}

	go func() {
		if err := performBackup(userID, token, hostname); err != nil {
			log.Printf("Manual backup failed for user %d: %v", userID, err)
		}
	}()

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]string{"status": "backup_started"})
}

// PCloudStatusHandler returns the current pCloud link status and backup info
func PCloudStatusHandler(w http.ResponseWriter, r *http.Request) {
	userID := getUserID(r)
	token, _, err := database.GetPCloudCredentials(userID)
	if err != nil {
		token = ""
	}

	interval, lastBackup, _ := database.GetBackupSettings(userID)

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{
		"linked":      token != "",
		"interval":    interval,
		"last_backup": lastBackup,
	})
}

// performBackup copies the database file and uploads it to pCloud
func performBackup(userID int64, accessToken, hostname string) error {
	if DBPath == "" {
		return fmt.Errorf("database path not configured")
	}

	// Create a copy of the database to avoid locking issues
	tmpPath := DBPath + ".backup"
	if err := copyFile(DBPath, tmpPath); err != nil {
		return fmt.Errorf("failed to copy database: %w", err)
	}
	defer os.Remove(tmpPath)

	// Ensure the backup folder exists on pCloud
	folderID, err := ensurePCloudFolder(accessToken, hostname, "InfoKeep Backups")
	if err != nil {
		return fmt.Errorf("failed to create backup folder: %w", err)
	}

	// Upload the file
	filename := fmt.Sprintf("infokeep_backup_%s.db", time.Now().Format("2006-01-02"))
	if err := uploadToPCloud(accessToken, hostname, tmpPath, folderID, filename); err != nil {
		return fmt.Errorf("failed to upload backup: %w", err)
	}

	// Update last backup time
	if err := database.SetLastBackupTime(userID, time.Now()); err != nil {
		log.Printf("Failed to update last backup time: %v", err)
	}

	log.Printf("Backup completed successfully for user %d: %s", userID, filename)
	return nil
}

// copyFile creates a copy of src at dst
func copyFile(src, dst string) error {
	source, err := os.Open(src)
	if err != nil {
		return err
	}
	defer source.Close()

	dest, err := os.Create(dst)
	if err != nil {
		return err
	}
	defer dest.Close()

	_, err = io.Copy(dest, source)
	return err
}

// ensurePCloudFolder creates a folder in pCloud root if it doesn't exist, returns folder ID
func ensurePCloudFolder(accessToken, hostname, folderName string) (int64, error) {
	// First, try to list the root folder to find if it exists
	listURL := fmt.Sprintf("https://%s/listfolder?access_token=%s&path=/",
		hostname, url.QueryEscape(accessToken))

	resp, err := http.Get(listURL)
	if err != nil {
		return 0, err
	}
	defer resp.Body.Close()

	var listResp struct {
		Metadata struct {
			Contents []struct {
				Name     string `json:"name"`
				FolderID int64  `json:"folderid"`
				IsFolder bool   `json:"isfolder"`
			} `json:"contents"`
		} `json:"metadata"`
	}

	if err := json.NewDecoder(resp.Body).Decode(&listResp); err != nil {
		return 0, err
	}

	// Check if folder already exists
	for _, item := range listResp.Metadata.Contents {
		if item.IsFolder && item.Name == folderName {
			return item.FolderID, nil
		}
	}

	// Create the folder
	createURL := fmt.Sprintf("https://%s/createfolder?access_token=%s&path=/%s",
		hostname, url.QueryEscape(accessToken), url.PathEscape(folderName))

	resp2, err := http.Get(createURL)
	if err != nil {
		return 0, err
	}
	defer resp2.Body.Close()

	var createResp struct {
		Metadata struct {
			FolderID int64 `json:"folderid"`
		} `json:"metadata"`
		Error   int    `json:"error"`
		Message string `json:"message"`
	}

	if err := json.NewDecoder(resp2.Body).Decode(&createResp); err != nil {
		return 0, err
	}

	if createResp.Error != 0 {
		return 0, fmt.Errorf("pCloud createfolder error: %s", createResp.Message)
	}

	return createResp.Metadata.FolderID, nil
}

// uploadToPCloud uploads a file to a specific folder on pCloud
func uploadToPCloud(accessToken, hostname, filePath string, folderID int64, filename string) error {
	file, err := os.Open(filePath)
	if err != nil {
		return err
	}
	defer file.Close()

	// Create multipart form
	pr, pw := io.Pipe()
	writer := multipart.NewWriter(pw)

	go func() {
		defer pw.Close()
		defer writer.Close()

		part, err := writer.CreateFormFile("file", filename)
		if err != nil {
			pw.CloseWithError(err)
			return
		}
		if _, err := io.Copy(part, file); err != nil {
			pw.CloseWithError(err)
			return
		}
	}()

	uploadURL := fmt.Sprintf("https://%s/uploadfile?access_token=%s&folderid=%d&filename=%s&renameifexists=1",
		hostname,
		url.QueryEscape(accessToken),
		folderID,
		url.QueryEscape(filename),
	)

	req, err := http.NewRequest("POST", uploadURL, pr)
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", writer.FormDataContentType())

	client := &http.Client{Timeout: 5 * time.Minute}
	resp, err := client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	var uploadResp struct {
		Error   int    `json:"error"`
		Message string `json:"message"`
	}

	body, _ := io.ReadAll(resp.Body)
	if err := json.Unmarshal(body, &uploadResp); err != nil {
		return fmt.Errorf("failed to parse upload response: %w", err)
	}

	if uploadResp.Error != 0 {
		return fmt.Errorf("pCloud upload error %d: %s", uploadResp.Error, uploadResp.Message)
	}

	return nil
}

// StartBackupScheduler runs a background goroutine that checks for due backups every hour
func StartBackupScheduler(dbPath string) {
	DBPath = dbPath
	ticker := time.NewTicker(1 * time.Hour)
	defer ticker.Stop()

	log.Println("Backup scheduler started (pCloud + Google Drive)")

	// Run an initial check after a short delay
	time.Sleep(30 * time.Second)
	checkAndRunBackups()
	checkAndRunGDriveBackups()

	for range ticker.C {
		checkAndRunBackups()
		checkAndRunGDriveBackups()
	}
}

func checkAndRunBackups() {
	users, err := database.GetAllUsersWithPCloud()
	if err != nil {
		log.Printf("Backup scheduler: failed to get users: %v", err)
		return
	}

	for _, u := range users {
		if isBackupDue(u) {
			log.Printf("Backup due for user %d, starting...", u.UserID)
			if err := performBackup(u.UserID, u.AccessToken, u.Hostname); err != nil {
				log.Printf("Scheduled backup failed for user %d: %v", u.UserID, err)
			}
		}
	}
}

func isBackupDue(u database.UserBackupInfo) bool {
	if !u.LastBackupAt.Valid || u.LastBackupAt.String == "" {
		return true // Never backed up
	}

	lastBackup, err := time.Parse("2006-01-02T15:04:05Z", u.LastBackupAt.String)
	if err != nil {
		// Try alternative format
		lastBackup, err = time.Parse("2006-01-02 15:04:05", u.LastBackupAt.String)
		if err != nil {
			// Try with timezone
			lastBackup, err = time.Parse(time.RFC3339, u.LastBackupAt.String)
			if err != nil {
				log.Printf("Failed to parse last backup time for user %d: %v", u.UserID, err)
				return true // Can't parse, assume due
			}
		}
	}

	nextDue := lastBackup.AddDate(0, 0, u.BackupIntervalDays)
	return time.Now().After(nextDue)
}

// splitHostPort is a helper if needed
func getBaseURL(r *http.Request) string {
	scheme := "https"
	if r.TLS == nil {
		if fwd := r.Header.Get("X-Forwarded-Proto"); fwd != "" {
			scheme = fwd
		} else {
			scheme = "http"
		}
	}
	return fmt.Sprintf("%s://%s", scheme, r.Host)
}
