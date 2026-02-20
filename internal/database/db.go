package database

import (
	"crypto/rand"
	"database/sql"
	"encoding/hex"
	"fmt"
	"log"
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
	`
	_, err := DB.Exec(schema)
	// Handle migrations
	_, _ = DB.Exec("ALTER TABLE bookmarks ADD COLUMN thumbnail TEXT")
	_, _ = DB.Exec("ALTER TABLE recipes ADD COLUMN source_url TEXT")
	_, _ = DB.Exec("ALTER TABLE users ADD COLUMN api_token TEXT")
	return err
}

func runMigrations() error {
	// Check if user_id column exists in items table
	rows, err := DB.Query("PRAGMA table_info(items)")
	if err != nil {
		return err
	}
	defer rows.Close()

	hasUserID := false
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
			break
		}
	}

	if !hasUserID {
		log.Println("Migrating database: Adding user_id column to items table")
		_, err = DB.Exec("ALTER TABLE items ADD COLUMN user_id INTEGER")
		if err != nil {
			return err
		}
		// Optional: assign existing items to the first user if any exist
		// This is a simple heuristic for single-user to multi-user migration
		var firstUserID int64
		err = DB.QueryRow("SELECT id FROM users ORDER BY id ASC LIMIT 1").Scan(&firstUserID)
		if err == nil {
			log.Printf("Assigning existing items to first user (ID: %d)\n", firstUserID)
			DB.Exec("UPDATE items SET user_id = ? WHERE user_id IS NULL", firstUserID)
		}
	}

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
		SELECT i.id, i.title, i.created_at, d.file_path 
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
		var title, createdAt, filePath sql.NullString
		if err := rows.Scan(&id, &title, &createdAt, &filePath); err != nil {
			return nil, err
		}

		tags, _ := GetItemTags(id)
		results = append(results, map[string]interface{}{
			"id":         id,
			"title":      title.String,
			"created_at": createdAt.String,
			"file_path":  filePath.String,
			"tags":       tags,
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
		SELECT i.id, i.title, i.created_at, b.url, b.description, b.favicon, b.thumbnail 
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
		var title, createdAt, url, description, favicon, thumbnail sql.NullString
		if err := rows.Scan(&id, &title, &createdAt, &url, &description, &favicon, &thumbnail); err != nil {
			return nil, err
		}

		tags, _ := GetItemTags(id)
		results = append(results, map[string]interface{}{
			"id":          id,
			"title":       title.String,
			"created_at":  createdAt.String,
			"url":         url.String,
			"description": description.String,
			"favicon":     favicon.String,
			"thumbnail":   thumbnail.String,
			"tags":        tags,
		})
	}
	return results, nil
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
		SELECT i.id, i.title, i.created_at, n.content 
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
		var title, createdAt, content sql.NullString
		if err := rows.Scan(&id, &title, &createdAt, &content); err != nil {
			return nil, err
		}

		tags, _ := GetItemTags(id)
		results = append(results, map[string]interface{}{
			"id":         id,
			"title":      title.String,
			"created_at": createdAt.String,
			"content":    content.String,
			"tags":       tags,
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

func GetRatedLists(userID int64, tagFilter string) ([]map[string]interface{}, error) {
	query := `
		SELECT i.id, i.title, i.created_at 
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
		var title, createdAt sql.NullString
		if err := rows.Scan(&id, &title, &createdAt); err != nil {
			return nil, err
		}
		tags, _ := GetItemTags(id)
		results = append(results, map[string]interface{}{
			"id":         id,
			"title":      title.String,
			"created_at": createdAt.String,
			"tags":       tags,
		})
	}
	return results, nil
}

func AddRatedListItem(listID int64, title string, score int, note string) error {
	_, err := DB.Exec("INSERT INTO rated_list_items (rated_list_id, title, score, note) VALUES (?, ?, ?, ?)",
		listID, title, score, note)
	return err
}

func GetRatedListItems(listID int64) ([]map[string]interface{}, error) {
	rows, err := DB.Query("SELECT id, title, score, note FROM rated_list_items WHERE rated_list_id = ? ORDER BY score DESC, title ASC", listID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var results []map[string]interface{}
	for rows.Next() {
		var id int64
		var title string
		var note sql.NullString
		var score int
		if err := rows.Scan(&id, &title, &score, &note); err != nil {
			return nil, err
		}
		results = append(results, map[string]interface{}{
			"id":    id,
			"title": title,
			"score": score,
			"note":  note.String,
		})
	}
	return results, nil
}

func GetRatedListItem(id int64) (map[string]interface{}, error) {
	var title, note sql.NullString
	var score int
	err := DB.QueryRow("SELECT title, score, note FROM rated_list_items WHERE id = ?", id).Scan(&title, &score, &note)
	if err != nil {
		return nil, err
	}
	return map[string]interface{}{
		"id":    id,
		"title": title.String,
		"score": score,
		"note":  note.String,
	}, nil
}

func UpdateRatedListItem(id int64, title string, score int, note string) error {
	_, err := DB.Exec("UPDATE rated_list_items SET title = ?, score = ?, note = ? WHERE id = ?", title, score, note, id)
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
		SELECT i.id, i.title, i.created_at 
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
		var title, createdAt sql.NullString
		if err := rows.Scan(&id, &title, &createdAt); err != nil {
			return nil, err
		}

		tags, _ := GetItemTags(id)
		results = append(results, map[string]interface{}{
			"id":         id,
			"title":      title.String,
			"created_at": createdAt.String,
			"tags":       tags,
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
		SELECT i.id, i.title, i.created_at, m.file_path, m.mime_type
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
		var title, createdAt, filePath, mimeType sql.NullString
		if err := rows.Scan(&id, &title, &createdAt, &filePath, &mimeType); err != nil {
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
		})
	}
	return results, nil
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
		SELECT i.id, i.title, i.created_at, r.ingredients, r.instructions, r.notes, r.thumbnail, r.source_url 
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
		var title, createdAt, ingredients, instructions, notes, thumbnail, sourceURL sql.NullString
		if err := rows.Scan(&id, &title, &createdAt, &ingredients, &instructions, &notes, &thumbnail, &sourceURL); err != nil {
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
