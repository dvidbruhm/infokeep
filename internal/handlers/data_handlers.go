package handlers

import (
	"archive/zip"
	"encoding/csv"
	"encoding/json"
	"fmt"
	"infokeep/internal/database"
	"net/http"
	"time"
)

// ExportDataHandler handles the export of all data
func ExportDataHandler(w http.ResponseWriter, r *http.Request) {
	format := r.URL.Query().Get("format")
	if format == "" {
		format = "json"
	}

	userID := getUserID(r)
	// Fetch all data
	bookmarks, err := database.GetBookmarks(userID, "")
	if err != nil {
		http.Error(w, "Failed to fetch bookmarks", http.StatusInternalServerError)
		return
	}
	notes, err := database.GetNotes(userID, "")
	if err != nil {
		http.Error(w, "Failed to fetch notes", http.StatusInternalServerError)
		return
	}
	drawings, err := database.GetDrawings(userID, "")
	if err != nil {
		http.Error(w, "Failed to fetch drawings", http.StatusInternalServerError)
		return
	}

	// Lists with items
	lists, err := database.GetLists(userID, "")
	if err != nil {
		http.Error(w, "Failed to fetch lists", http.StatusInternalServerError)
		return
	}
	for i, l := range lists {
		id := l["id"].(int64)
		items, _ := database.GetListItems(id)
		lists[i]["items"] = items
	}

	// Rated Lists with items
	ratedLists, err := database.GetRatedLists(userID, "")
	if err != nil {
		http.Error(w, "Failed to fetch rated lists", http.StatusInternalServerError)
		return
	}
	for i, l := range ratedLists {
		id := l["id"].(int64)
		items, _ := database.GetRatedListItems(id)
		ratedLists[i]["items"] = items
	}

	// Recipes
	recipes, err := database.GetRecipes(userID, "")
	if err != nil {
		http.Error(w, "Failed to fetch recipes", http.StatusInternalServerError)
		return
	}

	// Media
	media, err := database.GetMedia(userID, "")
	if err != nil {
		http.Error(w, "Failed to fetch media", http.StatusInternalServerError)
		return
	}

	timestamp := time.Now().Format("2006-01-02_150405")

	if format == "json" {
		data := map[string]interface{}{
			"version":     "1.0",
			"exported_at": time.Now(),
			"bookmarks":   bookmarks,
			"notes":       notes,
			"drawings":    drawings,
			"lists":       lists,
			"rated_lists": ratedLists,
			"recipes":     recipes,
			"media":       media,
		}

		w.Header().Set("Content-Type", "application/json")
		w.Header().Set("Content-Disposition", fmt.Sprintf("attachment; filename=\"infokeep_backup_%s.json\"", timestamp))

		enc := json.NewEncoder(w)
		enc.SetIndent("", "  ")
		if err := enc.Encode(data); err != nil {
			http.Error(w, "Failed to encode JSON", http.StatusInternalServerError)
		}
		return
	}

	if format == "csv" {
		w.Header().Set("Content-Type", "application/zip")
		w.Header().Set("Content-Disposition", fmt.Sprintf("attachment; filename=\"infokeep_export_%s.zip\"", timestamp))

		zw := zip.NewWriter(w)
		defer zw.Close()

		// Helper to write CSV
		writeCSV := func(filename string, header []string, rows [][]string) error {
			f, err := zw.Create(filename)
			if err != nil {
				return err
			}
			cw := csv.NewWriter(f)
			if err := cw.Write(header); err != nil {
				return err
			}
			if err := cw.WriteAll(rows); err != nil {
				return err
			}
			cw.Flush()
			return cw.Error()
		}

		// Bookmarks CSV
		bRows := [][]string{}
		for _, b := range bookmarks {
			// Extract tags (slice of strings) and join
			tags := ""
			if t, ok := b["tags"].([]string); ok {
				// join tags
				for i, tag := range t {
					if i > 0 {
						tags += ","
					}
					tags += tag
				}
			}
			bRows = append(bRows, []string{
				fmt.Sprintf("%v", b["id"]),
				fmt.Sprintf("%v", b["title"]),
				fmt.Sprintf("%v", b["url"]),
				fmt.Sprintf("%v", b["description"]),
				fmt.Sprintf("%v", b["created_at"]),
				tags,
			})
		}
		if err := writeCSV("bookmarks.csv", []string{"id", "title", "url", "description", "created_at", "tags"}, bRows); err != nil {
			http.Error(w, "Failed to write CSV", http.StatusInternalServerError)
			return
		}

		// Notes CSV
		nRows := [][]string{}
		for _, n := range notes {
			tags := ""
			if t, ok := n["tags"].([]string); ok {
				for i, tag := range t {
					if i > 0 {
						tags += ","
					}
					tags += tag
				}
			}
			nRows = append(nRows, []string{
				fmt.Sprintf("%v", n["id"]),
				fmt.Sprintf("%v", n["title"]),
				fmt.Sprintf("%v", n["content"]),
				fmt.Sprintf("%v", n["created_at"]),
				tags,
			})
		}
		writeCSV("notes.csv", []string{"id", "title", "content", "created_at", "tags"}, nRows)

		// Recipes CSV
		rRows := [][]string{}
		for _, r := range recipes {
			tags := ""
			if t, ok := r["tags"].([]string); ok {
				for i, tag := range t {
					if i > 0 {
						tags += ","
					}
					tags += tag
				}
			}
			rRows = append(rRows, []string{
				fmt.Sprintf("%v", r["id"]),
				fmt.Sprintf("%v", r["title"]),
				fmt.Sprintf("%v", r["ingredients"]),
				fmt.Sprintf("%v", r["instructions"]),
				fmt.Sprintf("%v", r["notes"]),
				fmt.Sprintf("%v", r["thumbnail"]),
				fmt.Sprintf("%v", r["source_url"]),
				fmt.Sprintf("%v", r["created_at"]),
				tags,
			})
		}
		writeCSV("recipes.csv", []string{"id", "title", "ingredients", "instructions", "notes", "thumbnail", "source_url", "created_at", "tags"}, rRows)

		// Lists CSV
		lRows := [][]string{}
		for _, l := range lists {
			tags := ""
			if t, ok := l["tags"].([]string); ok {
				for i, tag := range t {
					if i > 0 {
						tags += ","
					}
					tags += tag
				}
			}
			lRows = append(lRows, []string{
				fmt.Sprintf("%v", l["id"]),
				fmt.Sprintf("%v", l["title"]),
				fmt.Sprintf("%v", l["created_at"]),
				tags,
			})

			// List Items logic could be separate csv "list_items.csv" with list_id
			if items, ok := l["items"].([]map[string]interface{}); ok {
				liRows := [][]string{}
				for _, item := range items {
					liRows = append(liRows, []string{
						fmt.Sprintf("%v", item["id"]),
						fmt.Sprintf("%v", l["id"]), // parent list id
						fmt.Sprintf("%v", item["content"]),
						fmt.Sprintf("%v", item["completed"]),
					})
				}
				// Append to a global list_items.csv?
				// Better to iterate lists again or use a closure for global collection
			}
		}
		writeCSV("lists.csv", []string{"id", "title", "created_at", "tags"}, lRows)

		// Collect ALL list items
		liRows := [][]string{}
		for _, l := range lists {
			if items, ok := l["items"].([]map[string]interface{}); ok {
				for _, item := range items {
					liRows = append(liRows, []string{
						fmt.Sprintf("%v", item["id"]),
						fmt.Sprintf("%v", l["id"]),
						fmt.Sprintf("%v", item["content"]),
						fmt.Sprintf("%v", item["completed"]),
					})
				}
			}
		}
		writeCSV("list_items.csv", []string{"id", "list_id", "content", "completed"}, liRows)

		return
	}

	http.Error(w, "Invalid format", http.StatusBadRequest)
}

// ImportDataHandler handles the import of data from JSON
func ImportDataHandler(w http.ResponseWriter, r *http.Request) {
	// Parse multipart form
	err := r.ParseMultipartForm(10 << 20) // 10 MB max
	if err != nil {
		http.Error(w, "File too large", http.StatusBadRequest)
		return
	}

	file, _, err := r.FormFile("importFile")
	if err != nil {
		http.Error(w, "Failed to retrieve file", http.StatusBadRequest)
		return
	}
	defer file.Close()

	// Decode JSON
	var data struct {
		Bookmarks []map[string]interface{} `json:"bookmarks"`
		Notes     []map[string]interface{} `json:"notes"`
		Drawings  []map[string]interface{} `json:"drawings"`
		Lists     []struct {
			Title string `json:"title"`
			Items []struct {
				Content   string `json:"content"`
				Completed bool   `json:"completed"`
			} `json:"items"`
			Tags []string `json:"tags"`
		} `json:"lists"`
		RatedLists []struct {
			Title string `json:"title"`
			Items []struct {
				Title string `json:"title"`
				Score int    `json:"score"`
				Note  string `json:"note"`
			} `json:"items"`
			Tags []string `json:"tags"`
		} `json:"rated_lists"`
		Recipes []struct {
			Title        string   `json:"title"`
			Ingredients  string   `json:"ingredients"`
			Instructions string   `json:"instructions"`
			Notes        string   `json:"notes"`
			Thumbnail    string   `json:"thumbnail"`
			SourceURL    string   `json:"source_url"`
			Images       []string `json:"images"`
			Tags         []string `json:"tags"`
		} `json:"recipes"`
		// Media import if we want, but file paths might be broken if not uploaded
	}

	if err := json.NewDecoder(file).Decode(&data); err != nil {
		http.Error(w, "Invalid JSON file", http.StatusBadRequest)
		return
	}

	userID := getUserID(r)
	// Insert data
	// Bookmarks
	for _, b := range data.Bookmarks {
		title := fmt.Sprintf("%v", b["title"])
		url := fmt.Sprintf("%v", b["url"])
		desc := ""
		if v, ok := b["description"]; ok && v != nil {
			desc = fmt.Sprintf("%v", v)
		}
		fav := ""
		if v, ok := b["favicon"]; ok && v != nil {
			fav = fmt.Sprintf("%v", v)
		}
		thumb := ""
		if v, ok := b["thumbnail"]; ok && v != nil {
			thumb = fmt.Sprintf("%v", v)
		}

		id, err := database.CreateBookmark(userID, title, url, desc, fav, thumb)
		if err == nil {
			if tags, ok := b["tags"].([]interface{}); ok {
				tagStrs := []string{}
				for _, t := range tags {
					tagStrs = append(tagStrs, fmt.Sprintf("%v", t))
				}
				database.SetItemTags(id, tagStrs)
			}
		}
	}

	// Notes
	for _, n := range data.Notes {
		title := fmt.Sprintf("%v", n["title"])
		content := fmt.Sprintf("%v", n["content"])
		id, err := database.CreateNote(userID, title, content)
		if err == nil {
			if tags, ok := n["tags"].([]interface{}); ok {
				tagStrs := []string{}
				for _, t := range tags {
					tagStrs = append(tagStrs, fmt.Sprintf("%v", t))
				}
				database.SetItemTags(id, tagStrs)
			}
		}
	}

	// Lists
	for _, l := range data.Lists {
		id, err := database.CreateList(userID, l.Title)
		if err == nil {
			database.SetItemTags(id, l.Tags)
			for _, item := range l.Items {
				database.AddListItem(id, item.Content)
			}
		}
	}

	// Rated Lists
	for _, l := range data.RatedLists {
		id, err := database.CreateRatedList(userID, l.Title)
		if err == nil {
			database.SetItemTags(id, l.Tags)
			for _, item := range l.Items {
				database.AddRatedListItem(id, item.Title, item.Score, item.Note)
			}
		}
	}

	// Recipes
	for _, r := range data.Recipes {
		id, err := database.CreateRecipe(userID, r.Title, r.Ingredients, r.Instructions, r.Notes, r.Thumbnail, r.SourceURL, r.Images)
		if err == nil {
			database.SetItemTags(id, r.Tags)
		}
	}

	// Redirect back to settings with success message
	http.Redirect(w, r, "/settings?import=success", http.StatusSeeOther)
}
