package database

import (
	"crypto/rand"
	"database/sql"
	"encoding/hex"
	"fmt"
	"log"
	"net/url"
	"strings"
	"time"

	_ "github.com/mattn/go-sqlite3"
)

var DB *sql.DB

func InitDB(filepath string) error {
	var err error
	DB, err = sql.Open("sqlite3", filepath)
	if err != nil {
		return err
	}

	if err = DB.Ping(); err != nil {
		return err
	}

	if err = createSchema(); err != nil {
		return err
	}

	return runMigrations()
}

func createSchema() error {
	schema := `
	CREATE TABLE IF NOT EXISTS users (
		id INTEGER PRIMARY KEY AUTOINCREMENT,
		username TEXT UNIQUE NOT NULL,
		password_hash TEXT NOT NULL,
		api_token TEXT,
		created_at DATETIME DEFAULT CURRENT_TIMESTAMP
	);

	CREATE TABLE IF NOT EXISTS sessions (
		id TEXT PRIMARY KEY,
		user_id INTEGER NOT NULL,
		expires_at DATETIME NOT NULL,
		FOREIGN KEY(user_id) REFERENCES users(id) ON DELETE CASCADE
	);

	CREATE TABLE IF NOT EXISTS items (
		id INTEGER PRIMARY KEY AUTOINCREMENT,
		user_id INTEGER,
		title TEXT NOT NULL,
		type TEXT NOT NULL,
		created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
		updated_at DATETIME DEFAULT CURRENT_TIMESTAMP,
		FOREIGN KEY(user_id) REFERENCES users(id) ON DELETE CASCADE
	);

	CREATE TABLE IF NOT EXISTS bookmarks (
		item_id INTEGER PRIMARY KEY,
		url TEXT NOT NULL,
		description TEXT,
		favicon TEXT,
		thumbnail TEXT,
		FOREIGN KEY(item_id) REFERENCES items(id) ON DELETE CASCADE
	);

	CREATE TABLE IF NOT EXISTS notes (
		item_id INTEGER PRIMARY KEY,
		content TEXT,
		FOREIGN KEY(item_id) REFERENCES items(id) ON DELETE CASCADE
	);

	CREATE TABLE IF NOT EXISTS list_items (
		id INTEGER PRIMARY KEY AUTOINCREMENT,
		list_id INTEGER NOT NULL,
		content TEXT NOT NULL,
		completed BOOLEAN DEFAULT 0,
		FOREIGN KEY(list_id) REFERENCES items(id) ON DELETE CASCADE
	);

	CREATE TABLE IF NOT EXISTS rated_list_items (
		id INTEGER PRIMARY KEY AUTOINCREMENT,
		rated_list_id INTEGER NOT NULL,
		title TEXT NOT NULL,
		score INTEGER CHECK(score >= 0 AND score <= 10),
		note TEXT,
		FOREIGN KEY(rated_list_id) REFERENCES items(id) ON DELETE CASCADE
	);

	CREATE TABLE IF NOT EXISTS media (
		item_id INTEGER PRIMARY KEY,
		file_path TEXT NOT NULL,
		mime_type TEXT,
		FOREIGN KEY(item_id) REFERENCES items(id) ON DELETE CASCADE
	);

	CREATE TABLE IF NOT EXISTS tags (
		id INTEGER PRIMARY KEY AUTOINCREMENT,
		name TEXT UNIQUE NOT NULL
	);

	CREATE TABLE IF NOT EXISTS item_tags (
		item_id INTEGER NOT NULL,
		tag_id INTEGER NOT NULL,
		PRIMARY KEY (item_id, tag_id),
		FOREIGN KEY(item_id) REFERENCES items(id) ON DELETE CASCADE,
		FOREIGN KEY(tag_id) REFERENCES tags(id) ON DELETE CASCADE
	);

	CREATE TABLE IF NOT EXISTS recipes (
		id INTEGER PRIMARY KEY AUTOINCREMENT,
		item_id INTEGER NOT NULL,
		ingredients TEXT NOT NULL,
		instructions TEXT NOT NULL,
		notes TEXT,
		thumbnail TEXT,
		source_url TEXT,
		FOREIGN KEY(item_id) REFERENCES items(id) ON DELETE CASCADE
	);
	
	CREATE TABLE IF NOT EXISTS recipe_images (
		id INTEGER PRIMARY KEY AUTOINCREMENT,
		recipe_id INTEGER NOT NULL,
		file_path TEXT NOT NULL,
		display_order INTEGER DEFAULT 0,
		FOREIGN KEY(recipe_id) REFERENCES recipes(id) ON DELETE CASCADE
	);
	CREATE TABLE IF NOT EXISTS drawings (
		item_id INTEGER PRIMARY KEY,
		file_path TEXT NOT NULL,
		FOREIGN KEY(item_id) REFERENCES items(id) ON DELETE CASCADE
	);

	CREATE TABLE IF NOT EXISTS push_subscriptions (
		id INTEGER PRIMARY KEY AUTOINCREMENT,
		user_id INTEGER NOT NULL,
		endpoint TEXT NOT NULL,
		p256dh TEXT NOT NULL,
		auth TEXT NOT NULL,
		created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
		FOREIGN KEY(user_id) REFERENCES users(id) ON DELETE CASCADE
	);

	CREATE TABLE IF NOT EXISTS reminders (
		id INTEGER PRIMARY KEY AUTOINCREMENT,
		item_id INTEGER UNIQUE,
		user_id INTEGER NOT NULL,
		name TEXT NOT NULL,
		frequency TEXT NOT NULL,
		time_of_day TEXT NOT NULL,
		start_date DATE NOT NULL,
		end_date DATE,
		notification_type TEXT NOT NULL,
		emails TEXT,
		last_triggered_at DATETIME,
		is_pinned INTEGER DEFAULT 0,
		created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
		FOREIGN KEY(item_id) REFERENCES items(id) ON DELETE CASCADE,
		FOREIGN KEY(user_id) REFERENCES users(id) ON DELETE CASCADE
	);

	CREATE TABLE IF NOT EXISTS system_settings (
		key TEXT PRIMARY KEY,
		value TEXT NOT NULL
	);

	CREATE TABLE IF NOT EXISTS shared_links (
		id INTEGER PRIMARY KEY AUTOINCREMENT,
		link_hash TEXT UNIQUE NOT NULL,
		item_type TEXT NOT NULL,
		item_id INTEGER NOT NULL,
		user_id INTEGER NOT NULL,
		created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
		FOREIGN KEY(user_id) REFERENCES users(id) ON DELETE CASCADE
	);
	`
	_, err := DB.Exec(schema)
	return err
}

func runMigrations() error {
	// 1. Check if user_id column exists in items table
	rows, err := DB.Query("PRAGMA table_info(items)")
	if err != nil {
		return err
	}
	defer rows.Close()

	hasUserID := false
	hasIsPinnedItem := false
	for rows.Next() {
		var cid int
		var name string
		var dtype string
		var notnull int
		var dfltValue interface{}
		var pk int
		if err := rows.Scan(&cid, &name, &dtype, &notnull, &dfltValue, &pk); err != nil {
			return err
		}
		if name == "user_id" {
			hasUserID = true
		}
		if name == "is_pinned" {
			hasIsPinnedItem = true
		}
	}

	if !hasUserID {
		log.Println("Migrating database: Adding user_id column to items table")
		_, err = DB.Exec("ALTER TABLE items ADD COLUMN user_id INTEGER")
		if err != nil {
			log.Printf("Error adding user_id to items: %v", err)
		}
		// assign existing items to the first user if any exist
		var firstUserID int64
		err = DB.QueryRow("SELECT id FROM users ORDER BY id ASC LIMIT 1").Scan(&firstUserID)
		if err == nil {
			log.Printf("Assigning existing items to first user (ID: %d)\n", firstUserID)
			DB.Exec("UPDATE items SET user_id = ? WHERE user_id IS NULL", firstUserID)
		}
	}

	if !hasIsPinnedItem {
		log.Println("Migrating database: Adding is_pinned column to items table")
		_, err = DB.Exec("ALTER TABLE items ADD COLUMN is_pinned INTEGER DEFAULT 0")
		if err != nil {
			log.Printf("Error adding is_pinned to items: %v", err)
		}
	}

	// 2. Check reminders table for item_id and is_pinned
	rows, err = DB.Query("PRAGMA table_info(reminders)")
	if err == nil {
		defer rows.Close()
		hasItemIDRem := false
		hasIsPinnedRem := false
		for rows.Next() {
			var cid int
			var name string
			var dtype string
			var notnull int
			var dfltValue interface{}
			var pk int
			if err := rows.Scan(&cid, &name, &dtype, &notnull, &dfltValue, &pk); err == nil {
				if name == "item_id" {
					hasItemIDRem = true
				}
				if name == "is_pinned" {
					hasIsPinnedRem = true
				}
			}
		}
		if !hasItemIDRem {
			log.Println("Migrating database: Adding item_id column to reminders table")
			_, err = DB.Exec("ALTER TABLE reminders ADD COLUMN item_id INTEGER")
			if err != nil {
				log.Printf("Error adding item_id to reminders: %v", err)
			}
		}
		if !hasIsPinnedRem {
			log.Println("Migrating database: Adding is_pinned column to reminders table")
			_, err = DB.Exec("ALTER TABLE reminders ADD COLUMN is_pinned INTEGER DEFAULT 0")
			if err != nil {
				log.Printf("Error adding is_pinned to reminders: %v", err)
			}
		}
	}

	// 3. Migration loop for existing reminders to have an item_id
	rows, err = DB.Query("SELECT id, user_id, name FROM reminders WHERE item_id IS NULL")
	if err == nil {
		defer rows.Close()
		for rows.Next() {
			var rid, uid int64
			var name string
			if err := rows.Scan(&rid, &uid, &name); err == nil {
				res, err := DB.Exec("INSERT INTO items (user_id, title, type) VALUES (?, ?, ?)", uid, name, "reminder")
				if err == nil {
					itemID, _ := res.LastInsertId()
					DB.Exec("UPDATE reminders SET item_id = ? WHERE id = ?", itemID, rid)
				}
			}
		}
	}

	// 4. Other miscellaneous migrations (safe to run multiple times with _, _ =)
	_, _ = DB.Exec("ALTER TABLE bookmarks ADD COLUMN thumbnail TEXT")
	_, _ = DB.Exec("ALTER TABLE recipes ADD COLUMN source_url TEXT")
	_, _ = DB.Exec("ALTER TABLE users ADD COLUMN api_token TEXT")
	_, _ = DB.Exec("ALTER TABLE users ADD COLUMN pcloud_access_token TEXT")
	_, _ = DB.Exec("ALTER TABLE users ADD COLUMN pcloud_hostname TEXT")
	_, _ = DB.Exec("ALTER TABLE users ADD COLUMN backup_interval_days INTEGER DEFAULT 7")
	_, _ = DB.Exec("ALTER TABLE users ADD COLUMN last_backup_at DATETIME")
	_, _ = DB.Exec("ALTER TABLE users ADD COLUMN gdrive_access_token TEXT")
	_, _ = DB.Exec("ALTER TABLE users ADD COLUMN gdrive_refresh_token TEXT")
	_, _ = DB.Exec("ALTER TABLE users ADD COLUMN gdrive_folder_id TEXT")
	_, _ = DB.Exec("ALTER TABLE rated_list_items ADD COLUMN image_path TEXT")
	_, _ = DB.Exec("ALTER TABLE users ADD COLUMN default_page TEXT DEFAULT 'dashboard'")

	return nil
}

func GetUserByToken(token string) (int64, error) {
	var userID int64
	err := DB.QueryRow("SELECT id FROM users WHERE api_token = ?", token).Scan(&userID)
	if err != nil {
		return 0, err
	}
	return userID, nil
}

func GetAPIToken(userID int64) (string, error) {
	var token sql.NullString
	err := DB.QueryRow("SELECT api_token FROM users WHERE id = ?", userID).Scan(&token)
	if err != nil {
		return "", err
	}
	if !token.Valid || token.String == "" {
		return RegenerateAPIToken(userID)
	}
	return token.String, nil
}

func RegenerateAPIToken(userID int64) (string, error) {
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	token := hex.EncodeToString(b)
	_, err := DB.Exec("UPDATE users SET api_token = ? WHERE id = ?", token, userID)
	if err != nil {
		return "", err
	}
	return token, nil
}

func GetSystemSetting(key string) (string, error) {
	var value string
	err := DB.QueryRow("SELECT value FROM system_settings WHERE key = ?", key).Scan(&value)
	if err != nil {
		return "", err
	}
	return value, nil
}

func SetSystemSetting(key string, value string) error {
	_, err := DB.Exec("INSERT INTO system_settings (key, value) VALUES (?, ?) ON CONFLICT(key) DO UPDATE SET value=excluded.value", key, value)
	return err
}

// ... (skipping unchanged parts) ...

// Recipes

// Drawings
func CreateDrawing(userID int64, title, filePath string) (int64, error) {
	tx, err := DB.Begin()
	if err != nil {
		return 0, err
	}
	defer tx.Rollback()

	result, err := tx.Exec("INSERT INTO items (user_id, title, type) VALUES (?, ?, ?)", userID, title, "drawing")
	if err != nil {
		return 0, err
	}
	itemID, _ := result.LastInsertId()

	_, err = tx.Exec("INSERT INTO drawings (item_id, file_path) VALUES (?, ?)", itemID, filePath)
	if err != nil {
		return 0, err
	}

	if err = tx.Commit(); err != nil {
		return 0, err
	}

	return itemID, nil
}

func GetDrawings(userID int64, tagFilter string) ([]map[string]interface{}, error) {
	query := `
		SELECT i.id, i.title, i.created_at, d.file_path, COALESCE(i.is_pinned, 0)
		FROM items i 
		JOIN drawings d ON i.id = d.item_id 
		WHERE i.user_id = ?`
	args := []interface{}{userID}

	if tagFilter != "" {
		query += " AND i.id IN (SELECT item_id FROM item_tags it JOIN tags t ON it.tag_id = t.id WHERE t.name = ?)"
		args = append(args, tagFilter)
	}

	query += " ORDER BY i.created_at DESC"

	rows, err := DB.Query(query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var results []map[string]interface{}
	for rows.Next() {
		var id int64
		var isPinned int
		var title, createdAt, filePath sql.NullString
		if err := rows.Scan(&id, &title, &createdAt, &filePath, &isPinned); err != nil {
			return nil, err
		}

		tags, _ := GetItemTags(id)
		results = append(results, map[string]interface{}{
			"id":         id,
			"title":      title.String,
			"created_at": createdAt.String,
			"file_path":  filePath.String,
			"tags":       tags,
			"is_pinned":  isPinned == 1,
		})
	}
	return results, nil
}

func GetDrawing(userID int64, id int64) (map[string]interface{}, error) {
	var title, filePath sql.NullString
	err := DB.QueryRow(`
		SELECT i.title, d.file_path 
		FROM items i 
		JOIN drawings d ON i.id = d.item_id 
		WHERE i.id = ? AND i.user_id = ?`, id, userID).Scan(&title, &filePath)

	if err != nil {
		return nil, err
	}

	tags, _ := GetItemTags(id)
	return map[string]interface{}{
		"id":        id,
		"title":     title.String,
		"file_path": filePath.String,
		"tags":      tags,
	}, nil
}

func UpdateDrawing(id int64, title, filePath string) error {
	tx, err := DB.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()

	_, err = tx.Exec("UPDATE items SET title = ?, updated_at = CURRENT_TIMESTAMP WHERE id = ?", title, id)
	if err != nil {
		return err
	}

	if filePath != "" {
		_, err = tx.Exec("UPDATE drawings SET file_path = ? WHERE item_id = ?", filePath, id)
		if err != nil {
			return err
		}
	}

	return tx.Commit()
}

// Bookmarks

func CreateBookmark(userID int64, title, url, description, favicon, thumbnail string) (int64, error) {
	tx, err := DB.Begin()
	if err != nil {
		return 0, err
	}
	defer tx.Rollback()

	result, err := tx.Exec("INSERT INTO items (user_id, title, type) VALUES (?, ?, ?)", userID, title, "bookmark")
	if err != nil {
		return 0, err
	}
	itemID, _ := result.LastInsertId()

	_, err = tx.Exec(
		"INSERT INTO bookmarks (item_id, url, description, favicon, thumbnail) VALUES (?, ?, ?, ?, ?)",
		itemID, url, description, favicon, thumbnail,
	)
	if err != nil {
		return 0, err
	}

	if err = tx.Commit(); err != nil {
		return 0, err
	}

	return itemID, nil
}

func GetBookmarks(userID int64, tagFilter string) ([]map[string]interface{}, error) {
	query := `
		SELECT i.id, i.title, i.created_at, b.url, b.description, b.favicon, b.thumbnail, COALESCE(i.is_pinned, 0)
		FROM items i 
		JOIN bookmarks b ON i.id = b.item_id 
		WHERE i.user_id = ?`
	args := []interface{}{userID}

	if tagFilter != "" {
		query += " AND i.id IN (SELECT item_id FROM item_tags it JOIN tags t ON it.tag_id = t.id WHERE t.name = ?)"
		args = append(args, tagFilter)
	}

	query += " ORDER BY i.created_at DESC"

	rows, err := DB.Query(query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var results []map[string]interface{}
	for rows.Next() {
		var id int64
		var isPinned int
		var title, createdAt, rawURL, description, favicon, thumbnail sql.NullString
		if err := rows.Scan(&id, &title, &createdAt, &rawURL, &description, &favicon, &thumbnail, &isPinned); err != nil {
			return nil, err
		}

		// Auto-generate favicon URL from domain if not stored
		faviconURL := favicon.String
		if faviconURL == "" && rawURL.String != "" {
			if domain := extractDomain(rawURL.String); domain != "" {
				faviconURL = "https://www.google.com/s2/favicons?domain=" + domain + "&sz=32"
			}
		}

		tags, _ := GetItemTags(id)
		results = append(results, map[string]interface{}{
			"id":          id,
			"title":       title.String,
			"created_at":  createdAt.String,
			"url":         rawURL.String,
			"description": description.String,
			"favicon":     faviconURL,
			"thumbnail":   thumbnail.String,
			"tags":        tags,
			"is_pinned":   isPinned == 1,
		})
	}
	return results, nil
}

// extractDomain parses a raw URL and returns just the host (e.g. "github.com").
func extractDomain(rawURL string) string {
	u, err := url.Parse(rawURL)
	if err != nil || u.Host == "" {
		return ""
	}
	return u.Host
}

func GetBookmark(userID int64, id int64) (map[string]interface{}, error) {
	var title, url, description, favicon, thumbnail sql.NullString
	err := DB.QueryRow(`
		SELECT i.title, b.url, b.description, b.favicon, b.thumbnail 
		FROM items i 
		JOIN bookmarks b ON i.id = b.item_id 
		WHERE i.id = ? AND i.user_id = ?`, id, userID).Scan(&title, &url, &description, &favicon, &thumbnail)

	if err != nil {
		return nil, err
	}

	tags, _ := GetItemTags(id)
	return map[string]interface{}{
		"id":          id,
		"title":       title.String,
		"url":         url.String,
		"description": description.String,
		"favicon":     favicon.String,
		"thumbnail":   thumbnail.String,
		"tags":        tags,
	}, nil
}

func UpdateBookmark(userID int64, id int64, title, url, description string) error {
	tx, err := DB.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()

	_, err = tx.Exec("UPDATE items SET title = ?, updated_at = CURRENT_TIMESTAMP WHERE id = ? AND user_id = ?", title, id, userID)
	if err != nil {
		return err
	}

	_, err = tx.Exec("UPDATE bookmarks SET url = ?, description = ? WHERE item_id = ?", url, description, id)
	if err != nil {
		return err
	}

	return tx.Commit()
}

// Notes

func CreateNote(userID int64, title, content string) (int64, error) {
	tx, err := DB.Begin()
	if err != nil {
		return 0, err
	}
	defer tx.Rollback()

	result, err := tx.Exec("INSERT INTO items (user_id, title, type) VALUES (?, ?, ?)", userID, title, "note")
	if err != nil {
		return 0, err
	}
	itemID, _ := result.LastInsertId()

	_, err = tx.Exec("INSERT INTO notes (item_id, content) VALUES (?, ?)", itemID, content)
	if err != nil {
		return 0, err
	}

	if err = tx.Commit(); err != nil {
		return 0, err
	}

	return itemID, nil
}

func GetNotes(userID int64, tagFilter string) ([]map[string]interface{}, error) {
	query := `
		SELECT i.id, i.title, i.created_at, n.content, COALESCE(i.is_pinned, 0)
		FROM items i 
		JOIN notes n ON i.id = n.item_id 
		WHERE i.user_id = ?`
	args := []interface{}{userID}

	if tagFilter != "" {
		query += " AND i.id IN (SELECT item_id FROM item_tags it JOIN tags t ON it.tag_id = t.id WHERE t.name = ?)"
		args = append(args, tagFilter)
	}

	query += " ORDER BY i.created_at DESC"

	rows, err := DB.Query(query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var results []map[string]interface{}
	for rows.Next() {
		var id int64
		var isPinned int
		var title, createdAt, content sql.NullString
		if err := rows.Scan(&id, &title, &createdAt, &content, &isPinned); err != nil {
			return nil, err
		}

		tags, _ := GetItemTags(id)
		results = append(results, map[string]interface{}{
			"id":         id,
			"title":      title.String,
			"created_at": createdAt.String,
			"content":    content.String,
			"tags":       tags,
			"is_pinned":  isPinned == 1,
		})
	}
	return results, nil
}

func GetNote(userID int64, id int64) (map[string]interface{}, error) {
	var title, content sql.NullString
	err := DB.QueryRow(`
		SELECT i.title, n.content 
		FROM items i 
		JOIN notes n ON i.id = n.item_id 
		WHERE i.id = ? AND i.user_id = ?`, id, userID).Scan(&title, &content)

	if err != nil {
		return nil, err
	}

	tags, _ := GetItemTags(id)
	return map[string]interface{}{
		"id":      id,
		"title":   title.String,
		"content": content.String,
		"tags":    tags,
	}, nil
}

func UpdateNote(userID int64, id int64, title, content string) error {
	tx, err := DB.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()

	_, err = tx.Exec("UPDATE items SET title = ?, updated_at = CURRENT_TIMESTAMP WHERE id = ? AND user_id = ?", title, id, userID)
	if err != nil {
		return err
	}

	_, err = tx.Exec("UPDATE notes SET content = ? WHERE item_id = ?", content, id)
	if err != nil {
		return err
	}

	return tx.Commit()
}

// Rated Lists
func CreateRatedList(userID int64, title string) (int64, error) {
	result, err := DB.Exec("INSERT INTO items (user_id, title, type) VALUES (?, ?, ?)", userID, title, "rated_list")
	if err != nil {
		return 0, err
	}
	return result.LastInsertId()
}

func GetRatedList(userID int64, id int64) (map[string]interface{}, error) {
	var title, createdAt sql.NullString
	err := DB.QueryRow("SELECT title, created_at FROM items WHERE id = ? AND user_id = ? AND type = 'rated_list'", id, userID).Scan(&title, &createdAt)
	if err != nil {
		return nil, err
	}
	tags, _ := GetItemTags(id)
	return map[string]interface{}{
		"id":         id,
		"title":      title.String,
		"created_at": createdAt.String,
		"tags":       tags,
	}, nil
}

func GetRatedLists(userID int64, tagFilter string) ([]map[string]interface{}, error) {
	query := `
		SELECT i.id, i.title, i.created_at, COALESCE(i.is_pinned, 0)
		FROM items i 
		WHERE i.type = 'rated_list' AND i.user_id = ?`
	args := []interface{}{userID}

	if tagFilter != "" {
		query += " AND i.id IN (SELECT item_id FROM item_tags it JOIN tags t ON it.tag_id = t.id WHERE t.name = ?)"
		args = append(args, tagFilter)
	}

	query += " ORDER BY i.created_at DESC"

	rows, err := DB.Query(query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var results []map[string]interface{}
	for rows.Next() {
		var id int64
		var isPinned int
		var title, createdAt sql.NullString
		if err := rows.Scan(&id, &title, &createdAt, &isPinned); err != nil {
			return nil, err
		}
		tags, _ := GetItemTags(id)
		results = append(results, map[string]interface{}{
			"id":         id,
			"title":      title.String,
			"created_at": createdAt.String,
			"tags":       tags,
			"is_pinned":  isPinned == 1,
		})
	}
	return results, nil
}

func AddRatedListItem(listID int64, title string, score int, note string) (int64, error) {
	result, err := DB.Exec("INSERT INTO rated_list_items (rated_list_id, title, score, note) VALUES (?, ?, ?, ?)",
		listID, title, score, note)
	if err != nil {
		return 0, err
	}
	return result.LastInsertId()
}

func GetRatedListItems(listID int64) ([]map[string]interface{}, error) {
	rows, err := DB.Query("SELECT id, title, score, note, image_path FROM rated_list_items WHERE rated_list_id = ? ORDER BY score DESC, title ASC", listID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var results []map[string]interface{}
	for rows.Next() {
		var id int64
		var title string
		var note, imagePath sql.NullString
		var score int
		if err := rows.Scan(&id, &title, &score, &note, &imagePath); err != nil {
			return nil, err
		}
		results = append(results, map[string]interface{}{
			"id":         id,
			"title":      title,
			"score":      score,
			"note":       note.String,
			"image_path": imagePath.String,
		})
	}
	return results, nil
}

func GetRatedListItem(id int64) (map[string]interface{}, error) {
	var title, note, imagePath sql.NullString
	var score int
	err := DB.QueryRow("SELECT title, score, note, image_path FROM rated_list_items WHERE id = ?", id).Scan(&title, &score, &note, &imagePath)
	if err != nil {
		return nil, err
	}
	return map[string]interface{}{
		"id":         id,
		"title":      title.String,
		"score":      score,
		"note":       note.String,
		"image_path": imagePath.String,
	}, nil
}

func UpdateRatedListItem(id int64, title string, score int, note string) error {
	_, err := DB.Exec("UPDATE rated_list_items SET title = ?, score = ?, note = ? WHERE id = ?", title, score, note, id)
	return err
}

func UpdateRatedListItemImage(id int64, imagePath string) error {
	_, err := DB.Exec("UPDATE rated_list_items SET image_path = ? WHERE id = ?", imagePath, id)
	return err
}

// Lists (Checklists)

func CreateList(userID int64, title string) (int64, error) {
	result, err := DB.Exec("INSERT INTO items (user_id, title, type) VALUES (?, ?, ?)", userID, title, "list")
	if err != nil {
		return 0, err
	}
	return result.LastInsertId()
}

func GetLists(userID int64, tagFilter string) ([]map[string]interface{}, error) {
	query := `
		SELECT i.id, i.title, i.created_at, COALESCE(i.is_pinned, 0)
		FROM items i 
		WHERE i.type = 'list' AND i.user_id = ?`
	args := []interface{}{userID}

	if tagFilter != "" {
		query += " AND i.id IN (SELECT item_id FROM item_tags it JOIN tags t ON it.tag_id = t.id WHERE t.name = ?)"
		args = append(args, tagFilter)
	}

	query += " ORDER BY i.created_at DESC"

	rows, err := DB.Query(query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var results []map[string]interface{}
	for rows.Next() {
		var id int64
		var isPinned int
		var title, createdAt sql.NullString
		if err := rows.Scan(&id, &title, &createdAt, &isPinned); err != nil {
			return nil, err
		}

		tags, _ := GetItemTags(id)
		results = append(results, map[string]interface{}{
			"id":         id,
			"title":      title.String,
			"created_at": createdAt.String,
			"tags":       tags,
			"is_pinned":  isPinned == 1,
		})
	}
	return results, nil
}

func AddListItem(listID int64, content string) error {
	_, err := DB.Exec("INSERT INTO list_items (list_id, content) VALUES (?, ?)", listID, content)
	return err
}

func GetListItems(listID int64) ([]map[string]interface{}, error) {
	rows, err := DB.Query("SELECT id, content, completed FROM list_items WHERE list_id = ? ORDER BY completed ASC, id ASC", listID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var results []map[string]interface{}
	for rows.Next() {
		var id int64
		var content sql.NullString
		var completed bool
		if err := rows.Scan(&id, &content, &completed); err != nil {
			return nil, err
		}
		results = append(results, map[string]interface{}{
			"id":        id,
			"content":   content.String,
			"completed": completed,
		})
	}
	return results, nil
}

func GetListItemById(id int64) (map[string]interface{}, error) {
	var content sql.NullString
	err := DB.QueryRow("SELECT content FROM list_items WHERE id = ?", id).Scan(&content)
	if err != nil {
		return nil, err
	}
	return map[string]interface{}{
		"id":      id,
		"content": content.String,
	}, nil
}

func UpdateListItem(id int64, content string) error {
	_, err := DB.Exec("UPDATE list_items SET content = ? WHERE id = ?", content, id)
	return err
}

func ToggleListItem(itemID int64, completed bool) error {
	_, err := DB.Exec("UPDATE list_items SET completed = ? WHERE id = ?", completed, itemID)
	return err
}

// Media
func CreateMedia(userID int64, title, filePath, mimeType string) (int64, error) {
	tx, err := DB.Begin()
	if err != nil {
		return 0, err
	}
	defer tx.Rollback()

	res, err := tx.Exec("INSERT INTO items (user_id, title, type) VALUES (?, ?, ?)", userID, title, "media")
	if err != nil {
		return 0, err
	}

	itemID, err := res.LastInsertId()
	if err != nil {
		return 0, err
	}

	_, err = tx.Exec("INSERT INTO media (item_id, file_path, mime_type) VALUES (?, ?, ?)",
		itemID, filePath, mimeType)
	if err != nil {
		return 0, err
	}

	err = tx.Commit()
	return itemID, err
}

func GetMedia(userID int64, tagFilter string) ([]map[string]interface{}, error) {
	query := `
		SELECT i.id, i.title, i.created_at, m.file_path, m.mime_type, COALESCE(i.is_pinned, 0)
		FROM items i
		JOIN media m ON i.id = m.item_id 
		WHERE i.user_id = ?`

	args := []interface{}{userID}
	if tagFilter != "" {
		query += ` AND i.id IN (SELECT item_id FROM item_tags it ON i.id = it.item_id JOIN tags t ON it.tag_id = t.id WHERE t.name = ?)`
		args = append(args, tagFilter)
	}

	query += ` ORDER BY i.created_at DESC`

	rows, err := DB.Query(query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var results []map[string]interface{}
	for rows.Next() {
		var id int64
		var isPinned int
		var title, createdAt, filePath, mimeType sql.NullString
		if err := rows.Scan(&id, &title, &createdAt, &filePath, &mimeType, &isPinned); err != nil {
			return nil, err
		}
		tags, _ := GetItemTags(id)
		results = append(results, map[string]interface{}{
			"id":         id,
			"title":      title.String,
			"created_at": createdAt.String,
			"file_path":  filePath.String,
			"mime_type":  mimeType.String,
			"tags":       tags,
			"is_pinned":  isPinned == 1,
		})
	}
	return results, nil
}

func GetMediaItem(id int64, userID int64) (map[string]interface{}, error) {
	query := `
		SELECT i.id, i.title, i.created_at, m.file_path, m.mime_type
		FROM items i
		JOIN media m ON i.id = m.item_id 
		WHERE i.id = ? AND i.user_id = ?`

	var title, createdAt, filePath, mimeType sql.NullString
	err := DB.QueryRow(query, id, userID).Scan(&id, &title, &createdAt, &filePath, &mimeType)
	if err != nil {
		if err == sql.ErrNoRows {
			return nil, fmt.Errorf("media not found")
		}
		return nil, err
	}
	
	tags, _ := GetItemTags(id)
	return map[string]interface{}{
		"id":         id,
		"title":      title.String,
		"created_at": createdAt.String,
		"file_path":  filePath.String,
		"mime_type":  mimeType.String,
		"tags":       tags,
	}, nil
}

func UpdateMediaItem(id int64, userID int64, title string) error {
	_, err := DB.Exec("UPDATE items SET title = ? WHERE id = ? AND user_id = ?", title, id, userID)
	return err
}

// Global Deletion
func DeleteItem(userID int64, id int64) error {
	_, err := DB.Exec("DELETE FROM items WHERE id = ? AND user_id = ?", id, userID)
	return err
}

func DeleteListItem(id int64) error {
	_, err := DB.Exec("DELETE FROM list_items WHERE id = ?", id)
	return err
}

func DeleteRatedListItem(id int64) error {
	_, err := DB.Exec("DELETE FROM rated_list_items WHERE id = ?", id)
	return err
}

// Tags
type TagCount struct {
	Name  string
	Count int
}

func GetTagsWithCounts(userID int64) ([]TagCount, error) {
	rows, err := DB.Query(`
		SELECT t.name, COUNT(it.item_id) as count
		FROM tags t
		JOIN item_tags it ON t.id = it.tag_id
		JOIN items i ON it.item_id = i.id
		WHERE i.user_id = ?
		GROUP BY t.name
		ORDER BY count DESC`, userID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var results []TagCount
	for rows.Next() {
		var tc TagCount
		if err := rows.Scan(&tc.Name, &tc.Count); err != nil {
			return nil, err
		}
		results = append(results, tc)
	}
	return results, nil
}
func GetItemTags(itemID int64) ([]string, error) {
	rows, err := DB.Query(`
		SELECT t.name 
		FROM tags t 
		JOIN item_tags it ON t.id = it.tag_id 
		WHERE it.item_id = ?`, itemID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var tags []string
	for rows.Next() {
		var tag string
		if err := rows.Scan(&tag); err != nil {
			return nil, err
		}
		tags = append(tags, tag)
	}
	return tags, nil
}

func SetItemTags(itemID int64, tags []string) error {
	tx, err := DB.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()

	// Remove existing tags
	_, err = tx.Exec("DELETE FROM item_tags WHERE item_id = ?", itemID)
	if err != nil {
		return err
	}

	for _, tagName := range tags {
		tagName = strings.TrimSpace(strings.ToLower(tagName))
		if tagName == "" {
			continue
		}

		// Ensure tag exists
		_, err = tx.Exec("INSERT OR IGNORE INTO tags (name) VALUES (?)", tagName)
		if err != nil {
			return err
		}

		// Get tag ID
		var tagID int64
		err = tx.QueryRow("SELECT id FROM tags WHERE name = ?", tagName).Scan(&tagID)
		if err != nil {
			return err
		}

		// Associate tag with item
		_, err = tx.Exec("INSERT OR IGNORE INTO item_tags (item_id, tag_id) VALUES (?, ?)", itemID, tagID)
		if err != nil {
			return err
		}
	}

	return tx.Commit()
}

func GetAllUniqueTags() ([]string, error) {
	rows, err := DB.Query("SELECT name FROM tags ORDER BY name ASC")
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var tags []string
	for rows.Next() {
		var tag string
		if err := rows.Scan(&tag); err != nil {
			return nil, err
		}
		tags = append(tags, tag)
	}
	return tags, nil
}

// Recipes

func CreateRecipe(userID int64, title, ingredients, instructions, notes, thumbnail, sourceURL string, imagePaths []string) (int64, error) {
	tx, err := DB.Begin()
	if err != nil {
		return 0, err
	}
	defer tx.Rollback()

	// Create item
	result, err := tx.Exec("INSERT INTO items (title, type, user_id) VALUES (?, ?, ?)", title, "recipe", userID)
	if err != nil {
		return 0, err
	}
	itemID, _ := result.LastInsertId()

	_, err = tx.Exec(
		"INSERT INTO recipes (item_id, ingredients, instructions, notes, thumbnail, source_url) VALUES (?, ?, ?, ?, ?, ?)",
		itemID, ingredients, instructions, notes, thumbnail, sourceURL,
	)
	if err != nil {
		return 0, err
	}

	for _, path := range imagePaths {
		_, err = tx.Exec("INSERT INTO recipe_images (recipe_id, file_path) VALUES (?, ?)", itemID, path)
		if err != nil {
			return 0, err
		}
	}

	if err = tx.Commit(); err != nil {
		return 0, err
	}

	return itemID, nil
}

func GetRecipes(userID int64, tagFilter string) ([]map[string]interface{}, error) {
	query := `
		SELECT i.id, i.title, i.created_at, r.ingredients, r.instructions, r.notes, r.thumbnail, r.source_url, COALESCE(i.is_pinned, 0)
		FROM items i 
		JOIN recipes r ON i.id = r.item_id 
		WHERE i.user_id = ?`
	args := []interface{}{userID}

	if tagFilter != "" {
		query += " AND i.id IN (SELECT item_id FROM item_tags it JOIN tags t ON it.tag_id = t.id WHERE t.name = ?)"
		args = append(args, tagFilter)
	}

	query += " ORDER BY i.created_at DESC"

	rows, err := DB.Query(query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var results []map[string]interface{}
	for rows.Next() {
		var id int64
		var isPinned int
		var title, createdAt, ingredients, instructions, notes, thumbnail, sourceURL sql.NullString
		if err := rows.Scan(&id, &title, &createdAt, &ingredients, &instructions, &notes, &thumbnail, &sourceURL, &isPinned); err != nil {
			return nil, err
		}

		tags, _ := GetItemTags(id)
		results = append(results, map[string]interface{}{
			"id":           id,
			"title":        title.String,
			"created_at":   createdAt.String,
			"ingredients":  ingredients.String,
			"instructions": instructions.String,
			"notes":        notes.String,
			"thumbnail":    thumbnail.String,
			"source_url":   sourceURL.String,
			"tags":         tags,
			"is_pinned":    isPinned == 1,
		})
	}
	return results, nil
}

func GetRecipe(userID int64, id int64) (map[string]interface{}, error) {
	var title, ingredients, instructions, notes, thumbnail, sourceURL sql.NullString
	err := DB.QueryRow(`
		SELECT i.title, r.ingredients, r.instructions, r.notes, r.thumbnail, r.source_url 
		FROM items i 
		JOIN recipes r ON i.id = r.item_id 
		WHERE i.id = ? AND i.user_id = ?`, id, userID).Scan(&title, &ingredients, &instructions, &notes, &thumbnail, &sourceURL)

	if err != nil {
		return nil, err
	}

	tags, _ := GetItemTags(id)
	images, _ := GetRecipeImages(id)
	return map[string]interface{}{
		"id":           id,
		"title":        title.String,
		"ingredients":  ingredients.String,
		"instructions": instructions.String,
		"notes":        notes.String,
		"thumbnail":    thumbnail.String,
		"source_url":   sourceURL.String,
		"tags":         tags,
		"images":       images,
	}, nil
}

func UpdateRecipe(userID int64, id int64, title, ingredients, instructions, notes, thumbnail, sourceURL string) error {
	tx, err := DB.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()

	// Update item title
	_, err = tx.Exec("UPDATE items SET title = ?, updated_at = CURRENT_TIMESTAMP WHERE id = ? AND user_id = ?", title, id, userID)
	if err != nil {
		return err
	}

	_, err = tx.Exec(
		"UPDATE recipes SET ingredients = ?, instructions = ?, notes = ?, thumbnail = ?, source_url = ? WHERE item_id = ?",
		ingredients, instructions, notes, thumbnail, sourceURL, id,
	)
	if err != nil {
		return err
	}

	return tx.Commit()
}

func AddRecipeImage(recipeID int64, filePath string, order int) error {
	_, err := DB.Exec(
		"INSERT INTO recipe_images (recipe_id, file_path, display_order) VALUES (?, ?, ?)",
		recipeID, filePath, order,
	)
	return err
}

func GetRecipeImages(recipeID int64) ([]string, error) {
	rows, err := DB.Query(
		"SELECT file_path FROM recipe_images WHERE recipe_id = ? ORDER BY display_order",
		recipeID,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var images []string
	for rows.Next() {
		var filePath string
		if err := rows.Scan(&filePath); err != nil {
			return nil, err
		}
		images = append(images, filePath)
	}
	return images, nil
}

func DeleteRecipeImage(recipeID int64, filePath string) error {
	_, err := DB.Exec("DELETE FROM recipe_images WHERE recipe_id = ? AND file_path = ?", recipeID, filePath)
	return err
}

// User & Session Management

func CreateUser(username, passwordHash string) (int64, error) {
	result, err := DB.Exec("INSERT INTO users (username, password_hash) VALUES (?, ?)", username, passwordHash)
	if err != nil {
		return 0, err
	}
	return result.LastInsertId()
}

func GetUserByUsername(username string) (map[string]interface{}, error) {
	var id int64
	var passwordHash string
	err := DB.QueryRow("SELECT id, password_hash FROM users WHERE username = ?", username).Scan(&id, &passwordHash)
	if err != nil {
		return nil, err
	}
	return map[string]interface{}{
		"id":            id,
		"username":      username,
		"password_hash": passwordHash,
	}, nil
}

func CreateSession(userID int64, duration time.Duration) (string, error) {
	b := make([]byte, 16)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	sessionID := hex.EncodeToString(b)
	expiresAt := time.Now().Add(duration)

	_, err := DB.Exec("INSERT INTO sessions (id, user_id, expires_at) VALUES (?, ?, ?)", sessionID, userID, expiresAt)
	if err != nil {
		return "", err
	}
	return sessionID, nil
}

func GetSession(sessionID string) (int64, error) {
	var userID int64
	var expiresAt time.Time
	err := DB.QueryRow("SELECT user_id, expires_at FROM sessions WHERE id = ?", sessionID).Scan(&userID, &expiresAt)
	if err != nil {
		return 0, err
	}

	if time.Now().After(expiresAt) {
		DeleteSession(sessionID)
		return 0, fmt.Errorf("session expired")
	}

	return userID, nil
}

func DeleteSession(sessionID string) error {
	_, err := DB.Exec("DELETE FROM sessions WHERE id = ?", sessionID)
	return err
}

// pCloud Integration

type UserBackupInfo struct {
	UserID             int64
	AccessToken        string
	Hostname           string
	BackupIntervalDays int
	LastBackupAt       sql.NullString
}

func SetPCloudCredentials(userID int64, accessToken, hostname string) error {
	_, err := DB.Exec("UPDATE users SET pcloud_access_token = ?, pcloud_hostname = ? WHERE id = ?",
		accessToken, hostname, userID)
	return err
}

func GetPCloudCredentials(userID int64) (string, string, error) {
	var token, hostname sql.NullString
	err := DB.QueryRow("SELECT pcloud_access_token, pcloud_hostname FROM users WHERE id = ?", userID).Scan(&token, &hostname)
	if err != nil {
		return "", "", err
	}
	return token.String, hostname.String, nil
}

func ClearPCloudCredentials(userID int64) error {
	_, err := DB.Exec("UPDATE users SET pcloud_access_token = NULL, pcloud_hostname = NULL WHERE id = ?", userID)
	return err
}

func SetBackupInterval(userID int64, days int) error {
	_, err := DB.Exec("UPDATE users SET backup_interval_days = ? WHERE id = ?", days, userID)
	return err
}

func GetBackupSettings(userID int64) (int, string, error) {
	var interval sql.NullInt64
	var lastBackup sql.NullString
	err := DB.QueryRow("SELECT backup_interval_days, last_backup_at FROM users WHERE id = ?", userID).Scan(&interval, &lastBackup)
	if err != nil {
		return 7, "", err
	}
	days := 7
	if interval.Valid {
		days = int(interval.Int64)
	}
	return days, lastBackup.String, nil
}

func SetLastBackupTime(userID int64, t time.Time) error {
	_, err := DB.Exec("UPDATE users SET last_backup_at = ? WHERE id = ?", t, userID)
	return err
}

func GetAllUsersWithPCloud() ([]UserBackupInfo, error) {
	rows, err := DB.Query(`
		SELECT id, pcloud_access_token, pcloud_hostname, backup_interval_days, last_backup_at 
		FROM users 
		WHERE pcloud_access_token IS NOT NULL AND pcloud_access_token != ''`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var results []UserBackupInfo
	for rows.Next() {
		var u UserBackupInfo
		var interval sql.NullInt64
		if err := rows.Scan(&u.UserID, &u.AccessToken, &u.Hostname, &interval, &u.LastBackupAt); err != nil {
			return nil, err
		}
		u.BackupIntervalDays = 7
		if interval.Valid {
			u.BackupIntervalDays = int(interval.Int64)
		}
		results = append(results, u)
	}
	return results, nil
}

// Google Drive Integration

type UserGDriveBackupInfo struct {
	UserID             int64
	AccessToken        string
	RefreshToken       string
	FolderID           string
	BackupIntervalDays int
	LastBackupAt       sql.NullString
}

func SetGDriveCredentials(userID int64, accessToken, refreshToken string) error {
	_, err := DB.Exec("UPDATE users SET gdrive_access_token = ?, gdrive_refresh_token = ? WHERE id = ?",
		accessToken, refreshToken, userID)
	return err
}

func GetGDriveCredentials(userID int64) (string, string, error) {
	var accessToken, refreshToken sql.NullString
	err := DB.QueryRow("SELECT gdrive_access_token, gdrive_refresh_token FROM users WHERE id = ?", userID).Scan(&accessToken, &refreshToken)
	if err != nil {
		return "", "", err
	}
	return accessToken.String, refreshToken.String, nil
}

func ClearGDriveCredentials(userID int64) error {
	_, err := DB.Exec("UPDATE users SET gdrive_access_token = NULL, gdrive_refresh_token = NULL, gdrive_folder_id = NULL WHERE id = ?", userID)
	return err
}

func UpdateGDriveAccessToken(userID int64, accessToken string) error {
	_, err := DB.Exec("UPDATE users SET gdrive_access_token = ? WHERE id = ?", accessToken, userID)
	return err
}

func SetGDriveFolderID(userID int64, folderID string) error {
	_, err := DB.Exec("UPDATE users SET gdrive_folder_id = ? WHERE id = ?", folderID, userID)
	return err
}

func GetGDriveFolderID(userID int64) (string, error) {
	var folderID sql.NullString
	err := DB.QueryRow("SELECT gdrive_folder_id FROM users WHERE id = ?", userID).Scan(&folderID)
	if err != nil {
		return "", err
	}
	return folderID.String, nil
}

func GetAllUsersWithGDrive() ([]UserGDriveBackupInfo, error) {
	rows, err := DB.Query(`
		SELECT id, gdrive_access_token, gdrive_refresh_token, COALESCE(gdrive_folder_id, ''), backup_interval_days, last_backup_at 
		FROM users 
		WHERE gdrive_refresh_token IS NOT NULL AND gdrive_refresh_token != ''`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var results []UserGDriveBackupInfo
	for rows.Next() {
		var u UserGDriveBackupInfo
		var interval sql.NullInt64
		if err := rows.Scan(&u.UserID, &u.AccessToken, &u.RefreshToken, &u.FolderID, &interval, &u.LastBackupAt); err != nil {
			return nil, err
		}
		u.BackupIntervalDays = 7
		if interval.Valid {
			u.BackupIntervalDays = int(interval.Int64)
		}
		results = append(results, u)
	}
	return results, nil
}

// -----------------------------------------------------------------------------
// Reminders & Web Push
// -----------------------------------------------------------------------------

type Reminder struct {
	ID               int64          `json:"id"`
	ItemID           sql.NullInt64  `json:"item_id"`
	UserID           int64          `json:"user_id"`
	Name             string         `json:"name"`
	Frequency        string         `json:"frequency"` // Once, Daily, Weekly, Monthly, Yearly
	TimeOfDay        string         `json:"time_of_day"`
	StartDate        string         `json:"start_date"`        // YYYY-MM-DD
	EndDate          sql.NullString `json:"end_date"`          // Optional
	NotificationType string         `json:"notification_type"` // notification_only, email, or both
	Emails           sql.NullString `json:"emails"`            // comma-separated
	LastTriggeredAt  sql.NullString `json:"last_triggered_at"`
	IsPinned         bool           `json:"is_pinned"`
	CreatedAt        string         `json:"created_at"`
}

type PushSubscription struct {
	ID        int64  `json:"id"`
	UserID    int64  `json:"user_id"`
	Endpoint  string `json:"endpoint"`
	P256dh    string `json:"p256dh"`
	Auth      string `json:"auth"`
	CreatedAt string `json:"created_at"`
}

func CreateReminder(r *Reminder) (int64, error) {
	tx, err := DB.Begin()
	if err != nil {
		return 0, err
	}
	defer tx.Rollback()

	// Create generic item first
	result, err := tx.Exec(`
		INSERT INTO items (user_id, title, type) VALUES (?, ?, 'reminder')
	`, r.UserID, r.Name)
	if err != nil {
		return 0, err
	}
	itemID, _ := result.LastInsertId()

	res, err := tx.Exec(`
		INSERT INTO reminders (item_id, user_id, name, frequency, time_of_day, start_date, end_date, notification_type, emails)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)
	`, itemID, r.UserID, r.Name, r.Frequency, r.TimeOfDay, r.StartDate, r.EndDate, r.NotificationType, r.Emails)
	if err != nil {
		return 0, err
	}
	rid, _ := res.LastInsertId()

	if err = tx.Commit(); err != nil {
		return 0, err
	}
	return rid, nil
}

func GetRemindersForUser(userID int64) ([]Reminder, error) {
	rows, err := DB.Query(`
		SELECT id, item_id, user_id, name, frequency, time_of_day, start_date, end_date, notification_type, emails, last_triggered_at, COALESCE(is_pinned, 0), created_at
		FROM reminders
		WHERE user_id = ?
		ORDER BY created_at DESC
	`, userID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var results []Reminder
	for rows.Next() {
		var r Reminder
		var isPinned int
		if err := rows.Scan(
			&r.ID, &r.ItemID, &r.UserID, &r.Name, &r.Frequency, &r.TimeOfDay, &r.StartDate,
			&r.EndDate, &r.NotificationType, &r.Emails, &r.LastTriggeredAt, &isPinned, &r.CreatedAt,
		); err != nil {
			return nil, err
		}
		r.IsPinned = (isPinned == 1)
		results = append(results, r)
	}
	return results, nil
}

func GetDueReminders() ([]Reminder, error) {
	// This function fetches all active reminders. We will filter the actual "is it due right now" logic in the worker.
	// For efficiency, we exclude logically completed "Once" reminders if they have been triggered.
	rows, err := DB.Query(`
		SELECT id, item_id, user_id, name, frequency, time_of_day, start_date, end_date, notification_type, emails, last_triggered_at, COALESCE(is_pinned, 0), created_at
		FROM reminders
		WHERE (frequency != 'Once') OR (frequency = 'Once' AND last_triggered_at IS NULL)
	`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var results []Reminder
	for rows.Next() {
		var r Reminder
		var isPinned int
		if err := rows.Scan(
			&r.ID, &r.ItemID, &r.UserID, &r.Name, &r.Frequency, &r.TimeOfDay, &r.StartDate,
			&r.EndDate, &r.NotificationType, &r.Emails, &r.LastTriggeredAt, &isPinned, &r.CreatedAt,
		); err != nil {
			return nil, err
		}
		r.IsPinned = (isPinned == 1)
		results = append(results, r)
	}
	return results, nil
}

func MarkReminderTriggered(id int64, t time.Time) error {
	_, err := DB.Exec("UPDATE reminders SET last_triggered_at = ? WHERE id = ?", t, id)
	return err
}

func DeleteReminder(id int64, userID int64) error {
	// Fetch item_id first
	var itemID int64
	err := DB.QueryRow("SELECT item_id FROM reminders WHERE id = ? AND user_id = ?", id, userID).Scan(&itemID)
	if err == nil && itemID > 0 {
		DeleteItem(userID, itemID)
	}
	_, err = DB.Exec("DELETE FROM reminders WHERE id = ? AND user_id = ?", id, userID)
	return err
}

func SavePushSubscription(sub *PushSubscription) error {
	// Upsert based on endpoint (we don't want duplicates for the same browser)
	var existingID int64
	err := DB.QueryRow("SELECT id FROM push_subscriptions WHERE endpoint = ? AND user_id = ?", sub.Endpoint, sub.UserID).Scan(&existingID)

	if err == sql.ErrNoRows {
		_, err = DB.Exec(`
			INSERT INTO push_subscriptions (user_id, endpoint, p256dh, auth)
			VALUES (?, ?, ?, ?)
		`, sub.UserID, sub.Endpoint, sub.P256dh, sub.Auth)
		return err
	} else if err != nil {
		return err
	}

	// Update existing keys just in case they refreshed
	_, err = DB.Exec(`
		UPDATE push_subscriptions SET p256dh = ?, auth = ? WHERE id = ?
	`, sub.P256dh, sub.Auth, existingID)
	return err
}

func GetUserPushSubscriptions(userID int64) ([]PushSubscription, error) {
	rows, err := DB.Query("SELECT id, user_id, endpoint, p256dh, auth, created_at FROM push_subscriptions WHERE user_id = ?", userID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var subs []PushSubscription
	for rows.Next() {
		var s PushSubscription
		if err := rows.Scan(&s.ID, &s.UserID, &s.Endpoint, &s.P256dh, &s.Auth, &s.CreatedAt); err != nil {
			return nil, err
		}
		subs = append(subs, s)
	}
	return subs, nil
}

func DeletePushSubscription(endpoint string) error {
	_, err := DB.Exec("DELETE FROM push_subscriptions WHERE endpoint = ?", endpoint)
	return err
}

// ---------------------------------------------------------
// Shared Links Operations
// ---------------------------------------------------------

type SharedLink struct {
	ID        int64     `json:"id"`
	LinkHash  string    `json:"link_hash"`
	ItemType  string    `json:"item_type"` // e.g., "note", "recipe", "bookmark", "list"
	ItemID    int64     `json:"item_id"`
	UserID    int64     `json:"user_id"`
	CreatedAt time.Time `json:"created_at"`
}

func CreateSharedLink(hash, itemType string, itemID, userID int64) error {
	_, err := DB.Exec(`
		INSERT INTO shared_links (link_hash, item_type, item_id, user_id) 
		VALUES (?, ?, ?, ?)
	`, hash, itemType, itemID, userID)
	return err
}

func GetSharedLinkByHash(hash string) (SharedLink, error) {
	var link SharedLink
	err := DB.QueryRow(`
		SELECT id, link_hash, item_type, item_id, user_id, created_at 
		FROM shared_links WHERE link_hash = ?
	`, hash).Scan(&link.ID, &link.LinkHash, &link.ItemType, &link.ItemID, &link.UserID, &link.CreatedAt)
	return link, err
}

func GetSharedLinkByItem(itemType string, itemID, userID int64) (SharedLink, error) {
	var link SharedLink
	err := DB.QueryRow(`
		SELECT id, link_hash, item_type, item_id, user_id, created_at 
		FROM shared_links WHERE item_type = ? AND item_id = ? AND user_id = ?
	`, itemType, itemID, userID).Scan(&link.ID, &link.LinkHash, &link.ItemType, &link.ItemID, &link.UserID, &link.CreatedAt)
	return link, err
}

func DeleteSharedLink(hash string, userID int64) error {
	// Require userID to ensure only the owner can revoke it
	_, err := DB.Exec("DELETE FROM shared_links WHERE link_hash = ? AND user_id = ?", hash, userID)
	return err
}

// GetDefaultPage returns the user's configured default landing page (e.g., "dashboard", "bookmarks").
// Falls back to "dashboard" if not set.
func GetDefaultPage(userID int64) string {
	var page string
	err := DB.QueryRow("SELECT COALESCE(default_page, 'dashboard') FROM users WHERE id = ?", userID).Scan(&page)
	if err != nil || page == "" {
		return "dashboard"
	}
	return page
}

// SetDefaultPage updates the user's default landing page preference.
func SetDefaultPage(userID int64, page string) error {
	_, err := DB.Exec("UPDATE users SET default_page = ? WHERE id = ?", page, userID)
	return err
}

// GetPinnedItems returns all pinned items for a user across all types.
// Each result has: id, type, title, url (for bookmarks), thumbnail, favicon.
func GetPinnedItems(userID int64) ([]map[string]interface{}, error) {
	rows, err := DB.Query(`
		SELECT i.id, i.type, i.title,
			COALESCE(b.url, ''),
			COALESCE(b.thumbnail, r.thumbnail, d.file_path, m.file_path, ''),
			COALESCE(b.favicon, '')
		FROM items i
		LEFT JOIN bookmarks b ON i.id = b.item_id
		LEFT JOIN recipes r ON i.id = r.item_id
		LEFT JOIN drawings d ON i.id = d.item_id
		LEFT JOIN media m ON i.id = m.item_id
		WHERE i.user_id = ? AND i.is_pinned = 1
		ORDER BY i.updated_at DESC
	`, userID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var results []map[string]interface{}
	for rows.Next() {
		var id int64
		var itemType, title, rawURL, thumbnail, favicon string
		if err := rows.Scan(&id, &itemType, &title, &rawURL, &thumbnail, &favicon); err != nil {
			return nil, err
		}
		// Generate favicon if missing
		if favicon == "" && rawURL != "" {
			if domain := extractDomain(rawURL); domain != "" {
				favicon = "https://www.google.com/s2/favicons?domain=" + domain + "&sz=32"
			}
		}
		tags, _ := GetItemTags(id)
		results = append(results, map[string]interface{}{
			"id":        id,
			"type":      itemType,
			"title":     title,
			"url":       rawURL,
			"thumbnail": thumbnail,
			"favicon":   favicon,
			"tags":      tags,
		})
	}
	return results, nil
}

// TogglePinItem flips the is_pinned flag for an item owned by the user.
// Returns the new pinned state (true = now pinned).
func TogglePinItem(itemID, userID int64) (bool, error) {
	// Check items table first
	var current int
	err := DB.QueryRow("SELECT COALESCE(is_pinned, 0) FROM items WHERE id = ? AND user_id = ?", itemID, userID).Scan(&current)
	if err == nil {
		newVal := 0
		if current == 0 {
			newVal = 1
		}
		_, err = DB.Exec("UPDATE items SET is_pinned = ? WHERE id = ? AND user_id = ?", newVal, itemID, userID)
		return newVal == 1, err
	}

	// If not in items, check reminders table
	err = DB.QueryRow("SELECT COALESCE(is_pinned, 0) FROM reminders WHERE id = ? AND user_id = ?", itemID, userID).Scan(&current)
	if err == nil {
		newVal := 0
		if current == 0 {
			newVal = 1
		}
		_, err = DB.Exec("UPDATE reminders SET is_pinned = ? WHERE id = ? AND user_id = ?", newVal, itemID, userID)
		return newVal == 1, err
	}

	return false, err
}
