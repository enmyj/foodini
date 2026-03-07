<script>
  import { onMount } from 'svelte'
  import { getActivity, putActivity } from './api.js'

  let { date } = $props()

  let notes = $state('')
  let editing = $state(false)
  let saving = $state(false)
  let saveError = $state('')

  onMount(async () => {
    if (!date) return
    try {
      const res = await getActivity(date)
      notes = res.notes ?? ''
    } catch {}
  })

  async function save() {
    saving = true
    saveError = ''
    try {
      await putActivity(date, notes)
      editing = false
    } catch (e) {
      saveError = 'Failed to save. Try again.'
    } finally {
      saving = false
    }
  }

  function onKeyDown(e) {
    if (e.key === 'Enter' && (e.metaKey || e.ctrlKey)) save()
    if (e.key === 'Escape') editing = false
  }
</script>

<div class="activity">
  <h3>Activity / Notes</h3>
  {#if editing}
    <textarea
      bind:value={notes}
      onblur={save}
      onkeydown={onKeyDown}
      placeholder="What did you do today? (exercise, stress, unusual events…)"
      rows="3"
      autofocus
    ></textarea>
    {#if saving}<span class="hint">Saving…</span>{/if}
    {#if saveError}<span class="hint error">{saveError}</span>{/if}
  {:else}
    <!-- svelte-ignore a11y_click_events_have_key_events a11y_no_static_element_interactions -->
    <div class="note" class:placeholder={!notes} onclick={() => editing = true}>
      {notes || 'Tap to add activity notes…'}
    </div>
  {/if}
</div>

<style>
  .activity {
    margin-top: 2rem;
    padding-top: 1.25rem;
    border-top: 1px solid #e8e8e6;
  }

  h3 {
    text-transform: uppercase;
    font-size: 0.68rem;
    color: #888;
    letter-spacing: 0.08em;
    font-weight: 600;
    margin-bottom: 0.5rem;
  }

  .note {
    color: #1c1c1c;
    font-size: 0.9rem;
    cursor: pointer;
    line-height: 1.55;
    min-height: 1.55rem;
    padding: 0.2rem 0;
  }

  .note.placeholder {
    color: #bbb;
  }

  textarea {
    width: 100%;
    border: 1px solid #e8e8e6;
    border-bottom: 2px solid #2d2d2d;
    border-radius: 0;
    padding: 0.4rem 0;
    font-size: 0.9rem;
    font-family: inherit;
    resize: vertical;
    box-sizing: border-box;
    background: transparent;
    color: #1c1c1c;
  }

  textarea:focus {
    outline: none;
  }

  .hint {
    font-size: 0.75rem;
    color: #aaa;
    margin-top: 0.25rem;
  }

  .hint.error {
    color: #b91c1c;
  }
</style>
