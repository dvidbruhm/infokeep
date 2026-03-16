package handlers

import (
	"encoding/json"
	"fmt"
	"log"
	"net/smtp"
	"os"
	"strings"
	"time"

	"infokeep/internal/database"

	webpush "github.com/SherClockHolmes/webpush-go"
)

// StartReminderWorker begins a loop checking for due reminders every minute
func StartReminderWorker() {
	ticker := time.NewTicker(60 * time.Second)
	// Run once immediately on start, then wait for ticks
	checkReminders()

	for range ticker.C {
		checkReminders()
	}
}

func checkReminders() {
	now := time.Now()
	reminders, err := database.GetDueReminders()
	if err != nil {
		log.Printf("Worker: Error fetching reminders: %v", err)
		return
	}

	if len(reminders) > 0 {
		log.Printf("Worker: Found %d total active reminders to evaluate at %s", len(reminders), now.Format("15:04"))
	}

	for _, reminder := range reminders {
		if isReminderDue(reminder, now) {
			fireReminder(reminder, now)
		}
	}
}

func isReminderDue(r database.Reminder, now time.Time) bool {
	// 1. Time Check: Does it match the current HH:MM?
	// Note: We only check HH:MM, ignoring seconds or exact precision logic for simplicity.
	// We expect this ticker to fire roughly once a minute.
	currentTimeStr := now.Format("15:04")
	if r.TimeOfDay != currentTimeStr {
		return false
	}

	// 2. Start/End Date Check
	// Strip time component if present (e.g., "2026-03-12T00:00:00Z" -> "2026-03-12")
	cleanStartDate := strings.Split(r.StartDate, "T")[0]
	startDate, err := time.Parse("2006-01-02", cleanStartDate)
	if err == nil {
		startBound := time.Date(startDate.Year(), startDate.Month(), startDate.Day(), 0, 0, 0, 0, now.Location())
		if now.Before(startBound) {
			return false
		}
	} else {
		log.Printf("Worker Warning: Failed to parse start_date '%s' for reminder %d: %v", r.StartDate, r.ID, err)
	}

	if r.EndDate.Valid && r.EndDate.String != "" {
		cleanEndDate := strings.Split(r.EndDate.String, "T")[0]
		endDate, err := time.Parse("2006-01-02", cleanEndDate)
		if err == nil {
			endBound := time.Date(endDate.Year(), endDate.Month(), endDate.Day(), 23, 59, 59, 0, now.Location())
			if now.After(endBound) {
				return false
			}
		}
	}

	// 3. Frequency check based on last_triggered_at (to avoid duplicates, and respect Daily/Weekly rules)
	var lastTrigger time.Time
	if r.LastTriggeredAt.Valid && r.LastTriggeredAt.String != "" {
		lastTrigger, _ = time.Parse(time.RFC3339, r.LastTriggeredAt.String)
		// If already triggered today, do not fire again
		if lastTrigger.Year() == now.Year() && lastTrigger.YearDay() == now.YearDay() {
			return false
		}
	}

	switch r.Frequency {
	case "Once":
		if r.LastTriggeredAt.Valid {
			return false // Already fired
		}
		return true

	case "Daily":
		return true

	case "Weekly":
		// Only fire on the exact weekday as the start_date
		if startDate.Weekday() == now.Weekday() {
			return true
		}

	case "Monthly":
		// Only fire on the exact day of the month as the start_date
		if startDate.Day() == now.Day() {
			return true
		}

	case "Yearly":
		// Exact month and day
		if startDate.Month() == now.Month() && startDate.Day() == now.Day() {
			return true
		}
	}

	return false
}

// Temporary export for test script
func VerifyReminderDueExported(r database.Reminder, now time.Time) bool {
	return isReminderDue(r, now)
}

func fireReminder(r database.Reminder, now time.Time) {
	log.Printf("Worker: Firing reminder: %s (ID: %d) matching time %s", r.Name, r.ID, r.TimeOfDay)

	// Send Push Notification
	if strings.Contains(r.NotificationType, "notification") || r.NotificationType == "both" {
		log.Printf("Worker: Attempting to send push notification for %d", r.ID)
		sendPushNotification(r)
	}

	// Send Email
	if (strings.Contains(r.NotificationType, "email") || r.NotificationType == "both") && r.Emails.Valid && r.Emails.String != "" {
		sendEmail(r)
	}

	// Update DB to mark as triggered
	err := database.MarkReminderTriggered(r.ID, now)
	if err != nil {
		log.Printf("Worker: Failed to update last_triggered_at for reminder %d: %v", r.ID, err)
	}
}

func sendPushNotification(r database.Reminder) {
	subs, err := database.GetUserPushSubscriptions(r.UserID)
	if err != nil {
		log.Printf("Worker: Failed to get subscriptions for user %d: %v", r.UserID, err)
		return
	}

	if len(subs) == 0 {
		log.Printf("Worker: No push subscriptions found for user %d", r.UserID)
		return // No active browsers for this user to push to
	}

	log.Printf("Worker: Found %d push subscriptions for user %d", len(subs), r.UserID)

	// Setup payload matching the sw.js listener format
	type Payload struct {
		Title string `json:"title"`
		Body  string `json:"body"`
	}
	payloadBytes, _ := json.Marshal(Payload{
		Title: "InfoKeep Reminder",
		Body:  r.Name,
	})

	for _, sub := range subs {
		// Rehydrate the webpush subscription format
		wpSub := &webpush.Subscription{
			Endpoint: sub.Endpoint,
			Keys: webpush.Keys{
				Auth:   sub.Auth,
				P256dh: sub.P256dh,
			},
		}

		// Send it
		resp, err := webpush.SendNotification(payloadBytes, wpSub, &webpush.Options{
			Subscriber:      fmt.Sprintf("mailto:%s", "admin@infokeep.local"),
			VAPIDPublicKey:  VapidPublicKey,
			VAPIDPrivateKey: VapidPrivateKey,
			TTL:             43200, // 12 hours
		})

		if err != nil {
			log.Printf("Worker: WebPush failed for endpoint %s: %v", sub.Endpoint, err)
			if resp != nil && (resp.StatusCode == 410 || resp.StatusCode == 404) {
				log.Printf("Worker: Subscription is invalid or expired. Deleting it.")
				database.DeletePushSubscription(sub.Endpoint)
			}
		} else {
			resp.Body.Close() // Ensure connection is closed
		}
	}
}

// sendEmail sends an email if SMTP env vars differ from default
func sendEmail(r database.Reminder) {
	smtpHost := os.Getenv("SMTP_HOST")
	smtpPort := os.Getenv("SMTP_PORT")
	smtpUser := os.Getenv("SMTP_USER")
	smtpPass := os.Getenv("SMTP_PASS")
	smtpFrom := os.Getenv("SMTP_FROM")

	if smtpHost == "" || smtpPort == "" {
		log.Printf("Worker: Cannot send email for reminder %d, missing SMTP config", r.ID)
		return
	}
	if smtpFrom == "" {
		smtpFrom = "noreply@infokeep.local"
	}

	auth := smtp.PlainAuth("", smtpUser, smtpPass, smtpHost)
	to := strings.Split(r.Emails.String, ",") // Handle comma-separated list of emails

	msg := []byte("To: " + r.Emails.String + "\r\n" +
		"Subject: InfoKeep Reminder: " + r.Name + "\r\n" +
		"\r\n" +
		"This is an automated reminder from your InfoKeep dashboard regarding: " + r.Name + ".\r\n")

	err := smtp.SendMail(smtpHost+":"+smtpPort, auth, smtpFrom, to, msg)
	if err != nil {
		log.Printf("Worker: Failed to send email for reminder %d: %v", r.ID, err)
	} else {
		log.Printf("Worker: Successfully sent reminder email for %d to %s", r.ID, r.Emails.String)
	}
}
