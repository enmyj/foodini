<script>
    import { onMount } from "svelte";
    import LogView from "./lib/LogView.svelte";
    import ToastHost from "./lib/ToastHost.svelte";

    let authed = $state(null); // null=loading, false=logged out, true=logged in
    let schemaError = $state(false); // true if spreadsheet schema is incompatible
    let scopeError = $state(false); // true if Google permissions are missing
    let sessionExpired = $state(false);
    let loadError = $state("");

    async function readError(res) {
        const contentType = res.headers.get("content-type") || "";
        if (contentType.includes("application/json")) {
            const body = await res.json();
            return body?.error || `Could not load the app (${res.status})`;
        }
        const text = await res.text();
        return text || `Could not load the app (${res.status})`;
    }

    onMount(async () => {
        scopeError = false;
        schemaError = false;
        sessionExpired = false;
        loadError = "";
        try {
            const res = await fetch("/api/log");
            if (res.ok) {
                authed = true;
            } else if (res.status === 401) {
                sessionExpired = (await readError(res)) === "session_expired";
                authed = false;
            } else if (res.status === 403) {
                if ((await readError(res)) === "insufficient_scopes") {
                    scopeError = true;
                    authed = false;
                } else {
                    loadError = "Could not load the app. Try reloading, or sign out and back in.";
                    authed = false;
                }
            } else if (res.status === 409) {
                if ((await readError(res)) === "incompatible_spreadsheet") {
                    schemaError = true;
                    authed = false;
                } else {
                    loadError = "Could not load the app. Try reloading, or sign out and back in.";
                    authed = false;
                }
            } else {
                await readError(res);
                loadError = "Could not load the app. Try reloading, or sign out and back in.";
                authed = false;
            }
        } catch {
            loadError = "Could not reach the server. Try reloading.";
            authed = false;
        }
    });
</script>

{#if authed === null}
    <div class="center">Loading...</div>
{:else if scopeError}
    <div class="landing">
        <header class="top-nav">
            <span class="nav-title">Food Tracker</span>
            <a href="/auth/logout" class="btn">Sign out</a>
        </header>
        <main class="content">
            <p class="error-msg">
                Missing required Google permissions.<br />
                <a href="/auth/login?consent=1" class="btn" style="display:inline-block;margin-top:1rem;">Re-authorize</a>
            </p>
        </main>
    </div>
{:else if schemaError}
    <div class="landing">
        <header class="top-nav">
            <span class="nav-title">Food Tracker</span>
            <a href="/auth/logout" class="btn">Sign out</a>
        </header>
        <main class="content">
            <p class="error-msg">
                Your existing Food Tracker spreadsheet is from an older version.<br
                />
                Please rename it in Google Drive, then reload the page to create a
                fresh one.
            </p>
        </main>
    </div>
{:else if sessionExpired}
    <div class="landing">
        <header class="top-nav">
            <span class="nav-title">Food Tracker</span>
            <a href="/auth/login" class="btn">Sign in with Google</a>
        </header>
        <main class="content">
            <p class="error-msg">
                Your session expired or became invalid.<br />
                Sign in again to reload your data.
            </p>
        </main>
    </div>
{:else if loadError}
    <div class="landing">
        <header class="top-nav">
            <span class="nav-title">Food Tracker</span>
            <a href="/auth/logout" class="btn">Sign out</a>
        </header>
        <main class="content">
            <p class="error-msg">{loadError}</p>
        </main>
    </div>
{:else if authed === false}
    <div class="landing">
        <header class="top-nav">
            <span class="nav-title">Food Tracker</span>
            <a href="/auth/login" class="btn">Sign in with Google</a>
        </header>
        <main class="content">
            <section class="about">
                <h2>About</h2>
                <p>This app aims to be unique from other food trackers in two ways:</p>
                <ol>
                    <li>All data is stored in the user's own Google Drive for better data ownership.</li>
                    <li>Gemini Flash is used to parse natural-language meal descriptions into structured entries.</li>
                </ol>
                <p><a href="/auth/login" class="link">Sign in with Google</a> to get started.</p>
                <p>
                    The code is <a href="https://github.com/enmyj/foodini">open source</a
                    > — feel free to fork or self-host with your own API key.
                </p>
            </section>
        </main>
    </div>
{:else}
    <LogView />
{/if}

<ToastHost />

<style>
    .center {
        display: flex;
        align-items: center;
        justify-content: center;
        height: 100vh;
        color: var(--mute);
        font-size: var(--t-body-sm);
    }

    .landing {
        display: flex;
        flex-direction: column;
        min-height: 100vh;
    }

    .top-nav {
        display: flex;
        align-items: center;
        justify-content: space-between;
        padding: 0.75rem 1.5rem;
        border-bottom: 1px solid var(--rule);
        position: sticky;
        top: 0;
        background: var(--paper);
    }

    .nav-title {
        font-size: 0.95rem;
        font-weight: 500;
        color: var(--ink);
        letter-spacing: -0.01em;
    }

    .content {
        flex: 1;
        display: flex;
        justify-content: center;
        padding: 4rem 1.5rem;
    }

    .error-msg {
        font-size: var(--t-body-sm);
        color: var(--mute);
        max-width: 480px;
        line-height: 1.6;
        text-align: center;
    }

    .btn {
        padding: 0.5rem 1rem;
        border: 1px solid var(--ink-2);
        color: var(--ink-2);
        border-radius: var(--r-sm);
        text-decoration: none;
        font-size: 0.85rem;
        letter-spacing: 0.01em;
        white-space: nowrap;
    }

    .btn:hover {
        background: var(--ink-2);
        color: var(--paper);
    }

    .about {
        max-width: 480px;
        text-align: left;
    }

    .about h2 {
        font-size: var(--t-micro);
        text-transform: uppercase;
        letter-spacing: 0.08em;
        color: var(--mute-2);
        font-weight: 600;
        margin-bottom: 0.75rem;
    }

    .about p {
        font-size: var(--t-body-sm);
        color: var(--mute);
        line-height: 1.6;
        margin: 0;
    }

    .about p + p {
        margin-top: 0.75rem;
    }

    .about ol {
        font-size: var(--t-body-sm);
        color: var(--mute);
        line-height: 1.6;
        margin: 0.5rem 0 0.75rem 1.25rem;
        padding: 0;
        display: flex;
        flex-direction: column;
        gap: 0.25rem;
    }

    .link {
        color: var(--ink-2);
        text-decoration: underline;
        text-underline-offset: 2px;
    }

    .link:hover {
        color: var(--ink-mute);
    }
</style>
