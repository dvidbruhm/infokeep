package handlers

import (
	"bytes"
	"encoding/json"
	"fmt"
	"infokeep/internal/database"
	"io"
	"log"
	"mime/multipart"
	"net/http"
	"net/textproto"
	"net/url"
	"os"
	"time"
)

// Google Drive OAuth2 configuration (set via environment variables)
var (
	gdriveClientID     = os.Getenv("GDRIVE_CLIENT_ID")
	gdriveClientSecret = os.Getenv("GDRIVE_CLIENT_SECRET")
)

// GDriveLinkHandler redirects the user to Google's OAuth2 consent page
func GDriveLinkHandler(w http.ResponseWriter, r *http.Request) {
	if gdriveClientID == "" {
		http.Error(w, "Google Drive integration not configured (GDRIVE_CLIENT_ID not set)", http.StatusInternalServerError)
		return
	}

	redirectURI := getBaseURL(r) + "/settings/gdrive/callback"

	authorizeURL := fmt.Sprintf(
		"https://accounts.google.com/o/oauth2/v2/auth?client_id=%s&redirect_uri=%s&response_type=code&scope=%s&access_type=offline&prompt=consent",
		url.QueryEscape(gdriveClientID),
		url.QueryEscape(redirectURI),
		url.QueryEscape("https://www.googleapis.com/auth/drive.file"),
	)

	http.Redirect(w, r, authorizeURL, http.StatusFound)
}

// GDriveCallbackHandler handles the OAuth2 callback from Google
func GDriveCallbackHandler(w http.ResponseWriter, r *http.Request) {
	code := r.URL.Query().Get("code")
	if code == "" {
		errMsg := r.URL.Query().Get("error")
		log.Printf("Google Drive auth denied: %s", errMsg)
		http.Redirect(w, r, "/settings?gdrive=error", http.StatusFound)
		return
	}

	redirectURI := getBaseURL(r) + "/settings/gdrive/callback"

	// Exchange authorization code for tokens
	data := url.Values{
		"code":          {code},
		"client_id":     {gdriveClientID},
		"client_secret": {gdriveClientSecret},
		"redirect_uri":  {redirectURI},
		"grant_type":    {"authorization_code"},
	}

	resp, err := http.PostForm("https://oauth2.googleapis.com/token", data)
	if err != nil {
		log.Printf("Google Drive token exchange failed: %v", err)
		http.Error(w, "Failed to exchange authorization code", http.StatusInternalServerError)
		return
	}
	defer resp.Body.Close()

	var tokenResp struct {
		AccessToken  string `json:"access_token"`
		RefreshToken string `json:"refresh_token"`
		ExpiresIn    int    `json:"expires_in"`
		TokenType    string `json:"token_type"`
		Error        string `json:"error"`
		ErrorDesc    string `json:"error_description"`
	}

	if err := json.NewDecoder(resp.Body).Decode(&tokenResp); err != nil {
		log.Printf("Google Drive token decode failed: %v", err)
		http.Error(w, "Failed to parse token response", http.StatusInternalServerError)
		return
	}

	if tokenResp.Error != "" {
		log.Printf("Google Drive token error: %s - %s", tokenResp.Error, tokenResp.ErrorDesc)
		http.Error(w, fmt.Sprintf("Google error: %s", tokenResp.ErrorDesc), http.StatusBadRequest)
		return
	}

	if tokenResp.RefreshToken == "" {
		log.Printf("Google Drive: no refresh token received (user may have already authorized before)")
		http.Error(w, "No refresh token received. Try unlinking first, then re-link.", http.StatusBadRequest)
		return
	}

	// Save credentials
	userID := getUserID(r)
	if err := database.SetGDriveCredentials(userID, tokenResp.AccessToken, tokenResp.RefreshToken); err != nil {
		log.Printf("Failed to save Google Drive credentials: %v", err)
		http.Error(w, "Failed to save credentials", http.StatusInternalServerError)
		return
	}

	log.Printf("Google Drive account linked successfully for user %d", userID)
	http.Redirect(w, r, "/settings?gdrive=linked", http.StatusFound)
}

// GDriveUnlinkHandler removes Google Drive credentials
func GDriveUnlinkHandler(w http.ResponseWriter, r *http.Request) {
	userID := getUserID(r)
	if err := database.ClearGDriveCredentials(userID); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]string{"status": "unlinked"})
}

// GDriveBackupNowHandler triggers an immediate backup to Google Drive
func GDriveBackupNowHandler(w http.ResponseWriter, r *http.Request) {
	userID := getUserID(r)
	accessToken, refreshToken, err := database.GetGDriveCredentials(userID)
	if err != nil || refreshToken == "" {
		http.Error(w, "Google Drive not linked", http.StatusBadRequest)
		return
	}

	go func() {
		if err := performGDriveBackup(userID, accessToken, refreshToken); err != nil {
			log.Printf("Manual Google Drive backup failed for user %d: %v", userID, err)
		}
	}()

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]string{"status": "backup_started"})
}

// refreshGDriveToken uses the refresh token to get a new access token
func refreshGDriveToken(userID int64, refreshToken string) (string, error) {
	data := url.Values{
		"client_id":     {gdriveClientID},
		"client_secret": {gdriveClientSecret},
		"refresh_token": {refreshToken},
		"grant_type":    {"refresh_token"},
	}

	resp, err := http.PostForm("https://oauth2.googleapis.com/token", data)
	if err != nil {
		return "", fmt.Errorf("refresh request failed: %w", err)
	}
	defer resp.Body.Close()

	var tokenResp struct {
		AccessToken string `json:"access_token"`
		ExpiresIn   int    `json:"expires_in"`
		Error       string `json:"error"`
		ErrorDesc   string `json:"error_description"`
	}

	if err := json.NewDecoder(resp.Body).Decode(&tokenResp); err != nil {
		return "", fmt.Errorf("decode failed: %w", err)
	}

	if tokenResp.Error != "" {
		return "", fmt.Errorf("refresh error: %s - %s", tokenResp.Error, tokenResp.ErrorDesc)
	}

	// Save updated access token
	if err := database.UpdateGDriveAccessToken(userID, tokenResp.AccessToken); err != nil {
		log.Printf("Failed to save refreshed access token: %v", err)
	}

	return tokenResp.AccessToken, nil
}

// performGDriveBackup copies the database and uploads it to Google Drive
func performGDriveBackup(userID int64, accessToken, refreshToken string) error {
	if DBPath == "" {
		return fmt.Errorf("database path not configured")
	}

	// Always refresh the token first (access tokens expire after 1 hour)
	newToken, err := refreshGDriveToken(userID, refreshToken)
	if err != nil {
		return fmt.Errorf("failed to refresh token: %w", err)
	}
	accessToken = newToken

	// Create a copy of the database
	tmpPath := DBPath + ".gdrive-backup"
	if err := copyFile(DBPath, tmpPath); err != nil {
		return fmt.Errorf("failed to copy database: %w", err)
	}
	defer os.Remove(tmpPath)

	// Ensure the backup folder exists
	folderID, err := ensureGDriveFolder(userID, accessToken)
	if err != nil {
		return fmt.Errorf("failed to create backup folder: %w", err)
	}

	// Upload the file
	filename := fmt.Sprintf("infokeep_backup_%s.db", time.Now().Format("2006-01-02"))
	if err := uploadToGDrive(accessToken, tmpPath, folderID, filename); err != nil {
		return fmt.Errorf("failed to upload backup: %w", err)
	}

	// Update last backup time
	if err := database.SetLastBackupTime(userID, time.Now()); err != nil {
		log.Printf("Failed to update last backup time: %v", err)
	}

	log.Printf("Google Drive backup completed successfully for user %d: %s", userID, filename)
	return nil
}

// ensureGDriveFolder finds or creates an "InfoKeep Backups" folder in Google Drive
func ensureGDriveFolder(userID int64, accessToken string) (string, error) {
	// Check if we have a cached folder ID
	cachedID, _ := database.GetGDriveFolderID(userID)
	if cachedID != "" {
		// Verify it still exists
		checkURL := fmt.Sprintf("https://www.googleapis.com/drive/v3/files/%s?fields=id,trashed", cachedID)
		req, _ := http.NewRequest("GET", checkURL, nil)
		req.Header.Set("Authorization", "Bearer "+accessToken)
		resp, err := http.DefaultClient.Do(req)
		if err == nil {
			defer resp.Body.Close()
			if resp.StatusCode == 200 {
				var file struct {
					ID      string `json:"id"`
					Trashed bool   `json:"trashed"`
				}
				json.NewDecoder(resp.Body).Decode(&file)
				if !file.Trashed {
					return cachedID, nil
				}
			}
		}
	}

	// Search for existing folder
	searchURL := fmt.Sprintf(
		"https://www.googleapis.com/drive/v3/files?q=%s&spaces=drive&fields=files(id,name)",
		url.QueryEscape("name='InfoKeep Backups' and mimeType='application/vnd.google-apps.folder' and trashed=false"),
	)
	req, _ := http.NewRequest("GET", searchURL, nil)
	req.Header.Set("Authorization", "Bearer "+accessToken)

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	var searchResp struct {
		Files []struct {
			ID   string `json:"id"`
			Name string `json:"name"`
		} `json:"files"`
	}
	json.NewDecoder(resp.Body).Decode(&searchResp)

	if len(searchResp.Files) > 0 {
		folderID := searchResp.Files[0].ID
		database.SetGDriveFolderID(userID, folderID)
		return folderID, nil
	}

	// Create the folder
	metadata := map[string]interface{}{
		"name":     "InfoKeep Backups",
		"mimeType": "application/vnd.google-apps.folder",
	}
	body, _ := json.Marshal(metadata)

	req, _ = http.NewRequest("POST", "https://www.googleapis.com/drive/v3/files", bytes.NewReader(body))
	req.Header.Set("Authorization", "Bearer "+accessToken)
	req.Header.Set("Content-Type", "application/json")

	resp2, err := http.DefaultClient.Do(req)
	if err != nil {
		return "", err
	}
	defer resp2.Body.Close()

	var createResp struct {
		ID    string `json:"id"`
		Error *struct {
			Message string `json:"message"`
		} `json:"error"`
	}
	json.NewDecoder(resp2.Body).Decode(&createResp)

	if createResp.Error != nil {
		return "", fmt.Errorf("Drive folder creation error: %s", createResp.Error.Message)
	}

	database.SetGDriveFolderID(userID, createResp.ID)
	return createResp.ID, nil
}

// uploadToGDrive uploads a file to Google Drive using multipart upload
func uploadToGDrive(accessToken, filePath, folderID, filename string) error {
	file, err := os.Open(filePath)
	if err != nil {
		return err
	}
	defer file.Close()

	fileInfo, err := file.Stat()
	if err != nil {
		return err
	}

	// Build multipart/related body
	var buf bytes.Buffer
	writer := multipart.NewWriter(&buf)

	// Part 1: JSON metadata
	metaHeader := make(textproto.MIMEHeader)
	metaHeader.Set("Content-Type", "application/json; charset=UTF-8")
	metaPart, err := writer.CreatePart(metaHeader)
	if err != nil {
		return err
	}
	metadata := map[string]interface{}{
		"name":    filename,
		"parents": []string{folderID},
	}
	json.NewEncoder(metaPart).Encode(metadata)

	// Part 2: File content
	fileHeader := make(textproto.MIMEHeader)
	fileHeader.Set("Content-Type", "application/x-sqlite3")
	filePart, err := writer.CreatePart(fileHeader)
	if err != nil {
		return err
	}
	if _, err := io.Copy(filePart, file); err != nil {
		return err
	}
	writer.Close()

	uploadURL := "https://www.googleapis.com/upload/drive/v3/files?uploadType=multipart"

	req, err := http.NewRequest("POST", uploadURL, &buf)
	if err != nil {
		return err
	}
	req.Header.Set("Authorization", "Bearer "+accessToken)
	req.Header.Set("Content-Type", "multipart/related; boundary="+writer.Boundary())

	client := &http.Client{Timeout: 5 * time.Minute}
	resp, err := client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode != 200 {
		body, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("Google Drive upload failed (status %d): %s", resp.StatusCode, string(body))
	}

	log.Printf("Google Drive upload successful: %s (%d bytes)", filename, fileInfo.Size())
	return nil
}

// checkAndRunGDriveBackups is called by the scheduler to run Google Drive backups
func checkAndRunGDriveBackups() {
	users, err := database.GetAllUsersWithGDrive()
	if err != nil {
		log.Printf("GDrive backup scheduler: failed to get users: %v", err)
		return
	}

	for _, u := range users {
		if isGDriveBackupDue(u) {
			log.Printf("Google Drive backup due for user %d, starting...", u.UserID)
			if err := performGDriveBackup(u.UserID, u.AccessToken, u.RefreshToken); err != nil {
				log.Printf("Scheduled Google Drive backup failed for user %d: %v", u.UserID, err)
			}
		}
	}
}

func isGDriveBackupDue(u database.UserGDriveBackupInfo) bool {
	if !u.LastBackupAt.Valid || u.LastBackupAt.String == "" {
		return true
	}

	lastBackup, err := time.Parse("2006-01-02T15:04:05Z", u.LastBackupAt.String)
	if err != nil {
		lastBackup, err = time.Parse("2006-01-02 15:04:05", u.LastBackupAt.String)
		if err != nil {
			lastBackup, err = time.Parse(time.RFC3339, u.LastBackupAt.String)
			if err != nil {
				return true
			}
		}
	}

	nextDue := lastBackup.AddDate(0, 0, u.BackupIntervalDays)
	return time.Now().After(nextDue)
}
