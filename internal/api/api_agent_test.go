package api

import (
	"testing"
	"time"
)

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
