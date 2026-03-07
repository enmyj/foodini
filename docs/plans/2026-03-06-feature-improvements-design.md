# Feature Improvements Design

Date: 2026-03-06

## Goals

1. Split "Activity / Notes" into separate Activity and Feeling fields
2. Add chat confirmation flow before saving food entries
3. Redesign week view to surface feeling + activity, with food detail in a modal

Meal order unchanged: breakfast, snack, lunch, dinner.

---

## 1. Activity + Feeling Split

### Sheets Schema

`Activity` tab columns change from:
```
date | notes
```
to:
```
date | activity | feeling_score | feeling_notes
```

Backward compat: existing rows with data only in column B are treated as `activity`.

### Go — DayLog struct (replaces single string return)

```go
type DayLog struct {
    Date         string `json:"date"`
    Activity     string `json:"activity"`
    FeelingScore int    `json:"feeling_score"` // 0 = not set, 1–10
    FeelingNotes string `json:"feeling_notes"`
}
```

### API Changes

- `GET /api/activity?date=` → `{date, activity, feeling_score, feeling_notes}`
- `PUT /api/activity` → body: `{date, activity, feeling_score, feeling_notes}`
- `GET /api/log?week=true` → adds `daily_logs: []DayLog` to response

### Frontend — ActivityNote.svelte

Two labelled sections replacing the single textarea:
- **Activity** — textarea (what you did: exercise, stress, etc.)
- **Feeling** — row with 1–10 number input + textarea for notes

---

## 2. Chat Confirmation Flow

### Problem

Currently Gemini returns JSON → backend auto-saves. User has no chance to review.

### New Flow

1. User describes food
2. Gemini clarifies if needed (unchanged)
3. When Gemini has enough info: shows human-readable summary + asks "Does this look right?" + includes JSON in a code block
4. Backend detects JSON → does NOT save → returns `{done: false, pending: true, entries: [...], message: "..."}`
5. Frontend shows message + "Looks good, save it" button
6. **Confirm path:** user clicks button → `POST /api/chat/confirm` → backend saves → returns `{done: true, entries: [...]}`
7. **Correction path:** user types correction → conversation continues, pending state cleared

### Gemini Prompt Change

Replace "respond ONLY with JSON" with:
- When ready: show friendly human-readable bullet list of entries, ask "Does this look right?", include JSON in a code block
- This makes the pending-detection reliable (JSON still parsed by backend, human text shown to user)

### New Endpoint

`POST /api/chat/confirm`
- Body: `{entries: []FoodEntry}`
- Saves entries, clears user conversation history
- Returns `{done: true, entries: []FoodEntry}`

### Backend Change to Chat Handler

When `ParseEntries` finds entries in Gemini's response:
- Return `pending: true`, `entries`, and `message` (full Gemini text)
- Do NOT save, do NOT clear history

---

## 3. Week View Redesign

### Day Row

```
2026-03-01   6/10   ✓   >
```
- Date (left)
- Feeling score (`N/10`, muted if not set: `—`)
- Activity tick (`✓` if activity logged, empty if not)
- Chevron (right, indicates clickable)

### Modal

Clicking a row opens a full-day modal:
- Header: date
- Feeling: score + notes
- Activity: text
- Food: entries grouped by meal with per-meal totals
- Overall macro totals for the day
- Close button (or tap backdrop)

### Week API

`GET /api/log?week=true` response:
```json
{
  "entries": [...],
  "daily_logs": [{"date":"2026-03-01","activity":"ran 5k","feeling_score":6,"feeling_notes":"tired but ok"}],
  "start": "...",
  "end": "..."
}
```

---

## What Does Not Change

- Meal types: breakfast, snack, lunch, dinner
- Food entry schema (Food sheet unchanged)
- Inline editing of food entries
- All Go tests must remain passing
