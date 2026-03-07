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
	if len(row) != 10 {
		t.Fatalf("want 10 columns, got %d", len(row))
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

func TestDayLogFromRow_Full(t *testing.T) {
	row := []interface{}{"2026-03-06", "ran 5k", "7", "felt good"}
	d := sheets.DayLogFromRow(row)
	if d.Date != "2026-03-06" {
		t.Errorf("Date: got %q", d.Date)
	}
	if d.Activity != "ran 5k" {
		t.Errorf("Activity: got %q", d.Activity)
	}
	if d.FeelingScore != 7 {
		t.Errorf("FeelingScore: got %d, want 7", d.FeelingScore)
	}
	if d.FeelingNotes != "felt good" {
		t.Errorf("FeelingNotes: got %q", d.FeelingNotes)
	}
}

func TestDayLogFromRow_LegacyTwoColumn(t *testing.T) {
	row := []interface{}{"2026-03-06", "old activity notes"}
	d := sheets.DayLogFromRow(row)
	if d.Activity != "old activity notes" {
		t.Errorf("Activity: got %q", d.Activity)
	}
	if d.FeelingScore != 0 {
		t.Errorf("FeelingScore: got %d, want 0", d.FeelingScore)
	}
	if d.FeelingNotes != "" {
		t.Errorf("FeelingNotes: got %q, want empty", d.FeelingNotes)
	}
}

func TestDayLogToRow(t *testing.T) {
	d := sheets.DayLog{Date: "2026-03-06", Activity: "yoga", FeelingScore: 8, FeelingNotes: "great day"}
	row := d.ToRow()
	if len(row) != 4 {
		t.Fatalf("want 4 cols, got %d", len(row))
	}
	if row[0] != "2026-03-06" {
		t.Errorf("col 0: got %v", row[0])
	}
	if row[2] != "8" {
		t.Errorf("col 2 (feeling_score): got %v", row[2])
	}
}

func TestFoodEntryFiber_ToRow(t *testing.T) {
	e := sheets.FoodEntry{
		ID: "x", Date: "2026-03-07", Time: "12:00", MealType: "lunch",
		Description: "salad", Calories: 200, Protein: 5, Carbs: 20, Fat: 8, Fiber: 4,
	}
	row := e.ToRow()
	if len(row) != 10 {
		t.Fatalf("want 10 cols, got %d", len(row))
	}
	if row[9] != "4" {
		t.Errorf("col 9 (fiber): got %v, want 4", row[9])
	}
}

func TestFoodEntryFromRow_FiberBackwardCompat(t *testing.T) {
	// 9-col row (no fiber column) → Fiber defaults to 0
	row := []interface{}{"id", "2026-03-07", "12:00", "lunch", "salad", "200", "5", "20", "8"}
	e, err := sheets.FoodEntryFromRow(row)
	if err != nil {
		t.Fatal(err)
	}
	if e.Fiber != 0 {
		t.Errorf("fiber: got %d, want 0", e.Fiber)
	}
}

func TestFoodEntryFromRow_WithFiber(t *testing.T) {
	row := []interface{}{"id", "2026-03-07", "12:00", "lunch", "salad", "200", "5", "20", "8", "4"}
	e, err := sheets.FoodEntryFromRow(row)
	if err != nil {
		t.Fatal(err)
	}
	if e.Fiber != 4 {
		t.Errorf("fiber: got %d, want 4", e.Fiber)
	}
}

func TestDeleteFood_NotFound(t *testing.T) {
	// Compilation check — verify method exists on *Service
	var s *sheets.Service
	_ = s.DeleteFood // just verify method exists
}

func TestGetSchemaVersion_ReturnsValue(t *testing.T) {
	_ = sheets.CurrentSchemaVersion // verify the constant exists
	if sheets.CurrentSchemaVersion != 1 {
		t.Errorf("CurrentSchemaVersion: got %d, want 1", sheets.CurrentSchemaVersion)
	}
}

func TestUserProfileRoundTrip(t *testing.T) {
	p := sheets.UserProfile{Gender: "male", Height: "5'10\"", Weight: "170lbs", Notes: "vegetarian"}
	row := p.ToRow()
	if len(row) != 4 {
		t.Fatalf("want 4 cols, got %d", len(row))
	}
	if row[0] != "male" {
		t.Errorf("gender: got %v", row[0])
	}
	if row[3] != "vegetarian" {
		t.Errorf("notes: got %v", row[3])
	}
	got := sheets.UserProfileFromRow(row)
	if got.Height != "5'10\"" {
		t.Errorf("height round-trip: got %q", got.Height)
	}
}
