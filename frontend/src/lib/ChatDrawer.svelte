<script>
  import { chat, confirmChat } from './api.js'

  let { open, onClose, onEntriesAdded, date = null } = $props()

  let messages = $state([])
  let input = $state('')
  let sending = $state(false)
  let pendingEntries = $state(null)
  let inputEl = $state(null)
  let messagesEl = $state(null)

  $effect(() => {
    if (open) {
      setTimeout(() => inputEl?.focus(), 60)
    } else {
      messages = []
      input = ''
      pendingEntries = null
    }
  })

  $effect(() => {
    messages
    sending
    if (messagesEl) messagesEl.scrollTop = messagesEl.scrollHeight
  })

  async function send() {
    const text = input.trim()
    if (!text || sending) return
    messages = [...messages, { role: 'user', text }]
    input = ''
    pendingEntries = null
    sending = true
    try {
      const res = await chat(text, date)
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
      const res = await confirmChat(pendingEntries, date)
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
  <!-- svelte-ignore a11y_click_events_have_key_events a11y_no_static_element_interactions -->
  <div class="overlay" onclick={onClose}></div>
  <div class="drawer" role="dialog" aria-label="Log food">
    <div class="handle"></div>
    <div class="messages" bind:this={messagesEl}>
      {#if messages.length === 0}
        <p class="hint">What did you eat?<br><small>e.g. "I had oatmeal and coffee for breakfast"</small></p>
      {/if}
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
          <div class="msg {msg.role}">{msg.text}</div>
        {/if}
      {/each}
      {#if sending}
        <div class="msg assistant typing">…</div>
      {/if}
    </div>
    <div class="input-row">
      <textarea
        bind:this={inputEl}
        bind:value={input}
        onkeydown={onKeyDown}
        placeholder="What did you eat?"
        rows="2"
        disabled={sending}
      ></textarea>
      <button onclick={send} disabled={sending || !input.trim()}>Send</button>
    </div>
    {#if pendingEntries}
      <button class="confirm-btn" onclick={confirm} disabled={sending}>
        Looks good, save it
      </button>
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
    max-height: 65vh;
    padding: 0.75rem 1.25rem 1.5rem;
  }

  .handle {
    width: 36px;
    height: 3px;
    background: #e8e8e6;
    border-radius: 2px;
    margin: 0 auto 1rem;
  }

  .messages {
    flex: 1;
    overflow-y: auto;
    display: flex;
    flex-direction: column;
    gap: 0.6rem;
    margin-bottom: 0.75rem;
    padding: 0.25rem 0;
  }

  .hint {
    color: #bbb;
    font-size: 0.88rem;
    text-align: center;
    margin-top: 0.5rem;
    line-height: 1.6;
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

  .typing {
    color: #bbb;
  }

  .entry-line {
    line-height: 1.55;
    display: flex;
    flex-wrap: wrap;
    gap: 0.2rem 0.4rem;
    align-items: baseline;
  }

  .entry-line + .entry-line {
    margin-top: 0.25rem;
  }

  .entry-desc {
    font-weight: 500;
  }

  .entry-meta {
    color: #888;
    font-size: 0.82rem;
  }

  .input-row {
    display: flex;
    gap: 0.5rem;
    align-items: flex-end;
  }

  textarea {
    flex: 1;
    border: 1px solid #e8e8e6;
    border-radius: 8px;
    padding: 0.5rem 0.75rem;
    font-size: 0.95rem;
    resize: none;
    font-family: inherit;
    background: #fafaf9;
    color: #1c1c1c;
  }

  textarea:focus {
    outline: none;
    border-color: #2d2d2d;
  }

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

  button:disabled {
    opacity: 0.35;
    cursor: default;
  }

  .confirm-btn {
    width: 100%;
    margin-top: 0.5rem;
    padding: 0.6rem 1rem;
    background: #fafaf9;
    color: #2d2d2d;
    border: 1px solid #2d2d2d;
    border-radius: 8px;
    cursor: pointer;
    font-size: 0.9rem;
    font-family: inherit;
    font-weight: 500;
  }

  .confirm-btn:not(:disabled):hover {
    background: #2d2d2d;
    color: #fafaf9;
  }

  .confirm-btn:disabled {
    opacity: 0.35;
    cursor: default;
  }
</style>
