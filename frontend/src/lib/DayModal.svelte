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
