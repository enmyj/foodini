<script>
  let { day, onClose, onDelete = null, onAddFood = null } = $props()

  const MEAL_ORDER = ['breakfast', 'snack', 'lunch', 'dinner']

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

  function onWindowKeyDown(e) {
    if (e.key === 'Escape') onClose()
  }
</script>

<svelte:window onkeydown={onWindowKeyDown} />

<!-- svelte-ignore a11y_click_events_have_key_events -->
<div class="overlay" aria-hidden="true" onclick={onClose}></div>
<div class="modal" role="dialog" aria-modal="true" aria-labelledby="modal-title">
  <div class="modal-header">
    <h2 id="modal-title">{day.date}</h2>
    <button class="close" onclick={onClose}>✕</button>
  </div>

  <div class="modal-section">
    <h3>Food</h3>
    {#each MEAL_ORDER as meal}
      {@const group = (groupedByMeal(day.entries))[meal] ?? []}
      {#if group.length > 0}
        <div class="meal-group">
          <h4>{meal}</h4>
          {#each group as entry}
            <div class="entry-row">
              <div class="entry-main">
                <span class="entry-desc">{entry.description}</span>
                <span class="entry-macros">{entry.calories} cal · {entry.protein}g P · {entry.carbs}g C · {entry.fat}g F · {entry.fiber ?? 0}g Fb</span>
              </div>
              {#if onDelete}
                <button class="entry-del" onclick={() => onDelete(entry.id)} aria-label="Delete">×</button>
              {/if}
            </div>
          {/each}
        </div>
      {/if}
    {/each}
    {#each [totals(day.entries)] as t}
      <div class="day-totals">{t.calories} cal · {t.protein}g P · {t.carbs}g C · {t.fat}g F · {t.fiber}g Fb</div>
    {/each}
    {#if onAddFood}
      <button class="add-food-btn" onclick={() => onAddFood(day.date)}>
        + Add food for this day
      </button>
    {/if}
  </div>

  {#if day.dayLog?.activity}
    <div class="modal-section">
      <h3>Activity</h3>
      <p>{day.dayLog.activity}</p>
    </div>
  {/if}

  {#if day.dayLog?.feeling_notes || day.dayLog?.feeling_score}
    <div class="modal-section">
      <h3>Feeling</h3>
      <p>
        {#if day.dayLog.feeling_notes}{day.dayLog.feeling_notes}{:else}{day.dayLog.feeling_score}/10{/if}
      </p>
    </div>
  {/if}

  {#if day.dayLog?.poop || day.dayLog?.poop_notes}
    <div class="modal-section">
      <h3>💩</h3>
      <p>
        {day.dayLog.poop ? 'Yes' : 'No'}{#if day.dayLog.poop_notes} — {day.dayLog.poop_notes}{/if}
      </p>
    </div>
  {/if}

  {#if day.dayLog?.hydration}
    <div class="modal-section">
      <h3>💧 Water</h3>
      <p>{day.dayLog.hydration} L</p>
    </div>
  {/if}
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
    background: #fafaf9;
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
    font-size: 0.72rem;
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
    align-items: center;
    padding: 0.35rem 0;
    border-bottom: 1px solid #f3f3f2;
    gap: 0.5rem;
  }

  .entry-main {
    display: flex;
    flex-direction: column;
    gap: 0.1rem;
    min-width: 0;
  }

  .entry-desc {
    font-size: 0.88rem;
    color: #1c1c1c;
  }

  .entry-macros {
    font-size: 0.78rem;
    color: #aaa;
  }

  .day-totals {
    margin-top: 0.75rem;
    font-size: 0.8rem;
    color: #888;
    font-weight: 500;
  }

  .entry-del {
    background: none;
    border: none;
    color: #ccc;
    font-size: 0.9rem;
    cursor: pointer;
    padding: 0 0.15rem;
    margin-left: 0.5rem;
    flex-shrink: 0;
    line-height: 1;
  }

  @media (hover: hover) {
    .entry-del:hover {
      color: #888;
    }
  }

  .entry-del:focus-visible,
  .add-food-btn:focus-visible,
  .close:focus-visible {
    outline: 2px solid #2d2d2d;
    outline-offset: 2px;
  }

  .add-food-btn {
    width: 100%;
    margin-top: 1rem;
    padding: 0.5rem 1rem;
    background: #fafaf9;
    color: #2d2d2d;
    border: 1px solid #e8e8e6;
    border-radius: 8px;
    cursor: pointer;
    font-size: 0.88rem;
    font-family: inherit;
  }

  @media (hover: hover) {
    .add-food-btn:hover {
      border-color: #2d2d2d;
    }
  }
</style>
