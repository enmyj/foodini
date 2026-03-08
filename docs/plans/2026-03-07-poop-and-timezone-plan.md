# Poop Tracking + Client Timezone Implementation Plan

> **For Claude:** REQUIRED SUB-SKILL: Use superpowers:executing-plans to implement this plan task-by-task.

**Goal:** Add a 💩 tracking section to the daily log (with schema migration v1→v2) and fix timezone handling by sending the user's IANA timezone from the client.

**Architecture:** Timezone fix adds a `localNow(r)` helper in `api.go` replacing all bare `time.Now()` calls, and an `X-Timezone` header added to every fetch in `api.js`. Poop tracking extends `DayLog` with two new fields, bumps `CurrentSchemaVersion` to 2, adds a `MigrateSpreadsheet` function (appends columns to existing Activity sheet header instead of forcing recreate), and adds a 💩 section to `ActivityNote.svelte` plus indicators in history views.

**Tech Stack:** Go 1.22, Svelte 5 (runes), Google Sheets API. Run Go tests with `~/go-sdk/go/bin/go test ./...`. Run frontend build with `mise run build`.

---

## Task 1: Backend timezone helper

**Files:**
- Modify: `internal/api/api_test.go`
- Modify: `internal/api/api.go`

**Step 1: Write the failing test**

Add to `internal/api/api_test.go`:

```go
func TestLocalNow_WithTimezone(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.Header.Set("X-Timezone", "America/New_York")
	got := api.LocalNow(req)
	// Just verify it returns a time in the correct location
	if got.Location().String() != "America/New_York" {
		t.Errorf("location: got %q, want America/New_York", got.Location())
	}
}

func TestLocalNow_InvalidTimezone(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.Header.Set("X-Timezone", "Not/AReal/Zone")
	got := api.LocalNow(req)
	// Falls back gracefully — just verify it returns a non-zero time
	if got.IsZero() {
		t.Error("expected non-zero time for invalid timezone")
	}
}

func TestLocalNow_NoHeader(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	got := api.LocalNow(req)
	if got.IsZero() {
		t.Error("expected non-zero time with no header")
	}
}
```

**Step 2: Run test to verify it fails**

```bash
~/go-sdk/go/bin/go test ./internal/api/... -run TestLocalNow -v
```

Expected: FAIL — `api.LocalNow undefined`

**Step 3: Add `LocalNow` to `api.go`**

Add this function after the `writeErr` function (around line 84):

```go
// LocalNow returns the current time in the user's local timezone.
// It reads the IANA timezone name from the X-Timezone request header.
// Falls back to server time if the header is missing or invalid.
func LocalNow(r *http.Request) time.Time {
	tz := r.Header.Get("X-Timezone")
	if tz != "" {
		if loc, err := time.LoadLocation(tz); err == nil {
			return time.Now().In(loc)
		}
	}
	return time.Now()
}
```

**Step 4: Replace all `time.Now()` calls in `api.go`**

There are 6 occurrences to replace. Search for `time.Now()` in `api.go` and apply:

- `GetLog` line ~173: `today := sheets.DateString(time.Now())` → `today := sheets.DateString(LocalNow(r))`
- `GetLog` range line ~186: `start := sheets.DateString(time.Now().AddDate(0, 0, -(days - 1)))` → `start := sheets.DateString(LocalNow(r).AddDate(0, 0, -(days - 1)))`
- `Chat` line ~261: `targetDate = sheets.DateString(time.Now())` → `targetDate = sheets.DateString(LocalNow(r))`
- `ConfirmChat` line ~310: `targetDate = sheets.DateString(time.Now())` → `targetDate = sheets.DateString(LocalNow(r))`
- `ConfirmChat` line ~321: `now := time.Now()` → `now := LocalNow(r)`
- `GetActivity` line ~443: `date = sheets.DateString(time.Now())` → `date = sheets.DateString(LocalNow(r))`
- `PutActivity` line ~462: `req.Date = sheets.DateString(time.Now())` → `req.Date = sheets.DateString(LocalNow(r))`

**Step 5: Run tests**

```bash
~/go-sdk/go/bin/go test ./internal/api/... -v
```

Expected: all PASS

**Step 6: Commit**

```bash
git add internal/api/api.go internal/api/api_test.go
git commit -m "feat: add LocalNow helper for user timezone support"
```

---

## Task 2: Frontend timezone header

**Files:**
- Modify: `frontend/src/lib/api.js`

**Step 1: Add timezone constant and update all fetch calls**

At the top of `frontend/src/lib/api.js`, add:

```js
const TZ = Intl.DateTimeFormat().resolvedOptions().timeZone
```

Then update every `fetch` call to include `'X-Timezone': TZ` in headers.

For GET calls that currently have no options object, add one:

```js
// getLog — add header to GET
const res = await fetch(`/api/log${params}`, { headers: { 'X-Timezone': TZ } })

// getActivity — add header to GET
const res = await fetch(`/api/activity?date=${date}`, { headers: { 'X-Timezone': TZ } })

// getProfile — add header to GET
const res = await fetch('/api/profile', { headers: { 'X-Timezone': TZ } })

// deleteEntry — add header to DELETE
const res = await fetch(`/api/entries/${id}`, { method: 'DELETE', headers: { 'X-Timezone': TZ } })
```

For POST/PUT/PATCH calls that already have `headers: { 'Content-Type': 'application/json' }`, add `'X-Timezone': TZ` to that object:

```js
headers: { 'Content-Type': 'application/json', 'X-Timezone': TZ },
```

Apply this to: `chat`, `confirmChat`, `patchEntry`, `putActivity`, `putProfile`.

**Step 2: Build to verify no syntax errors**

```bash
mise run build
```

Expected: build succeeds

**Step 3: Commit**

```bash
git add frontend/src/lib/api.js
git commit -m "feat: send X-Timezone header with all API requests"
```

---

## Task 3: Extend DayLog for poop tracking

**Files:**
- Modify: `internal/sheets/sheets.go`
- Modify: `internal/sheets/sheets_test.go`

**Step 1: Update existing tests first (they'll still pass — we're adding fields)**

In `sheets_test.go`:

1. Find `TestDayLogToRow` — update it to expect 6 columns and check the new fields:

```go
func TestDayLogToRow(t *testing.T) {
	d := sheets.DayLog{Date: "2026-03-06", Activity: "yoga", FeelingScore: 8, FeelingNotes: "great day"}
	row := d.ToRow()
	if len(row) != 6 {
		t.Fatalf("want 6 cols, got %d", len(row))
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
}
```

2. Update `TestGetSchemaVersion_ReturnsValue` to expect 2:

```go
func TestGetSchemaVersion_ReturnsValue(t *testing.T) {
	_ = sheets.CurrentSchemaVersion
	if sheets.CurrentSchemaVersion != 2 {
		t.Errorf("CurrentSchemaVersion: got %d, want 2", sheets.CurrentSchemaVersion)
	}
}
```

3. Add new tests for the extended DayLog:

```go
func TestDayLogFromRow_WithPoop(t *testing.T) {
	row := []interface{}{"2026-03-07", "ran 5k", "8", "felt good", "true", "solid, once"}
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
	row := []interface{}{"2026-03-07", "yoga", "7", "good"}
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
	if len(row) != 6 {
		t.Fatalf("want 6 cols, got %d", len(row))
	}
	if row[4] != "true" {
		t.Errorf("col 4 (poop): got %v, want true", row[4])
	}
	if row[5] != "once" {
		t.Errorf("col 5 (poop_notes): got %v, want once", row[5])
	}
}
```

**Step 2: Run tests to verify they fail**

```bash
~/go-sdk/go/bin/go test ./internal/sheets/... -v
```

Expected: FAIL — `CurrentSchemaVersion is 1`, `DayLog.Poop undefined`, `ToRow len 4 not 6`

**Step 3: Update `sheets.go`**

a. Bump schema version:
```go
CurrentSchemaVersion = 2
```

b. Add fields to `DayLog`:
```go
type DayLog struct {
	Date         string `json:"date"`
	Activity     string `json:"activity"`
	FeelingScore int    `json:"feeling_score"`
	FeelingNotes string `json:"feeling_notes"`
	Poop         bool   `json:"poop"`
	PoopNotes    string `json:"poop_notes"`
}
```

c. Update `ToRow()` to return 6 columns:
```go
func (d DayLog) ToRow() []interface{} {
	return []interface{}{
		d.Date, d.Activity, strconv.Itoa(d.FeelingScore), d.FeelingNotes,
		strconv.FormatBool(d.Poop), d.PoopNotes,
	}
}
```

d. Update `DayLogFromRow` to handle columns E and F:
```go
func DayLogFromRow(row []interface{}) DayLog {
	str := func(v interface{}) string { return fmt.Sprintf("%v", v) }
	num := func(v interface{}) int {
		n, _ := strconv.Atoi(fmt.Sprintf("%v", v))
		return n
	}
	d := DayLog{}
	if len(row) >= 1 { d.Date = str(row[0]) }
	if len(row) >= 2 { d.Activity = str(row[1]) }
	if len(row) >= 3 { d.FeelingScore = num(row[2]) }
	if len(row) >= 4 { d.FeelingNotes = str(row[3]) }
	if len(row) >= 5 { d.Poop = str(row[4]) == "true" }
	if len(row) >= 6 { d.PoopNotes = str(row[5]) }
	return d
}
```

e. Update `GetActivity` and `GetActivityByDateRange` to read 6 columns (A:F instead of A:D):

In `GetActivity`: change `activitySheet+"!A:D"` → `activitySheet+"!A:F"`

In `GetActivityByDateRange`: change `activitySheet+"!A:D"` → `activitySheet+"!A:F"`

f. Update `SetActivity` to write 6 columns (A:F instead of A:D):

In `SetActivity`:
- Append range: `activitySheet+"!A:F"`
- Update range: `fmt.Sprintf("%s!A%d:F%d", activitySheet, rowNum, rowNum)`

g. Update `CreateSpreadsheet` Activity sheet header write:
- Range: `activitySheet+"!A1:F1"`
- Values: `{"date", "activity", "feeling_score", "feeling_notes", "poop", "poop_notes"}`

**Step 4: Run tests**

```bash
~/go-sdk/go/bin/go test ./internal/sheets/... -v
```

Expected: all PASS

**Step 5: Commit**

```bash
git add internal/sheets/sheets.go internal/sheets/sheets_test.go
git commit -m "feat: extend DayLog with poop tracking fields, bump schema to v2"
```

---

## Task 4: Schema migration (v1 → v2)

**Files:**
- Modify: `internal/sheets/sheets.go`
- Modify: `internal/sheets/sheets_test.go`
- Modify: `internal/api/api.go`
- Modify: `internal/api/api_test.go`

**Step 1: Write the test for MigrateSpreadsheet existence**

Add to `sheets_test.go`:

```go
func TestMigrateSpreadsheet_MethodExists(t *testing.T) {
	// Compilation check — verify the function signature exists
	var _ func(context.Context, oauth2.TokenSource, string) error = sheets.MigrateSpreadsheet
}
```

This needs `"context"` and `"golang.org/x/oauth2"` imports. Add them if missing.

**Step 2: Run test to verify it fails**

```bash
~/go-sdk/go/bin/go test ./internal/sheets/... -run TestMigrate -v
```

Expected: FAIL — `sheets.MigrateSpreadsheet undefined`

**Step 3: Implement `MigrateSpreadsheet` in `sheets.go`**

Add after `CreateSpreadsheet`:

```go
// MigrateSpreadsheet upgrades an existing spreadsheet from schema v1 to v2.
// It extends the Activity sheet header to include poop and poop_notes columns,
// then bumps the schema_version in the Meta sheet to 2.
func MigrateSpreadsheet(ctx context.Context, ts oauth2.TokenSource, spreadsheetID string) error {
	sheetsSvc, err := googlesheets.NewService(ctx, option.WithTokenSource(ts))
	if err != nil {
		return fmt.Errorf("sheets client: %w", err)
	}

	// Write full 6-column Activity header (safe to overwrite; data starts at row 2)
	actHeaders := &googlesheets.ValueRange{
		Values: [][]interface{}{{"date", "activity", "feeling_score", "feeling_notes", "poop", "poop_notes"}},
	}
	_, err = sheetsSvc.Spreadsheets.Values.Update(
		spreadsheetID, activitySheet+"!A1:F1", actHeaders,
	).ValueInputOption("RAW").Context(ctx).Do()
	if err != nil {
		return fmt.Errorf("migrate activity header: %w", err)
	}

	// Bump schema version to 2
	metaData := &googlesheets.ValueRange{
		Values: [][]interface{}{{strconv.Itoa(CurrentSchemaVersion)}},
	}
	_, err = sheetsSvc.Spreadsheets.Values.Update(
		spreadsheetID, metaSheet+"!A2", metaData,
	).ValueInputOption("RAW").Context(ctx).Do()
	if err != nil {
		return fmt.Errorf("migrate schema version: %w", err)
	}

	return nil
}
```

**Step 4: Update `ensureSpreadsheet` in `api.go`**

Find the block that checks the schema version (around line 137):

```go
version, err := svc.GetSchemaVersion(r.Context())
if err != nil || version < sheets.CurrentSchemaVersion {
    writeErr(w, http.StatusConflict, "incompatible_spreadsheet")
    return false
}
```

Replace with:

```go
version, err := svc.GetSchemaVersion(r.Context())
if err != nil {
    writeErr(w, http.StatusInternalServerError, "schema check failed")
    return false
}
if version == 1 {
    // Migrate v1 → v2: add poop columns to Activity sheet
    if err := sheets.MigrateSpreadsheet(r.Context(), ts, id); err != nil {
        writeErr(w, http.StatusInternalServerError, "migration failed: "+err.Error())
        return false
    }
} else if version < 1 {
    writeErr(w, http.StatusConflict, "incompatible_spreadsheet")
    return false
}
```

**Step 5: Run all Go tests**

```bash
~/go-sdk/go/bin/go test ./... -v
```

Expected: all PASS

**Step 6: Commit**

```bash
git add internal/sheets/sheets.go internal/sheets/sheets_test.go internal/api/api.go internal/api/api_test.go
git commit -m "feat: add MigrateSpreadsheet for v1→v2 schema migration"
```

---

## Task 5: Frontend — update `putActivity` in `api.js`

**Files:**
- Modify: `frontend/src/lib/api.js`

**Step 1: Update `putActivity` to include poop fields**

Find the `putActivity` function. Change its signature and body:

```js
export async function putActivity(date, { activity, feeling_score, feeling_notes, poop, poop_notes }) {
  const res = await fetch('/api/activity', {
    method: 'PUT',
    headers: { 'Content-Type': 'application/json', 'X-Timezone': TZ },
    body: JSON.stringify({ date, activity, feeling_score, feeling_notes, poop, poop_notes }),
  })
  if (!res.ok) throw new Error(await res.text())
  return res.json()
}
```

**Step 2: Build to verify**

```bash
mise run build
```

Expected: succeeds

**Step 3: Commit**

```bash
git add frontend/src/lib/api.js
git commit -m "feat: pass poop fields through putActivity API helper"
```

---

## Task 6: Frontend — 💩 section in `ActivityNote.svelte`

**Files:**
- Modify: `frontend/src/lib/ActivityNote.svelte`

**Step 1: Add poop state variables**

In the `<script>` block, after the existing state declarations, add:

```js
let poop = $state(false)
let poopNotes = $state('')
let editingPoop = $state(false)
```

**Step 2: Load poop fields from API response**

In the `onMount`, after the existing field assignments:

```js
poop = res.poop ?? false
poopNotes = res.poop_notes ?? ''
```

**Step 3: Include poop in the `save()` call**

Update the `putActivity` call in `save()`:

```js
await putActivity(date, { activity, feeling_score: feelingScore, feeling_notes: feelingNotes, poop, poop_notes: poopNotes })
```

Also update the `editingPoop = false` line — add it to the success block:

```js
editingActivity = false
editingFeeling = false
editingPoop = false
```

**Step 4: Add the 💩 section to the template**

After the closing `</div>` of the Feeling section (before the `{#if saving}` line), add:

```svelte
<div class="section">
  <h3>💩</h3>
  {#if editingPoop}
    <div class="poop-edit">
      <div class="poop-toggle">
        <button
          class="toggle-btn"
          class:selected={poop}
          onclick={() => poop = true}
        >Yes</button>
        <button
          class="toggle-btn"
          class:selected={!poop}
          onclick={() => poop = false}
        >No</button>
      </div>
      <textarea
        bind:value={poopNotes}
        onkeydown={onKeyDown}
        placeholder="Any details…"
        rows="2"
      ></textarea>
      <div class="edit-actions">
        <button class="save-btn" onclick={save} disabled={saving}>Save</button>
        <button class="cancel-btn" onclick={() => editingPoop = false}>Cancel</button>
      </div>
    </div>
  {:else}
    <!-- svelte-ignore a11y_click_events_have_key_events a11y_no_static_element_interactions -->
    <div class="note" class:placeholder={!poop && !poopNotes} onclick={() => editingPoop = true}>
      {#if poop}
        Yes{#if poopNotes} — {poopNotes}{/if}
      {:else if poopNotes}
        No — {poopNotes}
      {:else}
        Tap to log…
      {/if}
    </div>
  {/if}
</div>
```

**Step 5: Add styles**

In the `<style>` block, add after the existing styles:

```css
.poop-edit {
  display: flex;
  flex-direction: column;
  gap: 0.5rem;
}

.poop-toggle {
  display: flex;
  gap: 0.5rem;
}

.toggle-btn {
  padding: 0.25rem 0.75rem;
  border: 1px solid #e8e8e6;
  border-radius: 5px;
  background: none;
  color: #888;
  font-size: 0.82rem;
  font-family: inherit;
  cursor: pointer;
}

.toggle-btn.selected {
  background: #2d2d2d;
  color: #fafaf9;
  border-color: #2d2d2d;
}
```

**Step 6: Update `onKeyDown` to cancel poop editing on Escape**

Find:
```js
if (e.key === 'Escape') { editingActivity = false; editingFeeling = false }
```
Replace with:
```js
if (e.key === 'Escape') { editingActivity = false; editingFeeling = false; editingPoop = false }
```

**Step 7: Build**

```bash
mise run build
```

Expected: succeeds

**Step 8: Commit**

```bash
git add frontend/src/lib/ActivityNote.svelte
git commit -m "feat: add poop tracking section to ActivityNote"
```

---

## Task 7: Frontend — 💩 indicator in history and DayModal

**Files:**
- Modify: `frontend/src/lib/LogView.svelte`
- Modify: `frontend/src/lib/DayModal.svelte`

### LogView history row

**Step 1: Add 💩 column to `week-row`**

In `LogView.svelte`, find the `week-row` template (around line 177):

```svelte
<div class="week-row" onclick={() => selectedDay = { date, entries, dayLog }}>
  <span class="date">{date}</span>
  <span class="feeling-score">{dayLog?.feeling_score ? `${dayLog.feeling_score}/10` : '—'}</span>
  <span class="activity-tick">{dayLog?.activity ? '✓' : ''}</span>
  <span class="chevron">›</span>
</div>
```

Add a poop indicator span between `activity-tick` and `chevron`:

```svelte
<span class="poop-tick">{dayLog?.poop ? '💩' : ''}</span>
```

Add the style for it (after `.activity-tick`):

```css
.poop-tick {
  font-size: 0.82rem;
  min-width: 1rem;
  text-align: center;
}
```

### DayModal poop section

**Step 2: Add poop section to `DayModal.svelte`**

In `DayModal.svelte`, find the block that shows the activity section (around line 45):

```svelte
{#if day.dayLog?.activity}
  <div class="modal-section">
    <h3>Activity</h3>
    <p>{day.dayLog.activity}</p>
  </div>
{/if}
```

Add a poop section after it:

```svelte
{#if day.dayLog?.poop || day.dayLog?.poop_notes}
  <div class="modal-section">
    <h3>💩</h3>
    <p>
      {day.dayLog.poop ? 'Yes' : 'No'}{#if day.dayLog.poop_notes} — {day.dayLog.poop_notes}{/if}
    </p>
  </div>
{/if}
```

**Step 3: Build**

```bash
mise run build
```

Expected: succeeds

**Step 4: Commit**

```bash
git add frontend/src/lib/LogView.svelte frontend/src/lib/DayModal.svelte
git commit -m "feat: show poop indicator in history row and day modal"
```

---

## Task 8: Final verification

**Step 1: Run all Go tests**

```bash
~/go-sdk/go/bin/go test ./... -v
```

Expected: all PASS

**Step 2: Build frontend**

```bash
mise run build
```

Expected: succeeds, no errors

**Step 3: Smoke test mentally**

- New user: `CreateSpreadsheet` writes 6-column Activity header ✓
- Existing v1 user: `ensureSpreadsheet` calls `MigrateSpreadsheet` → extends header, bumps version ✓
- Logging food after midnight in user's timezone: `X-Timezone` header sent, `LocalNow(r)` returns correct local date ✓
- Tapping 💩 section: shows Yes/No toggle + notes, saves with rest of day data ✓
- History view: shows 💩 emoji for days where poop=true ✓
- Day modal: shows 💩 section when poop data exists ✓
