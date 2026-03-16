package handlers

import (
	"encoding/json"
	"log"
	"net/http"
	"os"
	"strconv"
	"strings"

	"infokeep/internal/database"

	webpush "github.com/SherClockHolmes/webpush-go"
)

// Ensure VAPID keys exist on startup
var (
	VapidPublicKey  string
	VapidPrivateKey string
)

func InitVAPIDKeys() {
	// 1. Try environment variables first (allows manual override)
	VapidPublicKey = os.Getenv("VAPID_PUBLIC_KEY")
	VapidPrivateKey = os.Getenv("VAPID_PRIVATE_KEY")

	// 2. Try Database if not in env
	if VapidPublicKey == "" || VapidPrivateKey == "" {
		dbPub, err1 := database.GetSystemSetting("vapid_public_key")
		dbPriv, err2 := database.GetSystemSetting("vapid_private_key")
		if err1 == nil && err2 == nil && dbPub != "" && dbPriv != "" {
			VapidPublicKey = dbPub
			VapidPrivateKey = dbPriv
			log.Println("Loaded existing VAPID keys from the database.")
		}
	}

	// 3. Generate new keys and save to Database if none exist
	if VapidPublicKey == "" || VapidPrivateKey == "" {
		log.Println("VAPID keys not found in DB or environment. Generating new ones...")
		privateKey, publicKey, err := webpush.GenerateVAPIDKeys()
		if err != nil {
			log.Fatalf("Failed to generate VAPID keys: %v", err)
		}
		VapidPublicKey = publicKey
		VapidPrivateKey = privateKey

		err1 := database.SetSystemSetting("vapid_public_key", publicKey)
		err2 := database.SetSystemSetting("vapid_private_key", privateKey)

		if err1 != nil || err2 != nil {
			log.Printf("Warning: Could not save VAPID keys to database: %v", err1)
		} else {
			log.Println("Saved new VAPID keys to the database successfully.")
		}
	}
}

// RemindersPageHandler renders the frontend UI for reminders
func RemindersPageHandler(w http.ResponseWriter, r *http.Request) {
	userID := getUserID(r)

	reminders, err := database.GetRemindersForUser(userID)
	if err != nil {
		http.Error(w, "Failed to load reminders", http.StatusInternalServerError)
		return
	}

	tags, _ := database.GetTagsWithCounts(userID)

	data := map[string]interface{}{
		"Reminders":      reminders,
		"VapidPublicKey": VapidPublicKey,
		"Tags":           tags,
		"ActiveTag":      "",
	}

	RenderTemplate(w, "reminders.html", data)
}

// AddReminderHandler handles the form submission for a new reminder
func AddReminderHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}
	userID := getUserID(r)

	err := r.ParseForm()
	if err != nil {
		http.Error(w, "Failed to parse form", http.StatusBadRequest)
		return
	}

	name := r.FormValue("name")
	frequency := r.FormValue("frequency")
	timeOfDay := r.FormValue("time_of_day")
	startDate := r.FormValue("start_date")
	endDate := r.FormValue("end_date")
	notificationType := r.FormValue("notification_type")
	emails := r.FormValue("emails")

	if name == "" || frequency == "" || timeOfDay == "" || startDate == "" {
		http.Error(w, "Missing required fields", http.StatusBadRequest)
		return
	}

	reminder := &database.Reminder{
		UserID:           userID,
		Name:             name,
		Frequency:        frequency,
		TimeOfDay:        timeOfDay,
		StartDate:        startDate,
		NotificationType: notificationType,
	}

	if frequency != "Once" && endDate != "" {
		reminder.EndDate.String = endDate
		reminder.EndDate.Valid = true
	}

	if strings.Contains(notificationType, "email") && emails != "" {
		reminder.Emails.String = emails
		reminder.Emails.Valid = true
	}

	_, err = database.CreateReminder(reminder)
	if err != nil {
		log.Printf("Error creating reminder: %v", err)
		http.Error(w, "Failed to create reminder", http.StatusInternalServerError)
		return
	}

	// Standard full-page redirect to clear POST data and reload the UI
	http.Redirect(w, r, "/reminders", http.StatusSeeOther)
}

// DeleteReminderHandler handles deleting a reminder
func DeleteReminderHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodDelete {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}
	userID := getUserID(r)
	pathParts := strings.Split(r.URL.Path, "/")
	if len(pathParts) < 3 {
		http.Error(w, "Missing reminder ID", http.StatusBadRequest)
		return
	}
	id, err := strconv.ParseInt(pathParts[2], 10, 64)
	if err != nil {
		http.Error(w, "Invalid ID", http.StatusBadRequest)
		return
	}

	err = database.DeleteReminder(id, userID)
	if err != nil {
		log.Printf("Error deleting reminder: %v", err)
		http.Error(w, "Failed to delete reminder", http.StatusInternalServerError)
		return
	}

	w.WriteHeader(http.StatusOK)
}

// SavePushSubscriptionHandler saves the browser's push subscription
func SavePushSubscriptionHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}
	userID := getUserID(r)

	var sub struct {
		Endpoint string `json:"endpoint"`
		Keys     struct {
			P256dh string `json:"p256dh"`
			Auth   string `json:"auth"`
		} `json:"keys"`
	}

	if err := json.NewDecoder(r.Body).Decode(&sub); err != nil {
		http.Error(w, "Invalid subscription JSON", http.StatusBadRequest)
		return
	}

	pushSub := &database.PushSubscription{
		UserID:   userID,
		Endpoint: sub.Endpoint,
		P256dh:   sub.Keys.P256dh,
		Auth:     sub.Keys.Auth,
	}

	err := database.SavePushSubscription(pushSub)
	if err != nil {
		log.Printf("Error saving push subscription: %v", err)
		http.Error(w, "Failed to save subscription", http.StatusInternalServerError)
		return
	}

	w.WriteHeader(http.StatusOK)
	w.Write([]byte(`{"status":"subscribed"}`))
}
