package sheets_test

import (
	"context"
	"testing"
	"time"

	"foodtracker/internal/sheets"
	"golang.org/x/oauth2"
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
	row := []any{"abc-123", "2026-03-05", "08:30", "breakfast", "oatmeal", "300", "8", "54", "9"}
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
	_, err := sheets.FoodEntryFromRow([]any{"only", "three", "cols"})
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
	row := []any{"2026-03-06", "ran 5k", "7", "felt good"}
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
	row := []any{"2026-03-06", "old activity notes"}
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
	if len(row) != 7 {
		t.Fatalf("want 7 cols, got %d", len(row))
	}
	if row[0] != "2026-03-06" {
		t.Errorf("col 0: got %v", row[0])
	}
	if row[2] != "8" {
		t.Errorf("col 2 (feeling_score): got %v", row[2])
	}
	if row[4] != "false" {
		t.Errorf("col 4 (poop): got %v, want false", row[4])
	}
	if row[5] != "" {
		t.Errorf("col 5 (poop_notes): got %v, want empty", row[5])
	}
	if row[6] != "0" {
		t.Errorf("col 6 (hydration): got %v, want 0", row[6])
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
	row := []any{"id", "2026-03-07", "12:00", "lunch", "salad", "200", "5", "20", "8"}
	e, err := sheets.FoodEntryFromRow(row)
	if err != nil {
		t.Fatal(err)
	}
	if e.Fiber != 0 {
		t.Errorf("fiber: got %d, want 0", e.Fiber)
	}
}

func TestFoodEntryFromRow_WithFiber(t *testing.T) {
	row := []any{"id", "2026-03-07", "12:00", "lunch", "salad", "200", "5", "20", "8", "4"}
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
	_ = sheets.CurrentSchemaVersion
	if sheets.CurrentSchemaVersion != 12 {
		t.Errorf("CurrentSchemaVersion: got %d, want 12", sheets.CurrentSchemaVersion)
	}
}

func TestEventToRow_Workout(t *testing.T) {
	e := sheets.Event{
		ID: "evt-1", Date: "2026-04-25", Time: "07:30",
		Kind: sheets.EventKindWorkout, Text: "ran 5k", Num: 30,
	}
	row := e.ToRow()
	if len(row) != 7 {
		t.Fatalf("want 7 cols, got %d", len(row))
	}
	if row[3] != sheets.EventKindWorkout {
		t.Errorf("kind: got %v", row[3])
	}
	if row[4] != "ran 5k" {
		t.Errorf("text: got %v", row[4])
	}
	if row[5] != "30" {
		t.Errorf("num: got %v, want 30", row[5])
	}
}

func TestEventFromRow_Water(t *testing.T) {
	row := []any{"evt-2", "2026-04-25", "10:15", "water", "", "500", ""}
	e, err := sheets.EventFromRow(row)
	if err != nil {
		t.Fatal(err)
	}
	if e.Kind != sheets.EventKindWater {
		t.Errorf("kind: got %q", e.Kind)
	}
	if e.Num != 500 {
		t.Errorf("num: got %v, want 500", e.Num)
	}
}

func TestEventRoundTrip(t *testing.T) {
	in := sheets.Event{
		ID: "x", Date: "2026-04-25", Time: "12:00",
		Kind: sheets.EventKindFeeling, Text: "tired", Num: 4, Notes: "post-lunch",
	}
	out, err := sheets.EventFromRow(in.ToRow())
	if err != nil {
		t.Fatal(err)
	}
	if *out != in {
		t.Errorf("round-trip mismatch: got %+v want %+v", *out, in)
	}
}

func TestEventFromRow_TooShort(t *testing.T) {
	_, err := sheets.EventFromRow([]any{"a", "b"})
	if err == nil {
		t.Error("expected error for short row")
	}
}

func TestUserProfileRoundTrip(t *testing.T) {
	p := sheets.UserProfile{Gender: "male", Height: "5'10\"", Weight: "170lbs", Notes: "vegetarian", Goals: "lose weight", DietaryRestrictions: "no gluten", BirthYear: "1990", NutritionExpertise: "intermediate"}
	row := p.ToRow()
	if len(row) != 8 {
		t.Fatalf("want 8 cols, got %d", len(row))
	}
	if row[0] != "male" {
		t.Errorf("gender: got %v", row[0])
	}
	if row[3] != "vegetarian" {
		t.Errorf("notes: got %v", row[3])
	}
	if row[4] != "lose weight" {
		t.Errorf("goals: got %v", row[4])
	}
	if row[5] != "no gluten" {
		t.Errorf("dietary_restrictions: got %v", row[5])
	}
	if row[6] != "1990" {
		t.Errorf("birth_year: got %v", row[6])
	}
	if row[7] != "intermediate" {
		t.Errorf("nutrition_expertise: got %v", row[7])
	}
	got := sheets.UserProfileFromRow(row)
	if got.Height != "5'10\"" {
		t.Errorf("height round-trip: got %q", got.Height)
	}
	if got.Goals != "lose weight" {
		t.Errorf("goals round-trip: got %q", got.Goals)
	}
	if got.DietaryRestrictions != "no gluten" {
		t.Errorf("dietary_restrictions round-trip: got %q", got.DietaryRestrictions)
	}
	if got.BirthYear != "1990" {
		t.Errorf("birth_year round-trip: got %q", got.BirthYear)
	}
	if got.NutritionExpertise != "intermediate" {
		t.Errorf("nutrition_expertise round-trip: got %q", got.NutritionExpertise)
	}
}

func TestUserProfileFromRow_BackwardCompatNoBirthYear(t *testing.T) {
	row := []any{"female", "165cm", "60kg", "notes", "maintain", "vegetarian"}
	got := sheets.UserProfileFromRow(row)
	if got.BirthYear != "" {
		t.Errorf("birth_year: got %q, want empty", got.BirthYear)
	}
	if got.DietaryRestrictions != "vegetarian" {
		t.Errorf("dietary_restrictions: got %q", got.DietaryRestrictions)
	}
}

func TestDayLogFromRow_WithPoop(t *testing.T) {
	row := []any{"2026-03-07", "ran 5k", "8", "felt good", "true", "solid, once"}
	d := sheets.DayLogFromRow(row)
	if !d.Poop {
		t.Error("Poop: want true")
	}
	if d.PoopNotes != "solid, once" {
		t.Errorf("PoopNotes: got %q", d.PoopNotes)
	}
}

func TestDayLogFromRow_BackwardCompatNoPoop(t *testing.T) {
	// 4-col row (old schema) — Poop defaults to false, PoopNotes to ""
	row := []any{"2026-03-07", "yoga", "7", "good"}
	d := sheets.DayLogFromRow(row)
	if d.Poop {
		t.Error("Poop: want false for old-schema row")
	}
	if d.PoopNotes != "" {
		t.Errorf("PoopNotes: want empty, got %q", d.PoopNotes)
	}
}

func TestDayLogToRow_WithPoop(t *testing.T) {
	d := sheets.DayLog{Date: "2026-03-07", Poop: true, PoopNotes: "once"}
	row := d.ToRow()
	if len(row) != 7 {
		t.Fatalf("want 7 cols, got %d", len(row))
	}
	if row[4] != "true" {
		t.Errorf("col 4 (poop): got %v, want true", row[4])
	}
	if row[5] != "once" {
		t.Errorf("col 5 (poop_notes): got %v, want once", row[5])
	}
	if row[6] != "0" {
		t.Errorf("col 6 (hydration): got %v, want 0", row[6])
	}
}

func TestMigrateSpreadsheet_FunctionExists(t *testing.T) {
	// Compilation check — verify the function signature exists
	var _ func(context.Context, oauth2.TokenSource, string) error = sheets.MigrateSpreadsheet
}
