<script>
  import { onMount } from 'svelte'
  import { getActivity, putActivity } from './api.js'

  let { date } = $props()

  let activity = $state('')
  let feelingScore = $state(0)
  let feelingNotes = $state('')
  let editingActivity = $state(false)
  let editingFeeling = $state(false)
  let saving = $state(false)
  let saveError = $state('')

  onMount(async () => {
    if (!date) return
    try {
      const res = await getActivity(date)
      activity = res.activity ?? ''
      feelingScore = res.feeling_score ?? 0
      feelingNotes = res.feeling_notes ?? ''
    } catch {}
  })

  async function save() {
    saving = true
    saveError = ''
    try {
      await putActivity(date, { activity, feeling_score: feelingScore, feeling_notes: feelingNotes })
      editingActivity = false
      editingFeeling = false
    } catch {
      saveError = 'Failed to save. Try again.'
    } finally {
      saving = false
    }
  }

  function onKeyDown(e) {
    if (e.key === 'Enter' && (e.metaKey || e.ctrlKey)) save()
    if (e.key === 'Escape') { editingActivity = false; editingFeeling = false }
  }
</script>

<div class="day-notes">
  <div class="section">
    <h3>Activity</h3>
    {#if editingActivity}
      <textarea
        bind:value={activity}
        onblur={save}
        onkeydown={onKeyDown}
        placeholder="Exercise, stress, unusual events…"
        rows="2"
        autofocus
      ></textarea>
    {:else}
      <!-- svelte-ignore a11y_click_events_have_key_events a11y_no_static_element_interactions -->
      <div class="note" class:placeholder={!activity} onclick={() => editingActivity = true}>
        {activity || 'Tap to add activity…'}
      </div>
    {/if}
  </div>

  <div class="section">
    <h3>Feeling</h3>
    {#if editingFeeling}
      <div class="feeling-edit">
        <div class="score-row">
          <span class="score-label">Score (1–10)</span>
          <input
            type="number"
            min="1"
            max="10"
            bind:value={feelingScore}
            onblur={save}
          />
        </div>
        <textarea
          bind:value={feelingNotes}
          onblur={save}
          onkeydown={onKeyDown}
          placeholder="Energy, digestion, mood, sleep…"
          rows="2"
        ></textarea>
      </div>
    {:else}
      <!-- svelte-ignore a11y_click_events_have_key_events a11y_no_static_element_interactions -->
      <div class="note" class:placeholder={!feelingScore && !feelingNotes} onclick={() => editingFeeling = true}>
        {#if feelingScore}
          <span class="score">{feelingScore}/10</span>{#if feelingNotes} — {feelingNotes}{/if}
        {:else}
          {feelingNotes || 'Tap to add how you were feeling…'}
        {/if}
      </div>
    {/if}
  </div>

  {#if saving}<p class="hint">Saving…</p>{/if}
  {#if saveError}<p class="hint error">{saveError}</p>{/if}
</div>

<style>
  .day-notes {
    margin-top: 2rem;
    border-top: 1px solid #e8e8e6;
    padding-top: 1.25rem;
    display: flex;
    flex-direction: column;
    gap: 1.25rem;
  }

  .section h3 {
    text-transform: uppercase;
    font-size: 0.68rem;
    color: #888;
    letter-spacing: 0.08em;
    font-weight: 600;
    margin-bottom: 0.4rem;
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

  .score {
    font-weight: 500;
  }

  .feeling-edit {
    display: flex;
    flex-direction: column;
    gap: 0.5rem;
  }

  .score-row {
    display: flex;
    align-items: center;
    gap: 0.75rem;
  }

  .score-label {
    font-size: 0.82rem;
    color: #888;
  }

  .score-row input {
    width: 56px;
    border: none;
    border-bottom: 2px solid #2d2d2d;
    padding: 2px 4px;
    font-family: inherit;
    font-size: 0.95rem;
    background: transparent;
    outline: none;
  }

  textarea {
    width: 100%;
    border: none;
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
