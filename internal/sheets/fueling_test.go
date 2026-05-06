package sheets_test

import (
	"testing"

	"foodtracker/internal/sheets"
)

func TestFuelingEntryToRow(t *testing.T) {
	f := sheets.FuelingEntry{
		ID:          "f-1",
		Date:        "2026-05-06",
		Time:        "10:15",
		EventID:     "ev-7",
		Description: "SIS Beta Fuel gel",
		CarbsG:      40,
		Calories:    160,
		SodiumMg:    50,
		Source:      sheets.FuelSourceGel,
	}
	row := f.ToRow()
	if len(row) != 9 {
		t.Fatalf("want 9 columns, got %d", len(row))
	}
	if row[0] != "f-1" || row[3] != "ev-7" || row[4] != "SIS Beta Fuel gel" {
		t.Errorf("string columns wrong: %+v", row)
	}
	if row[5] != "40" || row[6] != "160" || row[7] != "50" {
		t.Errorf("numeric columns should serialize as strings: %+v", row)
	}
	if row[8] != "gel" {
		t.Errorf("source col: got %q", row[8])
	}
}

func TestFuelingEntryFromRow(t *testing.T) {
	row := []any{"f-1", "2026-05-06", "10:15", "ev-7", "Maurten 100", "25", "100", "0", "gel"}
	f, err := sheets.FuelingEntryFromRow(row)
	if err != nil {
		t.Fatal(err)
	}
	if f.ID != "f-1" || f.EventID != "ev-7" || f.Description != "Maurten 100" {
		t.Errorf("string fields wrong: %+v", f)
	}
	if f.CarbsG != 25 || f.Calories != 100 || f.SodiumMg != 0 {
		t.Errorf("numeric fields wrong: %+v", f)
	}
	if f.Source != "gel" {
		t.Errorf("source: got %q", f.Source)
	}
}

func TestFuelingEntryFromRow_TooShort(t *testing.T) {
	row := []any{"f-1", "2026-05-06", "10:15"}
	if _, err := sheets.FuelingEntryFromRow(row); err == nil {
		t.Errorf("expected error for short row, got nil")
	}
}

func TestFuelingEntryFromRow_MissingTrailingCols(t *testing.T) {
	// 6 columns (id..carbs_g) is the documented minimum — calories/sodium/source default to 0/"".
	row := []any{"f-1", "2026-05-06", "10:15", "ev-7", "banana", "25"}
	f, err := sheets.FuelingEntryFromRow(row)
	if err != nil {
		t.Fatalf("expected ok with 6 columns, got %v", err)
	}
	if f.CarbsG != 25 || f.Calories != 0 || f.SodiumMg != 0 || f.Source != "" {
		t.Errorf("trailing fields should default: %+v", f)
	}
}
