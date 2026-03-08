<script>
  import { onMount } from 'svelte'
  import LogView from './lib/LogView.svelte'

  let authed = $state(null)       // null=loading, false=logged out, true=logged in
  let schemaError = $state(false) // true if spreadsheet schema is incompatible

  onMount(async () => {
    try {
      const res = await fetch('/api/log')
      if (res.status === 401) {
        authed = false
      } else if (res.status === 409) {
        const body = await res.json()
        if (body.error === 'incompatible_spreadsheet') {
          schemaError = true
          authed = false
        } else {
          authed = false
        }
      } else {
        authed = true
      }
    } catch {
      authed = false
    }
  })
</script>

{#if authed === null}
  <div class="center">Loading...</div>
{:else if schemaError}
  <div class="login">
    <h1>Food Tracker</h1>
    <p class="error-msg">
      Your existing Food Tracker spreadsheet is from an older version.<br>
      Please rename it in Google Drive, then reload the page to create a fresh one.
    </p>
    <a href="/auth/logout" class="btn">Sign out</a>
  </div>
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
    padding: 2rem;
    text-align: center;
  }

  .login h1 {
    font-size: 1.4rem;
    font-weight: 500;
    color: #1c1c1c;
    letter-spacing: -0.01em;
  }

  .error-msg {
    font-size: 0.9rem;
    color: #888;
    max-width: 360px;
    line-height: 1.6;
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
