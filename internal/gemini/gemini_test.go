package gemini_test

import (
	"testing"

	"foodtracker/internal/gemini"
)

func TestParseEntries_BareJSON(t *testing.T) {
	raw := `{"entries":[{"meal_type":"breakfast","description":"oatmeal","calories":300,"protein":8,"carbs":54,"fat":6}]}`
	entries, ok := gemini.ParseEntries(raw)
	if !ok {
		t.Fatal("expected ok=true")
	}
	if len(entries) != 1 {
		t.Fatalf("want 1 entry, got %d", len(entries))
	}
	if entries[0].MealType != "breakfast" {
		t.Errorf("MealType: got %q", entries[0].MealType)
	}
	if entries[0].Calories != 300 {
		t.Errorf("Calories: got %d", entries[0].Calories)
	}
}

func TestParseEntries_JSONInCodeFence(t *testing.T) {
	raw := "Here are your entries:\n```json\n{\"entries\":[{\"meal_type\":\"lunch\",\"description\":\"sandwich\",\"calories\":450,\"protein\":20,\"carbs\":50,\"fat\":15}]}\n```"
	entries, ok := gemini.ParseEntries(raw)
	if !ok {
		t.Fatal("expected ok=true for JSON in code fence")
	}
	if len(entries) != 1 || entries[0].MealType != "lunch" {
		t.Errorf("unexpected entries: %+v", entries)
	}
}

func TestParseEntries_Question(t *testing.T) {
	raw := `How much oatmeal did you have — about a cup?`
	_, ok := gemini.ParseEntries(raw)
	if ok {
		t.Error("expected ok=false for a plain question")
	}
}

func TestParseEntries_MultipleEntries(t *testing.T) {
	raw := `{"entries":[{"meal_type":"breakfast","description":"oatmeal","calories":300,"protein":8,"carbs":54,"fat":6},{"meal_type":"breakfast","description":"coffee","calories":5,"protein":0,"carbs":1,"fat":0}]}`
	entries, ok := gemini.ParseEntries(raw)
	if !ok {
		t.Fatal("expected ok=true")
	}
	if len(entries) != 2 {
		t.Fatalf("want 2 entries, got %d", len(entries))
	}
}
