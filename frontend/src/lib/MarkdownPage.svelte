<script>
    import { navigate } from './router.svelte.js';
    let { html } = $props();

    function go(e, href) {
        e.preventDefault();
        navigate(href);
    }
</script>

<div class="landing">
    <header class="top-nav">
        <a href="/" class="nav-title" onclick={(e) => go(e, '/')}>Food Tracker</a>
        <nav class="nav-links">
            <a href="/about" onclick={(e) => go(e, '/about')}>About</a>
            <a href="/legal" onclick={(e) => go(e, '/legal')}>Legal</a>
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

    .nav-links a {
        font-size: var(--t-meta);
        color: var(--mute);
        text-decoration: none;
    }

    .nav-links a:hover {
        color: var(--ink);
    }

    .btn {
        padding: 0.6rem 1rem;
        border: 1px solid var(--ink-2);
        color: var(--ink-2);
        border-radius: var(--r-sm);
        text-decoration: none;
        font-size: var(--t-body-sm);
        letter-spacing: 0.01em;
        white-space: nowrap;
    }

    .btn:hover {
        background: var(--ink-2);
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
