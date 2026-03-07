<script>
  import { getLog } from './api.js'
  import EntryRow from './EntryRow.svelte'
  import ChatDrawer from './ChatDrawer.svelte'
  import ActivityNote from './ActivityNote.svelte'
  import DayModal from './DayModal.svelte'

  const MEAL_ORDER = ['breakfast', 'snack', 'lunch', 'dinner']

  let view = $state('today')
  let data = $state(null)
  let loading = $state(true)
  let drawerOpen = $state(false)
  let selectedDay = $state(null)
  let drawerDate = $state(null)

  async function load() {
    loading = true
    try {
      data = await getLog(view === 'history' ? { days: 30 } : {})
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
    drawerDate = date
    drawerOpen = true
  }

  function onEntriesAdded(newEntries) {
    data = { ...data, entries: [...(data.entries ?? []), ...newEntries] }
    drawerOpen = false
  }

  $effect(() => { if (view) load() })
</script>

<div class="wrap">
  <header>
    <div class="header-top">
      <div class="toggle">
        <button class:active={view === 'today'} onclick={() => { view = 'today'; selectedDay = null; drawerDate = null }}>Today</button>
        <button class:active={view === 'history'} onclick={() => view = 'history'}>History</button>
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
        <h3>{meal}</h3>
        {#each group as entry}
          <EntryRow {entry} onUpdate={handleUpdate} onDelete={handleDelete} />
        {:else}
          <p class="empty">Nothing logged</p>
        {/each}
      </section>
    {/each}
    <ActivityNote date={data?.date} />
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
      <DayModal
        day={selectedDay}
        onClose={() => selectedDay = null}
        onDelete={handleDelete}
        onAddFood={openDrawerForDate}
      />
    {/if}
  {/if}
</div>

<button class="fab" onclick={() => drawerOpen = true} aria-label="Add food">+</button>
<ChatDrawer
  open={drawerOpen}
  onClose={() => { drawerOpen = false; drawerDate = null }}
  {onEntriesAdded}
  date={drawerDate}
/>

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
    align-items: center;
    padding: 0.75rem 0;
    border-bottom: 1px solid #e8e8e6;
    cursor: pointer;
    gap: 1rem;
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

  .chevron {
    color: #ccc;
    font-size: 1.1rem;
    line-height: 1;
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
