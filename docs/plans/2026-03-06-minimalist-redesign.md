# Minimalist Redesign Implementation Plan

> **For Claude:** REQUIRED SUB-SKILL: Use superpowers:executing-plans to implement this plan task-by-task.

**Goal:** Replace the Vite dark-mode default styles with a clean, editorial, light-only UI using a warm neutral palette.

**Architecture:** Pure CSS changes to existing Svelte components — no logic, no structure, no Svelte changes. Each task touches one file's `<style>` block (or `app.css`). No new files needed.

**Tech Stack:** Svelte 5, plain CSS (scoped component styles + global `app.css`), `mise run build` to verify.

---

### Palette Reference (use throughout)

| Token      | Value     |
|------------|-----------|
| bg         | `#fafaf9` |
| text       | `#1c1c1c` |
| muted      | `#888`    |
| accent     | `#2d2d2d` |
| border     | `#e8e8e6` |

---

### Task 1: Rewrite app.css

**Files:**
- Modify: `frontend/src/app.css`

**Step 1: Replace entire contents of `frontend/src/app.css`**

```css
*, *::before, *::after { box-sizing: border-box; margin: 0; padding: 0; }

body {
  font-family: -apple-system, BlinkMacSystemFont, 'Segoe UI', sans-serif;
  background: #fafaf9;
  color: #1c1c1c;
  -webkit-font-smoothing: antialiased;
}
```

**Step 2: Verify build succeeds**

Run: `mise run build`
Expected: exits 0, no CSS errors

**Step 3: Commit**

```bash
git add frontend/src/app.css
git commit -m "style: replace vite default css with clean light base"
```

---

### Task 2: Login and loading screen (App.svelte)

**Files:**
- Modify: `frontend/src/App.svelte` — `<style>` block only

**Step 1: Replace the `<style>` block**

```svelte
<style>
  .center {
    display: flex;
    align-items: center;
    justify-content: center;
    height: 100vh;
    color: #888;
    font-size: 0.9rem;
  }
  .login {
    display: flex;
    flex-direction: column;
    align-items: center;
    justify-content: center;
    height: 100vh;
    gap: 2rem;
  }
  .login h1 {
    font-size: 1.4rem;
    font-weight: 500;
    color: #1c1c1c;
    letter-spacing: -0.01em;
  }
  .btn {
    padding: 0.6rem 1.25rem;
    border: 1px solid #2d2d2d;
    color: #2d2d2d;
    border-radius: 6px;
    text-decoration: none;
    font-size: 0.9rem;
    letter-spacing: 0.01em;
  }
  .btn:hover {
    background: #2d2d2d;
    color: #fafaf9;
  }
</style>
```

**Step 2: Verify build**

Run: `mise run build`
Expected: exits 0

**Step 3: Commit**

```bash
git add frontend/src/App.svelte
git commit -m "style: clean login and loading screen"
```

---

### Task 3: Main layout and header (LogView.svelte)

**Files:**
- Modify: `frontend/src/lib/LogView.svelte` — `<style>` block only

**Step 1: Replace the `<style>` block**

```svelte
<style>
  .wrap {
    max-width: 640px;
    margin: 0 auto;
    padding: 0 1.25rem 6rem;
  }

  header {
    position: sticky;
    top: 0;
    background: #fafaf9;
    padding: 1rem 0 0.75rem;
    border-bottom: 1px solid #e8e8e6;
    margin-bottom: 1.25rem;
  }

  .header-top {
    display: flex;
    justify-content: space-between;
    align-items: baseline;
    margin-bottom: 0.5rem;
  }

  .toggle {
    display: flex;
    gap: 1.25rem;
  }

  .toggle button {
    background: none;
    border: none;
    border-bottom: 2px solid transparent;
    padding: 0 0 0.2rem;
    font-size: 0.95rem;
    font-weight: 500;
    color: #888;
    cursor: pointer;
    font-family: inherit;
  }

  .toggle button.active {
    color: #1c1c1c;
    border-bottom-color: #2d2d2d;
  }

  .totals {
    display: flex;
    gap: 1rem;
    font-size: 0.78rem;
    color: #888;
    padding-bottom: 0.1rem;
  }

  section {
    margin: 1.5rem 0;
  }

  h3 {
    text-transform: uppercase;
    font-size: 0.68rem;
    color: #888;
    letter-spacing: 0.08em;
    font-weight: 600;
    margin-bottom: 0.5rem;
  }

  .empty {
    color: #bbb;
    font-size: 0.85rem;
    padding: 0.3rem 0;
  }

  .state {
    color: #aaa;
    text-align: center;
    margin-top: 4rem;
    font-size: 0.9rem;
  }

  .week-row {
    display: flex;
    justify-content: space-between;
    padding: 0.65rem 0;
    border-bottom: 1px solid #e8e8e6;
    font-size: 0.88rem;
    color: #1c1c1c;
  }

  .date {
    font-weight: 500;
  }

  .week-row span:not(.date) {
    color: #888;
  }

  .fab {
    position: fixed;
    bottom: 2rem;
    right: 2rem;
    width: 3.25rem;
    height: 3.25rem;
    border-radius: 50%;
    background: #2d2d2d;
    color: #fafaf9;
    font-size: 1.75rem;
    border: none;
    cursor: pointer;
    box-shadow: 0 2px 8px rgba(0,0,0,0.18);
    display: flex;
    align-items: center;
    justify-content: center;
    line-height: 1;
  }

  .fab:hover {
    background: #1c1c1c;
  }
</style>
```

**Step 2: Update the `<header>` markup to add `header-top` wrapper**

The current header markup is:
```svelte
<header>
  <div class="toggle">
    ...
  </div>
  {#if data?.entries}
    {@const t = totals(data.entries)}
    <div class="totals">
      ...
    </div>
  {/if}
</header>
```

Replace with:
```svelte
<header>
  <div class="header-top">
    <div class="toggle">
      <button class:active={view === 'today'} onclick={() => view = 'today'}>Today</button>
      <button class:active={view === 'week'} onclick={() => view = 'week'}>Week</button>
    </div>
  </div>
  {#if data?.entries}
    {@const t = totals(data.entries)}
    <div class="totals">
      <span>{t.calories} cal</span>
      <span>{t.protein}g P</span>
      <span>{t.carbs}g C</span>
      <span>{t.fat}g F</span>
    </div>
  {/if}
</header>
```

**Step 3: Verify build**

Run: `mise run build`
Expected: exits 0

**Step 4: Commit**

```bash
git add frontend/src/lib/LogView.svelte
git commit -m "style: minimalist header, sections, and week view"
```

---

### Task 4: Entry rows (EntryRow.svelte)

**Files:**
- Modify: `frontend/src/lib/EntryRow.svelte` — `<style>` block only

**Step 1: Replace the `<style>` block**

```svelte
<style>
  .row {
    display: flex;
    justify-content: space-between;
    align-items: center;
    padding: 0.65rem 0;
    border-bottom: 1px solid #e8e8e6;
    gap: 1rem;
  }

  .desc {
    flex: 1;
    min-width: 0;
    font-size: 0.95rem;
    line-height: 1.4;
  }

  .editable {
    cursor: pointer;
  }

  .editable:hover {
    color: #555;
  }

  .macros {
    display: flex;
    gap: 0.75rem;
    font-size: 0.8rem;
    color: #888;
    flex-shrink: 0;
  }

  .macro {
    cursor: pointer;
  }

  .macro:hover {
    color: #555;
  }

  input {
    border: 1px solid #e8e8e6;
    border-bottom: 2px solid #2d2d2d;
    border-radius: 0;
    padding: 2px 4px;
    font-family: inherit;
    font-size: inherit;
    background: transparent;
    outline: none;
  }

  .num-input {
    width: 56px;
  }
</style>
```

**Step 2: Verify build**

Run: `mise run build`
Expected: exits 0

**Step 3: Commit**

```bash
git add frontend/src/lib/EntryRow.svelte
git commit -m "style: clean entry rows with editorial typography"
```

---

### Task 5: Chat drawer (ChatDrawer.svelte)

**Files:**
- Modify: `frontend/src/lib/ChatDrawer.svelte` — `<style>` block only

**Step 1: Replace the `<style>` block**

```svelte
<style>
  .overlay {
    position: fixed;
    inset: 0;
    background: rgba(0,0,0,0.2);
    z-index: 10;
  }

  .drawer {
    position: fixed;
    bottom: 0;
    left: 0;
    right: 0;
    background: #fff;
    border-radius: 16px 16px 0 0;
    box-shadow: 0 -2px 16px rgba(0,0,0,0.08);
    z-index: 11;
    display: flex;
    flex-direction: column;
    max-height: 65vh;
    padding: 0.75rem 1.25rem 1.5rem;
  }

  .handle {
    width: 36px;
    height: 3px;
    background: #e8e8e6;
    border-radius: 2px;
    margin: 0 auto 1rem;
  }

  .messages {
    flex: 1;
    overflow-y: auto;
    display: flex;
    flex-direction: column;
    gap: 0.6rem;
    margin-bottom: 0.75rem;
    padding: 0.25rem 0;
  }

  .hint {
    color: #bbb;
    font-size: 0.88rem;
    text-align: center;
    margin-top: 0.5rem;
    line-height: 1.6;
  }

  .msg {
    padding: 0.5rem 0.75rem;
    border-radius: 12px;
    max-width: 85%;
    font-size: 0.9rem;
    line-height: 1.45;
  }

  .msg.user {
    background: #2d2d2d;
    color: #fafaf9;
    align-self: flex-end;
  }

  .msg.assistant {
    background: #f3f3f2;
    color: #1c1c1c;
    align-self: flex-start;
  }

  .typing {
    color: #bbb;
  }

  .input-row {
    display: flex;
    gap: 0.5rem;
    align-items: flex-end;
  }

  textarea {
    flex: 1;
    border: 1px solid #e8e8e6;
    border-radius: 8px;
    padding: 0.5rem 0.75rem;
    font-size: 0.95rem;
    resize: none;
    font-family: inherit;
    background: #fafaf9;
    color: #1c1c1c;
  }

  textarea:focus {
    outline: none;
    border-color: #2d2d2d;
  }

  button {
    padding: 0.5rem 1rem;
    background: #2d2d2d;
    color: #fafaf9;
    border: none;
    border-radius: 8px;
    cursor: pointer;
    font-size: 0.9rem;
    font-family: inherit;
    white-space: nowrap;
  }

  button:disabled {
    opacity: 0.35;
    cursor: default;
  }
</style>
```

**Step 2: Verify build**

Run: `mise run build`
Expected: exits 0

**Step 3: Commit**

```bash
git add frontend/src/lib/ChatDrawer.svelte
git commit -m "style: clean chat drawer with warm neutral palette"
```

---

### Task 6: Activity notes (ActivityNote.svelte)

**Files:**
- Modify: `frontend/src/lib/ActivityNote.svelte` — `<style>` block only

**Step 1: Replace the `<style>` block**

```svelte
<style>
  .activity {
    margin-top: 2rem;
    padding-top: 1.25rem;
    border-top: 1px solid #e8e8e6;
  }

  h3 {
    text-transform: uppercase;
    font-size: 0.68rem;
    color: #888;
    letter-spacing: 0.08em;
    font-weight: 600;
    margin-bottom: 0.5rem;
  }

  .note {
    color: #1c1c1c;
    font-size: 0.9rem;
    cursor: pointer;
    line-height: 1.55;
    min-height: 1.55rem;
    padding: 0.2rem 0;
  }

  .note:empty::before,
  .note.placeholder {
    color: #bbb;
  }

  textarea {
    width: 100%;
    border: 1px solid #e8e8e6;
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

**Step 2: Update the `.note` div to add `placeholder` class conditionally**

The current markup:
```svelte
<div class="note" onclick={() => editing = true}>
  {notes || 'Tap to add activity notes…'}
</div>
```

Replace with:
```svelte
<div class="note" class:placeholder={!notes} onclick={() => editing = true}>
  {notes || 'Tap to add activity notes…'}
</div>
```

**Step 3: Verify build**

Run: `mise run build`
Expected: exits 0

**Step 4: Commit**

```bash
git add frontend/src/lib/ActivityNote.svelte
git commit -m "style: activity notes consistent with minimalist system"
```

---

### Task 7: Final verification

**Step 1: Full clean build**

Run: `mise run build`
Expected: exits 0, JS bundle < 60KB

**Step 2: Smoke-test locally**

Run: `mise run run`
Visit `http://localhost:8080` and verify:
- Login page: white bg, centered wordmark, bordered sign-in link
- After login: warm off-white bg, underline tab toggle, muted totals
- Meal sections: uppercase muted labels, hairline row separators
- FAB: dark charcoal, no blue anywhere
- Chat drawer: dark user bubbles, light assistant bubbles, dark send button
- Activity note: consistent label style, underline-style textarea

**Step 3: Final commit**

```bash
git add docs/plans/
git commit -m "docs: minimalist redesign plan and design doc"
```
