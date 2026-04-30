<script lang="ts">
    import { onMount } from "svelte";
    import { getSystemPrompt } from "./api.ts";

    let { onClose }: { onClose: () => void } = $props();

    let prompt = $state<string | null>(null);
    let error = $state("");

    onMount(async () => {
        try {
            prompt = await getSystemPrompt();
        } catch (e) {
            error = e instanceof Error ? e.message : "Could not load prompt.";
        }
    });

    function onKey(e: KeyboardEvent) {
        if (e.key === "Escape") onClose();
    }
</script>

<svelte:window onkeydown={onKey} />

<!-- svelte-ignore a11y_click_events_have_key_events a11y_no_static_element_interactions a11y_interactive_supports_focus -->
<div class="backdrop" onclick={onClose} role="presentation">
    <div class="modal" onclick={(e) => e.stopPropagation()} role="dialog" aria-label="System prompt" tabindex="-1">
        <header>
            <div>
                <h2>System prompt</h2>
                <p class="sub">The instructions sent to the AI assistant in the chat drawer.</p>
            </div>
            <button class="close" onclick={onClose} aria-label="Close">×</button>
        </header>
        <div class="body">
            {#if error}
                <p class="err">{error}</p>
            {:else if prompt === null}
                <p class="loading">Loading…</p>
            {:else}
                <pre>{prompt}</pre>
            {/if}
        </div>
    </div>
</div>

<style>
    .backdrop {
        position: fixed;
        inset: 0;
        background: rgba(0, 0, 0, 0.4);
        z-index: 100;
        display: flex;
        align-items: center;
        justify-content: center;
        padding: 1rem;
    }

    .modal {
        background: var(--paper);
        border: 1px solid var(--rule);
        border-radius: var(--r-md);
        max-width: 720px;
        width: 100%;
        max-height: 85vh;
        display: flex;
        flex-direction: column;
        box-shadow: 0 8px 32px rgba(0, 0, 0, 0.2);
    }

    header {
        display: flex;
        justify-content: space-between;
        align-items: flex-start;
        gap: 1rem;
        padding: 1rem 1.25rem 0.75rem;
        border-bottom: 1px solid var(--rule);
    }

    h2 {
        font-size: 0.95rem;
        font-weight: 600;
        color: var(--ink);
        margin: 0 0 0.2rem;
    }

    .sub {
        font-size: var(--t-meta);
        color: var(--mute);
        margin: 0;
        line-height: 1.4;
    }

    .close {
        background: none;
        border: none;
        color: var(--mute);
        font-size: 1.4rem;
        line-height: 1;
        cursor: pointer;
        padding: 0.1rem 0.4rem;
    }

    .close:hover {
        color: var(--ink);
    }

    .body {
        padding: 1rem 1.25rem;
        overflow: auto;
    }

    pre {
        font-family: ui-monospace, SFMono-Regular, Menlo, monospace;
        font-size: 0.78rem;
        color: var(--ink-2);
        line-height: 1.55;
        white-space: pre-wrap;
        word-break: break-word;
        margin: 0;
    }

    .loading,
    .err {
        font-size: var(--t-body-sm);
        color: var(--mute);
        margin: 0;
    }

    .err {
        color: var(--danger, #c33);
    }
</style>
