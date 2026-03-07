# Feature Improvements Implementation Plan

> **For Claude:** REQUIRED SUB-SKILL: Use superpowers:executing-plans to implement this plan task-by-task.

**Goal:** Add Activity/Feeling split with 1–10 score, chat confirmation before saving, and a redesigned week view with day-detail modal.

**Architecture:** Backend changes first (sheets schema → API handlers → gemini prompt), then frontend (api.js → components). Tasks 1–4 are Go only; Tasks 5–9 are Svelte only. Each task is independently testable/committable.

**Tech Stack:** Go 1.26, Svelte 5 (runes), Google Sheets API, Gemini API. Tests: `mise run test` (Go). Build: `mise run build`. Go binary: `~/go-sdk/go/bin/go`.

---

## Palette reference (do not change)
`#fafaf9` bg · `#1c1c1c` text · `#888` muted · `#2d2d2d` accent · `#e8e8e6` border

---

### Task 1: Sheets — DayLog struct + updated Activity functions

**Files:**
- Modify: `internal/sheets/sheets.go`
- Modify: `internal/sheets/sheets_test.go`

**Step 1: Add `DayLog` struct and helpers after `FoodEntry` in `sheets.go`**

Add after line 56 (after `TimeString`):

```go
// DayLog is one row in the Activity sheet.
// Schema: date | activity | feeling_score | feeling_notes
// Backward compat: old 2-column rows (date | notes) map notes → activity.
type DayLog struct {
	Date         string `json:"date"`
	Activity     string `json:"activity"`
	FeelingScore int    `json:"feeling_score"` // 0 = not set, 1–10
	FeelingNotes string `json:"feeling_notes"`
}

func (d DayLog) ToRow() []interface{} {
	return []interface{}{
		d.Date, d.Activity, strconv.Itoa(d.FeelingScore), d.FeelingNotes,
	}
}

func DayLogFromRow(row []interface{}) DayLog {
	str := func(v interface{}) string { return fmt.Sprintf("%v", v) }
	num := func(v interface{}) int {
		n, _ := strconv.Atoi(fmt.Sprintf("%v", v))
		return n
	}
	d := DayLog{}
	if len(row) >= 1 {
		d.Date = str(row[0])
	}
	if len(row) >= 2 {
		d.Activity = str(row[1])
	}
	if len(row) >= 3 {
		d.FeelingScore = num(row[2])
	}
	if len(row) >= 4 {
		d.FeelingNotes = str(row[3])
	}
	return d
}
```

**Step 2: Update `CreateSpreadsheet` Activity headers** — change the 2-column header to 4 columns:

```go
actHeaders := &googlesheets.ValueRange{
    Values: [][]interface{}{{"date", "activity", "feeling_score", "feeling_notes"}},
}
_, err = sheetsSvc.Spreadsheets.Values.Update(
    created.SpreadsheetId, activitySheet+"!A1:D1", actHeaders,
).ValueInputOption("RAW").Context(ctx).Do()
if err != nil {
    return "", fmt.Errorf("activity headers: %w", err)
}
```

**Step 3: Replace `GetActivity` to return `DayLog`**

Replace the current `GetActivity` function:

```go
// GetActivity returns the DayLog for the given date, or an empty DayLog if none.
func (s *Service) GetActivity(ctx context.Context, date string) (DayLog, error) {
	resp, err := s.svc.Spreadsheets.Values.Get(s.spreadsheetID, activitySheet+"!A:D").Context(ctx).Do()
	if err != nil {
		return DayLog{}, err
	}
	for i, row := range resp.Values {
		if i == 0 || len(row) < 1 {
			continue
		}
		if fmt.Sprintf("%v", row[0]) == date {
			return DayLogFromRow(row), nil
		}
	}
	return DayLog{Date: date}, nil
}
```

**Step 4: Replace `SetActivity` to accept `DayLog`**

Replace the current `SetActivity` function:

```go
// SetActivity upserts the DayLog for its date.
func (s *Service) SetActivity(ctx context.Context, log DayLog) error {
	resp, err := s.svc.Spreadsheets.Values.Get(s.spreadsheetID, activitySheet+"!A:A").Context(ctx).Do()
	if err != nil {
		return err
	}
	vr := &googlesheets.ValueRange{Values: [][]interface{}{log.ToRow()}}
	rowNum := -1
	for i, row := range resp.Values {
		if i == 0 {
			continue
		}
		if len(row) > 0 && fmt.Sprintf("%v", row[0]) == log.Date {
			rowNum = i + 1
			break
		}
	}
	if rowNum < 0 {
		_, err = s.svc.Spreadsheets.Values.Append(
			s.spreadsheetID, activitySheet+"!A:D", vr,
		).ValueInputOption("RAW").Context(ctx).Do()
	} else {
		_, err = s.svc.Spreadsheets.Values.Update(
			s.spreadsheetID,
			fmt.Sprintf("%s!A%d:D%d", activitySheet, rowNum, rowNum),
			vr,
		).ValueInputOption("RAW").Context(ctx).Do()
	}
	return err
}
```

**Step 5: Add `GetActivityByDateRange` for the week endpoint**

Add after `SetActivity`:

```go
// GetActivityByDateRange returns DayLogs where start <= date <= end.
func (s *Service) GetActivityByDateRange(ctx context.Context, start, end string) ([]DayLog, error) {
	resp, err := s.svc.Spreadsheets.Values.Get(s.spreadsheetID, activitySheet+"!A:D").Context(ctx).Do()
	if err != nil {
		return nil, err
	}
	var out []DayLog
	for i, row := range resp.Values {
		if i == 0 || len(row) < 1 {
			continue
		}
		d := fmt.Sprintf("%v", row[0])
		if d >= start && d <= end {
			out = append(out, DayLogFromRow(row))
		}
	}
	return out, nil
}
```

**Step 6: Write tests in `sheets_test.go`**

Add to `sheets_test.go`:

```go
func TestDayLogFromRow_Full(t *testing.T) {
	row := []interface{}{"2026-03-06", "ran 5k", "7", "felt good"}
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
	// Old rows had: date | notes — notes should map to Activity, score=0, notes=""
	row := []interface{}{"2026-03-06", "old activity notes"}
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
	if len(row) != 4 {
		t.Fatalf("want 4 cols, got %d", len(row))
	}
	if row[0] != "2026-03-06" {
		t.Errorf("col 0: got %v", row[0])
	}
	if row[2] != "8" {
		t.Errorf("col 2 (feeling_score): got %v", row[2])
	}
}
```

**Step 7: Run tests**

Run: `mise run test`
Expected: all pass, no compile errors

**Step 8: Commit**

```bash
git add internal/sheets/sheets.go internal/sheets/sheets_test.go
git commit -m "feat: DayLog struct with activity + feeling split in sheets"
```

---

### Task 2: API — Update activity handlers + week endpoint

**Files:**
- Modify: `internal/api/api.go`

**Step 1: Update `GetActivity` handler**

Replace the current `GetActivity` handler:

```go
// GET /api/activity?date=YYYY-MM-DD
func (h *Handler) GetActivity(w http.ResponseWriter, r *http.Request) {
	session := auth.SessionFromContext(r.Context())
	svc, err := h.sheetsSvc(r, session)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	date := r.URL.Query().Get("date")
	if date == "" {
		date = sheets.DateString(time.Now())
	}
	log, err := svc.GetActivity(r.Context(), date)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	WriteJSON(w, http.StatusOK, log)
}
```

**Step 2: Update `PutActivity` handler**

Replace the current `PutActivity` handler:

```go
// PUT /api/activity — body: DayLog JSON
func (h *Handler) PutActivity(w http.ResponseWriter, r *http.Request) {
	session := auth.SessionFromContext(r.Context())
	var req sheets.DayLog
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeErr(w, http.StatusBadRequest, "invalid body")
		return
	}
	if req.Date == "" {
		req.Date = sheets.DateString(time.Now())
	}
	svc, err := h.sheetsSvc(r, session)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	if err := svc.SetActivity(r.Context(), req); err != nil {
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	WriteJSON(w, http.StatusOK, req)
}
```

**Step 3: Update `GetLog` — today path returns `day_log`, week path adds `daily_logs`**

In the week branch, after fetching `entries`, add:

```go
dailyLogs, err := svc.GetActivityByDateRange(r.Context(), start, today)
if err != nil {
    writeErr(w, http.StatusInternalServerError, err.Error())
    return
}
WriteJSON(w, http.StatusOK, map[string]any{
    "entries":    entries,
    "daily_logs": dailyLogs,
    "start":      start,
    "end":        today,
})
```

In the today path, replace `"activity": activity` with `"day_log"` field:

```go
dayLog, _ := svc.GetActivity(r.Context(), date)
WriteJSON(w, http.StatusOK, map[string]any{
    "entries": entries,
    "day_log": dayLog,
    "date":    date,
})
```

**Step 4: Build to check for compile errors**

Run: `mise run build`
Expected: exits 0

**Step 5: Commit**

```bash
git add internal/api/api.go
git commit -m "feat: activity API returns DayLog, week endpoint includes daily_logs"
```

---

### Task 3: Gemini — system prompt + remove auto-clear on pending

**Files:**
- Modify: `internal/gemini/gemini.go`

**Step 1: Replace the `systemPrompt` constant**

```go
const systemPrompt = `You are a food tracking assistant. The user describes what they ate.

Your job:
1. Extract food items and estimate macros (calories, protein, carbs, fat in grams).
2. If quantities are ambiguous, ask ONE short clarifying question — nothing more.
3. Once you have enough information, show a friendly human-readable summary:

   Here's what I'm logging:
   • [description] ([meal_type]) — [calories] cal, [protein]g P, [carbs]g C, [fat]g F

   Does this look right?

   Then include the JSON in a code block:
   ` + "```json" + `
   {"entries":[{"meal_type":"breakfast","description":"oatmeal with milk","calories":300,"protein":8,"carbs":54,"fat":6}]}
   ` + "```" + `

4. If the user says yes / ok / looks good / save it / confirm, repeat the JSON code block exactly so it can be processed.

Rules:
- meal_type must be one of: breakfast, snack, lunch, dinner
- All numeric values are integers (round estimates are fine)
- Multiple foods in one meal → multiple entries, same meal_type
- Use reasonable common serving sizes for estimates`
```

**Step 2: Update `Chat` — don't clear history when entries detected**

In the `Chat` method, replace the section that currently clears history on JSON detection:

```go
entries, ok := ParseEntries(responseText)

// Always persist conversation history — clearing happens in ClearConversation
// when the user confirms via /api/chat/confirm
s.mu.Lock()
s.convs[userEmail] = chatSession.History
s.mu.Unlock()

if ok {
    return responseText, entries, nil
}
return responseText, nil, nil
```

**Step 3: Run tests**

Run: `mise run test`
Expected: all pass (gemini_test.go tests ParseEntries only, unaffected)

**Step 4: Commit**

```bash
git add internal/gemini/gemini.go
git commit -m "feat: gemini shows confirmation summary before returning entries"
```

---

### Task 4: API — Chat handler returns pending, add ConfirmChat endpoint

**Files:**
- Modify: `internal/api/api.go`
- Modify: `main.go`

**Step 1: Update `Chat` handler — return pending instead of saving**

Replace the section after `h.gemini.Chat` call that saves entries:

Current code to replace:
```go
// Clarifying question — nothing to write to Sheets yet
if len(entries) == 0 {
    WriteJSON(w, http.StatusOK, map[string]any{"done": false, "message": responseText})
    return
}

svc, err := h.sheetsSvc(r, session)
...
// (save loop)
WriteJSON(w, http.StatusOK, map[string]any{"done": true, "entries": saved})
```

New code:
```go
if len(entries) == 0 {
    WriteJSON(w, http.StatusOK, map[string]any{"done": false, "message": responseText})
    return
}

// Entries detected — return pending for user confirmation, do not save yet
WriteJSON(w, http.StatusOK, map[string]any{
    "done":    false,
    "pending": true,
    "entries": entries,
    "message": responseText,
})
```

**Step 2: Add `ConfirmChat` handler**

Add after the `Chat` handler:

```go
// POST /api/chat/confirm — body: {"entries": [...]}
// Saves confirmed entries returned from a pending chat response.
func (h *Handler) ConfirmChat(w http.ResponseWriter, r *http.Request) {
	session := auth.SessionFromContext(r.Context())
	if !h.ensureSpreadsheet(w, r, session) {
		return
	}

	var req struct {
		Entries []sheets.FoodEntry `json:"entries"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil || len(req.Entries) == 0 {
		writeErr(w, http.StatusBadRequest, "invalid request body")
		return
	}

	svc, err := h.sheetsSvc(r, session)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}

	now := time.Now()
	var saved []sheets.FoodEntry
	for _, e := range req.Entries {
		fe := sheets.FoodEntry{
			ID:          uuid.NewString(),
			Date:        sheets.DateString(now),
			Time:        sheets.TimeString(now),
			MealType:    e.MealType,
			Description: e.Description,
			Calories:    e.Calories,
			Protein:     e.Protein,
			Carbs:       e.Carbs,
			Fat:         e.Fat,
		}
		if err := svc.AppendFood(r.Context(), fe); err != nil {
			writeErr(w, http.StatusInternalServerError, fmt.Sprintf("sheet write: %v", err))
			return
		}
		saved = append(saved, fe)
	}

	h.gemini.ClearConversation(session.UserEmail)
	WriteJSON(w, http.StatusOK, map[string]any{"done": true, "entries": saved})
}
```

**Step 3: Register route in `main.go`**

Add after the existing `POST /api/chat` route:

```go
mux.HandleFunc("POST /api/chat/confirm", apiHandler.Authenticated(apiHandler.ConfirmChat))
```

**Step 4: Build**

Run: `mise run build`
Expected: exits 0

**Step 5: Commit**

```bash
git add internal/api/api.go main.go
git commit -m "feat: chat returns pending state, add /api/chat/confirm endpoint"
```

---

### Task 5: Frontend — api.js updates

**Files:**
- Modify: `frontend/src/lib/api.js`

**Step 1: Update `getActivity` and `putActivity`, add `confirmChat`**

Replace the entire contents of `frontend/src/lib/api.js`:

```js
export async function getLog({ date = null, week = false } = {}) {
  const params = week ? '?week=true' : date ? `?date=${date}` : ''
  const res = await fetch(`/api/log${params}`)
  if (!res.ok) throw new Error(await res.text())
  return res.json()
}

export async function chat(message) {
  const res = await fetch('/api/chat', {
    method: 'POST',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify({ message }),
  })
  if (!res.ok) throw new Error(await res.text())
  return res.json()
}

export async function confirmChat(entries) {
  const res = await fetch('/api/chat/confirm', {
    method: 'POST',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify({ entries }),
  })
  if (!res.ok) throw new Error(await res.text())
  return res.json()
}

export async function patchEntry(id, entry) {
  const res = await fetch(`/api/entries/${id}`, {
    method: 'PATCH',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify(entry),
  })
  if (!res.ok) throw new Error(await res.text())
  return res.json()
}

export async function getActivity(date) {
  const res = await fetch(`/api/activity?date=${date}`)
  if (!res.ok) throw new Error(await res.text())
  return res.json() // returns {date, activity, feeling_score, feeling_notes}
}

export async function putActivity(date, { activity, feeling_score, feeling_notes }) {
  const res = await fetch('/api/activity', {
    method: 'PUT',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify({ date, activity, feeling_score, feeling_notes }),
  })
  if (!res.ok) throw new Error(await res.text())
  return res.json()
}
```

**Step 2: Build**

Run: `mise run build`
Expected: exits 0

**Step 3: Commit**

```bash
git add frontend/src/lib/api.js
git commit -m "feat: api.js — confirmChat, updated activity with feeling fields"
```

---

### Task 6: Frontend — ActivityNote.svelte redesign

**Files:**
- Modify: `frontend/src/lib/ActivityNote.svelte`

**Step 1: Replace entire file contents**

```svelte
<script>
  import { onMount } from 'svelte'
  import { getActivity, putActivity } from './api.js'

  let { date } = $props()

  let activity = $state('')
  let feelingScore = $state(0)
  let feelingNotes = $state('')
  let editingActivity = $state(false)
  let editingFeeling = $state(false)
  let saving = $state(false)
  let saveError = $state('')

  onMount(async () => {
    if (!date) return
    try {
      const res = await getActivity(date)
      activity = res.activity ?? ''
      feelingScore = res.feeling_score ?? 0
      feelingNotes = res.feeling_notes ?? ''
    } catch {}
  })

  async function save() {
    saving = true
    saveError = ''
    try {
      await putActivity(date, { activity, feeling_score: feelingScore, feeling_notes: feelingNotes })
      editingActivity = false
      editingFeeling = false
    } catch {
      saveError = 'Failed to save. Try again.'
    } finally {
      saving = false
    }
  }

  function onKeyDown(e) {
    if (e.key === 'Enter' && (e.metaKey || e.ctrlKey)) save()
    if (e.key === 'Escape') { editingActivity = false; editingFeeling = false }
  }
</script>

<div class="day-notes">
  <div class="section">
    <h3>Activity</h3>
    {#if editingActivity}
      <textarea
        bind:value={activity}
        onblur={save}
        onkeydown={onKeyDown}
        placeholder="Exercise, stress, unusual events…"
        rows="2"
        autofocus
      ></textarea>
    {:else}
      <!-- svelte-ignore a11y_click_events_have_key_events a11y_no_static_element_interactions -->
      <div class="note" class:placeholder={!activity} onclick={() => editingActivity = true}>
        {activity || 'Tap to add activity…'}
      </div>
    {/if}
  </div>

  <div class="section">
    <h3>Feeling</h3>
    {#if editingFeeling}
      <div class="feeling-edit">
        <div class="score-row">
          <span class="score-label">Score (1–10)</span>
          <input
            type="number"
            min="1"
            max="10"
            bind:value={feelingScore}
            onblur={save}
          />
        </div>
        <textarea
          bind:value={feelingNotes}
          onblur={save}
          onkeydown={onKeyDown}
          placeholder="Energy, digestion, mood, sleep…"
          rows="2"
        ></textarea>
      </div>
    {:else}
      <!-- svelte-ignore a11y_click_events_have_key_events a11y_no_static_element_interactions -->
      <div class="note" class:placeholder={!feelingScore && !feelingNotes} onclick={() => editingFeeling = true}>
        {#if feelingScore}
          <span class="score">{feelingScore}/10</span>{#if feelingNotes} — {feelingNotes}{/if}
        {:else}
          {feelingNotes || 'Tap to add how you were feeling…'}
        {/if}
      </div>
    {/if}
  </div>

  {#if saving}<p class="hint">Saving…</p>{/if}
  {#if saveError}<p class="hint error">{saveError}</p>{/if}
</div>

<style>
  .day-notes {
    margin-top: 2rem;
    border-top: 1px solid #e8e8e6;
    padding-top: 1.25rem;
    display: flex;
    flex-direction: column;
    gap: 1.25rem;
  }

  .section h3 {
    text-transform: uppercase;
    font-size: 0.68rem;
    color: #888;
    letter-spacing: 0.08em;
    font-weight: 600;
    margin-bottom: 0.4rem;
  }

  .note {
    color: #1c1c1c;
    font-size: 0.9rem;
    cursor: pointer;
    line-height: 1.55;
    min-height: 1.55rem;
    padding: 0.2rem 0;
  }

  .note.placeholder {
    color: #bbb;
  }

  .score {
    font-weight: 500;
  }

  .feeling-edit {
    display: flex;
    flex-direction: column;
    gap: 0.5rem;
  }

  .score-row {
    display: flex;
    align-items: center;
    gap: 0.75rem;
  }

  .score-label {
    font-size: 0.82rem;
    color: #888;
  }

  .score-row input {
    width: 56px;
    border: none;
    border-bottom: 2px solid #2d2d2d;
    padding: 2px 4px;
    font-family: inherit;
    font-size: 0.95rem;
    background: transparent;
    outline: none;
  }

  textarea {
    width: 100%;
    border: none;
    border-bottom: 2px solid #2d2d2d;
    border-radius: 0;
    padding: 0.4rem 0;
    font-size: 0.9rem;
    font-family: inherit;
    resize: vertical;
    box-sizing: border-box;
    background: transparent;
    color: #1c1c1c;
  }

  textarea:focus {
    outline: none;
  }

  .hint {
    font-size: 0.75rem;
    color: #aaa;
    margin-top: 0.25rem;
  }

  .hint.error {
    color: #b91c1c;
  }
</style>
```

**Step 2: Build**

Run: `mise run build`
Expected: exits 0

**Step 3: Commit**

```bash
git add frontend/src/lib/ActivityNote.svelte
git commit -m "feat: ActivityNote split into Activity + Feeling (1-10 score)"
```

---

### Task 7: Frontend — ChatDrawer.svelte pending state + confirm button

**Files:**
- Modify: `frontend/src/lib/ChatDrawer.svelte`

**Step 1: Replace the `<script>` block**

```svelte
<script>
  import { chat, confirmChat } from './api.js'

  let { open, onClose, onEntriesAdded } = $props()

  let messages = $state([])
  let input = $state('')
  let sending = $state(false)
  let pendingEntries = $state(null)
  let inputEl = $state(null)
  let messagesEl = $state(null)

  $effect(() => {
    if (open) {
      setTimeout(() => inputEl?.focus(), 60)
    } else {
      messages = []
      input = ''
      pendingEntries = null
    }
  })

  $effect(() => {
    messages
    if (messagesEl) messagesEl.scrollTop = messagesEl.scrollHeight
  })

  async function send() {
    const text = input.trim()
    if (!text || sending) return
    messages = [...messages, { role: 'user', text }]
    input = ''
    pendingEntries = null
    sending = true
    try {
      const res = await chat(text)
      if (res.pending) {
        pendingEntries = res.entries
        messages = [...messages, { role: 'assistant', text: res.message }]
      } else if (res.done) {
        onEntriesAdded(res.entries)
      } else {
        messages = [...messages, { role: 'assistant', text: res.message }]
      }
    } catch {
      messages = [...messages, { role: 'assistant', text: 'Something went wrong. Please try again.' }]
    } finally {
      sending = false
    }
  }

  async function confirm() {
    if (!pendingEntries || sending) return
    sending = true
    try {
      const res = await confirmChat(pendingEntries)
      onEntriesAdded(res.entries)
    } catch {
      messages = [...messages, { role: 'assistant', text: 'Failed to save. Please try again.' }]
    } finally {
      sending = false
      pendingEntries = null
    }
  }

  function onKeyDown(e) {
    if (e.key === 'Enter' && !e.shiftKey) {
      e.preventDefault()
      send()
    }
  }
</script>
```

**Step 2: Update the template — add confirm button row after `input-row`**

The template after the `messages` div should be:

```svelte
    <div class="input-row">
      <textarea
        bind:this={inputEl}
        bind:value={input}
        onkeydown={onKeyDown}
        placeholder="What did you eat?"
        rows="2"
        disabled={sending}
      ></textarea>
      <button onclick={send} disabled={sending || !input.trim()}>Send</button>
    </div>
    {#if pendingEntries}
      <button class="confirm-btn" onclick={confirm} disabled={sending}>
        Looks good, save it
      </button>
    {/if}
```

**Step 3: Add `.confirm-btn` to the `<style>` block**

Add inside the existing `<style>` block:

```css
  .confirm-btn {
    width: 100%;
    margin-top: 0.5rem;
    padding: 0.6rem 1rem;
    background: #fafaf9;
    color: #2d2d2d;
    border: 1px solid #2d2d2d;
    border-radius: 8px;
    cursor: pointer;
    font-size: 0.9rem;
    font-family: inherit;
    font-weight: 500;
  }

  .confirm-btn:hover {
    background: #2d2d2d;
    color: #fafaf9;
  }

  .confirm-btn:disabled {
    opacity: 0.35;
    cursor: default;
  }
```

**Step 4: Build**

Run: `mise run build`
Expected: exits 0

**Step 5: Commit**

```bash
git add frontend/src/lib/ChatDrawer.svelte
git commit -m "feat: chat drawer pending state with confirm button"
```

---

### Task 8: Frontend — DayModal.svelte (new component)

**Files:**
- Create: `frontend/src/lib/DayModal.svelte`

**Step 1: Create the file**

```svelte
<script>
  let { day, onClose } = $props()

  const MEAL_ORDER = ['breakfast', 'snack', 'lunch', 'dinner']

  function groupedByMeal(entries) {
    const g = {}
    for (const e of entries ?? []) { (g[e.meal_type] ??= []).push(e) }
    return g
  }

  function totals(entries) {
    return (entries ?? []).reduce(
      (a, e) => ({ calories: a.calories + e.calories, protein: a.protein + e.protein, carbs: a.carbs + e.carbs, fat: a.fat + e.fat }),
      { calories: 0, protein: 0, carbs: 0, fat: 0 }
    )
  }
</script>

<!-- svelte-ignore a11y_click_events_have_key_events a11y_no_static_element_interactions -->
<div class="overlay" onclick={onClose}></div>
<div class="modal" role="dialog" aria-label={day.date}>
  <div class="modal-header">
    <h2>{day.date}</h2>
    <button class="close" onclick={onClose}>✕</button>
  </div>

  {#if day.dayLog?.feeling_score || day.dayLog?.feeling_notes}
    <div class="modal-section">
      <h3>Feeling</h3>
      <p>
        {#if day.dayLog.feeling_score}<span class="score">{day.dayLog.feeling_score}/10</span>{/if}
        {#if day.dayLog.feeling_score && day.dayLog.feeling_notes} — {/if}
        {#if day.dayLog.feeling_notes}{day.dayLog.feeling_notes}{/if}
      </p>
    </div>
  {/if}

  {#if day.dayLog?.activity}
    <div class="modal-section">
      <h3>Activity</h3>
      <p>{day.dayLog.activity}</p>
    </div>
  {/if}

  <div class="modal-section">
    <h3>Food</h3>
    {#each MEAL_ORDER as meal}
      {@const group = groupedByMeal(day.entries)[meal] ?? []}
      {#if group.length > 0}
        <div class="meal-group">
          <h4>{meal}</h4>
          {#each group as entry}
            <div class="entry-row">
              <span class="entry-desc">{entry.description}</span>
              <span class="entry-macros">{entry.calories} cal · {entry.protein}g P</span>
            </div>
          {/each}
        </div>
      {/if}
    {/each}
    {@const t = totals(day.entries)}
    <div class="day-totals">{t.calories} cal · {t.protein}g P · {t.carbs}g C · {t.fat}g F</div>
  </div>
</div>

<style>
  .overlay {
    position: fixed;
    inset: 0;
    background: rgba(0,0,0,0.25);
    z-index: 20;
  }

  .modal {
    position: fixed;
    top: 50%;
    left: 50%;
    transform: translate(-50%, -50%);
    background: #fff;
    border-radius: 12px;
    width: min(92vw, 520px);
    max-height: 80vh;
    overflow-y: auto;
    z-index: 21;
    padding: 1.5rem;
    box-shadow: 0 4px 24px rgba(0,0,0,0.12);
  }

  .modal-header {
    display: flex;
    justify-content: space-between;
    align-items: center;
    margin-bottom: 1.25rem;
  }

  .modal-header h2 {
    font-size: 1.05rem;
    font-weight: 600;
    color: #1c1c1c;
  }

  .close {
    background: none;
    border: none;
    font-size: 1rem;
    color: #888;
    cursor: pointer;
    padding: 0.25rem;
    line-height: 1;
  }

  .modal-section {
    margin-bottom: 1.25rem;
    padding-bottom: 1.25rem;
    border-bottom: 1px solid #e8e8e6;
  }

  .modal-section:last-child {
    border-bottom: none;
    margin-bottom: 0;
    padding-bottom: 0;
  }

  h3 {
    text-transform: uppercase;
    font-size: 0.68rem;
    color: #888;
    letter-spacing: 0.08em;
    font-weight: 600;
    margin-bottom: 0.4rem;
  }

  h4 {
    text-transform: capitalize;
    font-size: 0.75rem;
    color: #888;
    letter-spacing: 0.04em;
    margin: 0.75rem 0 0.3rem;
  }

  h4:first-child {
    margin-top: 0;
  }

  p {
    font-size: 0.9rem;
    color: #1c1c1c;
    line-height: 1.5;
  }

  .score {
    font-weight: 500;
  }

  .entry-row {
    display: flex;
    justify-content: space-between;
    padding: 0.3rem 0;
    font-size: 0.88rem;
    border-bottom: 1px solid #f3f3f2;
  }

  .entry-desc {
    color: #1c1c1c;
  }

  .entry-macros {
    color: #888;
    flex-shrink: 0;
    margin-left: 1rem;
  }

  .day-totals {
    margin-top: 0.75rem;
    font-size: 0.8rem;
    color: #888;
    font-weight: 500;
  }
</style>
```

**Step 2: Build**

Run: `mise run build`
Expected: exits 0

**Step 3: Commit**

```bash
git add frontend/src/lib/DayModal.svelte
git commit -m "feat: DayModal component for week view day detail"
```

---

### Task 9: Frontend — LogView.svelte week view update

**Files:**
- Modify: `frontend/src/lib/LogView.svelte`

**Step 1: Import DayModal**

Add to the `<script>` imports:

```js
import DayModal from './DayModal.svelte'
```

**Step 2: Add `selectedDay` state**

Add to the reactive state declarations:

```js
let selectedDay = $state(null)
```

**Step 3: Update the week view markup**

Replace the current week view block:

```svelte
{#else}
  {#each Object.entries(groupedByDate(data?.entries ?? [])).sort() as [date, entries]}
    {@const t = totals(entries)}
    <div class="week-row">
      <span class="date">{date}</span>
      <span>{t.calories} cal</span>
      <span>{t.protein}g P</span>
      <span>{t.carbs}g C</span>
      <span>{t.fat}g F</span>
    </div>
  {/each}
{/if}
```

With:

```svelte
{:else}
  {#each Object.entries(groupedByDate(data?.entries ?? [])).sort() as [date, entries]}
    {@const dayLog = (data?.daily_logs ?? []).find(d => d.date === date) ?? null}
    <!-- svelte-ignore a11y_click_events_have_key_events a11y_no_static_element_interactions -->
    <div class="week-row" onclick={() => selectedDay = { date, entries, dayLog }}>
      <span class="date">{date}</span>
      <span class="feeling-score">{dayLog?.feeling_score ? `${dayLog.feeling_score}/10` : '—'}</span>
      <span class="activity-tick">{dayLog?.activity ? '✓' : ''}</span>
      <span class="chevron">›</span>
    </div>
  {/each}
  {#if selectedDay}
    <DayModal day={selectedDay} onClose={() => selectedDay = null} />
  {/if}
{/if}
```

**Step 4: Update `.week-row` styles in the `<style>` block**

Replace the existing `.week-row`, `.date`, `.week-row span:not(.date)` rules with:

```css
  .week-row {
    display: flex;
    justify-content: space-between;
    align-items: center;
    padding: 0.75rem 0;
    border-bottom: 1px solid #e8e8e6;
    cursor: pointer;
    gap: 1rem;
  }

  .week-row:hover {
    background: #faf9f8;
  }

  .date {
    font-weight: 500;
    font-size: 0.88rem;
    flex: 1;
  }

  .feeling-score {
    font-size: 0.88rem;
    color: #888;
    min-width: 3rem;
    text-align: right;
  }

  .activity-tick {
    font-size: 0.82rem;
    color: #2d2d2d;
    min-width: 1rem;
    text-align: center;
  }

  .chevron {
    color: #ccc;
    font-size: 1.1rem;
    line-height: 1;
  }
```

**Step 5: Build**

Run: `mise run build`
Expected: exits 0

**Step 6: Commit**

```bash
git add frontend/src/lib/LogView.svelte
git commit -m "feat: week view shows feeling score + activity tick, click for day modal"
```

---

### Task 10: Final verification

**Step 1: Run all Go tests**

Run: `mise run test`
Expected: all pass

**Step 2: Full build**

Run: `mise run build`
Expected: exits 0, JS bundle under 65KB

**Step 3: Smoke-test locally**

Run: `mise run run` and verify:
- Today view: Activity section and Feeling section appear separately below food
- Feeling section shows 1–10 number input when editing
- Log a meal: Gemini shows human-readable summary + "Does this look right?"
- "Looks good, save it" button appears; clicking it saves entries
- Type a correction: pending clears, conversation continues
- Week tab: rows show `N/10`, `✓` if activity logged, click opens modal
- Modal shows food grouped by meal + feeling + activity + close button

**Step 4: Commit docs**

```bash
git add docs/
git commit -m "docs: feature improvements plan committed"
```
