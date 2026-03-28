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
  let drawerTab = $state('food')
  let activityRefreshKey = $state(0)
  let selectedDay = $state(null)
  let drawerDate = $state(null)
  let drawerMeal = $state(null)
  let drawerField = $state(null)
  let yesterdayByMeal = $state({})
  let repeating = $state(null)
  let repeatedMeals = $state(new Set())
  let repeatPicker = $state(null)
  let longPressTimer = null

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

  async function repeatMeal(targetMeal, sourceMeal = targetMeal) {
    if (repeating !== null) return
    repeating = targetMeal
    repeatPicker = null
    try {
      const entries = yesterdayByMeal[sourceMeal].map(e => ({ ...e, meal_type: targetMeal }))
      const res = await confirmChat(entries, data?.date ?? yesterdayString())
      data = { ...data, entries: [...(data.entries ?? []), ...res.entries] }
      yesterdayByMeal = { ...yesterdayByMeal, [targetMeal]: [] }
      repeatedMeals = new Set([...repeatedMeals, targetMeal])
    } catch {
      // silent fail — user can tap again
    } finally {
      repeating = null
    }
  }

  function startLongPress(meal) {
    longPressTimer = setTimeout(() => {
      longPressTimer = null
      repeatPicker = meal
    }, 500)
  }

  function endLongPress(meal) {
    if (longPressTimer !== null) {
      clearTimeout(longPressTimer)
      longPressTimer = null
      repeatMeal(meal)
    }
  }

  function cancelPress() {
    if (longPressTimer !== null) {
      clearTimeout(longPressTimer)
      longPressTimer = null
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
      (a, e) => ({ calories: a.calories + e.calories, protein: a.protein + e.protein, carbs: a.carbs + e.carbs, fat: a.fat + e.fat, fiber: a.fiber + (e.fiber ?? 0) }),
      { calories: 0, protein: 0, carbs: 0, fat: 0, fiber: 0 }
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
    drawerMeal = null
    drawerTab = 'food'
    drawerOpen = true
  }

  function openActivityDrawer(field = null) {
    drawerField = field
    drawerTab = 'activity'
    drawerDate = null
    drawerMeal = null
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
          <a class="sheet-link" href={spreadsheetUrl} target="_blank" rel="noopener" aria-label="Open Google Sheet" title="Open Google Sheet">
            <svg width="15" height="15" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"><path d="M14 2H6a2 2 0 0 0-2 2v16a2 2 0 0 0 2 2h12a2 2 0 0 0 2-2V8z"/><polyline points="14 2 14 8 20 8"/><line x1="8" y1="13" x2="16" y2="13"/><line x1="8" y1="17" x2="16" y2="17"/><polyline points="10 9 9 9 8 9"/></svg>
          </a>
        {/if}
        <button class="refresh-btn" onclick={() => location.reload()} aria-label="Refresh" title="Refresh">
            <svg width="15" height="15" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"><path d="M23 4v6h-6"/><path d="M1 20v-6h6"/><path d="M3.51 9a9 9 0 0 1 14.85-3.36L23 10M1 14l4.64 4.36A9 9 0 0 0 20.49 15"/></svg>
          </button>
        <button class="settings-btn" onclick={() => profileOpen = true} aria-label="Profile settings" title="Profile settings">⚙</button>
        <a class="signout-btn" href="/auth/logout" aria-label="Sign out" title="Sign out">
          <svg width="15" height="15" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"><path d="M9 21H5a2 2 0 0 1-2-2V5a2 2 0 0 1 2-2h4"/><polyline points="16 17 21 12 16 7"/><line x1="21" y1="12" x2="9" y2="12"/></svg>
        </a>
      </div>
    </div>
    {#if data?.entries && view === 'today'}
      {@const t = totals(data.entries)}
      <div class="totals">
        <span>{t.calories} cal</span>
        <span>{t.protein}g P</span>
        <span>{t.carbs}g C</span>
        <span>{t.fat}g F</span>
        <span>{t.fiber}g Fb</span>
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
          <button class="meal-name" onclick={() => { drawerMeal = meal; drawerTab = 'food'; drawerOpen = true }}>{meal}<span class="meal-add">+</span></button>
          {#if yesterdayByMeal[meal]?.length}
            {#if repeatPicker === meal}
              <div class="repeat-picker">
                {#each MEAL_ORDER.filter(m => yesterdayByMeal[m]?.length) as src}
                  <button class="pick-btn" onclick={() => repeatMeal(meal, src)}>{src}</button>
                {/each}
                <button class="pick-cancel" onclick={() => repeatPicker = null}>✕</button>
              </div>
            {:else}
              <button
                class="repeat-btn"
                class:spinning={repeating === meal}
                onpointerdown={() => startLongPress(meal)}
                onpointerup={() => endLongPress(meal)}
                onpointercancel={cancelPress}
                oncontextmenu={e => e.preventDefault()}
                disabled={repeating !== null}
                aria-label="Repeat yesterday's {meal}"
                title="Repeat yesterday's {meal} — hold for options"
              >↻</button>
            {/if}
          {/if}
        </div>
        {#each group as entry}
          <EntryRow {entry} onUpdate={handleUpdate} onDelete={handleDelete} />
        {:else}
          <button class="empty" onclick={() => { drawerMeal = meal; drawerOpen = true }}>Nothing logged</button>
        {/each}
      </section>
    {/each}
    <ActivityNote date={data?.date} onOpen={openActivityDrawer} refreshKey={activityRefreshKey} />
  {:else}
    {#each Object.entries(groupedByDate(data?.entries ?? [])).sort((a, b) => b[0].localeCompare(a[0])) as [date, entries]}
      {@const dayLog = (data?.daily_logs ?? []).find(d => d.date === date) ?? null}
      <!-- svelte-ignore a11y_click_events_have_key_events -->
      <div class="week-row" role="button" tabindex="0" onclick={() => selectedDay = { date, entries, dayLog }}>
        <span class="date">{date}</span>
        <span class="activity-tick">{dayLog?.activity ? '✓' : ''}</span>
        <span class="poop-tick">{dayLog?.poop ? '💩' : ''}</span>
        <span class="hydration-tick">{dayLog?.hydration ? '💧' : ''}</span>
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

<button class="fab" onclick={() => { drawerMeal = null; drawerTab = 'food'; drawerOpen = true }} aria-label="Add food">
  <svg width="22" height="22" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2.5" stroke-linecap="round"><line x1="12" y1="4" x2="12" y2="20"/><line x1="4" y1="12" x2="20" y2="12"/></svg>
</button>
<ChatDrawer
  open={drawerOpen}
  onClose={() => { if (drawerTab === 'activity') activityRefreshKey++; drawerOpen = false; drawerDate = null; drawerMeal = null; drawerTab = 'food'; drawerField = null }}
  {onEntriesAdded}
  date={drawerDate}
  meal={drawerMeal}
  initialTab={drawerTab}
  initialField={drawerField}
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
    z-index: 10;
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
    font-size: 0.72rem;
    color: #888;
    letter-spacing: 0.08em;
    font-weight: 600;
    cursor: pointer;
    display: inline-block;
    padding: 0.3rem 0;
    touch-action: manipulation;
  }

  @media (hover: hover) {
    .meal-name:hover {
      color: #2d2d2d;
    }
  }

  .meal-add {
    margin-left: 0.35rem;
    font-size: 0.82rem;
    opacity: 0.55;
    font-weight: 600;
  }

  @media (hover: hover) {
    .meal-name:hover .meal-add {
      opacity: 1;
    }
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

  @media (hover: hover) {
    .repeat-btn:hover:not(:disabled) {
      color: #555;
    }
  }

  .repeat-btn:disabled {
    cursor: default;
  }

  .repeat-picker {
    display: flex;
    align-items: center;
    gap: 0.25rem;
  }

  .pick-btn {
    background: none;
    border: 1px solid #d0d0ce;
    border-radius: 999px;
    padding: 0.15rem 0.55rem;
    font-size: 0.7rem;
    color: #555;
    cursor: pointer;
    font-family: inherit;
    font-weight: 500;
    text-transform: uppercase;
    letter-spacing: 0.04em;
    white-space: nowrap;
    touch-action: manipulation;
  }

  @media (hover: hover) {
    .pick-btn:hover { border-color: #2d2d2d; color: #2d2d2d; }
  }

  .pick-cancel {
    background: none;
    border: none;
    color: #ccc;
    font-size: 0.75rem;
    cursor: pointer;
    padding: 0.15rem 0.2rem;
    line-height: 1;
    font-family: inherit;
    touch-action: manipulation;
  }

  @media (hover: hover) {
    .pick-cancel:hover { color: #888; }
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
    padding: 0.75rem 0;
    cursor: pointer;
    touch-action: manipulation;
    width: 100%;
  }

  @media (hover: hover) {
    .empty:hover {
      color: #888;
    }
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

  @media (hover: hover) {
    .week-row:hover {
      background: #fafaf9;
    }
  }

  .date {
    font-weight: 500;
    font-size: 0.88rem;
    flex: 1;
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

  .hydration-tick {
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
    bottom: calc(2rem + env(safe-area-inset-bottom, 0px));
    right: 2rem;
    width: 3.5rem;
    height: 3.5rem;
    border-radius: 50%;
    background: #2d2d2d;
    color: #fafaf9;
    border: none;
    cursor: pointer;
    box-shadow: 0 2px 8px rgba(0,0,0,0.18);
    display: flex;
    align-items: center;
    justify-content: center;
    touch-action: manipulation;
  }

  @media (hover: hover) {
    .fab:hover {
      background: #1c1c1c;
    }
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
    min-height: 2.75rem;
  }

  @media (hover: hover) {
    .sheet-link:hover {
      color: #2d2d2d;
    }
  }

  .refresh-btn {
    background: none;
    border: none;
    display: flex;
    align-items: center;
    color: #888;
    cursor: pointer;
    padding: 0.5rem 0.4rem;
    line-height: 1;
    touch-action: manipulation;
    min-height: 2.75rem;
  }

  @media (hover: hover) {
    .refresh-btn:hover {
      color: #2d2d2d;
    }
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
    min-height: 2.75rem;
  }

  @media (hover: hover) {
    .settings-btn:hover {
      color: #2d2d2d;
    }
  }

  .signout-btn {
    display: flex;
    align-items: center;
    color: #888;
    padding: 0.5rem 0.4rem;
    text-decoration: none;
    touch-action: manipulation;
    min-height: 2.75rem;
  }

  @media (hover: hover) {
    .signout-btn:hover {
      color: #2d2d2d;
    }
  }
</style>
