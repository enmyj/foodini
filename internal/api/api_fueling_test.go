package api

import (
	"context"
	"strings"
	"testing"
	"time"

	"foodtracker/internal/sheets"
)

// addFueling validation paths short-circuit before any sheet write, so the
// executor's nil svc is never dereferenced. Each case asserts an error
// signature without touching the network.
func TestAddFueling_Validation(t *testing.T) {
	mkExec := func() *agentExecutor {
		return &agentExecutor{
			ctx:  context.Background(),
			date: "2026-05-06",
			now:  time.Date(2026, 5, 6, 10, 0, 0, 0, time.UTC),
		}
	}

	cases := []struct {
		name    string
		args    map[string]any
		wantErr string
	}{
		{
			name:    "missing event_id",
			args:    map[string]any{"items": []any{map[string]any{"description": "gel", "carbs_g": 25}}},
			wantErr: "event_id required",
		},
		{
			name:    "missing items",
			args:    map[string]any{"event_id": "ev-1"},
			wantErr: "items required",
		},
		{
			name: "blank description in item",
			args: map[string]any{
				"event_id": "ev-1",
				"items": []any{
					map[string]any{"description": "  ", "carbs_g": 25},
				},
			},
			wantErr: "description required",
		},
		{
			name: "non-positive carbs",
			args: map[string]any{
				"event_id": "ev-1",
				"items": []any{
					map[string]any{"description": "gel", "carbs_g": 0},
				},
			},
			wantErr: "carbs_g must be > 0",
		},
	}

	for _, tt := range cases {
		t.Run(tt.name, func(t *testing.T) {
			out := mkExec().addFueling(tt.args)
			gotErr, _ := out["error"].(string)
			if !strings.Contains(gotErr, tt.wantErr) {
				t.Errorf("error: got %q, want substring %q", gotErr, tt.wantErr)
			}
		})
	}
}

func TestWriteFuelingSection_GroupsByEventWithGPerHour(t *testing.T) {
	events := []sheets.Event{
		{ID: "ev-1", Kind: sheets.EventKindWorkout, Text: "long ride", Num: 120},
		{ID: "ev-2", Kind: sheets.EventKindWorkout, Text: "easy run", Num: 0},
	}
	fueling := []sheets.FuelingEntry{
		{ID: "f1", EventID: "ev-1", Time: "09:30", Description: "gel A", CarbsG: 25, Calories: 100},
		{ID: "f2", EventID: "ev-1", Time: "10:00", Description: "gel B", CarbsG: 25, Calories: 100, SodiumMg: 200},
		{ID: "f3", EventID: "ev-2", Time: "12:00", Description: "banana", CarbsG: 25, Calories: 100},
	}
	var b strings.Builder
	writeFuelingSection(&b, fueling, events)
	out := b.String()

	if !strings.Contains(out, "long ride (120min): 50g carbs / 200kcal — 25 g/hr") {
		t.Errorf("missing grouped header w/ g/hr line:\n%s", out)
	}
	if !strings.Contains(out, "[10:00] gel B — 25g carbs, 200mg sodium") {
		t.Errorf("missing sodium-bearing row:\n%s", out)
	}
	if !strings.Contains(out, "easy run: 25g carbs / 100kcal (no duration logged — g/hr unknown)") {
		t.Errorf("missing duration-less fallback:\n%s", out)
	}
	// No-fueling case should produce zero output.
	var empty strings.Builder
	writeFuelingSection(&empty, nil, events)
	if empty.Len() != 0 {
		t.Errorf("expected empty output, got %q", empty.String())
	}
}
