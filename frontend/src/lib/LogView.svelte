<script>
  import { getLog, confirmChat, getInsights } from './api.js'
  import EntryRow from './EntryRow.svelte'
  import ChatDrawer from './ChatDrawer.svelte'
  import ActivityNote from './ActivityNote.svelte'
  import ProfilePanel from './ProfilePanel.svelte'

  const MEAL_ORDER = ['breakfast', 'snack', 'lunch', 'dinner']
  const DAY_ABBREV = ['Su', 'Mo', 'Tu', 'We', 'Th', 'Fr', 'Sa']

  let view = $state('day')
  let currentDate = $state(todayStr())
  let spreadsheetUrl = $state('')
  let dayData = $state(null)
  let historyData = $state(null)
  let loading = $state(true)
  let profileOpen = $state(false)
  let drawerOpen = $state(false)
  let drawerTab = $state('food')
  let activityRefreshKey = $state(0)
  let drawerDate = $state(null)
  let drawerMeal = $state(null)
  let drawerField = $state(null)
  let yesterdayByMeal = $state({})
  let repeating = $state(null)
  let repeatedMeals = $state(new Set())
  let repeatPicker = $state(null)
  let longPressTimer = null
  let dateInputEl = $state(null)
  let insightsByWeek = $state({})
  let historyWeeks = $state(8)
  let weekGroupsData = $derived(weekGroups(historyData, historyWeeks))

  function todayStr() {
    const d = new Date()
    return [d.getFullYear(), String(d.getMonth() + 1).padStart(2, '0'), String(d.getDate()).padStart(2, '0')].join('-')
  }

  function addDays(dateStr, n) {
    const d = new Date(dateStr + 'T12:00:00')
    d.setDate(d.getDate() + n)
    return d.toISOString().slice(0, 10)
  }

  function formatDateNav(dateStr) {
    const today = todayStr()
    if (dateStr === today) return 'Today'
    if (dateStr === addDays(today, -1)) return 'Yesterday'
    const d = new Date(dateStr + 'T12:00:00')
    return d.toLocaleDateString('en-US', { weekday: 'short', month: 'short', day: 'numeric' })
  }

  function getMonday(dateStr) {
    const d = new Date(dateStr + 'T12:00:00')
    const day = d.getDay()
    const diff = day === 0 ? -6 : 1 - day
    d.setDate(d.getDate() + diff)
    return d.toISOString().slice(0, 10)
  }

  function formatWeekRange(start, end) {
    const s = new Date(start + 'T12:00:00')
    const e = new Date(end + 'T12:00:00')
    const sm = s.toLocaleDateString('en-US', { month: 'short' })
    const em = e.toLocaleDateString('en-US', { month: 'short' })
    if (sm === em) return `${sm} ${s.getDate()}–${e.getDate()}`
    return `${sm} ${s.getDate()} – ${em} ${e.getDate()}`
  }

  async function loadDay(date) {
    loading = true
    try {
      dayData = await getLog({ date })
      if (dayData?.spreadsheet_url && !spreadsheetUrl) spreadsheetUrl = dayData.spreadsheet_url
    } finally {
      loading = false
    }
  }

  async function loadHistory(weeks = historyWeeks) {
    loading = true
    try {
      historyData = await getLog({ days: weeks * 7 })
      if (historyData?.spreadsheet_url && !spreadsheetUrl) spreadsheetUrl = historyData.spreadsheet_url
    } finally {
      loading = false
    }
  }

  function groupedByMeal(entries) {
    const g = {}
    for (const e of entries ?? []) { (g[e.meal_type] ??= []).push(e) }
    return g
  }

  function totals(entries) {
    return (entries ?? []).reduce(
      (a, e) => ({ calories: a.calories + e.calories, protein: a.protein + e.protein, carbs: a.carbs + e.carbs, fat: a.fat + e.fat, fiber: a.fiber + (e.fiber ?? 0) }),
      { calories: 0, protein: 0, carbs: 0, fat: 0, fiber: 0 }
    )
  }

  function weekGroups(data, numWeeks = 8) {
    if (!data) return []
    const { entries = [], daily_logs = [] } = data
    const byDate = {}
    for (const e of entries) { (byDate[e.date] ??= { entries: [], dayLog: null }).entries.push(e) }
    for (const l of daily_logs) { (byDate[l.date] ??= { entries: [], dayLog: null }).dayLog = l }

    const today = todayStr()
    let monday = getMonday(addDays(today, -(numWeeks * 7 - 1)))
    const todayMonday = getMonday(today)
    const weeks = []
    while (monday <= todayMonday) {
      const days = Array.from({ length: 7 }, (_, i) => {
        const date = addDays(monday, i)
        const future = date > today
        return { date, future, ...(future ? { entries: [], dayLog: null } : (byDate[date] ?? { entries: [], dayLog: null })) }
      })
      const sunday = addDays(monday, 6)
      const weekTotal = days.reduce((t, d) => d.entries.reduce((s, e) => s + e.calories, t), 0)
      weeks.push({ weekStart: monday, weekEnd: sunday <= today ? sunday : today, days, weekTotal })
      monday = addDays(monday, 7)
    }
    return weeks.reverse()
  }

  async function loadYesterday() {
    const yStr = addDays(todayStr(), -1)
    try {
      const res = await getLog({ date: yStr })
      const g = {}
      for (const e of res.entries ?? []) { (g[e.meal_type] ??= []).push(e) }
      for (const meal of repeatedMeals) { g[meal] = [] }
      yesterdayByMeal = g
    } catch {}
  }

  async function repeatMeal(targetMeal, sourceMeal = targetMeal) {
    if (repeating !== null) return
    repeating = targetMeal
    repeatPicker = null
    try {
      const entries = yesterdayByMeal[sourceMeal].map(e => ({ ...e, meal_type: targetMeal }))
      const res = await confirmChat(entries, dayData?.date ?? todayStr())
      dayData = { ...dayData, entries: [...(dayData.entries ?? []), ...res.entries] }
      yesterdayByMeal = { ...yesterdayByMeal, [targetMeal]: [] }
      repeatedMeals = new Set([...repeatedMeals, targetMeal])
    } catch {} finally {
      repeating = null
    }
  }

  function startLongPress(meal) {
    longPressTimer = setTimeout(() => { longPressTimer = null; repeatPicker = meal }, 500)
  }

  function endLongPress(meal) {
    if (longPressTimer !== null) { clearTimeout(longPressTimer); longPressTimer = null; repeatMeal(meal) }
  }

  function cancelPress() {
    if (longPressTimer !== null) { clearTimeout(longPressTimer); longPressTimer = null }
  }

  function handleUpdate(updated) {
    dayData = { ...dayData, entries: dayData.entries.map(e => e.id === updated.id ? updated : e) }
  }

  function handleDelete(id) {
    dayData = { ...dayData, entries: (dayData.entries ?? []).filter(e => e.id !== id) }
  }

  function openActivityDrawer(field = null) {
    drawerField = field
    drawerTab = 'activity'
    drawerDate = null
    drawerMeal = null
    drawerOpen = true
  }

  function onEntriesAdded(newEntries) {
    dayData = { ...dayData, entries: [...(dayData.entries ?? []), ...newEntries] }
    drawerOpen = false
  }

  function openDatePicker() {
    if (!dateInputEl) return
    if (typeof dateInputEl.showPicker === 'function') {
      dateInputEl.showPicker()
    } else {
      dateInputEl.click()
    }
  }

  async function fetchInsights(weekStart, weekEnd) {
    insightsByWeek = { ...insightsByWeek, [weekStart]: { open: true, loading: true, text: null, error: null } }
    try {
      const res = await getInsights(weekStart, weekEnd)
      insightsByWeek = { ...insightsByWeek, [weekStart]: { open: true, loading: false, text: res.insight, error: null } }
    } catch {
      insightsByWeek = { ...insightsByWeek, [weekStart]: { open: true, loading: false, text: null, error: 'Could not load insights' } }
    }
  }

  function escapeHtml(str) {
    return str.replace(/&/g, '&amp;').replace(/</g, '&lt;').replace(/>/g, '&gt;').replace(/"/g, '&quot;')
  }

  function renderInsight(text) {
    return text
      .split('\n')
      .map(line => line.trim())
      .filter(line => line.length > 0)
      .map(line => escapeHtml(line).replace(/\*\*(.+?)\*\*/g, '<strong>$1</strong>'))
      .join('\n')
  }

  function toggleInsights(weekStart, weekEnd) {
    const cur = insightsByWeek[weekStart]
    if (!cur) {
      fetchInsights(weekStart, weekEnd)
    } else {
      insightsByWeek = { ...insightsByWeek, [weekStart]: { ...cur, open: !cur.open } }
    }
  }

  $effect(() => {
    const v = view
    const d = currentDate
    const hw = historyWeeks
    if (v === 'day') {
      repeatedMeals = new Set()
      loadDay(d)
      if (d === todayStr()) loadYesterday()
      else yesterdayByMeal = {}
    } else {
      loadHistory(hw)
    }
  })
</script>

<div class="wrap">
  <header>
    <div class="header-top">
      <div class="toggle">
        <button class:active={view === 'day'} onclick={() => { view = 'day' }}>Day</button>
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
    {#if view === 'history'}
      <div class="week-picker">
        {#each [4, 8, 12, 26] as w}
          <button class="wp-btn" class:active={historyWeeks === w} onclick={() => historyWeeks = w}>{w}w</button>
        {/each}
      </div>
    {/if}
    {#if view === 'day'}
      <div class="date-nav">
        <button class="nav-arrow" onclick={() => currentDate = addDays(currentDate, -1)} aria-label="Previous day">‹</button>
        <button class="nav-date" onclick={openDatePicker}>{formatDateNav(currentDate)}</button>
        <input
          type="date"
          class="date-input-hidden"
          bind:value={currentDate}
          bind:this={dateInputEl}
          max={todayStr()}
        />
        <button class="nav-arrow" class:dimmed={currentDate >= todayStr()} disabled={currentDate >= todayStr()} onclick={() => currentDate = addDays(currentDate, 1)} aria-label="Next day">›</button>
      </div>
      {#if dayData?.entries}
        {@const t = totals(dayData.entries)}
        <div class="totals">
          <span>{t.calories} cal</span>
          <span>{t.protein}g P</span>
          <span>{t.carbs}g C</span>
          <span>{t.fat}g F</span>
          <span>{t.fiber}g Fb</span>
        </div>
      {/if}
    {/if}
  </header>

  {#if loading}
    <p class="state">Loading…</p>
  {:else if view === 'day'}
    {#each MEAL_ORDER as meal}
      {@const group = (groupedByMeal(dayData?.entries)[meal] ?? [])}
      <section>
        <div class="meal-header">
          <button class="meal-name" onclick={() => { drawerMeal = meal; drawerDate = currentDate; drawerTab = 'food'; drawerOpen = true }}>{meal}<span class="meal-add">+</span></button>
          {#if currentDate === todayStr() && yesterdayByMeal[meal]?.length}
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
          <button class="empty" onclick={() => { drawerMeal = meal; drawerDate = currentDate; drawerOpen = true }}>Nothing logged</button>
        {/each}
      </section>
    {/each}
    <ActivityNote date={currentDate} onOpen={openActivityDrawer} refreshKey={activityRefreshKey} />
  {:else}
    {#each weekGroupsData as week}
      <div class="week-block">
        <div class="week-head">
          <div class="week-meta">
            <span class="week-range">{formatWeekRange(week.weekStart, week.weekEnd)}</span>
            {#if week.weekTotal > 0}
              <span class="week-cal">{week.weekTotal.toLocaleString()} cal</span>
            {/if}
          </div>
          {#if week.weekTotal > 0 || week.days.some(d => d.dayLog)}
            <button
              class="insights-btn"
              class:active={insightsByWeek[week.weekStart]?.open}
              onclick={() => toggleInsights(week.weekStart, week.weekEnd)}
              aria-label="AI insights for this week"
              title="AI insights"
            >✦ insights</button>
          {/if}
        </div>
        <div class="week-grid">
          {#each week.days as day}
            <button
              class="day-cell"
              class:future={day.future}
              class:has-food={day.entries.length > 0}
              onclick={() => { if (!day.future) { currentDate = day.date; view = 'day' } }}
              disabled={day.future}
              aria-label={day.date}
            >
              <span class="dc-abbrev">{DAY_ABBREV[new Date(day.date + 'T12:00:00').getDay()]}</span>
              <span class="dc-num">{new Date(day.date + 'T12:00:00').getDate()}</span>
              <span class="dc-indicators">
                {#if day.entries.length > 0}<span class="dc-food">●</span>{:else}<span class="dc-empty">○</span>{/if}
                {#if day.dayLog?.poop}<span class="dc-poop">💩</span>{/if}
              </span>
            </button>
          {/each}
        </div>
        {#if insightsByWeek[week.weekStart]?.open}
          <div class="insights-panel">
            {#if insightsByWeek[week.weekStart].loading}
              <span class="insights-loading">Thinking…</span>
            {:else if insightsByWeek[week.weekStart].error}
              <span class="insights-err">{insightsByWeek[week.weekStart].error}</span>
            {:else if insightsByWeek[week.weekStart].text}
              <!-- eslint-disable-next-line svelte/no-at-html-tags -->
              <p class="insights-text">{@html renderInsight(insightsByWeek[week.weekStart].text)}</p>
            {/if}
          </div>
        {/if}
      </div>
    {/each}
  {/if}
</div>

<button class="fab" onclick={() => { drawerDate = currentDate; drawerMeal = null; drawerTab = 'food'; drawerOpen = true }} aria-label="Add food">
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

  /* Week picker */
  .week-picker {
    display: flex;
    gap: 0.35rem;
    margin: 0.4rem 0 0.1rem;
  }

  .wp-btn {
    background: none;
    border: 1px solid #e0e0de;
    border-radius: 999px;
    color: #888;
    font-size: 0.72rem;
    padding: 0.2rem 0.6rem;
    cursor: pointer;
    font-family: inherit;
    letter-spacing: 0.02em;
    transition: border-color 0.12s, color 0.12s, background 0.12s;
    touch-action: manipulation;
  }

  .wp-btn.active {
    border-color: #2d2d2d;
    color: #2d2d2d;
    background: #f5f5f3;
  }

  @media (hover: hover) {
    .wp-btn:not(.active):hover { border-color: #aaa; color: #555; }
  }

  /* Date navigator */
  .date-nav {
    display: flex;
    align-items: center;
    justify-content: space-between;
    margin: 0.4rem 0 0.1rem;
    position: relative;
  }

  .nav-arrow {
    background: none;
    border: none;
    font-size: 1.6rem;
    color: #555;
    cursor: pointer;
    padding: 0.1rem 0.4rem;
    line-height: 1;
    touch-action: manipulation;
    font-family: inherit;
    min-height: 2.5rem;
    display: flex;
    align-items: center;
  }

  .nav-arrow.dimmed,
  .nav-arrow:disabled {
    color: #ccc;
    cursor: default;
  }

  .nav-date {
    background: none;
    border: none;
    font-family: inherit;
    font-size: 1rem;
    font-weight: 600;
    color: #1c1c1c;
    cursor: pointer;
    padding: 0.2rem 0.75rem;
    touch-action: manipulation;
    flex: 1;
    text-align: center;
    border-radius: 8px;
    transition: background 0.12s;
  }

  @media (hover: hover) {
    .nav-date:hover { background: #f0f0ee; }
  }

  .date-input-hidden {
    position: absolute;
    left: 50%;
    top: 100%;
    width: 1px;
    height: 1px;
    opacity: 0;
    pointer-events: none;
  }

  .totals {
    display: flex;
    gap: 1rem;
    font-size: 0.78rem;
    color: #888;
    padding-bottom: 0.1rem;
    padding-top: 0.3rem;
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
    .meal-name:hover { color: #2d2d2d; }
  }

  .meal-add {
    margin-left: 0.35rem;
    font-size: 0.82rem;
    opacity: 0.55;
    font-weight: 600;
  }

  @media (hover: hover) {
    .meal-name:hover .meal-add { opacity: 1; }
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
    .repeat-btn:hover:not(:disabled) { color: #555; }
  }

  .repeat-btn:disabled { cursor: default; }

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
    .empty:hover { color: #888; }
  }

  .state {
    color: #aaa;
    text-align: center;
    margin-top: 4rem;
    font-size: 0.9rem;
  }

  /* Weekly history */
  .week-block {
    border: 1px solid #e8e8e6;
    border-radius: 12px;
    overflow: hidden;
    margin-bottom: 1rem;
  }

  .week-head {
    display: flex;
    justify-content: space-between;
    align-items: center;
    padding: 0.65rem 0.875rem;
    border-bottom: 1px solid #e8e8e6;
  }

  .week-meta {
    display: flex;
    flex-direction: column;
    gap: 0.05rem;
  }

  .week-range {
    font-size: 0.88rem;
    font-weight: 600;
    color: #1c1c1c;
  }

  .week-cal {
    font-size: 0.72rem;
    color: #aaa;
  }

  .insights-btn {
    background: none;
    border: 1px solid #e0e0de;
    border-radius: 999px;
    color: #888;
    font-size: 0.72rem;
    padding: 0.2rem 0.65rem;
    cursor: pointer;
    touch-action: manipulation;
    font-family: inherit;
    letter-spacing: 0.02em;
    white-space: nowrap;
    transition: border-color 0.12s, color 0.12s, background 0.12s;
  }

  .insights-btn.active {
    border-color: #2d2d2d;
    color: #2d2d2d;
    background: #f5f5f3;
  }

  @media (hover: hover) {
    .insights-btn:hover { border-color: #2d2d2d; color: #2d2d2d; }
  }

  .week-grid {
    display: grid;
    grid-template-columns: repeat(7, 1fr);
    padding: 0.4rem 0.25rem 0.5rem;
    gap: 0;
  }

  .day-cell {
    background: none;
    border: none;
    display: flex;
    flex-direction: column;
    align-items: center;
    gap: 0.1rem;
    padding: 0.4rem 0.1rem;
    cursor: pointer;
    border-radius: 8px;
    touch-action: manipulation;
    font-family: inherit;
  }

  @media (hover: hover) {
    .day-cell:not(.future):hover { background: #f0f0ee; }
  }

  .day-cell.future {
    opacity: 0.2;
    cursor: default;
  }

  .dc-abbrev {
    font-size: 0.62rem;
    color: #aaa;
    text-transform: uppercase;
    letter-spacing: 0.03em;
    font-weight: 500;
    line-height: 1;
  }

  .dc-num {
    font-size: 0.85rem;
    font-weight: 500;
    color: #1c1c1c;
    line-height: 1.2;
  }

  .day-cell.has-food .dc-num {
    color: #1c1c1c;
  }

  .dc-indicators {
    display: flex;
    gap: 0.1rem;
    align-items: center;
    min-height: 0.9rem;
  }

  .dc-food {
    font-size: 0.4rem;
    color: #2d2d2d;
    line-height: 1;
  }

  .dc-empty {
    font-size: 0.4rem;
    color: #ddd;
    line-height: 1;
  }

  .dc-poop {
    font-size: 0.5rem;
    line-height: 1;
  }

  .insights-panel {
    padding: 0.75rem 0.875rem;
    border-top: 1px solid #e8e8e6;
    background: #f7f7f5;
  }

  .insights-loading {
    font-size: 0.85rem;
    color: #888;
    font-style: italic;
  }

  .insights-err {
    font-size: 0.85rem;
    color: #c44;
  }

  .insights-text {
    font-size: 0.85rem;
    color: #1c1c1c;
    line-height: 1.65;
    white-space: pre-line;
    margin: 0;
  }

  .insights-text :global(strong) {
    font-weight: 600;
    color: #1c1c1c;
  }

  /* FAB + shared actions */
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
    .fab:hover { background: #1c1c1c; }
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
    .sheet-link:hover { color: #2d2d2d; }
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
    .refresh-btn:hover { color: #2d2d2d; }
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
    .settings-btn:hover { color: #2d2d2d; }
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
    .signout-btn:hover { color: #2d2d2d; }
  }
</style>
