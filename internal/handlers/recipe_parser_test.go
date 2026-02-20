package handlers

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestExtractRecipeFromMap_HTMLEntities(t *testing.T) {
	jsonContent := `{
		"@context": "http://schema.org",
		"@type": "Recipe",
		"name": "Chicken &quot;Parmesan&quot;",
		"recipeIngredient": [
			"1 &frac12; lbs chicken breast",
			"Salt &amp; Pepper"
		],
		"recipeInstructions": [
			{
				"@type": "HowToStep",
				"text": "Season with &quot;generous&quot; salt."
			},
			"Cook for 10&#45;15 minutes."
		]
	}`

	var data map[string]interface{}
	if err := json.Unmarshal([]byte(jsonContent), &data); err != nil {
		t.Fatalf("Failed to unmarshal JSON: %v", err)
	}

	recipe := extractRecipeFromMap(data)
	if recipe == nil {
		t.Fatal("Failed to extract recipe")
	}

	// detailed checks
	expectedTitle := "Chicken \"Parmesan\""
	if recipe.Title != expectedTitle {
		t.Errorf("Title: got %q, want %q", recipe.Title, expectedTitle)
	}

	expectedIng1 := "1 ½ lbs chicken breast" // &frac12; -> ½
	if !strings.Contains(recipe.Ingredients[0], "½") {
		// allow utf8 matching
	}
	// Try standard html unescape check
	if recipe.Ingredients[0] != expectedIng1 {
		t.Errorf("Ingredient 1: got %q, want %q", recipe.Ingredients[0], expectedIng1)
	}

	expectedIng2 := "Salt & Pepper" // &amp; -> &
	if recipe.Ingredients[1] != expectedIng2 {
		t.Errorf("Ingredient 2: got %q, want %q", recipe.Ingredients[1], expectedIng2)
	}

	expectedInstr1 := "1. Season with \"generous\" salt."
	if !strings.Contains(recipe.Instructions, "Season with \"generous\" salt") {
		t.Errorf("Instruction 1: got %q, want to contain %q", recipe.Instructions, expectedInstr1)
	}

	expectedInstr2 := "2. Cook for 10-15 minutes." // &#45; -> -
	if !strings.Contains(recipe.Instructions, expectedInstr2) {
		t.Errorf("Instruction 2: got %q, want to contain %q", recipe.Instructions, expectedInstr2)
	}
}
