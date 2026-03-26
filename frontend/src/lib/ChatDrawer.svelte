<script>
  import { chat, confirmChat, getActivity, putActivity } from './api.js'

  let { open, onClose, onEntriesAdded, date = null, meal = null, initialTab = 'food' } = $props()

  const MEALS = ['breakfast', 'snack', 'lunch', 'dinner']

  function todayStr() {
    const d = new Date()
    return [d.getFullYear(), String(d.getMonth()+1).padStart(2,'0'), String(d.getDate()).padStart(2,'0')].join('-')
  }

  // Shared
  let tab = $state('food') // 'food' | 'activity'
  let selectedDate = $state('')
  let drawerEl = $state(null)

  // Food
  let messages = $state([])
  let input = $state('')
  let caption = $state('')
  let sending = $state(false)
  let pendingEntries = $state(null)
  let pendingImage = $state(null)
  let mode = $state('tiles') // 'tiles' | 'describe'
  let selectedMeal = $state(null)
  let inputEl = $state(null)
  let captionEl = $state(null)
  let messagesEl = $state(null)
  let fileInputEl = $state(null)

  // Activity
  let activityText = $state('')
  let feelingScore = $state('')
  let feelingNotes = $state('')
  let poop = $state(false)
  let poopNotes = $state('')
  let activitySaving = $state(false)
  let activityError = $state('')
  let activityLoadedFor = $state(null) // date string for which data is loaded

  // Drag-to-dismiss
  let dragStartY = null
  let dragCurrentY = 0

  function onDragStart(e) {
    const tag = e.target.tagName
    if (tag === 'TEXTAREA' || tag === 'INPUT' || tag === 'BUTTON' || tag === 'SELECT') return
    dragStartY = e.touches[0].clientY
    dragCurrentY = 0
    if (drawerEl) drawerEl.style.transition = 'none'
  }

  function onDragMove(e) {
    if (dragStartY === null) return
    const dy = e.touches[0].clientY - dragStartY
    if (dy < 0) return
    dragCurrentY = dy
    if (drawerEl) drawerEl.style.transform = `translateY(${dy}px)`
  }

  function onDragEnd() {
    if (dragStartY === null) return
    dragStartY = null
    if (drawerEl) {
      drawerEl.style.transition = ''
      if (dragCurrentY > 120) {
        drawerEl.style.transform = ''
        onClose()
      } else {
        drawerEl.style.transform = ''
      }
    }
    dragCurrentY = 0
  }

  $effect(() => {
    if (open) {
      tab = initialTab
      selectedDate = date || todayStr()
      selectedMeal = meal
      mode = 'tiles'
      messages = []
      input = ''
      caption = ''
      pendingEntries = null
      pendingImage = null
      activityError = ''
    } else {
      tab = 'food'
      selectedDate = ''
      selectedMeal = null
      mode = 'tiles'
      messages = []
      input = ''
      caption = ''
      pendingEntries = null
      pendingImage = null
      activityText = ''
      feelingScore = ''
      feelingNotes = ''
      poop = false
      poopNotes = ''
      activitySaving = false
      activityError = ''
      activityLoadedFor = null
    }
  })

  // Load activity data when tab is activity and date changes
  $effect(() => {
    if (open && tab === 'activity' && selectedDate && selectedDate !== activityLoadedFor) {
      loadActivity(selectedDate)
    }
  })

  async function loadActivity(d) {
    activityLoadedFor = d
    try {
      const res = await getActivity(d)
      activityText = res.activity ?? ''
      feelingScore = res.feeling_score ? String(res.feeling_score) : ''
      feelingNotes = res.feeling_notes ?? ''
      poop = res.poop ?? false
      poopNotes = res.poop_notes ?? ''
    } catch {}
  }

  async function saveActivity() {
    activitySaving = true
    activityError = ''
    try {
      await putActivity(selectedDate, {
        activity: activityText,
        feeling_score: feelingScore ? parseInt(feelingScore) : 0,
        feeling_notes: feelingNotes,
        poop,
        poop_notes: poopNotes,
      })
      onClose()
    } catch {
      activityError = 'Failed to save. Try again.'
    } finally {
      activitySaving = false
    }
  }

  $effect(() => {
    messages
    sending
    if (messagesEl) messagesEl.scrollTop = messagesEl.scrollHeight
  })

  function activateDescribe() {
    mode = 'describe'
    setTimeout(() => inputEl?.focus(), 30)
  }

  function activatePhoto() {
    fileInputEl.click()
  }

  async function compressImage(file) {
    return new Promise((resolve) => {
      const img = new Image()
      const url = URL.createObjectURL(file)
      img.onload = () => {
        URL.revokeObjectURL(url)
        const MAX = 1024
        const scale = Math.min(1, MAX / Math.max(img.width, img.height))
        const canvas = document.createElement('canvas')
        canvas.width = Math.round(img.width * scale)
        canvas.height = Math.round(img.height * scale)
        canvas.getContext('2d').drawImage(img, 0, 0, canvas.width, canvas.height)
        canvas.toBlob(blob => {
          const reader = new FileReader()
          reader.onload = () => {
            const dataUrl = reader.result
            resolve({ data: dataUrl.split(',')[1], mimeType: 'image/jpeg', previewUrl: dataUrl })
          }
          reader.readAsDataURL(blob)
        }, 'image/jpeg', 0.82)
      }
      img.src = url
    })
  }

  async function onFileSelected(e) {
    const file = e.target.files?.[0]
    if (!file) return
    pendingImage = await compressImage(file)
    fileInputEl.value = ''
    setTimeout(() => captionEl?.focus(), 30)
  }

  async function send() {
    if (sending) return
    const img = pendingImage
    const text = img ? caption.trim() : input.trim()
    if (!img && !text) return
    messages = [...messages, { role: 'user', text, previewUrl: img?.previewUrl }]
    input = ''
    caption = ''
    pendingEntries = null
    pendingImage = null
    sending = true
    try {
      const imagePayload = img ? { data: img.data, mime_type: img.mimeType } : null
      const res = await chat(text, selectedDate, imagePayload, selectedMeal)
      if (res.pending) {
        pendingEntries = res.entries
        messages = [...messages, { role: 'assistant', entries: res.entries }]
      } else {
        messages = [...messages, { role: 'assistant', text: res.message }]
      }
    } catch {
      messages = [...messages, { role: 'assistant', text: 'Something went wrong. Please try again.' }]
    } finally {
      sending = false
    }
  }

  async function confirm() {
    if (!pendingEntries || sending) return
    sending = true
    try {
      const res = await confirmChat(pendingEntries, selectedDate)
      messages = [...messages, { role: 'assistant', text: 'Saved!' }]
      onEntriesAdded(res.entries)
    } catch {
      messages = [...messages, { role: 'assistant', text: 'Failed to save. Please try again.' }]
    } finally {
      sending = false
      pendingEntries = null
    }
  }

  function onKeyDown(e) {
    if (e.key === 'Enter' && !e.shiftKey) {
      e.preventDefault()
      send()
    }
  }
</script>

{#if open}
  <!-- svelte-ignore a11y_click_events_have_key_events -->
  <div class="overlay" aria-hidden="true" onclick={onClose}></div>
  <div class="drawer" role="dialog" aria-label="Log" tabindex="-1"
    bind:this={drawerEl}
    ontouchstart={onDragStart}
    ontouchmove={onDragMove}
    ontouchend={onDragEnd}
  >
    <div class="handle"></div>

    <!-- Tab switcher -->
    <div class="tabs">
      <button class="tab-btn" class:active={tab === 'food'} onclick={() => tab = 'food'}>{tab === 'food' ? '🌯 ' : ''}Food</button>
      <button class="tab-btn" class:active={tab === 'activity'} onclick={() => tab = 'activity'}>{tab === 'activity' ? '🌯 ' : ''}Activity</button>
    </div>

    <!-- Date row (shared) -->
    <div class="date-wrap">
      <span class="date-label">Date</span>
      <input class="date-input" type="date" bind:value={selectedDate} max={todayStr()} />
    </div>

    {#if tab === 'food'}
      <!-- Meal pills -->
      {#if messages.length === 0}
        <div class="meal-pills-wrap">
          <span class="meal-pills-label">Meal</span>
          <div class="meal-pills">
            {#each MEALS as m}
              <button
                class="meal-pill"
                class:selected={selectedMeal === m}
                onclick={() => selectedMeal = selectedMeal === m ? null : m}
              >{m}</button>
            {/each}
          </div>
        </div>
      {:else}
        <div class="meta-locked">
          {#if selectedMeal}<span class="meta-chip">{selectedMeal}</span>{/if}
        </div>
      {/if}

      <!-- Messages -->
      <div class="messages" bind:this={messagesEl}>
        {#each messages as msg}
          {#if msg.entries}
            <div class="msg assistant">
              {#each msg.entries as e}
                <div class="entry-line">
                  <span class="entry-desc">{e.description}</span>
                  <span class="entry-meta">({e.meal_type}) — {e.calories} cal, {e.protein}g P, {e.carbs}g C, {e.fat}g F{e.fiber ? `, ${e.fiber}g Fb` : ''}</span>
                </div>
              {/each}
            </div>
          {:else}
            <div class="msg {msg.role}">
              {#if msg.previewUrl}<img src={msg.previewUrl} alt="" class="msg-img" />{/if}
              {#if msg.text}{msg.text}{/if}
            </div>
          {/if}
        {/each}
        {#if sending}
          <div class="msg assistant typing">…</div>
        {/if}
      </div>

      <input bind:this={fileInputEl} type="file" accept="image/*" class="file-input" onchange={onFileSelected} />

      {#if mode === 'tiles' && messages.length === 0 && !pendingImage}
        <div class="input-tiles">
          <button class="tile" onclick={activatePhoto} disabled={sending}>
            <svg width="22" height="22" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="1.75" stroke-linecap="round" stroke-linejoin="round"><path d="M23 19a2 2 0 0 1-2 2H3a2 2 0 0 1-2-2V8a2 2 0 0 1 2-2h4l2-3h6l2 3h4a2 2 0 0 1 2 2z"/><circle cx="12" cy="13" r="4"/></svg>
            Photo
          </button>
          <button class="tile" onclick={activateDescribe} disabled={sending}>
            <svg width="22" height="22" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="1.75" stroke-linecap="round" stroke-linejoin="round"><path d="M21 15a2 2 0 0 1-2 2H7l-4 4V5a2 2 0 0 1 2-2h14a2 2 0 0 1 2 2z"/></svg>
            Describe
          </button>
        </div>
      {:else if pendingImage}
        <div class="photo-card">
          <div class="photo-preview">
            <img src={pendingImage.previewUrl} alt="Selected meal" />
            <button class="photo-remove" onclick={() => pendingImage = null} aria-label="Remove photo">✕</button>
            <button class="photo-replace" onclick={() => fileInputEl.click()} disabled={sending} aria-label="Replace photo" title="Replace photo">
              <svg width="16" height="16" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"><path d="M23 19a2 2 0 0 1-2 2H3a2 2 0 0 1-2-2V8a2 2 0 0 1 2-2h4l2-3h6l2 3h4a2 2 0 0 1 2 2z"/><circle cx="12" cy="13" r="4"/></svg>
            </button>
          </div>
          <div class="caption-row">
            <textarea bind:this={captionEl} bind:value={caption} onkeydown={onKeyDown} placeholder="Add a note… (optional)" rows="1" disabled={sending}></textarea>
            <button onclick={send} disabled={sending || !selectedMeal}>Send</button>
          </div>
        </div>
      {:else}
        <div class="input-row">
          <textarea bind:this={inputEl} bind:value={input} onkeydown={onKeyDown} placeholder="What did you eat?" rows="2" disabled={sending}></textarea>
          <button onclick={send} disabled={sending || !input.trim() || !selectedMeal}>Send</button>
        </div>
      {/if}

      {#if pendingEntries}
        <button class="confirm-btn" onclick={confirm} disabled={sending}>Looks good, save it</button>
      {/if}

    {:else}
      <!-- Activity form -->
      <div class="activity-form">
        <div class="activity-field">
          <label class="field-label" for="act-activity">Activity</label>
          <textarea id="act-activity" bind:value={activityText} placeholder="Exercise, stress, unusual events…" rows="2"></textarea>
        </div>
        <div class="activity-field">
          <label class="field-label" for="act-feeling-score">Feeling</label>
          <div class="feeling-row">
            <input id="act-feeling-score" class="score-input" type="number" min="1" max="10" bind:value={feelingScore} placeholder="1–10" />
            <textarea bind:value={feelingNotes} placeholder="Energy, digestion, mood, sleep…" rows="2"></textarea>
          </div>
        </div>
        <div class="activity-field">
          <span class="field-label">💩</span>
          <div class="poop-row">
            <button class="toggle-btn" class:selected={poop === true} onclick={() => poop = true}>Yes</button>
            <button class="toggle-btn" class:selected={poop === false && poopNotes === '' && activityLoadedFor !== null} onclick={() => poop = false}>No</button>
            <textarea bind:value={poopNotes} placeholder="Any details…" rows="1"></textarea>
          </div>
        </div>
        {#if activityError}
          <p class="activity-error">{activityError}</p>
        {/if}
        <button class="save-activity-btn" onclick={saveActivity} disabled={activitySaving}>
          {activitySaving ? 'Saving…' : 'Save'}
        </button>
      </div>
    {/if}
  </div>
{/if}

<style>
  .overlay {
    position: fixed;
    inset: 0;
    background: rgba(0,0,0,0.2);
    z-index: 10;
  }

  .drawer {
    position: fixed;
    bottom: 0;
    left: 0;
    right: 0;
    background: #fafaf9;
    border-radius: 16px 16px 0 0;
    box-shadow: 0 -2px 16px rgba(0,0,0,0.08);
    z-index: 11;
    display: flex;
    flex-direction: column;
    height: min(60vh, 460px);
    padding: 0.75rem 1.25rem 1.5rem;
    transition: transform 0.2s ease;
    will-change: transform;
  }

  .handle {
    width: 36px;
    height: 3px;
    background: #e8e8e6;
    border-radius: 2px;
    margin: 0 auto 0.75rem;
  }

  /* --- Tabs --- */
  .tabs {
    display: flex;
    gap: 0.4rem;
    margin-bottom: 0.75rem;
  }

  .tab-btn {
    background: none;
    border: none;
    border-radius: 999px;
    padding: 0.25rem 0.75rem;
    font-size: 0.85rem;
    font-weight: 500;
    color: #888;
    cursor: pointer;
    font-family: inherit;
  }

  .tab-btn.active {
    background: #efefed;
    color: #1c1c1c;
  }

  /* --- Date row --- */
  .date-wrap {
    display: flex;
    align-items: center;
    gap: 0.6rem;
    margin-bottom: 0.5rem;
  }

  .date-label {
    font-size: 0.72rem;
    text-transform: uppercase;
    letter-spacing: 0.08em;
    font-weight: 600;
    color: #aaa;
    flex-shrink: 0;
  }

  .date-input {
    border: 1px solid #d0d0ce;
    border-radius: 8px;
    padding: 0.3rem 0.6rem;
    font-size: 0.8rem;
    font-family: inherit;
    color: #555;
    font-weight: 500;
    background: #fafaf9;
  }

  .date-input:focus {
    outline: none;
    border-color: #2d2d2d;
  }

  /* --- Meal pills --- */
  .meal-pills-wrap {
    display: flex;
    align-items: center;
    gap: 0.6rem;
    flex-wrap: wrap;
    margin-bottom: 0.75rem;
  }

  .meal-pills-label {
    font-size: 0.72rem;
    text-transform: uppercase;
    letter-spacing: 0.08em;
    font-weight: 600;
    color: #aaa;
    flex-shrink: 0;
  }

  .meal-pills {
    display: flex;
    gap: 0.4rem;
    flex-wrap: wrap;
  }

  .meal-pill {
    padding: 0.3rem 0.75rem;
    border: 1px solid #d0d0ce;
    border-radius: 999px;
    background: none;
    font-family: inherit;
    font-size: 0.8rem;
    color: #555;
    cursor: pointer;
    white-space: nowrap;
    touch-action: manipulation;
    font-weight: 500;
  }

  .meal-pill:hover:not(:disabled) {
    border-color: #2d2d2d;
    color: #2d2d2d;
  }

  .meal-pill.selected {
    background: #2d2d2d;
    border-color: #2d2d2d;
    color: #fafaf9;
  }

  .meta-locked {
    margin-bottom: 0.5rem;
  }

  .meta-chip {
    font-size: 0.72rem;
    text-transform: uppercase;
    letter-spacing: 0.06em;
    color: #aaa;
    font-weight: 600;
  }

  /* --- Food: messages --- */
  .messages {
    flex: 1;
    overflow-y: auto;
    display: flex;
    flex-direction: column;
    gap: 0.6rem;
    margin-bottom: 0.75rem;
    padding: 0.25rem 0;
  }

  .msg {
    padding: 0.5rem 0.75rem;
    border-radius: 12px;
    max-width: 85%;
    font-size: 0.9rem;
    line-height: 1.45;
  }

  .msg.user {
    background: #2d2d2d;
    color: #fafaf9;
    align-self: flex-end;
  }

  .msg.assistant {
    background: #f3f3f2;
    color: #1c1c1c;
    align-self: flex-start;
  }

  .typing { color: #bbb; }

  .entry-line {
    line-height: 1.55;
    display: flex;
    flex-wrap: wrap;
    gap: 0.2rem 0.4rem;
    align-items: baseline;
  }

  .entry-line + .entry-line { margin-top: 0.25rem; }
  .entry-desc { font-weight: 500; }
  .entry-meta { color: #888; font-size: 0.82rem; }

  .msg-img {
    display: block;
    max-width: 100%;
    max-height: 180px;
    border-radius: 6px;
    margin-bottom: 0.3rem;
  }

  .file-input { display: none; }

  /* --- Tiles --- */
  .input-tiles {
    display: flex;
    gap: 0.75rem;
  }

  .tile {
    flex: 1;
    padding: 1.1rem 0.5rem;
    border: 1px solid #e8e8e6;
    border-radius: 10px;
    background: none;
    display: flex;
    flex-direction: column;
    align-items: center;
    gap: 0.45rem;
    cursor: pointer;
    font-family: inherit;
    font-size: 0.85rem;
    color: #888;
    touch-action: manipulation;
  }

  .tile:hover:not(:disabled) { border-color: #2d2d2d; color: #2d2d2d; }
  .tile:disabled { opacity: 0.35; cursor: default; }

  /* --- Photo card --- */
  .photo-card { display: flex; flex-direction: column; gap: 0.6rem; }

  .photo-preview {
    position: relative;
    align-self: center;
    width: 100%;
    max-width: 300px;
  }

  .photo-preview img {
    display: block;
    width: 100%;
    max-height: 200px;
    object-fit: cover;
    border-radius: 12px;
    border: 1px solid #e8e8e6;
  }

  .photo-remove {
    position: absolute;
    top: -8px; right: -8px;
    width: 22px; height: 22px;
    border-radius: 50%;
    background: #2d2d2d;
    color: #fafaf9;
    border: none;
    font-size: 0.65rem;
    cursor: pointer;
    display: flex;
    align-items: center;
    justify-content: center;
    padding: 0;
    line-height: 1;
    z-index: 1;
  }

  .photo-replace {
    position: absolute;
    bottom: 8px; right: 8px;
    width: 30px; height: 30px;
    border-radius: 50%;
    background: rgba(0,0,0,0.45);
    color: #fff;
    border: none;
    cursor: pointer;
    display: flex;
    align-items: center;
    justify-content: center;
    padding: 0;
  }

  .photo-replace:hover:not(:disabled) { background: rgba(0,0,0,0.65); }
  .photo-replace:disabled { opacity: 0.35; cursor: default; }

  .caption-row { display: flex; gap: 0.5rem; align-items: flex-end; }

  .caption-row textarea {
    flex: 1;
    border: 1px solid #e8e8e6;
    border-radius: 8px;
    padding: 0.5rem 0.75rem;
    font-size: 1rem;
    resize: none;
    font-family: inherit;
    background: #fafaf9;
    color: #1c1c1c;
  }

  .caption-row textarea:focus { outline: none; border-color: #2d2d2d; }

  /* --- Describe / chat --- */
  .input-row { display: flex; gap: 0.5rem; align-items: flex-end; }

  textarea {
    flex: 1;
    border: 1px solid #e8e8e6;
    border-radius: 8px;
    padding: 0.5rem 0.75rem;
    font-size: 1rem;
    resize: none;
    font-family: inherit;
    background: #fafaf9;
    color: #1c1c1c;
  }

  textarea:focus { outline: none; border-color: #2d2d2d; }

  button {
    padding: 0.5rem 1rem;
    background: #2d2d2d;
    color: #fafaf9;
    border: none;
    border-radius: 8px;
    cursor: pointer;
    font-size: 0.9rem;
    font-family: inherit;
    white-space: nowrap;
  }

  button:disabled { opacity: 0.35; cursor: default; }

  .confirm-btn {
    width: 100%;
    margin-top: 0.5rem;
    padding: 0.75rem 1rem;
    background: #16a34a;
    color: #fff;
    border-radius: 8px;
    font-size: 0.95rem;
    font-weight: 600;
    touch-action: manipulation;
  }

  .confirm-btn:not(:disabled):hover { background: #15803d; }
  .confirm-btn:disabled { opacity: 0.35; cursor: default; }

  /* --- Activity form --- */
  .activity-form {
    display: flex;
    flex-direction: column;
    gap: 1rem;
    overflow-y: auto;
    flex: 1;
  }

  .activity-field {
    display: flex;
    flex-direction: column;
    gap: 0.35rem;
  }

  .field-label {
    font-size: 0.72rem;
    text-transform: uppercase;
    letter-spacing: 0.08em;
    font-weight: 600;
    color: #aaa;
  }

  .feeling-row {
    display: flex;
    gap: 0.5rem;
    align-items: center;
  }

  .score-input {
    width: 58px;
    flex-shrink: 0;
    border: 1px solid #e8e8e6;
    border-radius: 8px;
    padding: 0.5rem 0.5rem;
    font-size: 1rem;
    font-family: inherit;
    background: #fafaf9;
    color: #1c1c1c;
    text-align: center;
  }

  .score-input:focus { outline: none; border-color: #2d2d2d; }

  .poop-row {
    display: flex;
    gap: 0.4rem;
    align-items: center;
    flex-wrap: wrap;
  }

  .toggle-btn {
    padding: 0.3rem 0.85rem;
    border: 1px solid #d0d0ce;
    border-radius: 999px;
    background: none;
    font-family: inherit;
    font-size: 0.8rem;
    color: #555;
    cursor: pointer;
    font-weight: 500;
    flex-shrink: 0;
    touch-action: manipulation;
  }

  .toggle-btn.selected {
    background: #2d2d2d;
    border-color: #2d2d2d;
    color: #fafaf9;
  }

  .activity-error {
    font-size: 0.82rem;
    color: #b91c1c;
  }

  .save-activity-btn {
    width: 100%;
    padding: 0.75rem 1rem;
    background: #2d2d2d;
    color: #fafaf9;
    border: none;
    border-radius: 8px;
    cursor: pointer;
    font-size: 0.95rem;
    font-family: inherit;
    font-weight: 600;
    touch-action: manipulation;
    margin-top: auto;
  }

  .save-activity-btn:not(:disabled):hover { background: #1c1c1c; }
  .save-activity-btn:disabled { opacity: 0.35; cursor: default; }
</style>
