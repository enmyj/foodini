<script>
    import { onMount } from "svelte";
    import LogView from "./lib/LogView.svelte";

    let authed = $state(null); // null=loading, false=logged out, true=logged in
    let schemaError = $state(false); // true if spreadsheet schema is incompatible
    let scopeError = $state(false); // true if Google permissions are missing

    onMount(async () => {
        try {
            const res = await fetch("/api/log");
            if (res.status === 401) {
                authed = false;
            } else if (res.status === 403) {
                const body = await res.json();
                if (body.error === "insufficient_scopes") {
                    scopeError = true;
                    authed = false;
                } else {
                    authed = false;
                }
            } else if (res.status === 409) {
                const body = await res.json();
                if (body.error === "incompatible_spreadsheet") {
                    schemaError = true;
                    authed = false;
                } else {
                    authed = false;
                }
            } else {
                authed = true;
            }
        } catch {
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
                    <li>An LLM (currently Gemini 2.5 Flash) is used to facilitate food log entries.</li>
                </ol>
                <p><a href="/auth/login" class="link">Sign in with Google</a> to get started.</p>
                <p>
                    The code for this project is <a
                        href="https://github.com/enmyj/foodini">open source</a
                    > so feel free to make Pull Requests, Forks, etc. This version
                    is running on a Raspberry Pi at my house and uses my own LLM API
                    key, so please go easy on me.
                </p>
            </section>
        </main>
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
        border-bottom: 1px solid #e8e8e6;
        position: sticky;
        top: 0;
        background: #fafaf9;
    }

    .nav-title {
        font-size: 0.95rem;
        font-weight: 500;
        color: #1c1c1c;
        letter-spacing: -0.01em;
    }

    .content {
        flex: 1;
        display: flex;
        justify-content: center;
        padding: 4rem 1.5rem;
    }

    .error-msg {
        font-size: 0.9rem;
        color: #888;
        max-width: 480px;
        line-height: 1.6;
        text-align: center;
    }

    .btn {
        padding: 0.5rem 1rem;
        border: 1px solid #2d2d2d;
        color: #2d2d2d;
        border-radius: 6px;
        text-decoration: none;
        font-size: 0.85rem;
        letter-spacing: 0.01em;
        white-space: nowrap;
    }

    .btn:hover {
        background: #2d2d2d;
        color: #fafaf9;
    }

    .about {
        max-width: 480px;
        text-align: left;
    }

    .about h2 {
        font-size: 0.72rem;
        text-transform: uppercase;
        letter-spacing: 0.08em;
        color: #aaa;
        font-weight: 600;
        margin-bottom: 0.75rem;
    }

    .about p {
        font-size: 0.9rem;
        color: #888;
        line-height: 1.6;
        margin: 0;
    }

    .about p + p {
        margin-top: 0.75rem;
    }

    .about ol {
        font-size: 0.9rem;
        color: #888;
        line-height: 1.6;
        margin: 0.5rem 0 0.75rem 1.25rem;
        padding: 0;
        display: flex;
        flex-direction: column;
        gap: 0.25rem;
    }

    .link {
        color: #2d2d2d;
        text-decoration: underline;
        text-underline-offset: 2px;
    }

    .link:hover {
        color: #555;
    }
</style>
