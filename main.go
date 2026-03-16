package main

import (
	"infokeep/internal/database"
	"infokeep/internal/handlers"
	"log"
	"net/http"
	"os"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"
)

func main() {
	// Initialize database
	dbPath := "infokeep.db"
	if err := database.InitDB(dbPath); err != nil {
		log.Fatalf("Failed to initialize database: %v", err)
	}
	defer database.DB.Close()

	// Start backup scheduler in background
	go handlers.StartBackupScheduler(dbPath)

	// Initialize Web Push VAPID keys
	handlers.InitVAPIDKeys()
	// Start the Reminders scheduler
	go handlers.StartReminderWorker()

	r := chi.NewRouter()

	// Middleware
	r.Use(middleware.Logger)
	r.Use(middleware.Recoverer)

	// Static files
	workDir, _ := os.Getwd()
	filesDir := http.Dir(workDir + "/web/static")
	r.Handle("/static/*", http.StripPrefix("/static/", http.FileServer(filesDir)))

	// Service worker must be served from root for full scope
	r.Get("/sw.js", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/javascript")
		http.ServeFile(w, r, workDir+"/web/static/sw.js")
	})

	// Routes
	r.Get("/login", handlers.LoginHandler)
	r.Post("/login", handlers.LoginHandler)
	r.Get("/register", handlers.RegisterHandler)
	r.Post("/register", handlers.RegisterHandler)
	r.Post("/logout", handlers.LogoutHandler)

	// Public Share Route (No Auth Required)
	r.Get("/shared/{hash}", handlers.PublicViewHandler)

	// Protected Routes
	r.Group(func(r chi.Router) {
		r.Use(handlers.AuthMiddleware)

		r.Get("/", handlers.IndexHandler)
		r.Get("/share", handlers.ShareHandler)
		r.Get("/bookmarks", handlers.BookmarkHandler)
		r.Post("/bookmarks", handlers.BookmarkHandler)
		r.Get("/bookmarks/{id}", handlers.GetBookmarkHandler)
		r.Post("/bookmarks/{id}", handlers.UpdateBookmarkHandler)
		r.Get("/notes", handlers.NoteHandler)
		r.Post("/notes", handlers.NoteHandler)
		r.Get("/notes/{id}", handlers.GetNoteHandler)
		r.Post("/notes/{id}", handlers.UpdateNoteHandler)
		r.Get("/rated-lists", handlers.RatedListHandler)
		r.Post("/rated-lists", handlers.RatedListHandler)
		r.Get("/rated-lists/{id}/items", handlers.RatedListItemHandler)
		r.Post("/rated-lists/{id}/items", handlers.RatedListItemHandler)
		r.Get("/rated-list-items/{id}", handlers.GetRatedListItemHandler)
		r.Post("/rated-list-items/{id}", handlers.UpdateRatedListItemHandler)
		r.Get("/drawings", handlers.DrawingsHandler)
		r.Post("/drawings", handlers.CreateDrawingHandler)
		r.Get("/drawings/{id}", handlers.GetDrawingHandler)
		r.Post("/drawings/{id}", handlers.UpdateDrawingHandler)
		r.Get("/lists", handlers.ListHandler)
		r.Post("/lists", handlers.ListHandler)
		r.Get("/lists/{id}/items", handlers.ListItemHandler)
		r.Post("/lists/{id}/items", handlers.ListItemHandler)
		r.Get("/list-items/{id}", handlers.GetListItemByIdHandler)
		r.Post("/list-items/{id}", handlers.UpdateListItemHandler)
		r.Post("/list-items/{itemID}/toggle", handlers.ToggleListItemHandler)
		r.Get("/media", handlers.MediaHandler)
		r.Post("/media", handlers.MediaHandler)
		r.Get("/recipes", handlers.RecipeHandler)
		r.Post("/recipes", handlers.CreateRecipeHandler)
		r.Get("/recipes/import", handlers.ImportRecipeHandler)
		r.Post("/recipes/share-import", handlers.ShareImportRecipeHandler)
		r.Get("/recipes/{id}", handlers.GetRecipeHandler)
		r.Post("/recipes/{id}", handlers.UpdateRecipeHandler)
		r.Get("/search", handlers.SearchHandler)
		r.Get("/tags/suggestions", handlers.TagSuggestionsHandler)

		// Settings Routes
		r.Get("/settings", handlers.SettingsHandler)
		r.Post("/settings/import", handlers.ImportDataHandler)
		r.Get("/settings/export", handlers.ExportDataHandler)

		// pCloud Routes
		r.Get("/settings/pcloud/link", handlers.PCloudLinkHandler)
		r.Get("/settings/pcloud/callback", handlers.PCloudCallbackHandler)
		r.Post("/settings/pcloud/unlink", handlers.PCloudUnlinkHandler)
		r.Post("/settings/pcloud/backup-now", handlers.PCloudBackupNowHandler)
		r.Post("/settings/pcloud/interval", handlers.PCloudUpdateIntervalHandler)
		r.Get("/settings/pcloud/status", handlers.PCloudStatusHandler)

		// Google Drive Routes
		r.Get("/settings/gdrive/link", handlers.GDriveLinkHandler)
		r.Get("/settings/gdrive/callback", handlers.GDriveCallbackHandler)
		r.Post("/settings/gdrive/unlink", handlers.GDriveUnlinkHandler)
		r.Post("/settings/gdrive/backup-now", handlers.GDriveBackupNowHandler)

		// Reminders & Web Push
		r.Get("/reminders", handlers.RemindersPageHandler)
		r.Post("/reminders", handlers.AddReminderHandler)
		r.Delete("/reminders/{id}", handlers.DeleteReminderHandler)
		r.Post("/api/push/subscribe", handlers.SavePushSubscriptionHandler)
	})

	// API Routes (CORS enabled in handlers)
	r.Route("/api", func(r chi.Router) {
		r.Use(handlers.CorsMiddleware)
		r.Use(handlers.AuthMiddleware)

		// CorsMiddleware already returns 200 for OPTIONS, so this just ensures chi doesn't 404 preflight requests
		r.Options("/*", func(w http.ResponseWriter, r *http.Request) {})
		r.Get("/health", handlers.HealthHandler)
		r.Post("/bookmarks", handlers.ApiCreateBookmarkClipperHandler)
		r.Post("/notes", handlers.ApiCreateNoteClipperHandler)
		r.Get("/rated-lists", handlers.ApiGetRatedListsHandler)
		r.Post("/rated-lists/{id}/items", handlers.ApiAddRatedListItemHandler)
		r.Post("/recipes/clipper", handlers.ApiCreateRecipeClipperHandler)
		r.Get("/tags", handlers.ApiGetTagsHandler)

		// Share Links
		r.Post("/share", handlers.GenerateShareLinkHandler)
		r.Delete("/share/{hash}", handlers.RevokeShareLinkHandler)
	})

	log.Println("Server starting on :8080")

	if err := http.ListenAndServe(":8080", r); err != nil {
		log.Fatal(err)
	}
}
