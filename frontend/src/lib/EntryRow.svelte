<script>
  import { patchEntry, deleteEntry } from './api.js'

  let { entry, onUpdate, onDelete } = $props()

  const MEALS = ['breakfast', 'snack', 'lunch', 'dinner']

  let modalOpen = $state(false)
  let editDesc = $state('')
  let editMeal = $state('')
  let editCal = $state(0)
  let editProtein = $state(0)
  let editCarbs = $state(0)
  let editFat = $state(0)
  let editFiber = $state(0)
  let saving = $state(false)
  let deleting = $state(false)
  let pendingDelete = $state(false)
  let deleteTimer = null

  function openModal() {
    editDesc = entry.description
    editMeal = entry.meal_type
    editCal = entry.calories
    editProtein = entry.protein
    editCarbs = entry.carbs
    editFat = entry.fat
    editFiber = entry.fiber ?? 0
    modalOpen = true
  }

  async function save() {
    if (saving) return
    saving = true
    try {
      const updated = {
        ...entry,
        description: editDesc,
        meal_type: editMeal,
        calories: editCal,
        protein: editProtein,
        carbs: editCarbs,
        fat: editFat,
        fiber: editFiber,
      }
      const saved = await patchEntry(entry.id, updated)
      onUpdate(saved)
      modalOpen = false
    } catch (e) {
      console.error('patch failed', e)
    } finally {
      saving = false
    }
  }

  function handleDelete() {
    if (deleting) return
    if (!pendingDelete) {
      pendingDelete = true
      deleteTimer = setTimeout(() => { pendingDelete = false }, 2500)
      return
    }
    clearTimeout(deleteTimer)
    pendingDelete = false
    doDelete()
  }

  async function doDelete() {
    deleting = true
    try {
      await deleteEntry(entry.id)
      onDelete(entry.id)
    } catch (e) {
      console.error('delete failed', e)
      deleting = false
    }
  }

  function onKeyDown(e) {
    if (e.key === 'Escape') modalOpen = false
    if (e.key === 'Enter' && (e.metaKey || e.ctrlKey)) save()
  }
</script>

<div class="row" class:fading={deleting}>
  <!-- svelte-ignore a11y_click_events_have_key_events -->
  <div class="main" role="button" tabindex="0" onclick={openModal}>
    <span class="desc">{entry.description}</span>
    <span class="macros">{entry.calories} cal · {entry.protein}g P · {entry.carbs}g C · {entry.fat}g F{entry.fiber ? ` · ${entry.fiber}g Fb` : ''}</span>
  </div>
  <button class="del" class:confirm={pendingDelete} onclick={handleDelete} disabled={deleting} aria-label={pendingDelete ? 'Tap again to confirm delete' : 'Delete entry'}>{pendingDelete ? '?' : '×'}</button>
</div>

{#if modalOpen}
  <!-- svelte-ignore a11y_click_events_have_key_events -->
  <div class="overlay" aria-hidden="true" onclick={() => modalOpen = false}></div>
  <div class="modal" role="dialog" aria-label="Edit entry" tabindex="-1" onkeydown={onKeyDown}>
    <h3>Edit entry</h3>

    <label class="field">
      <span class="label">Description</span>
      <textarea bind:value={editDesc} rows="2" disabled={saving}></textarea>
    </label>

    <label class="field">
      <span class="label">Meal</span>
      <select bind:value={editMeal} disabled={saving}>
        {#each MEALS as m}
          <option value={m}>{m}</option>
        {/each}
      </select>
    </label>

    <div class="num-grid">
      <label class="field">
        <span class="label">Calories</span>
        <input type="number" bind:value={editCal} disabled={saving} />
      </label>
      <label class="field">
        <span class="label">Protein (g)</span>
        <input type="number" bind:value={editProtein} disabled={saving} />
      </label>
      <label class="field">
        <span class="label">Carbs (g)</span>
        <input type="number" bind:value={editCarbs} disabled={saving} />
      </label>
      <label class="field">
        <span class="label">Fat (g)</span>
        <input type="number" bind:value={editFat} disabled={saving} />
      </label>
      <label class="field">
        <span class="label">Fiber (g)</span>
        <input type="number" bind:value={editFiber} disabled={saving} />
      </label>
    </div>

    <div class="actions">
      <button class="save-btn" onclick={save} disabled={saving}>{saving ? 'Saving…' : 'Save'}</button>
      <button class="cancel-btn" onclick={() => modalOpen = false} disabled={saving}>Cancel</button>
    </div>
  </div>
{/if}

<style>
  .row {
    display: flex;
    align-items: center;
    padding: 0.75rem 0;
    border-bottom: 1px solid #e8e8e6;
    gap: 0.5rem;
  }

  .row.fading {
    opacity: 0.4;
  }

  .main {
    flex: 1;
    min-width: 0;
    cursor: pointer;
    display: flex;
    flex-direction: column;
    gap: 0.25rem;
  }

  .main:hover .desc {
    color: #555;
  }

  .desc {
    font-size: 0.95rem;
    line-height: 1.4;
    color: #1c1c1c;
  }

  .macros {
    font-size: 0.78rem;
    color: #888;
    line-height: 1.3;
  }

  .del {
    background: none;
    border: none;
    color: #ccc;
    font-size: 1.1rem;
    line-height: 1;
    cursor: pointer;
    padding: 0;
    flex-shrink: 0;
    min-width: 2.75rem;
    min-height: 2.75rem;
    display: flex;
    align-items: center;
    justify-content: center;
    touch-action: manipulation;
  }

  .del.confirm {
    color: #e11d48;
    font-weight: 600;
  }

  @media (hover: hover) {
    .del:hover {
      color: #888;
    }
  }

  .del:focus-visible {
    outline: 2px solid #2d2d2d;
    outline-offset: 2px;
  }

  .del:disabled {
    opacity: 0.35;
    cursor: default;
  }

  /* Modal */
  .overlay {
    position: fixed;
    inset: 0;
    background: rgba(0,0,0,0.25);
    z-index: 30;
  }

  .modal {
    position: fixed;
    top: 50%;
    left: 50%;
    transform: translate(-50%, -50%);
    background: #fafaf9;
    border-radius: 12px;
    width: min(92vw, 420px);
    max-height: 85vh;
    overflow-y: auto;
    z-index: 31;
    padding: 1.5rem;
    box-shadow: 0 4px 24px rgba(0,0,0,0.12);
  }

  .modal h3 {
    font-size: 0.95rem;
    font-weight: 600;
    color: #1c1c1c;
    margin-bottom: 1.25rem;
    text-transform: none;
    letter-spacing: 0;
  }

  .field {
    display: flex;
    flex-direction: column;
    gap: 0.3rem;
    margin-bottom: 1rem;
  }

  .label {
    font-size: 0.72rem;
    font-weight: 600;
    text-transform: uppercase;
    letter-spacing: 0.06em;
    color: #888;
  }

  textarea, input, select {
    border: 1px solid #e8e8e6;
    border-radius: 6px;
    padding: 0.5rem 0.6rem;
    font-family: inherit;
    font-size: 1rem;
    background: #fff;
    color: #1c1c1c;
    outline: none;
    width: 100%;
    box-sizing: border-box;
  }

  textarea:focus, input:focus, select:focus {
    border-color: #2d2d2d;
  }

  textarea {
    resize: none;
    line-height: 1.4;
  }

  select {
    appearance: none;
    background-image: url("data:image/svg+xml,%3Csvg xmlns='http://www.w3.org/2000/svg' width='12' height='8' viewBox='0 0 12 8'%3E%3Cpath d='M1 1l5 5 5-5' stroke='%23888' stroke-width='1.5' fill='none' stroke-linecap='round'/%3E%3C/svg%3E");
    background-repeat: no-repeat;
    background-position: right 0.6rem center;
    padding-right: 2rem;
    cursor: pointer;
  }

  .num-grid {
    display: grid;
    grid-template-columns: repeat(3, 1fr);
    gap: 0.75rem;
    margin-bottom: 1rem;
  }

  .num-grid .field {
    margin-bottom: 0;
  }

  .actions {
    display: flex;
    gap: 0.5rem;
    margin-top: 0.25rem;
  }

  .save-btn {
    flex: 1;
    padding: 0.6rem 1rem;
    background: #2d2d2d;
    color: #fafaf9;
    border: none;
    border-radius: 8px;
    cursor: pointer;
    font-size: 0.9rem;
    font-family: inherit;
    font-weight: 500;
    touch-action: manipulation;
  }

  @media (hover: hover) {
    .save-btn:hover:not(:disabled) {
      background: #1c1c1c;
    }
  }

  .save-btn:disabled {
    opacity: 0.5;
    cursor: default;
  }

  .cancel-btn {
    padding: 0.6rem 1rem;
    background: none;
    color: #888;
    border: 1px solid #e8e8e6;
    border-radius: 8px;
    cursor: pointer;
    font-size: 0.9rem;
    font-family: inherit;
    touch-action: manipulation;
  }

  @media (hover: hover) {
    .cancel-btn:hover:not(:disabled) {
      border-color: #888;
    }
  }
</style>
