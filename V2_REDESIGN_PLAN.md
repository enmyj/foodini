# V2 UX Redesign: Auto-save, Unified Drawer Editing, Per-Meal Insights

## Context

User testing revealed three problems:
1. The describe → review → save flow is confusing — users expect food to be logged immediately
2. Insights are detached from meals and not actionable enough
3. Suggestions swing between too vague ("eat protein + grain + veg") and too complex (full recipes)

This redesign makes logging frictionless (auto-save), editing powerful (drawer as unified edit UI), and insights contextual (attached per-meal, generated async).

---

## Phase 1: Auto-save on Chat Submit

### Backend Changes

**`internal/api/api_chat.go`** — Modify `Chat` handler:
- When Gemini returns entries (not a clarifying question), auto-save them to Sheets immediately
- Assign UUIDs, date, time, call `svc.AppendFood()` (same logic currently in `ConfirmChat`)
- Return saved entries with IDs in response
- Clear conversation history after save
- Keep `/api/chat/confirm` endpoint for now (used by favorites/repeat flows) but new chat entries no longer need it

**Response shape change:**
```json
// Before: { "done": false, "pending": true, "entries": [...], "message": "Got it!" }
// After:  { "done": true, "entries": [{ ...with IDs... }], "message": "Got it!" }
```

When `done: true` and entries have IDs → frontend knows they're already saved.

### Frontend Changes

**`frontend/src/lib/ChatDrawer.svelte`**:
- Remove the "Save" confirmation step for new entries
- When chat response has `done: true` with entries: call `onEntriesAdded(entries)` immediately
- Remove the result-card editing step (entries are already saved; editing happens via "Edit" flow)
- Keep clarifying question flow as-is

---

## Phase 2: Drawer as Unified Edit Interface

### Backend Changes

**`internal/api/api_chat.go`** — New endpoint `POST /api/chat/edit`:
```
Request: {
  "message": "remove the toast",           // natural language edit instruction
  "entries": [{ id, description, calories, ... }],  // current entries
  "date": "2026-04-17",
  "meal_type": "breakfast"
}
Response: {
  "entries": [{ id, description, calories, ... }],  // updated entries
  "message": "Removed the toast."
}
```
- Sends existing entries + edit instruction to Gemini
- Gemini returns modified entry list (may add/remove/change entries)
- Backend diffs: patches changed entries, deletes removed ones, appends new ones
- Returns final entry list with IDs

**`internal/gemini/gemini.go`** — New method `EditEntries()`:
- System prompt tells Gemini: "You are editing an existing meal. The user's entries are: [entries]. Apply the user's requested change. Return the full updated entry list."
- Same JSON response schema as `Chat()` (message + entries array)
- Entries that should be removed are simply omitted from the response

**`router.go`** — Register new route:
```go
api.POST("/chat/edit", h.EditChat)
```

### Frontend Changes

**`frontend/src/lib/ChatDrawer.svelte`** — Add edit mode:
- New props: `editEntries` (array of existing entries to edit), `editMealType`
- When `editEntries` is provided, drawer opens in edit mode:
  - Shows existing entries in result-card format (inline editable)
  - Chat input placeholder: "Describe changes..." instead of "What did you eat?"
  - Whole-meal multiplier buttons: ×0.75, ×1.25, ×1.5 (scales all entries, auto-saves via `patchEntry`)
  - Submitting text calls `POST /api/chat/edit` instead of `POST /api/chat`
  - After edit response: patch changed entries, show updated list
- Inline edits (changing a number directly) auto-save on blur via `patchEntry()`
- Add delete button per entry in edit mode

**`frontend/src/lib/LogView.svelte`** — Meal row changes:
- Replace `"breakfast · 450 cal"` with `"breakfast"` + `[Insights]` + `[Edit]` buttons
- `[Edit]` opens drawer in edit mode with that meal's entries
- Remove per-meal calorie total from the header (calories shown at top summary + per entry row)
- Move repeat (↻) and scale (⊕) buttons from meal rows into the drawer
- Meal row becomes: meal name + [Insights] + [Edit] — clean and simple
- Drawer edit mode includes: multiplier buttons, repeat from previous day, quick-add from favorites, natural language edits, inline edits
- Favorites in drawer: searchable list of saved favorites, tap to add to current meal (auto-saves)

**`frontend/src/lib/api.js`** — New function:
```js
export async function editChat(message, entries, date, mealType) { ... }
```

---

## Phase 3: Per-Meal Async Insights

### Backend Changes

**`internal/api/api_chat.go`** — After auto-save, kick off goroutine:
```go
go h.generateMealInsight(spreadsheetID, date, mealType, geminiAPIKey)
```

**`internal/api/api_insights.go`** — New handler + goroutine:
- `generateMealInsight()`: fetches last 3-5 meals from Sheets, calls `gemini.MealInsight()`, saves to Insights sheet
- Insight type in InsightRecord: `"meal:breakfast"`, `"meal:lunch"`, etc.
- Same start_date = end_date = meal date
- New endpoints:
  - `GET /api/insights/meal?date=YYYY-MM-DD&meal=breakfast` — fetch stored meal insight
  - `POST /api/insights/meal` — regenerate (for after edits)

**`internal/gemini/gemini.go`** — New method `MealInsight()`:
- Input: current meal entries + previous 3-5 meals + user profile
- System prompt: "Give a 1-2 sentence actionable insight about this meal in the context of recent eating patterns. Reference specific foods. Suggest concrete swaps if improvement is possible. Be direct, no motivational language."
- Returns plain text insight

**`internal/sheets/sheets.go`** — No schema migration needed:
- Meal insights stored in existing Insights sheet using type `"meal:breakfast"` etc.
- `GetInsight("meal:breakfast", date, date)` retrieves it
- `SaveInsight(...)` stores it

### Frontend Changes

**`frontend/src/lib/LogView.svelte`**:
- `[Insights]` button on meal row: fetches/shows per-meal insight inline
- Poll or check for insight after logging (since it's async, may not be ready immediately)
  - Simple approach: fetch on click, show loading spinner, retry once after 2s if not found
- Show insight text below meal header when expanded
- Add regenerate button on insight

**`frontend/src/lib/api.js`** — New functions:
```js
export async function fetchMealInsight(date, meal) { ... }
export async function generateMealInsight(date, meal) { ... }
```

---

## Phase 4: Actionable Insight & Suggestion Prompts

### Backend Changes

**`internal/gemini/gemini.go`** — Update prompt constants:

**`dayInsightsSystemPrompt`**: Update to emphasize:
- Reference specific foods the user ate
- Suggest concrete swaps ("swap X for Y to get +Ng fiber")
- Keep to 2-3 bullets max

**`mealSuggestionsSystemPrompt`**: Update to find middle ground:
- Not just food groups, not full recipes
- Named dishes with key ingredients and why they help
- Example: "**Lentil soup with spinach** — covers your protein gap and adds fiber (~400 cal, 25g protein)"

**`insightsSystemPrompt`** (weekly): Similar actionability improvements

**`weekMealSuggestionsSystemPrompt`**: Similar middle-ground approach

---

## Files to Modify

| File | Changes |
|------|---------|
| `internal/api/api_chat.go` | Auto-save in Chat, new EditChat handler |
| `internal/api/api_insights.go` | Meal insight endpoints, async goroutine |
| `internal/gemini/gemini.go` | EditEntries method, MealInsight method, updated prompts |
| `router.go` | New routes: /chat/edit, /insights/meal |
| `frontend/src/lib/ChatDrawer.svelte` | Edit mode, auto-save flow, multipliers, favorites picker |
| `frontend/src/lib/LogView.svelte` | Meal row UI ([Insights] [Edit]), remove meal calories, remove repeat/scale buttons |
| `frontend/src/lib/api.js` | editChat, fetchMealInsight, generateMealInsight |

---

## Verification

1. **Auto-save**: Open drawer → describe food → entries appear in log immediately (no Save button)
2. **Clarifying question**: Describe ambiguous food → LLM asks question → answer → auto-save
3. **Edit via drawer**: Click [Edit] on a meal → drawer opens with entries → type "remove the toast" → entry removed
4. **Multipliers**: In edit drawer → click ×1.25 → all entries scaled and saved
5. **Inline edit**: In edit drawer → change calorie number → blur → saved
6. **Favorites in drawer**: In edit drawer → search favorites → tap oatmeal → added to meal
7. **Repeat in drawer**: In edit drawer → tap repeat → yesterday's same meal entries added
8. **Meal insight**: Log a meal → click [Insights] → insight appears (may take a moment for async generation)
9. **Re-generate insight**: Edit a meal → insight regenerates
10. **Prompt quality**: Insights reference specific foods and suggest concrete swaps
11. **Run Go tests**: `~/go-sdk/go/bin/go test ./...`
12. **Run frontend build**: `cd frontend && npm run build`
