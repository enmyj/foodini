<script>
  import { onMount } from 'svelte'
  import { getProfile, putProfile } from './api.js'
  import { autosize } from './autosize.js'
  import { showError } from './toast.js'

  let { onClose } = $props()

  let gender = $state('')
  let age = $state('')
  let height = $state('')
  let weight = $state('')
  let notes = $state('')
  let goals = $state('')
  let dietaryRestrictions = $state('')
  let saving = $state(false)
  let loaded = $state(false)

  onMount(async () => {
    try {
      const p = await getProfile()
      gender = p.gender ?? ''
      age = p.age ?? ''
      height = p.height ?? ''
      weight = p.weight ?? ''
      notes = p.notes ?? ''
      goals = p.goals ?? ''
      dietaryRestrictions = p.dietary_restrictions ?? ''
    } catch (err) {
      showError(err, 'Failed to load profile.')
    }
    loaded = true
  })

  async function save() {
    saving = true
    try {
      await putProfile({ gender, age, height, weight, notes, goals, dietary_restrictions: dietaryRestrictions })
      onClose()
    } catch (err) {
      showError(err, 'Failed to save profile.')
    } finally {
      saving = false
    }
  }

  function onKeyDown(e) {
    if (e.key === 'Escape') onClose()
    if (e.key === 'Enter' && (e.metaKey || e.ctrlKey)) save()
  }
</script>

<svelte:window onkeydown={onKeyDown} />

<!-- svelte-ignore a11y_click_events_have_key_events -->
<div class="overlay" aria-hidden="true" onclick={onClose}></div>
<div class="panel" role="dialog" aria-modal="true" aria-labelledby="profile-title">
  <div class="panel-header">
    <h2 id="profile-title">Profile</h2>
    <button class="close" onclick={onClose}>✕</button>
  </div>
  <p class="hint-text">This information helps the AI estimate macros more accurately.</p>

  {#if loaded}
    <div class="fields">
      <label>
        <span>Gender</span>
        <input class="text-entry" type="text" bind:value={gender} placeholder="e.g. male, female, non-binary" disabled={saving} />
      </label>
      <label>
        <span>Age</span>
        <input class="text-entry" type="text" inputmode="numeric" bind:value={age} placeholder="e.g. 34" disabled={saving} />
      </label>
      <label>
        <span>Height</span>
        <input class="text-entry" type="text" bind:value={height} placeholder="e.g. 5'10&quot; or 178cm" disabled={saving} />
      </label>
      <label>
        <span>Weight</span>
        <input class="text-entry" type="text" bind:value={weight} placeholder="e.g. 170lbs or 77kg" disabled={saving} />
      </label>
      <label>
        <span>Notes</span>
        <textarea
          class="text-entry"
          use:autosize
          bind:value={notes}
          placeholder="Dietary restrictions, allergies…"
          rows="3"
          disabled={saving}
        ></textarea>
      </label>
      <label>
        <span>Goals</span>
        <textarea
          class="text-entry"
          use:autosize
          bind:value={goals}
          placeholder="e.g. lose weight, build muscle, eat more protein…"
          rows="3"
          disabled={saving}
        ></textarea>
      </label>
      <label>
        <span>Dietary Restrictions</span>
        <textarea
          class="text-entry"
          use:autosize
          bind:value={dietaryRestrictions}
          placeholder="e.g. vegetarian, no gluten, lactose intolerant…"
          rows="2"
          disabled={saving}
        ></textarea>
      </label>
    </div>
    <div class="actions">
      <button class="save-btn" onclick={save} disabled={saving}>{saving ? 'Saving…' : 'Save'}</button>
      <button class="cancel-btn" onclick={onClose} disabled={saving}>Cancel</button>
    </div>
  {:else}
    <p class="status">Loading…</p>
  {/if}
</div>

<style>
  .overlay {
    position: fixed;
    inset: 0;
    background: rgba(0,0,0,0.2);
    z-index: 20;
  }

  .panel {
    position: fixed;
    top: 50%;
    left: 50%;
    transform: translate(-50%, -50%);
    background: #fafaf9;
    border-radius: 12px;
    width: min(92vw, 420px);
    max-height: 80vh;
    overflow-y: auto;
    z-index: 21;
    padding: 1.5rem;
    box-shadow: 0 4px 24px rgba(0,0,0,0.12);
  }

  .panel-header {
    display: flex;
    justify-content: space-between;
    align-items: center;
    margin-bottom: 0.5rem;
  }

  .panel-header h2 {
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

  .hint-text {
    font-size: 0.82rem;
    color: #888;
    margin-bottom: 1.25rem;
    line-height: 1.5;
  }

  .fields {
    display: flex;
    flex-direction: column;
    gap: 1rem;
  }

  label {
    display: flex;
    flex-direction: column;
    gap: 0.3rem;
  }

  label span {
    font-size: 0.68rem;
    text-transform: uppercase;
    letter-spacing: 0.08em;
    color: #888;
    font-weight: 600;
  }

  .status {
    font-size: 0.78rem;
    color: #aaa;
    margin-top: 0.75rem;
  }

  .actions {
    display: flex;
    gap: 0.5rem;
    margin-top: 1.25rem;
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

  .save-btn:hover:not(:disabled) {
    background: #1c1c1c;
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

  .cancel-btn:hover:not(:disabled) {
    border-color: #888;
  }

</style>
