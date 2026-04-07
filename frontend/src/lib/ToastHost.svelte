<script>
  import { dismissToast, toasts } from './toast.js'
</script>

<div class="toast-host" aria-live="polite" aria-atomic="true">
  {#each $toasts as toast (toast.id)}
    <div class="toast" class:error={toast.tone === 'error'} role="status">
      <span class="toast-message">{toast.message}</span>
      <button class="toast-close" onclick={() => dismissToast(toast.id)} aria-label="Dismiss notification">✕</button>
    </div>
  {/each}
</div>

<style>
  .toast-host {
    position: fixed;
    left: 50%;
    bottom: 1rem;
    transform: translateX(-50%);
    width: min(92vw, 480px);
    display: flex;
    flex-direction: column;
    gap: 0.6rem;
    z-index: 100;
    pointer-events: none;
  }

  .toast {
    display: flex;
    align-items: flex-start;
    gap: 0.75rem;
    padding: 0.85rem 1rem;
    border-radius: var(--r-md);
    background: rgba(28,0,0,0.96);
    color: var(--paper);
    box-shadow: 0 10px 30px rgba(0,0,0,0.18);
    pointer-events: auto;
  }

  .toast.error {
    background: rgba(45, 45, 45, 0.97);
  }

  .toast-message {
    flex: 1;
    min-width: 0;
    font-size: 0.88rem;
    line-height: 1.45;
  }

  .toast-close {
    background: none;
    border: none;
    color: inherit;
    cursor: pointer;
    font-size: 0.9rem;
    line-height: 1;
    padding: 0.1rem;
    opacity: 0.75;
  }

  .toast-close:hover {
    opacity: 1;
  }
</style>
