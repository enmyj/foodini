<script>
    import { onMount } from "svelte";
    import { QueryClientProvider } from "@tanstack/svelte-query";
    import { queryClient } from "./lib/queryClient.js";
    import { navigate, init as initRouter, getCurrent } from "./lib/router.svelte.js";
    import { initTheme } from "./lib/theme.svelte.js";
    import { marked } from "marked";
    import LogView from "./lib/LogView.svelte";
    import MarkdownPage from "./lib/MarkdownPage.svelte";
    import ThemeToggle from "./lib/ThemeToggle.svelte";
    import ToastHost from "./lib/ToastHost.svelte";

    import aboutMd from "../content/about.md?raw";
    import legalMd from "../content/legal.md?raw";

    const aboutHtml = marked.parse(aboutMd);
    const legalHtml = marked.parse(legalMd);

    let authed = $state(null); // null=loading, false=logged out, true=logged in
    let schemaError = $state(false);
    let scopeError = $state(false);
    let sessionExpired = $state(false);
    let loadError = $state("");

    let path = $derived(getCurrent());

    async function readError(res) {
        const contentType = res.headers.get("content-type") || "";
        if (contentType.includes("application/json")) {
            const body = await res.json();
            return body?.error || `Could not load the app (${res.status})`;
        }
        const text = await res.text();
        return text || `Could not load the app (${res.status})`;
    }

    async function checkAuth() {
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
    }

    onMount(() => {
        initTheme();
        initRouter();
        if (path === "/app") {
            checkAuth();
        } else if (path === "/") {
            // Redirect to app if user already has a session.
            // No loading gate — the cookie check is fast and the landing
            // page renders immediately either way.
            fetch("/auth/check").then((res) => {
                if (res.ok && getCurrent() === "/") {
                    authed = null;
                    navigate("/app");
                    checkAuth();
                }
            }).catch(() => {});
        }
    });

    // Re-check auth when navigating to /app
    $effect(() => {
        if (path === "/app" && authed === null) {
            checkAuth();
        }
    });

    function go(e, href) {
        e.preventDefault();
        navigate(href);
    }

    async function startApp(e) {
        e.preventDefault();
        try {
            const res = await fetch("/auth/check");
            if (res.ok) {
                authed = null;
                navigate("/app");
                checkAuth();
                return;
            }
        } catch {}
        window.location.href = "/auth/login";
    }

</script>

{#if path === "/about"}
    <MarkdownPage html={aboutHtml} />
{:else if path === "/legal"}
    <MarkdownPage html={legalHtml} />
{:else if path === "/app"}
    {#if authed === null}
        <div class="center">Loading...</div>
    {:else if scopeError}
        <div class="landing">
            <header class="top-nav">
                <a href="/" class="nav-title" onclick={(e) => go(e, '/')}>Food Tracker</a>
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
                <a href="/" class="nav-title" onclick={(e) => go(e, '/')}>Food Tracker</a>
                <a href="/auth/logout" class="btn">Sign out</a>
            </header>
            <main class="content">
                <p class="error-msg">
                    Your existing Food Tracker spreadsheet is from an older version.<br />
                    Please rename it in Google Drive, then reload the page to create a fresh one.
                </p>
            </main>
        </div>
    {:else if sessionExpired}
        <div class="landing">
            <header class="top-nav">
                <a href="/" class="nav-title" onclick={(e) => go(e, '/')}>Food Tracker</a>
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
                <a href="/" class="nav-title" onclick={(e) => go(e, '/')}>Food Tracker</a>
                <a href="/auth/logout" class="btn">Sign out</a>
            </header>
            <main class="content">
                <p class="error-msg">{loadError}</p>
            </main>
        </div>
    {:else if authed === false}
        <div class="landing">
            <header class="top-nav">
                <a href="/" class="nav-title" onclick={(e) => go(e, '/')}>Food Tracker</a>
                <a href="/auth/login" class="btn">Sign in with Google</a>
            </header>
            <main class="content">
                <p class="error-msg">Please sign in to use the app.</p>
            </main>
        </div>
    {:else}
        <QueryClientProvider client={queryClient}>
            <LogView />
        </QueryClientProvider>
    {/if}
{:else}
    <!-- Home / Landing page -->
    <div class="landing">
        <header class="top-nav">
            <a href="/" class="nav-title" onclick={(e) => go(e, '/')}>Food Tracker</a>
            <nav class="nav-links">
                <ThemeToggle />
                <a href="/about" onclick={(e) => go(e, '/about')}>About</a>
                <a href="/legal" onclick={(e) => go(e, '/legal')}>Privacy Policy</a>
                <a href="/auth/login" class="btn" onclick={startApp}>Open app</a>
            </nav>
        </header>
        <main class="content">
            <section class="hero">
                <h1>Track what you eat,<br />in plain English.</h1>
                <p class="subtitle">
                    Describe your meals however you want. AI handles the calories and macros.
                    Your data lives in a Google Sheet you own — not our database.
                </p>
                <a href="/auth/login" class="cta" onclick={startApp}>Get started with Google</a>
            </section>
            <section class="details">
                <div class="detail">
                    <h3>No food database to search</h3>
                    <p>Type "scrambled eggs with toast and a coffee" and get structured entries back. Edit anything before you save.</p>
                </div>
                <div class="detail">
                    <h3>Your spreadsheet, your data</h3>
                    <p>Everything is stored in Google Sheets on your own Drive. Export it, delete it, do whatever you want with it.</p>
                </div>
                <div class="detail">
                    <h3>Open source</h3>
                    <p><a href="https://github.com/enmyj/foodini" class="link">Fork it</a> or self-host with your own API key. No lock-in.</p>
                </div>
            </section>
        </main>
    </div>
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
        text-decoration: none;
    }

    .nav-links {
        display: flex;
        gap: 1.25rem;
        align-items: center;
    }

    .nav-links a:not(.btn) {
        font-size: var(--t-meta);
        color: var(--mute);
        text-decoration: none;
    }

    .nav-links a:not(.btn):hover {
        color: var(--ink);
    }

    .nav-links :global(.theme-toggle) {
        font-size: var(--t-meta);
        color: var(--mute);
    }

    .content {
        flex: 1;
        display: flex;
        flex-direction: column;
        align-items: center;
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
        padding: 0.6rem 1rem;
        background: var(--ink);
        border: 1px solid var(--ink);
        color: var(--paper);
        border-radius: var(--r-sm);
        text-decoration: none;
        font-size: var(--t-body-sm);
        letter-spacing: 0.01em;
        white-space: nowrap;
    }

    .btn:hover {
        background: var(--ink-2);
        border-color: var(--ink-2);
        color: var(--paper);
    }

    .hero {
        max-width: 520px;
        text-align: center;
        padding-top: 2rem;
    }

    .hero h1 {
        font-size: 1.75rem;
        font-weight: 600;
        color: var(--ink);
        letter-spacing: -0.03em;
        line-height: 1.3;
        margin-bottom: 1rem;
    }

    .hero .subtitle {
        font-size: var(--t-body-sm);
        color: var(--mute);
        line-height: 1.6;
        margin-bottom: 2rem;
    }

    .cta {
        display: inline-block;
        padding: 0.6rem 1.5rem;
        background: var(--ink);
        color: var(--paper);
        border-radius: var(--r-sm);
        text-decoration: none;
        font-size: var(--t-body-sm);
        font-weight: 500;
        letter-spacing: -0.01em;
    }

    .cta:hover {
        background: var(--ink-2);
    }

    .details {
        display: flex;
        gap: 2.5rem;
        max-width: 680px;
        margin-top: 4rem;
        padding-top: 3rem;
        border-top: 1px solid var(--rule);
    }

    .detail h3 {
        font-size: var(--t-meta);
        font-weight: 600;
        color: var(--ink);
        margin-bottom: 0.4rem;
    }

    .detail p {
        font-size: var(--t-meta);
        color: var(--mute);
        line-height: 1.55;
    }

    .link {
        color: var(--ink-2);
        text-decoration: underline;
        text-underline-offset: 2px;
    }

    .link:hover {
        color: var(--ink-mute);
    }

@media (max-width: 600px) {
        .details {
            flex-direction: column;
            gap: 1.5rem;
        }

        .hero h1 {
            font-size: 1.4rem;
        }
    }
</style>
