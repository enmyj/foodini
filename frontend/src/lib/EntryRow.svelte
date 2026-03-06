<script>
  import { patchEntry } from './api.js'

  let { entry, onUpdate } = $props()

  let editing = $state(null)
  let editValue = $state('')
  let saving = $state(false)

  const numFields = new Set(['calories', 'protein', 'carbs', 'fat'])

  function startEdit(field) {
    editing = field
    editValue = String(entry[field])
  }

  async function commitEdit() {
    if (!editing) return
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
    if (e.key === 'Escape') editing = null
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
  .row { display: flex; justify-content: space-between; align-items: center; padding: 0.5rem 0; border-bottom: 1px solid #f0f0f0; gap: 1rem; }
  .desc { flex: 1; min-width: 0; }
  .editable { cursor: pointer; border-bottom: 1px dashed transparent; }
  .editable:hover { border-bottom-color: #aaa; }
  .macros { display: flex; gap: 0.6rem; font-size: 0.82rem; color: #666; flex-shrink: 0; }
  input { border: 1px solid #4285f4; border-radius: 3px; padding: 2px 4px; font-family: inherit; }
  .num-input { width: 58px; }
</style>
