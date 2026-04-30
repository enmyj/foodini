<script lang="ts">
    import { navigate } from './router.svelte.ts';
    import ThemeToggle from './ThemeToggle.svelte';
    import type { RoutePath } from './types.ts';

    let { html }: { html: string } = $props();
    let moreOpen = $state(false);

    function go(e: MouseEvent, href: RoutePath) {
        e.preventDefault();
        navigate(href);
    }
</script>

<div class="landing">
    <header class="top-nav">
        <a href="/" class="nav-title" onclick={(e) => go(e, '/')}>simplelog.food</a>
        <nav class="nav-links">
            <ThemeToggle />
            <div class="more-wrap">
                <button
                    class="more-btn"
                    onclick={() => (moreOpen = !moreOpen)}
                    aria-expanded={moreOpen}
                    aria-label="More"
                >More <span class="caret" aria-hidden="true">▾</span></button>
                {#if moreOpen}
                    <!-- svelte-ignore a11y_click_events_have_key_events -->
                    <div class="menu-backdrop" aria-hidden="true" onclick={() => (moreOpen = false)}></div>
                    <div class="more-menu">
                        <a href="/about" onclick={(e) => { moreOpen = false; go(e, '/about'); }}>About</a>
                        <a href="/legal" onclick={(e) => { moreOpen = false; go(e, '/legal'); }}>Privacy Policy</a>
                    </div>
                {/if}
            </div>
            <a href="/app" class="btn" onclick={(e) => go(e, '/app')}>Open app</a>
        </nav>
    </header>
    <main class="md-content">
        <article class="prose">
            {@html html}
        </article>
    </main>
</div>

<style>
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

    .more-wrap {
        position: relative;
        display: flex;
        align-items: center;
    }

    .more-btn {
        background: none;
        border: none;
        font-family: inherit;
        font-size: var(--t-meta);
        color: var(--mute);
        cursor: pointer;
        padding: 0.25rem 0;
    }

    .more-btn:hover {
        color: var(--ink);
    }

    .caret {
        font-size: 0.75em;
        margin-left: 0.15em;
    }

    .menu-backdrop {
        position: fixed;
        inset: 0;
        z-index: 9;
    }

    .more-menu {
        position: absolute;
        top: 100%;
        right: 0;
        margin-top: 0.4rem;
        background: var(--paper);
        border: 1px solid var(--rule);
        border-radius: var(--r-md);
        box-shadow: 0 4px 16px rgba(0, 0, 0, 0.1);
        z-index: 10;
        min-width: 160px;
        padding: 0.35rem 0;
        display: flex;
        flex-direction: column;
    }

    .more-menu a {
        padding: 0.55rem 1rem;
        font-size: var(--t-body-sm);
        color: var(--mute);
        text-decoration: none;
    }

    .more-menu a:hover {
        background: var(--paper-4);
        color: var(--ink);
    }

    .nav-links :global(.theme-toggle) {
        font-size: var(--t-meta);
        color: var(--mute);
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

    .md-content {
        flex: 1;
        display: flex;
        justify-content: center;
        padding: 3rem 1.5rem 4rem;
    }

    .prose {
        max-width: 600px;
        width: 100%;
        font-size: var(--t-body-sm);
        color: var(--ink-2);
        line-height: 1.7;
    }

    .prose :global(h1) {
        font-size: 1.4rem;
        font-weight: 600;
        color: var(--ink);
        margin-bottom: 1.5rem;
        letter-spacing: -0.02em;
    }

    .prose :global(h2) {
        font-size: 1.05rem;
        font-weight: 600;
        color: var(--ink);
        margin-top: 2rem;
        margin-bottom: 0.75rem;
    }

    .prose :global(h3) {
        font-size: 0.95rem;
        font-weight: 600;
        color: var(--ink);
        margin-top: 1.5rem;
        margin-bottom: 0.5rem;
    }

    .prose :global(p) {
        margin-bottom: 0.75rem;
    }

    .prose :global(ol),
    .prose :global(ul) {
        margin: 0.5rem 0 1rem 1.25rem;
        display: flex;
        flex-direction: column;
        gap: 0.3rem;
    }

    .prose :global(a) {
        color: var(--ink-2);
        text-decoration: underline;
        text-underline-offset: 2px;
    }

    .prose :global(a:hover) {
        color: var(--ink-mute);
    }

    .prose :global(strong) {
        font-weight: 600;
        color: var(--ink);
    }

    .prose :global(hr) {
        border: none;
        border-top: 1px solid var(--rule);
        margin: 2rem 0;
    }

    .prose :global(em) {
        color: var(--mute);
    }
</style>
