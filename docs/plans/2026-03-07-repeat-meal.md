# Repeat Meal Implementation Plan

> **For Claude:** REQUIRED SUB-SKILL: Use superpowers:executing-plans to implement this plan task-by-task.

**Goal:** Add a "↻" button to each meal section header that instantly re-logs yesterday's entries for that meal.

**Architecture:** Frontend-only change to `LogView.svelte`. After today's log loads, fire a background fetch for yesterday's log. Store it as `yesterdayByMeal` (meal → entries[]). Each meal header shows a repeat button if yesterday had entries for that meal. On click, call the existing `confirmChat` API with those entries dated to today.

**Tech Stack:** Svelte 5 runes (`$state`, `$effect`), existing `getLog` and `confirmChat` from `frontend/src/lib/api.js`.

---

### Task 1: Add yesterday state + background fetch

**Files:**
- Modify: `frontend/src/lib/LogView.svelte`

The only file changing is `LogView.svelte`. No backend changes, no `api.js` changes.

**Step 1: Add imports and new state variables**

In the `<script>` block, add `confirmChat` to the existing import from `./api.js`:

```js
import { getLog, confirmChat } from './api.js'
```

Add these state variables after the existing ones (after `let drawerPrefill = $state('')`):

```js
let yesterdayByMeal = $state({})
let repeating = $state(null)
```

**Step 2: Add the yesterday date helper**

Add this pure function anywhere in the script (e.g. after `groupedByDate`):

```js
function yesterdayString() {
  const d = new Date()
  d.setDate(d.getDate() - 1)
  return d.toISOString().slice(0, 10)
}
```

**Step 3: Add loadYesterday function**

Add this function after `yesterdayString`:

```js
async function loadYesterday() {
  try {
    const res = await getLog({ date: yesterdayString() })
    const g = {}
    for (const e of res.entries ?? []) {
      ;(g[e.meal_type] ??= []).push(e)
    }
    yesterdayByMeal = g
  } catch {}
}
```

Errors are silently swallowed — if yesterday's fetch fails, `yesterdayByMeal` stays `{}` and no repeat buttons appear.

**Step 4: Call loadYesterday when Today view loads**

Update the existing `$effect` at the bottom of the script block from:

```js
$effect(() => { if (view) load() })
```

to:

```js
$effect(() => {
  if (view) {
    load()
    if (view === 'today') loadYesterday()
  }
})
```

**Step 5: Verify the frontend builds without errors**

```bash
cd /home/imyjer/repos/foodtracker/frontend && npm run build
```

Expected: build succeeds, no errors (warnings about a11y are ok).

**Step 6: Commit**

```bash
git add frontend/src/lib/LogView.svelte
git commit -m "feat: fetch yesterday's log for repeat-meal feature"
```

---

### Task 2: Add repeatMeal function + update meal section headers

**Files:**
- Modify: `frontend/src/lib/LogView.svelte`

**Step 1: Add repeatMeal function**

Add this after `loadYesterday`:

```js
async function repeatMeal(meal) {
  if (repeating !== null) return
  repeating = meal
  try {
    const res = await confirmChat(yesterdayByMeal[meal], data?.date ?? yesterdayString())
    // data.date is today's date string from the API response
    // confirmChat returns { done: true, entries: [...] } with new IDs and today's date
    data = { ...data, entries: [...(data.entries ?? []), ...res.entries] }
  } catch {
    // silent fail — user can tap again
  } finally {
    repeating = null
  }
}
```

Note: `data.date` is today's date string returned by `GET /api/log` (e.g. `"2026-03-07"`). Passing it to `confirmChat` ensures entries get today's date even if it's past midnight.

**Step 2: Update the meal section header markup**

Find the existing Today view section in the template:

```svelte
<section>
  <!-- svelte-ignore a11y_click_events_have_key_events a11y_no_static_element_interactions -->
  <h3 onclick={() => { drawerPrefill = `for ${meal}, I had `; drawerOpen = true }}>{meal}</h3>
```

Replace just the `<h3>` line with a wrapper div containing the h3 and a conditional repeat button:

```svelte
<section>
  <div class="meal-header">
    <!-- svelte-ignore a11y_click_events_have_key_events a11y_no_static_element_interactions -->
    <h3 onclick={() => { drawerPrefill = `for ${meal}, I had `; drawerOpen = true }}>{meal}</h3>
    {#if yesterdayByMeal[meal]?.length}
      <button
        class="repeat-btn"
        class:spinning={repeating === meal}
        onclick={() => repeatMeal(meal)}
        disabled={repeating !== null}
        aria-label="Repeat yesterday's {meal}"
      >↻</button>
    {/if}
  </div>
```

The `disabled={repeating !== null}` prevents tapping a second meal's repeat button while one is already in flight.

**Step 3: Add CSS for .meal-header, .repeat-btn, and @keyframes spin**

The existing `h3` CSS has `margin-bottom: 0.5rem`. Move that to `.meal-header` and remove it from `h3`.

Find in `<style>`:

```css
h3 {
  text-transform: uppercase;
  font-size: 0.68rem;
  color: #888;
  letter-spacing: 0.08em;
  font-weight: 600;
  margin-bottom: 0.5rem;
  cursor: pointer;
  display: inline-block;
  padding: 0.3rem 0;
  touch-action: manipulation;
}
```

Replace with:

```css
h3 {
  text-transform: uppercase;
  font-size: 0.68rem;
  color: #888;
  letter-spacing: 0.08em;
  font-weight: 600;
  cursor: pointer;
  display: inline-block;
  padding: 0.3rem 0;
  touch-action: manipulation;
}
```

Then add these new rules after the `h3:hover` block:

```css
.meal-header {
  display: flex;
  align-items: center;
  gap: 0.5rem;
  margin-bottom: 0.5rem;
}

.repeat-btn {
  background: none;
  border: none;
  color: #ccc;
  font-size: 1rem;
  line-height: 1;
  cursor: pointer;
  padding: 0.2rem 0.3rem;
  touch-action: manipulation;
  display: flex;
  align-items: center;
}

.repeat-btn:hover:not(:disabled) {
  color: #555;
}

.repeat-btn:disabled {
  cursor: default;
}

.repeat-btn.spinning {
  animation: spin 0.7s linear infinite;
  color: #888;
}

@keyframes spin {
  from { transform: rotate(0deg); }
  to { transform: rotate(360deg); }
}
```

**Step 4: Build and verify**

```bash
cd /home/imyjer/repos/foodtracker/frontend && npm run build
```

Expected: build succeeds.

**Step 5: Commit**

```bash
git add frontend/src/lib/LogView.svelte
git commit -m "feat: repeat-meal button on meal section headers"
```

---

### Task 3: Final verification

**Step 1: Run Go tests to confirm nothing broke**

```bash
~/go-sdk/go/bin/go test ./... -v 2>&1 | tail -20
```

Expected: all tests pass.

**Step 2: Build the production binary**

```bash
cd /home/imyjer/repos/foodtracker && mise run build
```

Expected: binary builds successfully.

**Step 3: Manual smoke test checklist**

- Open Today view. If yesterday had breakfast entries, a `↻` appears next to BREAKFAST.
- Meals with no yesterday entries show no repeat button.
- Tapping `↻` shows it spinning briefly, then new entries appear in that meal section.
- Tapping `↻` on one meal disables all other repeat buttons until it completes.
- Switching to History view and back to Today re-fetches yesterday (button appears/disappears correctly).
