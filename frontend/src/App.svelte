<script>
    import { onMount } from "svelte";
    import LogView from "./lib/LogView.svelte";

    let authed = $state(null); // null=loading, false=logged out, true=logged in
    let schemaError = $state(false); // true if spreadsheet schema is incompatible

    onMount(async () => {
        try {
            const res = await fetch("/api/log");
            if (res.status === 401) {
                authed = false;
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
{:else if schemaError}
    <div class="login">
        <h1>Food Tracker</h1>
        <p class="error-msg">
            Your existing Food Tracker spreadsheet is from an older version.<br
            />
            Please rename it in Google Drive, then reload the page to create a fresh
            one.
        </p>
        <a href="/auth/logout" class="btn">Sign out</a>
    </div>
{:else if authed === false}
    <div class="login">
        <h1>Food Tracker</h1>
        <a href="/auth/login" class="btn">Sign in with Google</a>
        <hr class="divider" />
        <section class="about">
            <h2>About</h2>
            <p>
                I was having digestive issues in 2024 and doctors recommended
                using a food log and trying the FODMAP diet. I found most
                existing food logs finnicky to use and ended up just storing
                mine in a Google Sheet. Now that we're living in the
                LLM-assisted future, I figured I'd attempt to make a slightly
                nicer interface for my own solution.
            </p>
            <p>
                This app aims to be unique from other food trackers in two ways.
                1. All data is stored in the user's Google Drive for better data
                ownership and hopefully simpler downstream analytics and 2. an
                LLM (currently: gemini-2.5-flash) is used to facilitate food log
                entries.
            </p>
            <p>
                This is running on a raspberry pi at my house and uses my own
                LLM API Key so please go easy on me here.
            </p>
        </section>
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

    .divider {
        border: none;
        border-top: 1px solid #e8e8e6;
        width: 100%;
        max-width: 360px;
        margin: 0;
    }

    .about {
        max-width: 360px;
        text-align: left;
    }

    .about h2 {
        font-size: 0.68rem;
        text-transform: uppercase;
        letter-spacing: 0.08em;
        color: #aaa;
        font-weight: 600;
        margin-bottom: 0.5rem;
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
</style>
