<script>
  import { getActivity } from './api.js'
  import { showError } from './toast.js'

  let { date, onOpen, refreshKey = 0 } = $props()

  let data = $state(null)

  $effect(() => {
    refreshKey // re-run when this changes
    if (!date) return
    getActivity(date).then(res => { data = res }).catch(err => {
      data = null
      showError(err, 'Failed to load activity.')
    })
  })
</script>

<div class="activity-row">
  <div class="section-header">Other</div>
  {#each [
    { label: 'Activity', value: data?.activity, field: 'activity' },
    { label: 'Feeling', value: data?.feeling_notes || (data?.feeling_score ? `${data.feeling_score}/10` : null), field: 'feeling' },
    { label: '💩', value: data?.poop ? (data.poop_notes ? `Yes — ${data.poop_notes}` : 'Yes') : data?.poop_notes ? `No — ${data.poop_notes}` : null, field: 'poop' },
    { label: '💧', value: data?.hydration ? `${data.hydration} L` : null, field: 'hydration' },
  ] as section}
    <!-- svelte-ignore a11y_click_events_have_key_events -->
    <div class="section" role="button" tabindex="0" onclick={() => onOpen(section.field)}>
      <span class="section-label">{section.label}</span>
      <span class="section-value" class:placeholder={!section.value}>{section.value || '—'}</span>
    </div>
  {/each}
</div>

<style>
  .activity-row {
    display: flex;
    flex-direction: column;
    gap: 0.15rem;
    margin-top: 1.5rem;
  }

  .section-header {
    text-transform: uppercase;
    font-size: 0.72rem;
    color: #888;
    letter-spacing: 0.08em;
    font-weight: 600;
    padding: 0 0 0.4rem;
    border-top: 1px solid #e8e8e6;
    padding-top: 1.25rem;
    margin-bottom: 0.1rem;
  }

  .section {
    display: flex;
    align-items: baseline;
    gap: 0.6rem;
    padding: 0.35rem 0;
    border-radius: 8px;
    cursor: pointer;
    min-width: 0;
  }

  @media (hover: hover) {
    .section:hover {
      background: #f3f3f2;
    }
  }

  .section:focus-visible {
    outline: 2px solid #2d2d2d;
    outline-offset: -2px;
    border-radius: 8px;
  }

  .section-label {
    font-size: 0.72rem;
    text-transform: uppercase;
    letter-spacing: 0.08em;
    font-weight: 600;
    color: #aaa;
    flex-shrink: 0;
    display: flex;
    align-items: center;
    gap: 0.2rem;
    min-width: 4.5rem;
  }

  .section-value {
    font-size: 0.82rem;
    color: #1c1c1c;
    line-height: 1.4;
    min-width: 0;
  }

  .section-value.placeholder {
    color: #ccc;
  }
</style>
