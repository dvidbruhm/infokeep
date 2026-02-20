package handlers

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strings"

	"golang.org/x/net/html"
)

type RecipeData struct {
	Title        string   `json:"title"`
	Ingredients  []string `json:"ingredients"`
	Instructions string   `json:"instructions"`
	Image        string   `json:"image"`
}

// ParseRecipeFromURL attempts to extract recipe data from a URL
func ParseRecipeFromURL(url string) (*RecipeData, error) {
	// Create request with User-Agent to avoid being blocked
	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}
	req.Header.Set("User-Agent", "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/91.0.4472.124 Safari/537.36")

	// Fetch the HTML
	client := &http.Client{}
	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("failed to fetch URL: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("bad status code: %d", resp.StatusCode)
	}

	// Parse HTML once
	doc, err := html.Parse(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to parse HTML: %w", err)
	}

	// Try JSON-LD first (most reliable)
	if recipe := extractJSONLD(doc); recipe != nil {
		return recipe, nil
	}

	// Fallback to HTML parsing
	return extractFromHTML(doc)
}

// extractJSONLD extracts recipe data from JSON-LD structured data in the DOM
func extractJSONLD(n *html.Node) *RecipeData {
	var recipe *RecipeData

	var f func(*html.Node)
	f = func(n *html.Node) {
		if recipe != nil {
			return // Found it
		}

		if n.Type == html.ElementNode && n.Data == "script" {
			isJSONLD := false
			for _, attr := range n.Attr {
				if attr.Key == "type" && attr.Val == "application/ld+json" {
					isJSONLD = true
					break
				}
			}

			if isJSONLD && n.FirstChild != nil {
				content := n.FirstChild.Data
				if r := parseJSONLDContent(content); r != nil {
					recipe = r
					return
				}
			}
		}

		for c := n.FirstChild; c != nil; c = c.NextSibling {
			f(c)
		}
	}
	f(n)

	return recipe
}

func parseJSONLDContent(content string) *RecipeData {
	var data interface{}
	if err := json.Unmarshal([]byte(content), &data); err != nil {
		return nil
	}

	// Handle @graph structure (common in WordPress)
	if obj, ok := data.(map[string]interface{}); ok {
		if graph, ok := obj["@graph"].([]interface{}); ok {
			for _, item := range graph {
				if recipe := extractRecipeFromMap(item); recipe != nil {
					return recipe
				}
			}
		} else {
			// Try top-level object
			if recipe := extractRecipeFromMap(obj); recipe != nil {
				return recipe
			}
		}
	} else if arr, ok := data.([]interface{}); ok {
		// Handle top-level array
		for _, item := range arr {
			if recipe := extractRecipeFromMap(item); recipe != nil {
				return recipe
			}
		}
	}

	return nil
}

func extractRecipeFromMap(data interface{}) *RecipeData {
	obj, ok := data.(map[string]interface{})
	if !ok {
		return nil
	}

	// Check if it's a Recipe type
	typeVal, ok := obj["@type"]
	if !ok {
		return nil
	}

	// Handle both string and array types for @type
	var isRecipe bool
	switch v := typeVal.(type) {
	case string:
		isRecipe = strings.Contains(strings.ToLower(v), "recipe")
	case []interface{}:
		for _, t := range v {
			if str, ok := t.(string); ok && strings.Contains(strings.ToLower(str), "recipe") {
				isRecipe = true
				break
			}
		}
	}

	if !isRecipe {
		return nil
	}

	recipe := &RecipeData{}

	// Extract title
	if title, ok := obj["name"].(string); ok {
		recipe.Title = html.UnescapeString(title)
	} else if title, ok := obj["headline"].(string); ok {
		recipe.Title = html.UnescapeString(title)
	}

	// Extract ingredients
	if recipeIngredients, ok := obj["recipeIngredient"].([]interface{}); ok {
		for _, ing := range recipeIngredients {
			if str, ok := ing.(string); ok {
				recipe.Ingredients = append(recipe.Ingredients, html.UnescapeString(str))
			}
		}
	}

	// Extract instructions
	if instructions, ok := obj["recipeInstructions"]; ok {
		recipe.Instructions = extractInstructions(instructions)
	}

	// Extract image
	if image, ok := obj["image"]; ok {
		recipe.Image = extractImage(image)
	}

	return recipe
}

// extractFromHTML is a fallback parser for sites without structured data
func extractFromHTML(doc *html.Node) (*RecipeData, error) {
	recipe := &RecipeData{
		Ingredients: []string{},
	}

	// Simple heuristic-based extraction
	var f func(*html.Node)
	f = func(n *html.Node) {
		if n.Type == html.ElementNode {
			// Try to find title
			if recipe.Title == "" && (n.Data == "h1" || n.Data == "title") {
				if n.FirstChild != nil && n.FirstChild.Type == html.TextNode {
					recipe.Title = strings.TrimSpace(n.FirstChild.Data)
				}
			}

			// Try to find ingredients (look for lists with "ingredient" in class/id)
			if n.Data == "li" {
				for _, attr := range n.Attr {
					if (attr.Key == "class" || attr.Key == "id") &&
						strings.Contains(strings.ToLower(attr.Val), "ingredient") {
						if n.FirstChild != nil && n.FirstChild.Type == html.TextNode {
							recipe.Ingredients = append(recipe.Ingredients, strings.TrimSpace(n.FirstChild.Data))
						}
					}
				}
			}
		}

		for c := n.FirstChild; c != nil; c = c.NextSibling {
			f(c)
		}
	}
	f(doc)

	if recipe.Title == "" {
		return nil, fmt.Errorf("could not extract recipe data")
	}

	return recipe, nil
}

// extractInstructions handles various instruction formats including HowToSection
func extractInstructions(instructions interface{}) string {
	var steps []string

	processStep := func(step interface{}) {
		switch s := step.(type) {
		case string:
			steps = append(steps, html.UnescapeString(s))
		case map[string]interface{}:
			// Handle HowToSection (grouped steps)
			if typeVal, ok := s["@type"].(string); ok && (typeVal == "HowToSection" || strings.Contains(typeVal, "Section")) {
				if name, ok := s["name"].(string); ok {
					steps = append(steps, fmt.Sprintf("\n**%s**", name))
				}
				if items, ok := s["itemListElement"].([]interface{}); ok {
					for _, item := range items {
						if text, ok := extractStepText(item); ok {
							steps = append(steps, html.UnescapeString(text))
						}
					}
				}
				return
			}

			// Handle regular HowToStep
			if text, ok := extractStepText(s); ok {
				steps = append(steps, html.UnescapeString(text))
			}
		}
	}

	switch v := instructions.(type) {
	case string:
		return html.UnescapeString(v)
	case []interface{}:
		for _, step := range v {
			processStep(step)
		}
	case map[string]interface{}: // Single step or section object
		processStep(v)
	}

	// Format steps with numbers
	var formatted []string
	for i, step := range steps {
		if strings.HasPrefix(step, "\n**") {
			formatted = append(formatted, step) // Keep headers as is
		} else {
			formatted = append(formatted, fmt.Sprintf("%d. %s", i+1, step))
		}
	}

	// Clean up newlines if header is first
	result := strings.Join(formatted, "\n")
	return strings.TrimSpace(result)
}

func extractStepText(step interface{}) (string, bool) {
	switch s := step.(type) {
	case string:
		return s, true
	case map[string]interface{}:
		if text, ok := s["text"].(string); ok {
			return html.UnescapeString(text), true
		}
		if text, ok := s["name"].(string); ok {
			return html.UnescapeString(text), true
		}
	}
	return "", false
}

// extractImage handles various image formats
func extractImage(image interface{}) string {
	switch v := image.(type) {
	case string:
		return v
	case []interface{}:
		if len(v) > 0 {
			return extractImage(v[0]) // Recurse on first item
		}
	case map[string]interface{}:
		if url, ok := v["url"].(string); ok {
			return url
		}
		// Some schemas nest it in "image" again
		if img, ok := v["image"]; ok {
			return extractImage(img)
		}
	}
	return ""
}
