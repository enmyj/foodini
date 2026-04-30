package api

import (
	"fmt"
	"reflect"
	"testing"
	"time"

	"foodtracker/internal/gemini"
	"foodtracker/internal/sheets"
)

func seqIDGen() func() string {
	n := 0
	return func() string {
		n++
		return fmt.Sprintf("new-%d", n)
	}
}

func TestDefaultMealTime(t *testing.T) {
	cases := map[string]string{
		"breakfast":   "08:00",
		"snack":       "15:00",
		"lunch":       "12:30",
		"dinner":      "18:30",
		"supplements": "09:00",
		"":            "12:00",
		"junk":        "12:00",
	}
	for in, want := range cases {
		if got := defaultMealTime(in); got != want {
			t.Errorf("defaultMealTime(%q): got %q, want %q", in, got, want)
		}
	}
}

func TestResolveLogMealTime(t *testing.T) {
	now := time.Date(2026, 4, 26, 10, 17, 0, 0, time.UTC)

	tests := []struct {
		name           string
		date           string
		mealType       string
		userMealTime   string
		toolTime       string
		want           string
	}{
		{
			name: "today, no overrides → wall clock",
			date: "2026-04-26", want: "10:17",
		},
		{
			name: "retroactive date, no overrides → meal default",
			date: "2026-04-25", mealType: "dinner", want: "18:30",
		},
		{
			name: "user-supplied time wins over default",
			date: "2026-04-25", mealType: "dinner", userMealTime: "19:45", want: "19:45",
		},
		{
			name: "tool-supplied time wins over user time",
			date: "2026-04-25", mealType: "dinner", userMealTime: "19:45", toolTime: "20:15", want: "20:15",
		},
		{
			name: "invalid tool time falls through to user time",
			date: "2026-04-25", mealType: "dinner", userMealTime: "19:45", toolTime: "garbage", want: "19:45",
		},
		{
			name: "invalid user time falls through to default",
			date: "2026-04-25", mealType: "lunch", userMealTime: "25:99", want: "12:30",
		},
		{
			name: "tool time on today's date overrides wall clock",
			date: "2026-04-26", toolTime: "07:30", want: "07:30",
		},
		{
			name: "whitespace tool time treated as empty",
			date: "2026-04-26", toolTime: "   ", want: "10:17",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := resolveLogMealTime(now, tt.date, tt.mealType, tt.userMealTime, tt.toolTime)
			if got != tt.want {
				t.Errorf("got %q, want %q", got, tt.want)
			}
		})
	}
}

func TestLatestEntryID(t *testing.T) {
	if got := latestEntryID(nil); got != "" {
		t.Errorf("empty: got %q, want empty", got)
	}
}

func TestPlanEditMeal_RenameAndAdd(t *testing.T) {
	current := []sheets.FoodEntry{
		{ID: "a", Description: "rice", Calories: 200, MealType: "lunch", Date: "2026-04-29", Time: "12:00"},
	}
	items := []gemini.AgentEntry{
		{Description: "rice", Calories: 220},
		{Description: "salmon", Calories: 300},
	}
	plan := planEditMeal(current, items, "lunch", "2026-04-29", "12:30", seqIDGen())

	if len(plan.toUpdate) != 1 || plan.toUpdate[0].ID != "a" || plan.toUpdate[0].Calories != 220 {
		t.Fatalf("expected rice updated in place: %+v", plan.toUpdate)
	}
	if len(plan.toAppend) != 1 || plan.toAppend[0].ID != "new-1" || plan.toAppend[0].Description != "salmon" {
		t.Fatalf("expected salmon appended with fresh id: %+v", plan.toAppend)
	}
	if plan.toAppend[0].Time != "12:30" {
		t.Errorf("appended row should use newTime, got %q", plan.toAppend[0].Time)
	}
	if len(plan.toDelete) != 0 {
		t.Errorf("nothing should be deleted: %v", plan.toDelete)
	}
	if len(plan.saved) != 2 || plan.saved[0].Description != "rice" || plan.saved[1].Description != "salmon" {
		t.Errorf("saved should preserve item order: %+v", plan.saved)
	}
}

func TestPlanEditMeal_DuplicateDescriptionsClaimDistinct(t *testing.T) {
	// Two existing rows both named "egg" and two new "egg" items. The new
	// list must update both rows, not claim the same row twice.
	current := []sheets.FoodEntry{
		{ID: "a", Description: "egg", Calories: 70},
		{ID: "b", Description: "egg", Calories: 70},
	}
	items := []gemini.AgentEntry{
		{Description: "egg", Calories: 80},
		{Description: "egg", Calories: 90},
	}
	plan := planEditMeal(current, items, "breakfast", "2026-04-29", "08:00", seqIDGen())

	if len(plan.toUpdate) != 2 {
		t.Fatalf("expected both rows updated, got %d", len(plan.toUpdate))
	}
	gotIDs := []string{plan.toUpdate[0].ID, plan.toUpdate[1].ID}
	if !reflect.DeepEqual(gotIDs, []string{"a", "b"}) {
		t.Errorf("expected updates to claim a then b, got %v", gotIDs)
	}
	if plan.toUpdate[0].Calories != 80 || plan.toUpdate[1].Calories != 90 {
		t.Errorf("calories not applied to distinct rows: %+v", plan.toUpdate)
	}
	if len(plan.toAppend) != 0 || len(plan.toDelete) != 0 {
		t.Errorf("no appends/deletes expected: append=%v delete=%v", plan.toAppend, plan.toDelete)
	}
}

func TestPlanEditMeal_RemovesUnclaimed(t *testing.T) {
	current := []sheets.FoodEntry{
		{ID: "a", Description: "rice"},
		{ID: "b", Description: "broccoli"},
		{ID: "c", Description: "tofu"},
	}
	items := []gemini.AgentEntry{{Description: "rice", Calories: 200}}
	plan := planEditMeal(current, items, "lunch", "2026-04-29", "12:00", seqIDGen())

	if len(plan.toUpdate) != 1 || plan.toUpdate[0].ID != "a" {
		t.Fatalf("expected only rice updated: %+v", plan.toUpdate)
	}
	if !reflect.DeepEqual(plan.toDelete, []string{"b", "c"}) {
		t.Errorf("expected unclaimed rows deleted in order: %v", plan.toDelete)
	}
}

func TestPlanEditMeal_MealTypePropagatesOnUpdate(t *testing.T) {
	// edit_meal can be called with a different meal context; existing rows
	// should adopt the new meal_type so reclassification works.
	current := []sheets.FoodEntry{{ID: "a", Description: "yogurt", MealType: "breakfast"}}
	items := []gemini.AgentEntry{{Description: "yogurt", Calories: 120}}
	plan := planEditMeal(current, items, "snack", "2026-04-29", "15:00", seqIDGen())

	if plan.toUpdate[0].MealType != "snack" {
		t.Errorf("expected meal_type updated to snack, got %q", plan.toUpdate[0].MealType)
	}
}
