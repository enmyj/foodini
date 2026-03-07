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
  .center {
    display: flex;
    align-items: center;
    justify-content: center;
    height: 100vh;
    color: #888;
    font-size: 0.9rem;
  }
  .login {
    display: flex;
    flex-direction: column;
    align-items: center;
    justify-content: center;
    height: 100vh;
    gap: 2rem;
  }
  .login h1 {
    font-size: 1.4rem;
    font-weight: 500;
    color: #1c1c1c;
    letter-spacing: -0.01em;
  }
  .btn {
    padding: 0.6rem 1.25rem;
    border: 1px solid #2d2d2d;
    color: #2d2d2d;
    border-radius: 6px;
    text-decoration: none;
    font-size: 0.9rem;
    letter-spacing: 0.01em;
  }
  .btn:hover {
    background: #2d2d2d;
    color: #fafaf9;
  }
</style>
