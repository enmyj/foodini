<script>
  import { patchEntry } from './api.js'

  let { entry, onUpdate } = $props()

  let editing = $state(null)
  let editValue = $state('')
  let saving = $state(false)
  let cancelFlag = false

  const numFields = new Set(['calories', 'protein', 'carbs', 'fat'])

  function startEdit(field) {
    editing = field
    editValue = String(entry[field])
  }

  async function commitEdit() {
    if (!editing || cancelFlag) return
    saving = true
    try {
      const value = numFields.has(editing) ? (parseInt(editValue) || 0) : editValue
      const saved = await patchEntry(entry.id, { ...entry, [editing]: value })
      onUpdate(saved)
    } catch (e) {
      console.error('patch failed', e)
    } finally {
      editing = null
      saving = false
    }
  }

  function onKeyDown(e) {
    if (e.key === 'Enter') commitEdit()
    if (e.key === 'Escape') {
      cancelFlag = true
      editing = null
      setTimeout(() => { cancelFlag = false }, 0)
    }
  }
</script>

<div class="row">
  <div class="desc">
    {#if editing === 'description'}
      <input bind:value={editValue} onblur={commitEdit} onkeydown={onKeyDown} autofocus />
    {:else}
      <span class="editable" onclick={() => startEdit('description')}>{entry.description}</span>
    {/if}
  </div>
  <div class="macros">
    {#each ['calories', 'protein', 'carbs', 'fat'] as field}
      {#if editing === field}
        <input class="num-input" type="number" bind:value={editValue}
               onblur={commitEdit} onkeydown={onKeyDown} autofocus />
      {:else}
        <span class="editable macro" title={field} onclick={() => startEdit(field)}>
          {entry[field]}{field === 'calories' ? ' cal' : 'g'}
        </span>
      {/if}
    {/each}
  </div>
</div>

<style>
  .row {
    display: flex;
    justify-content: space-between;
    align-items: center;
    padding: 0.65rem 0;
    border-bottom: 1px solid #e8e8e6;
    gap: 1rem;
  }

  .desc {
    flex: 1;
    min-width: 0;
    font-size: 0.95rem;
    line-height: 1.4;
  }

  .editable {
    cursor: pointer;
  }

  .editable:hover {
    color: #555;
  }

  .macros {
    display: flex;
    gap: 0.75rem;
    font-size: 0.8rem;
    color: #888;
    flex-shrink: 0;
  }

  .macro {
    cursor: pointer;
  }

  .macro:hover {
    color: #555;
  }

  input {
    border: none;
    border-bottom: 2px solid #2d2d2d;
    border-radius: 0;
    padding: 2px 4px;
    font-family: inherit;
    font-size: inherit;
    background: transparent;
    outline: none;
  }

  .num-input {
    width: 56px;
  }
</style>
