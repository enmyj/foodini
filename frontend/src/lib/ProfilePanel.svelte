<script>
  import { onMount } from 'svelte'
  import { getProfile, putProfile } from './api.js'

  let { onClose } = $props()

  let gender = $state('')
  let height = $state('')
  let weight = $state('')
  let notes = $state('')
  let saving = $state(false)
  let saveError = $state('')
  let loaded = $state(false)

  onMount(async () => {
    try {
      const p = await getProfile()
      gender = p.gender ?? ''
      height = p.height ?? ''
      weight = p.weight ?? ''
      notes = p.notes ?? ''
    } catch {}
    loaded = true
  })

  async function save() {
    saving = true
    saveError = ''
    try {
      await putProfile({ gender, height, weight, notes })
    } catch {
      saveError = 'Failed to save. Try again.'
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

<!-- svelte-ignore a11y_click_events_have_key_events a11y_no_static_element_interactions -->
<div class="overlay" onclick={onClose}></div>
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
        <input type="text" bind:value={gender} onblur={save} placeholder="e.g. male, female, non-binary" />
      </label>
      <label>
        <span>Height</span>
        <input type="text" bind:value={height} onblur={save} placeholder="e.g. 5'10&quot; or 178cm" />
      </label>
      <label>
        <span>Weight</span>
        <input type="text" bind:value={weight} onblur={save} placeholder="e.g. 170lbs or 77kg" />
      </label>
      <label>
        <span>Notes</span>
        <textarea
          bind:value={notes}
          onblur={save}
          placeholder="Dietary restrictions, goals, allergies…"
          rows="3"
        ></textarea>
      </label>
    </div>
    {#if saving}<p class="status">Saving…</p>{/if}
    {#if saveError}<p class="status error">{saveError}</p>{/if}
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

  input, textarea {
    border: none;
    border-bottom: 2px solid #2d2d2d;
    padding: 0.3rem 0;
    font-size: 0.9rem;
    font-family: inherit;
    background: transparent;
    color: #1c1c1c;
    outline: none;
    resize: vertical;
  }

  .status {
    font-size: 0.78rem;
    color: #aaa;
    margin-top: 0.75rem;
  }

  .status.error {
    color: #b91c1c;
  }
</style>
