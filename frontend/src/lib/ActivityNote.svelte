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
    { label: 'Stool', value: data?.poop ? (data.poop_notes ? `Yes — ${data.poop_notes}` : 'Yes') : data?.poop_notes ? `No — ${data.poop_notes}` : null, field: 'poop' },
    { label: 'Water', value: data?.hydration ? `${data.hydration} L` : null, field: 'hydration' },
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
    font-size: var(--t-micro);
    color: var(--mute);
    letter-spacing: 0.08em;
    font-weight: 600;
    padding: 0 0 0.4rem;
    border-top: 1px solid var(--rule);
    padding-top: 1.25rem;
    margin-bottom: 0.1rem;
  }

  .section {
    display: flex;
    align-items: baseline;
    gap: 0.6rem;
    padding: 0.35rem 0;
    border-radius: var(--r-sm);
    cursor: pointer;
    min-width: 0;
  }

  @media (hover: hover) {
    .section:hover {
      background: var(--paper-3);
    }
  }

  .section:focus-visible {
    outline: 2px solid var(--ink-2);
    outline-offset: -2px;
    border-radius: var(--r-sm);
  }

  .section-label {
    font-size: var(--t-micro);
    text-transform: uppercase;
    letter-spacing: 0.08em;
    font-weight: 600;
    color: var(--mute-2);
    flex-shrink: 0;
    display: flex;
    align-items: center;
    gap: 0.2rem;
    min-width: 4.5rem;
  }

  .section-value {
    font-size: var(--t-meta);
    color: var(--ink);
    line-height: 1.4;
    min-width: 0;
  }

  .section-value.placeholder {
    color: var(--mute-4);
  }
</style>
