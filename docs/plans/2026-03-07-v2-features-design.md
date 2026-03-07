# V2 Features Design

Date: 2026-03-07

## Goals

1. Replace week view with a 30-day rolling calendar; past days editable via chat
2. Schema versioning with `Meta` sheet (version integer stored in spreadsheet)
3. Add fiber to food entries
4. User profile sheet (gender, height, weight, free-form notes) injected into Gemini context
5. Delete food entries
6. Spreadsheet find-or-create on login (search Drive for existing spreadsheet)

No v0→v1 migration needed — no production users yet. App launches at schema v1.

---

## 1. Calendar View + Past Day Editing

### Replaces the "Week" tab

The second tab is renamed from "Week" to "History" (or keep as "Week" — TBD by implementer based on label width). Shows 30 days rolling, most recent at top.

### Day row (same as current week row)

```
2026-03-06   7/10   ✓   ›
2026-03-05   —          ›
```

- Date (left, flex:1)
- Feeling score (`N/10` or `—`)
- Activity tick (`✓` if logged)
- Chevron (right)

### Day detail modal (DayModal)

Current `DayModal` gains:
- **"Add food for this day"** button → opens ChatDrawer with `date` prop set to that day
- Food entries shown with edit (existing inline edit) and delete (× button, new)
- Activity and Feeling sections inline-editable (already works via `ActivityNote`)

### Past-day chat

`ChatDrawer` gets optional `date` prop (defaults to today).

- `POST /api/chat` body gains optional `"date": "YYYY-MM-DD"` field
- `POST /api/chat/confirm` body gains optional `"date": "YYYY-MM-DD"` field — saves entries with that date instead of `time.Now()`
- Gemini conversation history keyed by `userEmail + ":" + date` so past-day chats don't interfere with today's conversation

### API change

`GET /api/log?week=true` → `GET /api/log?days=30` (or any integer, default 30).

Response shape unchanged: `{entries, daily_logs, start, end}`.

Old `?week=true` parameter removed.

---

## 2. Schema Versioning

### Meta sheet

`CreateSpreadsheet` creates a `Meta` sheet with columns `key | value` and writes:
```
schema_version | 1
```

### Go

```go
const CurrentSchemaVersion = 1

func GetSchemaVersion(ctx, svc, spreadsheetID) (int, error)
func SetSchemaVersion(ctx, svc, spreadsheetID, v int) error
```

### Migration check

`ensureSpreadsheet` in `api.go`, after finding/creating the spreadsheet, calls:

```go
version, err := sheets.GetSchemaVersion(ctx, ts, spreadsheetID)
if err != nil { ... }
if version == 0 {
    // No Meta sheet — pre-versioning spreadsheet, incompatible
    writeErr(w, 409, "incompatible_spreadsheet")
    return false
}
if version < sheets.CurrentSchemaVersion {
    // Future: run migrations here
    // For now: no migrations needed
}
```

Frontend handles `incompatible_spreadsheet` error with:
> "Your existing Food Tracker spreadsheet is from an older version. Please rename it in Google Drive and reload the page."

---

## 3. Fiber Column

### Go — FoodEntry

```go
type FoodEntry struct {
    ID          string `json:"id"`
    Date        string `json:"date"`
    Time        string `json:"time"`
    MealType    string `json:"meal_type"`
    Description string `json:"description"`
    Calories    int    `json:"calories"`
    Protein     int    `json:"protein"`
    Carbs       int    `json:"carbs"`
    Fat         int    `json:"fat"`
    Fiber       int    `json:"fiber"`
}
```

- `ToRow()` returns 10 columns (A:J)
- `FoodEntryFromRow` reads index 9 (defaults 0 if missing — backward compat for any stray old rows)
- Food sheet headers: `id, date, time, meal_type, description, calories, protein, carbs, fat, fiber`
- All Sheets API calls updated to use `A:J` range

### Gemini prompt

Add to the JSON schema example:
```json
{"entries":[{"meal_type":"breakfast","description":"oatmeal","calories":300,"protein":8,"carbs":54,"fat":6,"fiber":4}]}
```

Add to rules: `- Include fiber (grams) as an estimated integer`

### Frontend

- `EntryRow.svelte`: show fiber in macro display (e.g. `4g F` → `6g F  4g Fb`)
- `DayModal.svelte`: include fiber in per-entry macros and day totals
- `ActivityNote` / week rows: not shown (too much noise)

---

## 4. User Profile

### Profile sheet

`CreateSpreadsheet` creates a `Profile` sheet with columns `key | value` and writes:
```
gender         |
height         |
weight         |
notes          |
```
(empty values — user fills in later)

### Go

```go
type UserProfile struct {
    Gender string `json:"gender"`
    Height string `json:"height"`
    Weight string `json:"weight"`
    Notes  string `json:"notes"`
}

func (s *Service) GetProfile(ctx) (UserProfile, error)
func (s *Service) SetProfile(ctx, p UserProfile) error
```

### API

- `GET /api/profile` → `UserProfile` JSON
- `PUT /api/profile` → body: `UserProfile` JSON

### Gemini integration

`gemini.Service.Chat` gains a `profile UserProfile` parameter (passed from the API handler).

System prompt prefix injected when profile has any non-empty field:
```
User profile: [gender], [height], [weight].
[notes]
```

### Frontend

- Settings icon (⚙) in header (top-right)
- Opens a `ProfilePanel.svelte` — either inline slide-down or overlay panel
- Fields: Gender (text input), Height (text), Weight (text), Notes (textarea)
- Save on blur or explicit Save button
- Loaded once on mount, saved via `PUT /api/profile`

---

## 5. Delete Food Entries

### Go — sheets.go

```go
func (s *Service) DeleteFood(ctx context.Context, id string) error
```

Uses `batchUpdate` with `DeleteDimensionRequest` to remove the row.
Needs to first fetch the sheet's numeric sheetId from `Spreadsheets.Get`.

### API — api.go

```go
// DELETE /api/entries/{id}
func (h *Handler) DeleteEntry(w http.ResponseWriter, r *http.Request)
```

Returns 204 No Content on success.

### Route — main.go

```go
mux.HandleFunc("DELETE /api/entries/{id}", apiHandler.Authenticated(apiHandler.DeleteEntry))
```

### Frontend

- `api.js`: `deleteEntry(id)` → `DELETE /api/entries/{id}`
- `EntryRow.svelte`: × button (top-right of row), calls `onDelete(entry.id)` prop
- `LogView.svelte`: `handleDelete(id)` → filters entry from `data.entries`
- `DayModal.svelte`: pass `onDelete` through to entry rows shown in modal

---

## 6. Spreadsheet Find-or-Create

### Flow (on every ensureSpreadsheet call when SpreadsheetID is empty)

1. Search Drive for `"Food Tracker — {email}"` (mimeType spreadsheet, not trashed)
2. If found → read Meta sheet schema version
   - version == 0 or Meta missing → return `incompatible_spreadsheet` error
   - version == CurrentSchemaVersion → reuse
3. If not found → create new (CreateSpreadsheet), which writes schema v1

### Go — sheets.go

```go
func FindExistingSpreadsheet(ctx context.Context, ts oauth2.TokenSource, userEmail string) (string, error)
```

Uses Drive API `files.list` with query: `name='Food Tracker — {email}' and mimeType='application/vnd.google-apps.spreadsheet' and trashed=false`

### Frontend — App.svelte

Detect `incompatible_spreadsheet` error from any API call, show:
> "Your existing Food Tracker spreadsheet is from an older version. Please rename it in Google Drive, then reload this page to create a fresh one."

---

## What Does Not Change

- Auth flow (Google OAuth)
- Cookie/session structure (SpreadsheetID still in cookie)
- Activity / Feeling schema (already at v1 from previous session)
- Inline editing of food entries (PATCH /api/entries/{id})
- Chat confirmation flow
- Palette: `#fafaf9` bg · `#1c1c1c` text · `#888` muted · `#2d2d2d` accent · `#e8e8e6` border
