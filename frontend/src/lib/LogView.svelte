<script>
  import { getLog, confirmChat } from './api.js'
  import EntryRow from './EntryRow.svelte'
  import ChatDrawer from './ChatDrawer.svelte'
  import ActivityNote from './ActivityNote.svelte'
  import DayModal from './DayModal.svelte'
  import ProfilePanel from './ProfilePanel.svelte'

  const MEAL_ORDER = ['breakfast', 'snack', 'lunch', 'dinner']

  let view = $state('today')
  let profileOpen = $state(false)
  let spreadsheetUrl = $state('')
  let data = $state(null)
  let loading = $state(true)
  let drawerOpen = $state(false)
  let selectedDay = $state(null)
  let drawerDate = $state(null)
  let drawerPrefill = $state('')
  let yesterdayByMeal = $state({})
  let repeating = $state(null)
  let repeatedMeals = $state(new Set())

  async function load() {
    loading = true
    try {
      data = await getLog(view === 'history' ? { days: 30 } : {})
      if (data?.spreadsheet_url && !spreadsheetUrl) spreadsheetUrl = data.spreadsheet_url
    } finally {
      loading = false
    }
  }

  function groupedByMeal(entries) {
    const g = {}
    for (const e of entries ?? []) {
      ;(g[e.meal_type] ??= []).push(e)
    }
    return g
  }

  function yesterdayString() {
    const d = new Date()
    d.setDate(d.getDate() - 1)
    return [d.getFullYear(), String(d.getMonth() + 1).padStart(2, '0'), String(d.getDate()).padStart(2, '0')].join('-')
  }

  async function loadYesterday() {
    try {
      const res = await getLog({ date: yesterdayString() })
      const g = {}
      for (const e of res.entries ?? []) {
        ;(g[e.meal_type] ??= []).push(e)
      }
      for (const meal of repeatedMeals) {
        g[meal] = []
      }
      yesterdayByMeal = g
    } catch {}
  }

  async function repeatMeal(meal) {
    if (repeating !== null) return
    repeating = meal
    try {
      const res = await confirmChat(yesterdayByMeal[meal], data?.date ?? yesterdayString())
      data = { ...data, entries: [...(data.entries ?? []), ...res.entries] }
      yesterdayByMeal = { ...yesterdayByMeal, [meal]: [] }
      repeatedMeals = new Set([...repeatedMeals, meal])
    } catch {
      // silent fail — user can tap again
    } finally {
      repeating = null
    }
  }

  function groupedByDate(entries) {
    const g = {}
    for (const e of entries ?? []) {
      ;(g[e.date] ??= []).push(e)
    }
    return g
  }

  function totals(entries) {
    return (entries ?? []).reduce(
      (a, e) => ({ calories: a.calories + e.calories, protein: a.protein + e.protein, carbs: a.carbs + e.carbs, fat: a.fat + e.fat }),
      { calories: 0, protein: 0, carbs: 0, fat: 0 }
    )
  }

  function handleUpdate(updated) {
    data = { ...data, entries: data.entries.map(e => e.id === updated.id ? updated : e) }
  }

  function handleDelete(id) {
    data = { ...data, entries: (data.entries ?? []).filter(e => e.id !== id) }
  }

  function openDrawerForDate(date) {
    selectedDay = null
    drawerDate = date
    drawerOpen = true
  }

  function onEntriesAdded(newEntries) {
    data = { ...data, entries: [...(data.entries ?? []), ...newEntries] }
    drawerOpen = false
  }

  $effect(() => {
    if (view) {
      load()
      if (view === 'today') loadYesterday()
    }
  })
</script>

<div class="wrap">
  <header>
    <div class="header-top">
      <div class="toggle">
        <button class:active={view === 'today'} onclick={() => { view = 'today'; selectedDay = null; drawerDate = null }}>Today</button>
        <button class:active={view === 'history'} onclick={() => view = 'history'}>History</button>
      </div>
      <div class="header-actions">
        {#if spreadsheetUrl}
          <a class="sheet-link" href={spreadsheetUrl} target="_blank" rel="noopener" aria-label="Open Google Sheet">
            <svg width="15" height="15" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"><path d="M14 2H6a2 2 0 0 0-2 2v16a2 2 0 0 0 2 2h12a2 2 0 0 0 2-2V8z"/><polyline points="14 2 14 8 20 8"/><line x1="8" y1="13" x2="16" y2="13"/><line x1="8" y1="17" x2="16" y2="17"/><polyline points="10 9 9 9 8 9"/></svg>
          </a>
        {/if}
        <button class="settings-btn" onclick={() => profileOpen = true} aria-label="Profile settings">⚙</button>
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

  {#if loading}
    <p class="state">Loading…</p>
  {:else if view === 'today'}
    {#each MEAL_ORDER as meal}
      {@const group = (groupedByMeal(data?.entries)[meal] ?? [])}
      <section>
        <div class="meal-header">
          <button class="meal-name" onclick={() => { drawerPrefill = `for ${meal}, I had `; drawerOpen = true }}>{meal}</button>
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
        {#each group as entry}
          <EntryRow {entry} onUpdate={handleUpdate} onDelete={handleDelete} />
        {:else}
          <button class="empty" onclick={() => { drawerPrefill = `for ${meal}, I had `; drawerOpen = true }}>Nothing logged</button>
        {/each}
      </section>
    {/each}
    <ActivityNote date={data?.date} />
  {:else}
    {#each Object.entries(groupedByDate(data?.entries ?? [])).sort() as [date, entries]}
      {@const dayLog = (data?.daily_logs ?? []).find(d => d.date === date) ?? null}
      <!-- svelte-ignore a11y_click_events_have_key_events -->
      <div class="week-row" role="button" tabindex="0" onclick={() => selectedDay = { date, entries, dayLog }}>
        <span class="date">{date}</span>
        <span class="feeling-score">{dayLog?.feeling_score ? `${dayLog.feeling_score}/10` : '—'}</span>
        <span class="activity-tick">{dayLog?.activity ? '✓' : ''}</span>
        <span class="poop-tick">{dayLog?.poop ? '💩' : ''}</span>
        <span class="chevron">›</span>
      </div>
    {/each}
    {#if selectedDay}
      <DayModal
        day={selectedDay}
        onClose={() => selectedDay = null}
        onDelete={handleDelete}
        onAddFood={openDrawerForDate}
      />
    {/if}
  {/if}
</div>

<button class="fab" onclick={() => { drawerPrefill = ''; drawerOpen = true }} aria-label="Add food">+</button>
<ChatDrawer
  open={drawerOpen}
  onClose={() => { drawerOpen = false; drawerDate = null; drawerPrefill = '' }}
  {onEntriesAdded}
  date={drawerDate}
  prefill={drawerPrefill}
/>
{#if profileOpen}
  <ProfilePanel onClose={() => profileOpen = false} />
{/if}

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

  .meal-name {
    background: none;
    border: none;
    font-family: inherit;
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

  .meal-name:hover {
    color: #2d2d2d;
  }

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

  .empty {
    background: none;
    border: none;
    font-family: inherit;
    text-align: left;
    color: #bbb;
    font-size: 0.85rem;
    padding: 0.6rem 0;
    cursor: pointer;
    touch-action: manipulation;
    width: 100%;
  }

  .empty:hover {
    color: #888;
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
    align-items: center;
    padding: 0.85rem 0;
    border-bottom: 1px solid #e8e8e6;
    cursor: pointer;
    gap: 1rem;
    touch-action: manipulation;
  }

  .week-row:hover {
    background: #fafaf9;
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

  .poop-tick {
    font-size: 0.82rem;
    min-width: 1rem;
    text-align: center;
  }

  .chevron {
    color: #ccc;
    font-size: 1.1rem;
    line-height: 1;
  }

  .fab {
    position: fixed;
    bottom: 2rem;
    right: 2rem;
    width: 3.5rem;
    height: 3.5rem;
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
    touch-action: manipulation;
  }

  .fab:hover {
    background: #1c1c1c;
  }

  .header-actions {
    display: flex;
    align-items: center;
    gap: 0.25rem;
  }

  .sheet-link {
    display: flex;
    align-items: center;
    color: #888;
    padding: 0.5rem 0.4rem;
    text-decoration: none;
    touch-action: manipulation;
  }

  .sheet-link:hover {
    color: #2d2d2d;
  }

  .settings-btn {
    background: none;
    border: none;
    font-size: 1.1rem;
    color: #888;
    cursor: pointer;
    padding: 0.5rem 0.5rem;
    line-height: 1;
    touch-action: manipulation;
  }

  .settings-btn:hover {
    color: #2d2d2d;
  }
</style>
