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
    <div class="note" onclick={() => editing = true}>
      {notes || 'Tap to add activity notes…'}
    </div>
  {/if}
</div>

<style>
  .activity { margin-top: 2rem; padding-top: 1.25rem; border-top: 1px solid #eee; }
  h3 { font-size: 0.75rem; color: #999; letter-spacing: 0.06em; margin-bottom: 0.5rem; }
  .note { color: #555; font-size: 0.9rem; cursor: pointer; line-height: 1.5; min-height: 1.5rem; padding: 0.2rem 0; }
  .note:hover { color: #888; }
  textarea { width: 100%; border: 1px solid #4285f4; border-radius: 6px; padding: 0.5rem; font-size: 0.9rem; font-family: inherit; resize: vertical; box-sizing: border-box; }
  textarea:focus { outline: none; }
  .hint { font-size: 0.78rem; color: #aaa; }
  .hint.error { color: #c62828; }
</style>
