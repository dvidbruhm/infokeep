package handlers

import (
	"crypto/rand"
	"encoding/json"
	"fmt"
	"log"
	"math/big"
	"net/http"
	"strconv"

	"infokeep/internal/database"

	"github.com/go-chi/chi/v5"
)

// Generate a random base62 string of a given length
func generateSecureHash(length int) (string, error) {
	const charset = "abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ0123456789"
	b := make([]byte, length)
	for i := range b {
		n, err := rand.Int(rand.Reader, big.NewInt(int64(len(charset))))
		if err != nil {
			return "", err
		}
		b[i] = charset[n.Int64()]
	}
	return string(b), nil
}

// ShareRequest represents the incoming JSON to generate a link
type ShareRequest struct {
	ItemType string `json:"item_type"` // note, recipe, bookmark, list
	ItemID   string `json:"item_id"`   // comes as string from JS sometimes, easier to parse here
}

// GenerateShareLinkHandler creates or retrieves a public share link for an item
func GenerateShareLinkHandler(w http.ResponseWriter, r *http.Request) {
	userID := getUserID(r)
	if userID == 0 {
		http.Error(w, "Unauthorized", http.StatusUnauthorized)
		return
	}

	var req ShareRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "Invalid request body", http.StatusBadRequest)
		return
	}

	itemID, err := strconv.ParseInt(req.ItemID, 10, 64)
	if err != nil {
		http.Error(w, "Invalid item ID", http.StatusBadRequest)
		return
	}

	// Important: Validate that the user actually owns this item!
	// We'll use the item-specific getter since it already checks userID
	var exists bool
	switch req.ItemType {
	case "note":
		_, err = database.GetNote(userID, itemID)
	case "recipe":
		_, err = database.GetRecipe(userID, itemID)
	case "bookmark":
		_, err = database.GetBookmark(userID, itemID)
	case "list":
		_, err = database.GetRatedList(userID, itemID)
	default:
		http.Error(w, "Unsupported item type", http.StatusBadRequest)
		return
	}

	if err != nil {
		http.Error(w, "Item not found or unauthorized", http.StatusForbidden)
		return
	}
	exists = true
	_ = exists // used switch for side effect of checking existence

	// Check if it's already shared
	existingLink, err := database.GetSharedLinkByItem(req.ItemType, itemID, userID)
	if err == nil && existingLink.LinkHash != "" {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]string{
			"link_hash": existingLink.LinkHash,
			"url":       fmt.Sprintf("http://%s/shared/%s", r.Host, existingLink.LinkHash),
		})
		return
	}

	// Generate a new 10-character secure hash
	hash, err := generateSecureHash(10)
	if err != nil {
		log.Printf("Failed to generate hash: %v", err)
		http.Error(w, "Internal server error", http.StatusInternalServerError)
		return
	}

	err = database.CreateSharedLink(hash, req.ItemType, itemID, userID)
	if err != nil {
		log.Printf("Failed to save shared link: %v", err)
		http.Error(w, "Failed to create link", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]string{
		"link_hash": hash,
		"url":       fmt.Sprintf("http://%s/shared/%s", r.Host, hash),
	})
}

// RevokeShareLinkHandler deletes a shared link
func RevokeShareLinkHandler(w http.ResponseWriter, r *http.Request) {
	userID := getUserID(r)
	if userID == 0 {
		http.Error(w, "Unauthorized", http.StatusUnauthorized)
		return
	}

	hash := chi.URLParam(r, "hash")
	if hash == "" {
		http.Error(w, "Hash is required", http.StatusBadRequest)
		return
	}

	err := database.DeleteSharedLink(hash, userID)
	if err != nil {
		log.Printf("Failed to revoke link: %v", err)
		http.Error(w, "Failed to revoke link", http.StatusInternalServerError)
		return
	}

	w.WriteHeader(http.StatusOK)
}

// PublicViewHandler serves the read-only view of a shared item to unauthenticated guests
func PublicViewHandler(w http.ResponseWriter, r *http.Request) {
	hash := chi.URLParam(r, "hash")

	if hash == "" {
		http.Error(w, "Not found", http.StatusNotFound)
		return
	}

	// 1. Look up the link hash
	link, err := database.GetSharedLinkByHash(hash)
	if err != nil || link.LinkHash == "" {
		http.Error(w, "This link is invalid or has been revoked.", http.StatusNotFound)
		return
	}

	// 2. Fetch the corresponding item data and render the public template
	switch link.ItemType {
	case "note":
		note, err := database.GetNote(link.UserID, link.ItemID)
		if err != nil {
			http.Error(w, "Note not found", http.StatusNotFound)
			return
		}
		RenderPublicTemplate(w, "public_note.html", map[string]interface{}{
			"Note": note,
		})

	case "recipe":
		recipe, err := database.GetRecipe(link.UserID, link.ItemID)
		if err != nil {
			http.Error(w, "Recipe not found", http.StatusNotFound)
			return
		}
		RenderPublicTemplate(w, "public_recipe.html", map[string]interface{}{
			"Recipe": recipe,
		})

	case "bookmark":
		bookmark, err := database.GetBookmark(link.UserID, link.ItemID)
		if err != nil {
			http.Error(w, "Bookmark not found", http.StatusNotFound)
			return
		}
		RenderPublicTemplate(w, "public_bookmark.html", map[string]interface{}{
			"Bookmark": bookmark,
		})

	case "list":
		list, err := database.GetRatedList(link.UserID, link.ItemID)
		if err != nil {
			http.Error(w, "List not found", http.StatusNotFound)
			return
		}
		items, err := database.GetRatedListItems(link.ItemID)
		if err != nil {
			http.Error(w, "List items not found", http.StatusNotFound)
			return
		}
		RenderPublicTemplate(w, "public_list.html", map[string]interface{}{
			"List":  list,
			"Items": items,
		})

	default:
		http.Error(w, "Unsupported item type", http.StatusBadRequest)
	}
}
