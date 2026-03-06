package sheets_test

import (
	"testing"
	"time"

	"foodtracker/internal/sheets"
)

func TestFoodEntryToRow(t *testing.T) {
	e := sheets.FoodEntry{
		ID:          "abc-123",
		Date:        "2026-03-05",
		Time:        "08:30",
		MealType:    "breakfast",
		Description: "oatmeal with milk",
		Calories:    300,
		Protein:     8,
		Carbs:       54,
		Fat:         6,
	}
	row := e.ToRow()
	if len(row) != 9 {
		t.Fatalf("want 9 columns, got %d", len(row))
	}
	if row[0] != "abc-123" {
		t.Errorf("col 0 (id): got %q", row[0])
	}
	if row[4] != "oatmeal with milk" {
		t.Errorf("col 4 (description): got %q", row[4])
	}
}

func TestFoodEntryFromRow(t *testing.T) {
	row := []interface{}{"abc-123", "2026-03-05", "08:30", "breakfast", "oatmeal", "300", "8", "54", "9"}
	e, err := sheets.FoodEntryFromRow(row)
	if err != nil {
		t.Fatal(err)
	}
	if e.ID != "abc-123" {
		t.Errorf("ID: got %q", e.ID)
	}
	if e.Calories != 300 {
		t.Errorf("Calories: got %d, want 300", e.Calories)
	}
	if e.MealType != "breakfast" {
		t.Errorf("MealType: got %q", e.MealType)
	}
}

func TestFoodEntryFromRow_TooShort(t *testing.T) {
	_, err := sheets.FoodEntryFromRow([]interface{}{"only", "three", "cols"})
	if err == nil {
		t.Error("expected error for short row")
	}
}

func TestDateString(t *testing.T) {
	d := sheets.DateString(time.Date(2026, 3, 5, 8, 30, 0, 0, time.UTC))
	if d != "2026-03-05" {
		t.Errorf("got %q, want 2026-03-05", d)
	}
}

func TestTimeString(t *testing.T) {
	s := sheets.TimeString(time.Date(2026, 3, 5, 8, 30, 0, 0, time.UTC))
	if s != "08:30" {
		t.Errorf("got %q, want 08:30", s)
	}
}
