# Design: 💩 Poop Tracking

## Overview

Add a poop tracking section to the daily log, consistent with the existing Activity and Feeling sections in `ActivityNote.svelte`. Includes a schema migration (v1 → v2) that extends the Activity sheet rather than forcing users to recreate their spreadsheet.

## UI

- New section at the bottom of `ActivityNote.svelte` with heading "💩"
- Display state: shows "Yes" (+ notes if present) or muted placeholder "Tap to log…"
- Edit state: Yes/No toggle buttons + textarea for notes, Save/Cancel (Cmd/Ctrl+Enter to save, Escape to cancel)
- History `week-row` in `LogView.svelte`: adds a "💩" indicator column (like the existing "✓" for activity)
- `DayModal.svelte`: shows a 💩 section when `poop` is true or `poop_notes` is non-empty

## Data Model

`DayLog` struct gains two fields:

```go
Poop      bool   `json:"poop"`
PoopNotes string `json:"poop_notes"`
```

Activity sheet columns extend from 4 to 6:

```
date | activity | feeling_score | feeling_notes | poop | poop_notes
```

`DayLogFromRow` already handles short rows via `len(row) >= N` guards — new fields default to zero values for old rows.

`ToRow()` returns all 6 values.

`GetActivity` and `GetActivityByDateRange` read columns A:F instead of A:D.
`SetActivity` writes the full 6-column row.

## Schema Migration (v1 → v2)

`CurrentSchemaVersion` bumps to 2.

New `MigrateSpreadsheet(ctx, ts, spreadsheetID)` function:
1. Reads Activity sheet header row (A1:F1)
2. If columns E/F are missing, writes the full 6-column header
3. Updates Meta sheet A2 to "2"

`ensureSpreadsheet` in `api.go`:
- If `version == 1`: call `MigrateSpreadsheet`, continue (no 409 error)
- If `version < 1`: return 409 as before (truly incompatible)

`CreateSpreadsheet` writes the 6-column Activity header for new spreadsheets.

## API

`putActivity` / `getActivity` API and `api.js` helpers pass the new fields through unchanged — the `DayLog` struct is already used as the request/response body.

`putActivity` in `api.js` gains `poop` and `poop_notes` params.
