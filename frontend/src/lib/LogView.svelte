<script>
  import { onMount } from 'svelte'
  import { getLog } from './api.js'
  import EntryRow from './EntryRow.svelte'
  import ChatDrawer from './ChatDrawer.svelte'
  import ActivityNote from './ActivityNote.svelte'

  const MEAL_ORDER = ['breakfast', 'snack', 'lunch', 'dinner']

  let view = $state('today')
  let data = $state(null)
  let loading = $state(true)
  let drawerOpen = $state(false)

  async function load() {
    loading = true
    try {
      data = await getLog({ week: view === 'week' })
    } finally {
      loading = false
    }
  }

  onMount(load)

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

  function onEntriesAdded(newEntries) {
    data = { ...data, entries: [...(data.entries ?? []), ...newEntries] }
    drawerOpen = false
  }

  $effect(() => { if (view) load() })
</script>

<div class="wrap">
  <header>
    <div class="toggle">
      <button class:active={view === 'today'} onclick={() => view = 'today'}>Today</button>
      <button class:active={view === 'week'} onclick={() => view = 'week'}>Week</button>
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
          <EntryRow {entry} onUpdate={handleUpdate} />
        {:else}
          <p class="empty">Nothing logged</p>
        {/each}
      </section>
    {/each}
    <ActivityNote date={data?.date} />
  {:else}
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
</div>

<button class="fab" onclick={() => drawerOpen = true} aria-label="Add food">+</button>
<ChatDrawer open={drawerOpen} onClose={() => drawerOpen = false} {onEntriesAdded} />

<style>
  .wrap { max-width: 640px; margin: 0 auto; padding: 1rem; padding-bottom: 6rem; }
  header { position: sticky; top: 0; background: white; padding: 0.75rem 0; border-bottom: 2px solid #eee; display: flex; justify-content: space-between; align-items: center; margin-bottom: 0.5rem; }
  .toggle button { padding: 0.3rem 0.85rem; border: 1px solid #ddd; background: none; cursor: pointer; font-size: 0.9rem; }
  .toggle button:first-child { border-radius: 4px 0 0 4px; }
  .toggle button:last-child { border-radius: 0 4px 4px 0; }
  .toggle button.active { background: #4285f4; color: white; border-color: #4285f4; }
  .totals { display: flex; gap: 0.75rem; font-size: 0.82rem; font-weight: 500; color: #333; }
  section { margin: 1.25rem 0; }
  h3 { text-transform: capitalize; font-size: 0.75rem; color: #999; letter-spacing: 0.06em; margin-bottom: 0.4rem; }
  .empty { color: #ccc; font-size: 0.85rem; font-style: italic; padding: 0.25rem 0; }
  .state { color: #aaa; text-align: center; margin-top: 3rem; }
  .week-row { display: flex; justify-content: space-between; padding: 0.6rem 0; border-bottom: 1px solid #f0f0f0; font-size: 0.9rem; }
  .date { font-weight: 500; }
  .fab { position: fixed; bottom: 2rem; right: 2rem; width: 3.5rem; height: 3.5rem; border-radius: 50%; background: #4285f4; color: white; font-size: 2rem; border: none; cursor: pointer; box-shadow: 0 4px 12px rgba(0,0,0,0.2); display: flex; align-items: center; justify-content: center; line-height: 1; }
</style>
