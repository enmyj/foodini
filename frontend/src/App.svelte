<script>
  import { onMount } from 'svelte'
  import LogView from './lib/LogView.svelte'

  let authed = $state(null) // null=loading, false=logged out, true=logged in

  onMount(async () => {
    try {
      const res = await fetch('/api/log')
      authed = res.status !== 401
    } catch {
      authed = false
    }
  })
</script>

{#if authed === null}
  <div class="center">Loading...</div>
{:else if authed === false}
  <div class="login">
    <h1>Food Tracker</h1>
    <a href="/auth/login" class="btn">Sign in with Google</a>
  </div>
{:else}
  <LogView />
{/if}

<style>
  :global(*, *::before, *::after) { box-sizing: border-box; margin: 0; padding: 0; }
  :global(body) { font-family: -apple-system, BlinkMacSystemFont, 'Segoe UI', sans-serif; }
  .center { display: flex; align-items: center; justify-content: center; height: 100vh; color: #888; }
  .login { display: flex; flex-direction: column; align-items: center; justify-content: center; height: 100vh; gap: 1.5rem; }
  .login h1 { font-size: 1.75rem; color: #333; }
  .btn { padding: 0.75rem 1.5rem; background: #4285f4; color: white; border-radius: 4px; text-decoration: none; font-size: 1rem; }
</style>
