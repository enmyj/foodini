package gemini_test

import (
	"encoding/json"
	"testing"

	"foodtracker/internal/gemini"
)

func TestResponseUnmarshal(t *testing.T) {
	raw := `{"message":"Got it!","entries":[{"meal_type":"breakfast","description":"oatmeal","calories":300,"protein":8,"carbs":54,"fat":6,"fiber":4}]}`

	var resp gemini.Response
	if err := json.Unmarshal([]byte(raw), &resp); err != nil {
		t.Fatalf("unmarshal error: %v", err)
	}

	if resp.Message != "Got it!" {
		t.Errorf("Message: got %q, want %q", resp.Message, "Got it!")
	}
	if len(resp.Entries) != 1 {
		t.Fatalf("want 1 entry, got %d", len(resp.Entries))
	}
	if resp.Entries[0].MealType != "breakfast" {
		t.Errorf("MealType: got %q", resp.Entries[0].MealType)
	}
	if resp.Entries[0].Calories != 300 {
		t.Errorf("Calories: got %d", resp.Entries[0].Calories)
	}
}

func TestResponseUnmarshal_Question(t *testing.T) {
	raw := `{"message":"How much oatmeal did you have — about a cup?","entries":[]}`

	var resp gemini.Response
	if err := json.Unmarshal([]byte(raw), &resp); err != nil {
		t.Fatalf("unmarshal error: %v", err)
	}

	if resp.Message == "" {
		t.Error("expected non-empty message")
	}
	if len(resp.Entries) != 0 {
		t.Errorf("expected empty entries, got %d", len(resp.Entries))
	}
}

func TestResponseUnmarshal_MultipleEntries(t *testing.T) {
	raw := `{"message":"Logged!","entries":[{"meal_type":"breakfast","description":"oatmeal","calories":300,"protein":8,"carbs":54,"fat":6,"fiber":4},{"meal_type":"breakfast","description":"coffee","calories":5,"protein":0,"carbs":1,"fat":0,"fiber":0}]}`

	var resp gemini.Response
	if err := json.Unmarshal([]byte(raw), &resp); err != nil {
		t.Fatalf("unmarshal error: %v", err)
	}

	if len(resp.Entries) != 2 {
		t.Fatalf("want 2 entries, got %d", len(resp.Entries))
	}
}
