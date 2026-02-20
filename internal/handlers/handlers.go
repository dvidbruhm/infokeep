package handlers

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"html/template"
	"infokeep/internal/database"
	"io"
	"log"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"
	"golang.org/x/crypto/bcrypt"
)

type contextKey string

const userIDKey contextKey = "user_id"

func getUserID(r *http.Request) int64 {
	userID, ok := r.Context().Value(userIDKey).(int64)
	if !ok {
		return 0
	}
	return userID
}

func getTagColor(tag string) string {
	tag = strings.ToLower(strings.TrimSpace(tag))
	// Hash-like deterministic color selection
	colors := []string{"is-info", "is-success", "is-warning", "is-danger", "is-primary", "is-link"}
	sum := 0
	for _, char := range tag {
		sum += int(char)
	}
	return colors[sum%len(colors)]
}

func parseTags(tagsStr string) []string {
	if tagsStr == "" {
		return nil
	}
	parts := strings.Split(tagsStr, ",")
	var tags []string
	for _, p := range parts {
		t := strings.TrimSpace(p)
		if t != "" {
			tags = append(tags, t)
		}
	}
	return tags
}

func RenderTemplate(w http.ResponseWriter, tmpl string, data interface{}) {
	tmplPath := filepath.Join("web", "templates", tmpl)
	layoutPath := filepath.Join("web", "templates", "layout.html")

	files := []string{layoutPath, tmplPath}
	fragments, _ := filepath.Glob(filepath.Join("web", "templates", "fragments", "*.html"))
	files = append(files, fragments...)

	t, err := template.New(filepath.Base(layoutPath)).Funcs(template.FuncMap{
		"getTagColor": getTagColor,
	}).ParseFiles(files...)

	if err != nil {
		fmt.Printf("RenderTemplate Parse Error: %v\n", err)
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	err = t.ExecuteTemplate(w, "layout.html", data)
	if err != nil {
		fmt.Printf("RenderTemplate Execute Error: %v\n", err)
		http.Error(w, err.Error(), http.StatusInternalServerError)
	}
}

func RenderFragment(w http.ResponseWriter, tmpl string, data interface{}) {
	tmplPath := filepath.Join("web", "templates", "fragments", tmpl)
	t, err := template.New(filepath.Base(tmplPath)).Funcs(template.FuncMap{
		"getTagColor": getTagColor,
	}).ParseFiles(tmplPath)
	if err != nil {
		fmt.Printf("RenderFragment Parse Error: %v\n", err)
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	err = t.Execute(w, data)
	if err != nil {
		fmt.Printf("RenderFragment Execute Error: %v\n", err)
		http.Error(w, err.Error(), http.StatusInternalServerError)
	}
}

func IndexHandler(w http.ResponseWriter, r *http.Request) {
	userID := getUserID(r)
	tagFilter := r.URL.Query().Get("tag")
	bookmarks, _ := database.GetBookmarks(userID, tagFilter)
	notes, _ := database.GetNotes(userID, tagFilter)
	drawings, _ := database.GetDrawings(userID, tagFilter)
	ratedLists, _ := database.GetRatedLists(userID, tagFilter)
	checklists, _ := database.GetLists(userID, tagFilter)
	media, _ := database.GetMedia(userID, tagFilter)
	recipes, _ := database.GetRecipes(userID, tagFilter)
	tags, _ := database.GetTagsWithCounts(userID)

	data := map[string]interface{}{
		"Bookmarks":  bookmarks,
		"Notes":      notes,
		"Drawings":   drawings,
		"RatedLists": ratedLists,
		"Checklists": checklists,
		"Media":      media,
		"Recipes":    recipes,
		"Tags":       tags,
		"ActiveTag":  tagFilter,
	}
	RenderTemplate(w, "index.html", data)
}

func fetchThumbnail(targetURL string) string {
	client := &http.Client{
		Timeout: 5 * time.Second,
	}
	resp, err := client.Get(targetURL)
	if err != nil {
		fmt.Printf("Error fetching thumbnail for %s: %v\n", targetURL, err)
		return ""
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		fmt.Printf("Thumbnail fetch returned status %d for %s\n", resp.StatusCode, targetURL)
		return ""
	}

	body, err := io.ReadAll(io.LimitReader(resp.Body, 1024*100)) // Limit to 100KB
	if err != nil {
		return ""
	}

	htmlBody := string(body)

	// More robust regex for og:image
	ogImageRegex := regexp.MustCompile(`(?i)<meta\s+[^>]*?(?:property|name)=["']og:image["']\s+[^>]*?content=["']([^"']+)["']|<meta\s+[^>]*?content=["']([^"']+)["']\s+[^>]*?(?:property|name)=["']og:image["']`)
	matches := ogImageRegex.FindStringSubmatch(htmlBody)
	if len(matches) > 1 {
		if matches[1] != "" {
			return matches[1]
		}
		if len(matches) > 2 && matches[2] != "" {
			return matches[2]
		}
	}

	// Fallback to twitter:image
	twitterImageRegex := regexp.MustCompile(`(?i)<meta\s+[^>]*?(?:property|name)=["']twitter:image["']\s+[^>]*?content=["']([^"']+)["']|<meta\s+[^>]*?content=["']([^"']+)["']\s+[^>]*?(?:property|name)=["']twitter:image["']`)
	matches = twitterImageRegex.FindStringSubmatch(htmlBody)
	if len(matches) > 1 {
		if matches[1] != "" {
			return matches[1]
		}
		if len(matches) > 2 && matches[2] != "" {
			return matches[2]
		}
	}

	return ""
}

func getFaviconURL(targetURL string) string {
	u, err := url.Parse(targetURL)
	if err != nil {
		return ""
	}
	return fmt.Sprintf("%s://%s/favicon.ico", u.Scheme, u.Host)
}

func BookmarkHandler(w http.ResponseWriter, r *http.Request) {
	userID := getUserID(r)
	if r.Method == http.MethodPost {
		title := r.FormValue("title")
		targetURL := r.FormValue("url")
		description := r.FormValue("description")
		tags := strings.Split(r.FormValue("tags"), ",")

		// Clean tags
		var cleanTags []string
		for _, t := range tags {
			trimmed := strings.TrimSpace(t)
			if trimmed != "" {
				cleanTags = append(cleanTags, trimmed)
			}
		}

		// Try to fetch favicon
		favicon := ""
		if targetURL != "" {
			favicon = getFaviconURL(targetURL)
		}

		// Try to fetch thumbnail
		thumbnail := fetchThumbnail(targetURL)

		itemID, err := database.CreateBookmark(userID, title, targetURL, description, favicon, thumbnail)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}

		if len(cleanTags) > 0 {
			database.SetItemTags(itemID, cleanTags)
		}

		// Return fragment if HTMX
		if r.Header.Get("HX-Request") != "" {
			bookmarks, _ := database.GetBookmarks(userID, "")
			RenderFragment(w, "bookmark_list.html", bookmarks)
			return
		}
	}

	tagFilter := r.URL.Query().Get("tag")
	bookmarks, _ := database.GetBookmarks(userID, tagFilter)

	if r.Header.Get("HX-Request") != "" && r.Method == http.MethodGet {
		RenderFragment(w, "bookmark_list.html", bookmarks)
		return
	}

	tagsWithCounts, _ := database.GetTagsWithCounts(userID)
	data := map[string]interface{}{
		"Bookmarks": bookmarks,
		"Tags":      tagsWithCounts,
		"ActiveTag": tagFilter,
	}
	RenderTemplate(w, "bookmarks.html", data)
}

func GetBookmarkHandler(w http.ResponseWriter, r *http.Request) {
	userID := getUserID(r)
	id, _ := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	bookmark, err := database.GetBookmark(userID, id)
	if err != nil {
		http.Error(w, "Bookmark not found", http.StatusNotFound)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(bookmark)
}

func UpdateBookmarkHandler(w http.ResponseWriter, r *http.Request) {
	userID := getUserID(r)
	id, _ := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	title := r.FormValue("title")
	url := r.FormValue("url")
	description := r.FormValue("description")
	tags := strings.Split(r.FormValue("tags"), ",")

	if title == "" || url == "" {
		http.Error(w, "Title and URL are required", http.StatusBadRequest)
		return
	}

	err := database.UpdateBookmark(userID, id, title, url, description)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	var cleanTags []string
	for _, t := range tags {
		trimmed := strings.TrimSpace(t)
		if trimmed != "" {
			cleanTags = append(cleanTags, trimmed)
		}
	}
	database.SetItemTags(id, cleanTags)

	// Return fragment if HTMX
	if r.Header.Get("HX-Request") != "" {
		bookmarks, _ := database.GetBookmarks(userID, "")
		data := map[string]interface{}{
			"Bookmarks": bookmarks,
		}
		RenderFragment(w, "bookmark_list.html", data)
		return
	}

	http.Redirect(w, r, "/bookmarks", http.StatusFound)
}

func NoteHandler(w http.ResponseWriter, r *http.Request) {
	userID := getUserID(r)
	if r.Method == http.MethodPost {
		title := r.FormValue("title")
		content := r.FormValue("content")

		tags := strings.Split(r.FormValue("tags"), ",")

		itemID, err := database.CreateNote(userID, title, content)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}

		var cleanTags []string
		for _, t := range tags {
			trimmed := strings.TrimSpace(t)
			if trimmed != "" {
				cleanTags = append(cleanTags, trimmed)
			}
		}
		if len(cleanTags) > 0 {
			database.SetItemTags(itemID, cleanTags)
		}

		if r.Header.Get("HX-Request") != "" {
			notes, _ := database.GetNotes(userID, "")
			RenderFragment(w, "note_list.html", notes)
			return
		}
	}

	tagFilter := r.URL.Query().Get("tag")
	notes, _ := database.GetNotes(userID, tagFilter)

	if r.Header.Get("HX-Request") != "" && r.Method == http.MethodGet {
		RenderFragment(w, "note_list.html", notes)
		return
	}

	tagsWithCounts, _ := database.GetTagsWithCounts(userID)
	data := map[string]interface{}{
		"Notes":     notes,
		"Tags":      tagsWithCounts,
		"ActiveTag": tagFilter,
	}
	RenderTemplate(w, "notes.html", data)
}

func GetNoteHandler(w http.ResponseWriter, r *http.Request) {
	userID := getUserID(r)
	id, _ := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	note, err := database.GetNote(userID, id)
	if err != nil {
		http.Error(w, "Note not found", http.StatusNotFound)
		return
	}
	json.NewEncoder(w).Encode(note)
}

func UpdateNoteHandler(w http.ResponseWriter, r *http.Request) {
	userID := getUserID(r)
	id, _ := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	title := r.FormValue("title")
	content := r.FormValue("content")
	tags := strings.Split(r.FormValue("tags"), ",")

	if title == "" {
		http.Error(w, "Title is required", http.StatusBadRequest)
		return
	}

	err := database.UpdateNote(userID, id, title, content)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	var cleanTags []string
	for _, t := range tags {
		trimmed := strings.TrimSpace(t)
		if trimmed != "" {
			cleanTags = append(cleanTags, trimmed)
		}
	}
	database.SetItemTags(id, cleanTags)

	if r.Header.Get("HX-Request") != "" {
		notes, _ := database.GetNotes(userID, "")
		data := map[string]interface{}{
			"Notes": notes,
		}
		RenderFragment(w, "note_list.html", data)
		return
	}

	http.Redirect(w, r, "/notes", http.StatusFound)
}

func RatedListHandler(w http.ResponseWriter, r *http.Request) {
	userID := getUserID(r)
	if r.Method == http.MethodPost {
		title := r.FormValue("title")
		tags := parseTags(r.FormValue("tags"))

		itemID, err := database.CreateRatedList(userID, title)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}

		if len(tags) > 0 {
			database.SetItemTags(itemID, tags)
		}

		// Return fragment if HTMX
		if r.Header.Get("HX-Request") != "" {
			lists, _ := database.GetRatedLists(userID, "")
			RenderFragment(w, "rated_list_nav.html", lists)
			return
		}
	}

	tagFilter := r.URL.Query().Get("tag")
	lists, _ := database.GetRatedLists(userID, tagFilter)

	if r.Header.Get("HX-Request") != "" && r.Method == http.MethodGet {
		RenderFragment(w, "rated_list_nav.html", lists)
		return
	}

	tagsWithCounts, _ := database.GetTagsWithCounts(userID)

	activeID := r.URL.Query().Get("id")
	data := map[string]interface{}{
		"Lists":     lists,
		"ActiveID":  activeID,
		"Tags":      tagsWithCounts,
		"ActiveTag": tagFilter,
	}
	RenderTemplate(w, "rated_lists.html", data)
}

func RatedListItemHandler(w http.ResponseWriter, r *http.Request) {
	listIDStr := chi.URLParam(r, "id")
	var listID int64
	fmt.Sscanf(listIDStr, "%d", &listID)

	if r.Method == http.MethodPost {
		title := r.FormValue("title")
		scoreStr := r.FormValue("score")
		var score int
		fmt.Sscanf(scoreStr, "%d", &score)
		note := r.FormValue("note")

		err := database.AddRatedListItem(listID, title, score, note)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}

		items, _ := database.GetRatedListItems(listID)
		RenderFragment(w, "rated_list_items.html", items)
		return
	}

	items, _ := database.GetRatedListItems(listID)
	RenderFragment(w, "rated_list_items.html", items)
}

func GetRatedListItemHandler(w http.ResponseWriter, r *http.Request) {
	idStr := chi.URLParam(r, "id")
	var id int64
	fmt.Sscanf(idStr, "%d", &id)

	item, err := database.GetRatedListItem(id)
	if err != nil {
		http.Error(w, "Item not found", http.StatusNotFound)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(item)
}

func UpdateRatedListItemHandler(w http.ResponseWriter, r *http.Request) {
	idStr := chi.URLParam(r, "id")
	var id int64
	fmt.Sscanf(idStr, "%d", &id)

	title := r.FormValue("title")
	scoreStr := r.FormValue("score")
	var score int
	fmt.Sscanf(scoreStr, "%d", &score)
	note := r.FormValue("note")

	err := database.UpdateRatedListItem(id, title, score, note)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	// We need the listID to re-render the list.
	// The client should send it or we can look it up.
	// Actually, the client knows the listID from the context.
	listIDStr := r.FormValue("list_id")
	var listID int64
	fmt.Sscanf(listIDStr, "%d", &listID)

	items, _ := database.GetRatedListItems(listID)
	RenderFragment(w, "rated_list_items.html", items)
}

func ListHandler(w http.ResponseWriter, r *http.Request) {
	userID := getUserID(r)
	if r.Method == http.MethodPost {
		title := r.FormValue("title")
		tags := parseTags(r.FormValue("tags"))
		itemID, err := database.CreateList(userID, title)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		if len(tags) > 0 {
			database.SetItemTags(itemID, tags)
		}

		if r.Header.Get("HX-Request") != "" {
			lists, _ := database.GetLists(userID, "")
			RenderFragment(w, "list_nav.html", lists)
			return
		}
	}

	tagFilter := r.URL.Query().Get("tag")
	lists, _ := database.GetLists(userID, tagFilter)

	if r.Header.Get("HX-Request") != "" && r.Method == http.MethodGet {
		RenderFragment(w, "list_nav.html", lists)
		return
	}

	tagsWithCounts, _ := database.GetTagsWithCounts(userID)

	activeID := r.URL.Query().Get("id")
	data := map[string]interface{}{
		"Lists":     lists,
		"ActiveID":  activeID,
		"Tags":      tagsWithCounts,
		"ActiveTag": tagFilter,
	}
	RenderTemplate(w, "lists.html", data)
}

func ListItemHandler(w http.ResponseWriter, r *http.Request) {
	listIDStr := chi.URLParam(r, "id")
	var listID int64
	fmt.Sscanf(listIDStr, "%d", &listID)

	if r.Method == http.MethodPost {
		content := r.FormValue("content")
		err := database.AddListItem(listID, content)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
	}

	items, _ := database.GetListItems(listID)
	RenderFragment(w, "list_items.html", items)
}

func GetListItemByIdHandler(w http.ResponseWriter, r *http.Request) {
	idStr := chi.URLParam(r, "id")
	var id int64
	fmt.Sscanf(idStr, "%d", &id)

	item, err := database.GetListItemById(id)
	if err != nil {
		http.Error(w, "Item not found", http.StatusNotFound)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(item)
}

func UpdateListItemHandler(w http.ResponseWriter, r *http.Request) {
	idStr := chi.URLParam(r, "id")
	var id int64
	fmt.Sscanf(idStr, "%d", &id)

	content := r.FormValue("content")

	err := database.UpdateListItem(id, content)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	listIDStr := r.FormValue("list_id")
	var listID int64
	fmt.Sscanf(listIDStr, "%d", &listID)

	items, _ := database.GetListItems(listID)
	RenderFragment(w, "list_items.html", items)
}

func ToggleListItemHandler(w http.ResponseWriter, r *http.Request) {
	itemIDStr := chi.URLParam(r, "itemID")
	var itemID int64
	fmt.Sscanf(itemIDStr, "%d", &itemID)

	completed := r.FormValue("completed") == "true"
	database.ToggleListItem(itemID, completed)

	w.WriteHeader(http.StatusOK)
}

func MediaHandler(w http.ResponseWriter, r *http.Request) {
	userID := getUserID(r)
	if r.Method == http.MethodPost {
		file, header, err := r.FormFile("file")
		if err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		defer file.Close()

		title := r.FormValue("title")
		if title == "" {
			title = header.Filename
		}

		ext := filepath.Ext(header.Filename)
		fileName := fmt.Sprintf("%d%s", time.Now().UnixNano(), ext)
		savePath := filepath.Join("web", "static", "uploads", fileName)

		out, err := os.Create(savePath)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		defer out.Close()

		_, err = io.Copy(out, file)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}

		tags := parseTags(r.FormValue("tags"))
		relPath := "/static/uploads/" + fileName
		itemID, err := database.CreateMedia(userID, title, relPath, header.Header.Get("Content-Type"))
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		if len(tags) > 0 {
			database.SetItemTags(itemID, tags)
		}

		if r.Header.Get("HX-Request") != "" {
			media, _ := database.GetMedia(userID, "")
			RenderFragment(w, "media_grid.html", media)
			return
		}
	}

	tagFilter := r.URL.Query().Get("tag")
	media, _ := database.GetMedia(userID, tagFilter)

	if r.Header.Get("HX-Request") != "" && r.Method == http.MethodGet {
		RenderFragment(w, "media_grid.html", media)
		return
	}

	tagsWithCounts, _ := database.GetTagsWithCounts(userID)
	data := map[string]interface{}{
		"Media":     media,
		"Tags":      tagsWithCounts,
		"ActiveTag": tagFilter,
	}
	RenderTemplate(w, "media.html", data)
}

func DrawingsHandler(w http.ResponseWriter, r *http.Request) {
	userID := getUserID(r)
	tagFilter := r.URL.Query().Get("tag")
	drawings, err := database.GetDrawings(userID, tagFilter)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	if r.Header.Get("HX-Request") != "" {
		RenderFragment(w, "drawing_list.html", drawings)
		return
	}

	tagsWithCounts, _ := database.GetTagsWithCounts(userID)
	data := map[string]interface{}{
		"Drawings":  drawings,
		"Tags":      tagsWithCounts,
		"ActiveTag": tagFilter,
	}
	RenderTemplate(w, "drawings.html", data)
}

func CreateDrawingHandler(w http.ResponseWriter, r *http.Request) {
	userID := getUserID(r)
	err := r.ParseForm()
	if err != nil {
		fmt.Printf("ParseForm Error: %v\n", err)
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	title := r.FormValue("title")
	imageData := r.FormValue("image") // Base64 data URL

	if title == "" || imageData == "" {
		http.Error(w, "Title and image data are required", http.StatusBadRequest)
		return
	}

	// Remove data:image/png;base64, prefix
	b64data := imageData[strings.IndexByte(imageData, ',')+1:]
	decoded, err := base64.StdEncoding.DecodeString(b64data)
	if err != nil {
		http.Error(w, "Invalid image data", http.StatusBadRequest)
		return
	}

	// Save to uploads folder
	filename := fmt.Sprintf("drawing_%d.png", time.Now().UnixNano())
	savePath := filepath.Join("web", "static", "uploads", filename)

	// Ensure directory exists
	if err := os.MkdirAll(filepath.Dir(savePath), 0755); err != nil {
		http.Error(w, "Failed to create uploads directory", http.StatusInternalServerError)
		return
	}

	err = os.WriteFile(savePath, decoded, 0644)
	if err != nil {
		fmt.Printf("Drawing save error: %v\n", err)
		http.Error(w, "Failed to save drawing", http.StatusInternalServerError)
		return
	}

	tags := parseTags(r.FormValue("tags"))
	relPath := "/static/uploads/" + filename
	itemID, err := database.CreateDrawing(userID, title, relPath)
	if err != nil {
		fmt.Printf("CreateDrawing DB Error: %v\n", err)
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	if len(tags) > 0 {
		database.SetItemTags(itemID, tags)
	}

	// Redirect or return success
	w.Header().Set("HX-Trigger", "newDrawing")
	DrawingsHandler(w, r)
}

func GetDrawingHandler(w http.ResponseWriter, r *http.Request) {
	userID := getUserID(r)
	id, _ := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	drawing, err := database.GetDrawing(userID, id)
	if err != nil {
		http.Error(w, "Drawing not found", http.StatusNotFound)
		return
	}
	json.NewEncoder(w).Encode(drawing)
}

func UpdateDrawingHandler(w http.ResponseWriter, r *http.Request) {
	idStr := chi.URLParam(r, "id")
	var id int64
	fmt.Sscanf(idStr, "%d", &id)

	err := r.ParseForm()
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	title := r.FormValue("title")
	imageData := r.FormValue("image") // Base64 data URL

	if title == "" {
		http.Error(w, "Title is required", http.StatusBadRequest)
		return
	}

	var relPath string
	if imageData != "" {
		// New image provided, save it
		b64data := imageData[strings.IndexByte(imageData, ',')+1:]
		decoded, err := base64.StdEncoding.DecodeString(b64data)
		if err != nil {
			http.Error(w, "Invalid image data", http.StatusBadRequest)
			return
		}

		filename := fmt.Sprintf("drawing_%d.png", time.Now().UnixNano())
		savePath := filepath.Join("web", "static", "uploads", filename)

		if err := os.MkdirAll(filepath.Dir(savePath), 0755); err != nil {
			http.Error(w, "Failed to create directory", http.StatusInternalServerError)
			return
		}

		err = os.WriteFile(savePath, decoded, 0644)
		if err != nil {
			http.Error(w, "Failed to save drawing", http.StatusInternalServerError)
			return
		}
		relPath = "/static/uploads/" + filename
	}

	tags := parseTags(r.FormValue("tags"))
	err = database.UpdateDrawing(id, title, relPath)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	database.SetItemTags(id, tags)

	w.Header().Set("HX-Trigger", "newDrawing")
	DrawingsHandler(w, r)
}

func SettingsHandler(w http.ResponseWriter, r *http.Request) {
	userID := getUserID(r)
	token, _ := database.GetAPIToken(userID)
	RenderTemplate(w, "settings.html", map[string]interface{}{
		"APIToken": token,
	})
}

func HealthHandler(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]string{"status": "ok"})
}

func RegenerateTokenHandler(w http.ResponseWriter, r *http.Request) {
	userID := getUserID(r)
	token, err := database.RegenerateAPIToken(userID)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]string{"token": token})
}

func DeleteItemHandler(w http.ResponseWriter, r *http.Request) {
	itemIDStr := chi.URLParam(r, "id")
	var itemID int64
	fmt.Sscanf(itemIDStr, "%d", &itemID)

	userID := getUserID(r)
	err := database.DeleteItem(userID, itemID)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	w.WriteHeader(http.StatusOK)
}

func DeleteListItemHandler(w http.ResponseWriter, r *http.Request) {
	idStr := chi.URLParam(r, "id")
	var id int64
	fmt.Sscanf(idStr, "%d", &id)

	err := database.DeleteListItem(id)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	w.WriteHeader(http.StatusOK)
}

func DeleteRatedListItemHandler(w http.ResponseWriter, r *http.Request) {
	idStr := chi.URLParam(r, "id")
	var id int64
	fmt.Sscanf(idStr, "%d", &id)

	err := database.DeleteRatedListItem(id)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	w.WriteHeader(http.StatusOK)
}

// Recipes

func RecipeHandler(w http.ResponseWriter, r *http.Request) {
	userID := getUserID(r)
	tagFilter := r.URL.Query().Get("tag")
	recipes, err := database.GetRecipes(userID, tagFilter)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	if r.Header.Get("HX-Request") != "" {
		RenderFragment(w, "recipe_list.html", recipes)
		return
	}

	tagsWithCounts, _ := database.GetTagsWithCounts(userID)
	data := map[string]interface{}{
		"Recipes":   recipes,
		"Tags":      tagsWithCounts,
		"ActiveTag": tagFilter,
	}
	RenderTemplate(w, "recipes.html", data)
}

func CreateRecipeHandler(w http.ResponseWriter, r *http.Request) {
	userID := getUserID(r)
	if err := r.ParseMultipartForm(32 << 20); err != nil { // 32MB max
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	title := r.FormValue("title")
	ingredients := r.FormValue("ingredients")
	instructions := r.FormValue("instructions")
	notes := r.FormValue("notes")
	thumbnail := r.FormValue("thumbnail")
	sourceURL := r.FormValue("source_url")
	tags := parseTags(r.FormValue("tags"))

	// Handle multiple image uploads
	var imagePaths []string
	files := r.MultipartForm.File["images"]
	for _, fileHeader := range files {
		file, err := fileHeader.Open()
		if err != nil {
			continue
		}
		defer file.Close()

		// Save file
		ext := filepath.Ext(fileHeader.Filename)
		fileName := fmt.Sprintf("%d%s", time.Now().UnixNano(), ext)
		savePath := filepath.Join("web", "static", "uploads", fileName)

		out, err := os.Create(savePath)
		if err != nil {
			continue
		}
		defer out.Close()

		if _, err = io.Copy(out, file); err != nil {
			continue
		}

		imagePaths = append(imagePaths, "/static/uploads/"+fileName)
	}

	itemID, err := database.CreateRecipe(userID, title, ingredients, instructions, notes, thumbnail, sourceURL, imagePaths)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	if len(tags) > 0 {
		database.SetItemTags(itemID, tags)
	}

	recipes, _ := database.GetRecipes(userID, "")
	RenderFragment(w, "recipe_list.html", recipes)
}

func GetRecipeHandler(w http.ResponseWriter, r *http.Request) {
	userID := getUserID(r)
	idStr := chi.URLParam(r, "id")
	id, err := strconv.ParseInt(idStr, 10, 64)
	if err != nil {
		http.Error(w, "Invalid ID", http.StatusBadRequest)
		return
	}

	recipe, err := database.GetRecipe(userID, id)
	if err != nil {
		http.Error(w, err.Error(), http.StatusNotFound)
		return
	}

	// Check if JSON is requested (for edit modal or API)
	if r.Header.Get("Accept") == "application/json" || r.URL.Query().Get("json") == "true" {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(recipe)
		return
	}

	// Prepare data for HTML view
	var ingredientsList []string
	if ing, ok := recipe["ingredients"].(string); ok && ing != "" {
		for _, s := range strings.Split(ing, "\n") {
			if trimmed := strings.TrimSpace(s); trimmed != "" {
				ingredientsList = append(ingredientsList, trimmed)
			}
		}
	}

	var instructionsList []string
	if inst, ok := recipe["instructions"].(string); ok && inst != "" {
		for _, s := range strings.Split(inst, "\n") {
			if trimmed := strings.TrimSpace(s); trimmed != "" {
				instructionsList = append(instructionsList, trimmed)
			}
		}
	}

	data := map[string]interface{}{
		"Recipe":           recipe,
		"IngredientsList":  ingredientsList,
		"InstructionsList": instructionsList,
	}

	RenderTemplate(w, "recipe_detail_page.html", data)
}

func UpdateRecipeHandler(w http.ResponseWriter, r *http.Request) {
	userID := getUserID(r)
	id, _ := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)

	if err := r.ParseMultipartForm(32 << 20); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	title := r.FormValue("title")
	ingredients := r.FormValue("ingredients")
	instructions := r.FormValue("instructions")
	notes := r.FormValue("notes")
	thumbnail := r.FormValue("thumbnail")
	sourceURL := r.FormValue("source_url")
	tags := parseTags(r.FormValue("tags"))

	if err := database.UpdateRecipe(userID, id, title, ingredients, instructions, notes, thumbnail, sourceURL); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	database.SetItemTags(id, tags)

	recipes, _ := database.GetRecipes(userID, "")
	RenderFragment(w, "recipe_list.html", recipes)
}

func ImportRecipeHandler(w http.ResponseWriter, r *http.Request) {
	url := r.URL.Query().Get("url")
	if url == "" {
		http.Error(w, "URL parameter required", http.StatusBadRequest)
		return
	}

	recipeData, err := ParseRecipeFromURL(url)
	if err != nil {
		http.Error(w, fmt.Sprintf("Failed to parse recipe: %v", err), http.StatusInternalServerError)
		return
	}

	// Convert ingredients array to newline-separated string
	ingredientsStr := strings.Join(recipeData.Ingredients, "\n")

	response := map[string]interface{}{
		"title":        recipeData.Title,
		"ingredients":  ingredientsStr,
		"instructions": recipeData.Instructions,
		"thumbnail":    recipeData.Image,
		"source_url":   url,
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(response)
}

func SearchHandler(w http.ResponseWriter, r *http.Request) {
	userID := getUserID(r)
	query := strings.ToLower(r.URL.Query().Get("q"))
	category := r.URL.Query().Get("category")

	switch category {
	case "bookmarks":
		items, _ := database.GetBookmarks(userID, "")
		filtered := filterAndSort(items, query, []string{"title", "url", "description"})
		RenderFragment(w, "bookmark_list.html", filtered)
	case "notes":
		items, _ := database.GetNotes(userID, "")
		filtered := filterAndSort(items, query, []string{"title", "content"})
		RenderFragment(w, "note_list.html", filtered)
	case "drawings":
		items, _ := database.GetDrawings(userID, "")
		filtered := filterAndSort(items, query, []string{"title"})
		RenderFragment(w, "drawing_list.html", filtered)
	case "rated-lists":
		items, _ := database.GetRatedLists(userID, "")
		filtered := filterAndSort(items, query, []string{"title"})
		RenderFragment(w, "rated_list_nav.html", filtered)
	case "lists":
		items, _ := database.GetLists(userID, "")
		filtered := filterAndSort(items, query, []string{"title"})
		RenderFragment(w, "list_nav.html", filtered)
	case "media":
		items, _ := database.GetMedia(userID, "")
		filtered := filterAndSort(items, query, []string{"title"})
		RenderFragment(w, "media_grid.html", filtered)
	case "recipes":
		items, _ := database.GetRecipes(userID, "")
		filtered := filterAndSort(items, query, []string{"title", "ingredients", "instructions"})
		RenderFragment(w, "recipe_list.html", filtered)
	case "dashboard":
		bookmarks, _ := database.GetBookmarks(userID, "")
		notes, _ := database.GetNotes(userID, "")
		drawings, _ := database.GetDrawings(userID, "")
		ratedLists, _ := database.GetRatedLists(userID, "")
		checklists, _ := database.GetLists(userID, "")
		recipes, _ := database.GetRecipes(userID, "")

		data := map[string]interface{}{
			"Bookmarks":  filterAndSort(bookmarks, query, []string{"title", "url", "description"}),
			"Notes":      filterAndSort(notes, query, []string{"title", "content"}),
			"Drawings":   filterAndSort(drawings, query, []string{"title"}),
			"RatedLists": filterAndSort(ratedLists, query, []string{"title"}),
			"Checklists": filterAndSort(checklists, query, []string{"title"}),
			"Recipes":    filterAndSort(recipes, query, []string{"title", "ingredients", "instructions"}),
		}
		RenderFragment(w, "search_results.html", data)
	default:
		// Generic cross-category search for dashboard or undefined
		w.WriteHeader(http.StatusNoContent)
	}
}

func filterAndSort(items []map[string]interface{}, query string, fields []string) []map[string]interface{} {
	if query == "" {
		return items
	}

	type ScoredItem struct {
		item  map[string]interface{}
		score int
	}

	var scored []ScoredItem

	for _, item := range items {
		score := 0
		found := false

		for _, field := range fields {
			val, ok := item[field].(string)
			if !ok {
				continue
			}
			valLower := strings.ToLower(val)

			if strings.Contains(valLower, query) {
				found = true
				// Higher weight for title
				weight := 1
				if field == "title" {
					weight = 10
				} else if field == "url" || field == "description" || field == "content" {
					weight = 5
				}

				// Bonus for exact start
				if strings.HasPrefix(valLower, query) {
					weight *= 2
				}

				score += weight
			}
		}

		// Search tags
		if tags, ok := item["tags"].([]string); ok {
			for _, tag := range tags {
				tagLower := strings.ToLower(tag)
				if strings.Contains(tagLower, query) {
					found = true
					weight := 15 // Higher weight for tag match
					if strings.HasPrefix(tagLower, query) {
						weight *= 2
					}
					score += weight
				}
			}
		}

		if found {
			scored = append(scored, ScoredItem{item: item, score: score})
		}
	}

	// Sort by score descending
	for i := 0; i < len(scored); i++ {
		for j := i + 1; j < len(scored); j++ {
			if scored[j].score > scored[i].score {
				scored[i], scored[j] = scored[j], scored[i]
			}
		}
	}

	var results []map[string]interface{}
	for _, s := range scored {
		results = append(results, s.item)
	}
	return results
}

func TagSuggestionsHandler(w http.ResponseWriter, r *http.Request) {
	query := strings.ToLower(r.URL.Query().Get("q"))
	allTags, _ := database.GetAllUniqueTags()

	var suggestions []string
	for _, tag := range allTags {
		if strings.Contains(strings.ToLower(tag), query) {
			suggestions = append(suggestions, tag)
		}
	}

	w.Header().Set("Content-Type", "text/html")
	for _, s := range suggestions {
		fmt.Fprintf(w, `<option value="%s">`, s)
	}
}

// struct for API requests
type ApiBookmarkRequest struct {
	Title       string `json:"title"`
	URL         string `json:"url"`
	Description string `json:"description"`
	Tags        string `json:"tags"`
}

type ApiNoteRequest struct {
	Title   string `json:"title"`
	Content string `json:"content"`
	Tags    string `json:"tags"`
}

type ApiRatedListItemRequest struct {
	Title string `json:"title"`
	Score int    `json:"score"`
	Note  string `json:"note"`
	Tags  string `json:"tags"`
}

func CorsMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		origin := r.Header.Get("Origin")
		if origin != "" {
			w.Header().Set("Access-Control-Allow-Origin", origin)
			w.Header().Set("Access-Control-Allow-Credentials", "true")
		} else {
			w.Header().Set("Access-Control-Allow-Origin", "*")
		}
		w.Header().Set("Access-Control-Allow-Methods", "POST, GET, OPTIONS, PUT, DELETE")
		w.Header().Set("Access-Control-Allow-Headers", "Accept, Content-Type, Content-Length, Accept-Encoding, X-CSRF-Token, Authorization")

		if r.Method == "OPTIONS" {
			w.WriteHeader(http.StatusOK)
			return
		}

		next.ServeHTTP(w, r)
	})
}

func ApiCreateBookmarkClipperHandler(w http.ResponseWriter, r *http.Request) {
	userID := getUserID(r)
	var input struct {
		Title       string `json:"title"`
		URL         string `json:"url"`
		Description string `json:"description"`
		Notes       string `json:"notes"`
		Tags        string `json:"tags"`
	}

	if err := json.NewDecoder(r.Body).Decode(&input); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	tags := parseTags(input.Tags)
	itemID, err := database.CreateBookmark(userID, input.Title, input.URL, input.Description, input.Notes, "")
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	if len(tags) > 0 {
		database.SetItemTags(itemID, tags)
	}

	w.WriteHeader(http.StatusCreated)
	json.NewEncoder(w).Encode(map[string]interface{}{"id": itemID, "status": "created"})
}

func ApiCreateNoteClipperHandler(w http.ResponseWriter, r *http.Request) {
	userID := getUserID(r)
	var input struct {
		Title   string `json:"title"`
		Content string `json:"content"`
		Tags    string `json:"tags"`
	}

	if err := json.NewDecoder(r.Body).Decode(&input); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	tags := parseTags(input.Tags)
	itemID, err := database.CreateNote(userID, input.Title, input.Content)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	if len(tags) > 0 {
		database.SetItemTags(itemID, tags)
	}

	w.WriteHeader(http.StatusCreated)
	json.NewEncoder(w).Encode(map[string]interface{}{"id": itemID, "status": "created"})
}

func ApiGetRatedListsHandler(w http.ResponseWriter, r *http.Request) {
	userID := getUserID(r)
	lists, err := database.GetRatedLists(userID, "")
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(lists)
}

func ApiAddRatedListItemHandler(w http.ResponseWriter, r *http.Request) {
	listIDStr := chi.URLParam(r, "id")
	var listID int64
	fmt.Sscanf(listIDStr, "%d", &listID)

	var req ApiRatedListItemRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	err := database.AddRatedListItem(listID, req.Title, req.Score, req.Note)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	w.WriteHeader(http.StatusCreated)
	json.NewEncoder(w).Encode(map[string]interface{}{"status": "created"})
}

func ApiGetTagsHandler(w http.ResponseWriter, r *http.Request) {
	tags, err := database.GetAllUniqueTags()
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(tags)
}
func ApiCreateRecipeClipperHandler(w http.ResponseWriter, r *http.Request) {
	var body struct {
		URL  string `json:"url"`
		Tags string `json:"tags"`
	}

	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		http.Error(w, "Invalid request body", http.StatusBadRequest)
		return
	}

	if body.URL == "" {
		http.Error(w, "URL is required", http.StatusBadRequest)
		return
	}

	// 1. Import/Parse the recipe
	recipeData, err := ParseRecipeFromURL(body.URL)
	if err != nil {
		http.Error(w, fmt.Sprintf("Failed to parse recipe: %v", err), http.StatusInternalServerError)
		return
	}

	// 2. Prepare data for database
	ingredientsStr := strings.Join(recipeData.Ingredients, "\n")

	// 3. Create the recipe
	userID := getUserID(r)
	itemID, err := database.CreateRecipe(
		userID,
		recipeData.Title,
		ingredientsStr,
		recipeData.Instructions,
		"", // Notes empty for now
		recipeData.Image,
		body.URL,
		nil, // No extra image paths
	)
	if err != nil {
		http.Error(w, fmt.Sprintf("Failed to save recipe: %v", err), http.StatusInternalServerError)
		return
	}

	// 4. Add tags if provided
	if body.Tags != "" {
		tags := strings.Split(body.Tags, ",")
		for i := range tags {
			tags[i] = strings.TrimSpace(tags[i])
		}
		if err := database.SetItemTags(itemID, tags); err != nil {
			log.Printf("Error adding tags to recipe %d: %v", itemID, err)
			// Don't fail the whole request for tag errors
		}
	}

	w.WriteHeader(http.StatusCreated)
	json.NewEncoder(w).Encode(map[string]interface{}{
		"status": "success",
		"id":     itemID,
		"title":  recipeData.Title,
	})
}

// Authentication

func AuthMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Allow public static assets
		if strings.HasPrefix(r.URL.Path, "/static/") || r.URL.Path == "/favicon.ico" {
			next.ServeHTTP(w, r)
			return
		}

		// Allow login/register
		if r.URL.Path == "/login" || r.URL.Path == "/register" {
			next.ServeHTTP(w, r)
			return
		}

		// Allow health check
		if r.URL.Path == "/api/health" {
			next.ServeHTTP(w, r)
			return
		}

		// Check for API token in Authorization header (used by browser extension)
		if authHeader := r.Header.Get("Authorization"); strings.HasPrefix(authHeader, "Bearer ") {
			token := strings.TrimPrefix(authHeader, "Bearer ")
			userID, err := database.GetUserByToken(token)
			if err == nil {
				ctx := context.WithValue(r.Context(), userIDKey, userID)
				next.ServeHTTP(w, r.WithContext(ctx))
				return
			}
			// Invalid token
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusUnauthorized)
			json.NewEncoder(w).Encode(map[string]string{"error": "Invalid API token"})
			return
		}

		cookie, err := r.Cookie("session_id")
		if err != nil {
			if strings.HasPrefix(r.URL.Path, "/api/") {
				w.Header().Set("Content-Type", "application/json")
				w.WriteHeader(http.StatusUnauthorized)
				json.NewEncoder(w).Encode(map[string]string{"error": "Unauthorized"})
				return
			}
			http.Redirect(w, r, "/login", http.StatusFound)
			return
		}

		userID, err := database.GetSession(cookie.Value)
		if err != nil {
			if strings.HasPrefix(r.URL.Path, "/api/") {
				w.Header().Set("Content-Type", "application/json")
				w.WriteHeader(http.StatusUnauthorized)
				json.NewEncoder(w).Encode(map[string]string{"error": "Unauthorized"})
				return
			}
			http.Redirect(w, r, "/login", http.StatusFound)
			return
		}

		ctx := context.WithValue(r.Context(), userIDKey, userID)
		next.ServeHTTP(w, r.WithContext(ctx))
	})
}

func LoginHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method == http.MethodGet {
		RenderTemplate(w, "login.html", nil)
		return
	}

	if r.Method == http.MethodPost {
		username := r.FormValue("username")
		password := r.FormValue("password")
		stayConnected := r.FormValue("stay_connected") == "on"

		user, err := database.GetUserByUsername(username)
		if err != nil {
			http.Redirect(w, r, "/login?error=invalid", http.StatusFound)
			return
		}

		passwordHash := user["password_hash"].(string)
		err = bcrypt.CompareHashAndPassword([]byte(passwordHash), []byte(password))
		if err != nil {
			http.Redirect(w, r, "/login?error=invalid", http.StatusFound)
			return
		}

		duration := 24 * time.Hour
		if stayConnected {
			duration = 30 * 24 * time.Hour
		}

		sessionID, err := database.CreateSession(user["id"].(int64), duration)
		if err != nil {
			http.Error(w, "Failed to create session", http.StatusInternalServerError)
			return
		}

		http.SetCookie(w, &http.Cookie{
			Name:     "session_id",
			Value:    sessionID,
			Expires:  time.Now().Add(duration),
			HttpOnly: true,
			Path:     "/",
		})

		http.Redirect(w, r, "/", http.StatusFound)
	}
}

func RegisterHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method == http.MethodGet {
		RenderTemplate(w, "register.html", nil)
		return
	}

	if r.Method == http.MethodPost {
		username := r.FormValue("username")
		password := r.FormValue("password")

		if username == "" || password == "" {
			http.Redirect(w, r, "/register?error=empty", http.StatusFound)
			return
		}

		hashedPassword, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
		if err != nil {
			http.Error(w, "Error hashing password", http.StatusInternalServerError)
			return
		}

		_, err = database.CreateUser(username, string(hashedPassword))
		if err != nil {
			http.Redirect(w, r, "/register?error=exists", http.StatusFound)
			return
		}

		http.Redirect(w, r, "/login?registered=true", http.StatusFound)
	}
}

func LogoutHandler(w http.ResponseWriter, r *http.Request) {
	cookie, err := r.Cookie("session_id")
	if err == nil {
		database.DeleteSession(cookie.Value)
	}

	http.SetCookie(w, &http.Cookie{
		Name:     "session_id",
		Value:    "",
		Expires:  time.Unix(0, 0),
		HttpOnly: true,
		Path:     "/",
	})

	http.Redirect(w, r, "/login", http.StatusFound)
}
