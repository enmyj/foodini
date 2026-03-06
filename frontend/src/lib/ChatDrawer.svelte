<script>
  import { chat } from './api.js'

  let { open, onClose, onEntriesAdded } = $props()

  let messages = $state([])
  let input = $state('')
  let sending = $state(false)
  let inputEl = $state(null)

  $effect(() => {
    if (open) {
      setTimeout(() => inputEl?.focus(), 60)
    } else {
      messages = []
      input = ''
    }
  })

  async function send() {
    const text = input.trim()
    if (!text || sending) return
    messages = [...messages, { role: 'user', text }]
    input = ''
    sending = true
    try {
      const res = await chat(text)
      if (res.done) {
        onEntriesAdded(res.entries)
      } else {
        messages = [...messages, { role: 'assistant', text: res.message }]
      }
    } catch {
      messages = [...messages, { role: 'assistant', text: 'Something went wrong. Please try again.' }]
    } finally {
      sending = false
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
    <div class="messages">
      {#if messages.length === 0}
        <p class="hint">What did you eat?<br><small>e.g. "I had oatmeal and coffee for breakfast"</small></p>
      {/if}
      {#each messages as msg}
        <div class="msg {msg.role}">{msg.text}</div>
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
  </div>
{/if}

<style>
  .overlay { position: fixed; inset: 0; background: rgba(0,0,0,0.3); z-index: 10; }
  .drawer { position: fixed; bottom: 0; left: 0; right: 0; background: white; border-radius: 16px 16px 0 0; box-shadow: 0 -4px 24px rgba(0,0,0,0.12); z-index: 11; display: flex; flex-direction: column; max-height: 65vh; padding: 0.75rem 1rem 1.25rem; }
  .handle { width: 40px; height: 4px; background: #ddd; border-radius: 2px; margin: 0 auto 0.75rem; }
  .messages { flex: 1; overflow-y: auto; display: flex; flex-direction: column; gap: 0.5rem; margin-bottom: 0.75rem; padding: 0.25rem 0; }
  .hint { color: #bbb; font-size: 0.9rem; text-align: center; margin-top: 0.5rem; line-height: 1.6; }
  .msg { padding: 0.5rem 0.75rem; border-radius: 12px; max-width: 85%; font-size: 0.9rem; line-height: 1.4; }
  .msg.user { background: #4285f4; color: white; align-self: flex-end; }
  .msg.assistant { background: #f1f1f1; color: #333; align-self: flex-start; }
  .typing { color: #bbb; }
  .input-row { display: flex; gap: 0.5rem; align-items: flex-end; }
  textarea { flex: 1; border: 1px solid #ddd; border-radius: 8px; padding: 0.5rem; font-size: 0.95rem; resize: none; font-family: inherit; }
  textarea:focus { outline: none; border-color: #4285f4; }
  button { padding: 0.5rem 1rem; background: #4285f4; color: white; border: none; border-radius: 8px; cursor: pointer; font-size: 0.9rem; white-space: nowrap; }
  button:disabled { opacity: 0.45; cursor: default; }
</style>
