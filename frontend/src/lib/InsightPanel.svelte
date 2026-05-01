<script lang="ts">
    import InsightSkeleton from "./InsightSkeleton.svelte";
    import { formatGeneratedAt, renderInsight } from "./insight.ts";

    type Variant = "default" | "suggestion";

    let {
        loading,
        error = null,
        text = null,
        generatedAt = null,
        label = null,
        variant = "default",
        closeable = false,
        collapsed = false,
        showMoreToggle = false,
        expanded = true,
        onClose = null,
        onToggleExpanded = null,
        onRegenerate = null,
        onDiscuss = null,
    }: {
        loading: boolean;
        error?: string | null;
        text?: string | null;
        generatedAt?: string | null;
        label?: string | null;
        variant?: Variant;
        closeable?: boolean;
        collapsed?: boolean;
        showMoreToggle?: boolean;
        expanded?: boolean;
        onClose?: (() => void) | null;
        onToggleExpanded?: (() => void) | null;
        onRegenerate?: (() => void) | null;
        onDiscuss?: (() => void) | null;
    } = $props();

    let showToggle = $derived(
        Boolean(showMoreToggle && generatedAt && onToggleExpanded),
    );
    let showDiscuss = $derived(Boolean(onDiscuss && text && generatedAt));
    let footerVisible = $derived(
        Boolean(generatedAt) || showToggle || showDiscuss,
    );
</script>

<div class="insights-panel" class:suggestions-panel={variant === "suggestion"}>
    {#if closeable && onClose}
        <button class="insight-close" onclick={onClose} aria-label="Close insights">
            ✕
        </button>
    {/if}
    {#if loading}
        <InsightSkeleton />
    {:else if error}
        <span class="insights-err">{error}</span>
    {:else if text != null}
        {#if label}
            <span class="suggestions-label">{label}</span>
        {/if}
        <!-- eslint-disable-next-line svelte/no-at-html-tags -->
        {#if showToggle && !expanded}
            <button class="insights-text-btn collapsed-text" onclick={onToggleExpanded} aria-label="Show more">
                <p class="insights-text">
                    {@html renderInsight(text)}
                </p>
                <span class="collapsed-fade"></span>
            </button>
        {:else}
            <p class="insights-text">
                {@html renderInsight(text)}
            </p>
        {/if}
        {#if footerVisible}
            <div class="insight-footer">
                {#if generatedAt}
                    <span class="insight-ts">{formatGeneratedAt(generatedAt)}</span>
                {/if}
                {#if showDiscuss && onDiscuss}
                    <button class="insight-discuss" onclick={onDiscuss}>
                        discuss with coach
                    </button>
                {/if}
                {#if onRegenerate}
                    <button class="insight-regen" onclick={onRegenerate}>
                        regenerate
                    </button>
                {/if}
            </div>
        {/if}
    {/if}
</div>

<style>
    .insights-panel {
        position: relative;
        padding: 0.9rem 2.2rem 0.95rem 0.95rem;
        background: var(--paper-2);
        border-radius: var(--r-sm);
        margin-top: 0.5rem;
    }

    .suggestions-panel {
        background: var(--sugg-paper);
    }

    .insight-close {
        position: absolute;
        top: 0.55rem;
        right: 0.55rem;
        width: 1.5rem;
        height: 1.5rem;
        background: none;
        border: none;
        border-radius: 50%;
        font-size: 0.75rem;
        color: var(--mute-3);
        cursor: pointer;
        padding: 0;
        line-height: 1;
        display: flex;
        align-items: center;
        justify-content: center;
    }

    @media (hover: hover) {
        .insight-close:hover {
            color: var(--ink-mute);
            background: var(--paper-4);
        }
    }

    .insights-err {
        font-size: var(--t-meta);
        color: var(--danger);
    }

    .insights-text {
        font-size: var(--t-meta);
        color: var(--ink);
        line-height: 1.65;
        white-space: pre-line;
        overflow-wrap: break-word;
        word-break: break-word;
        margin: 0;
    }

    .insights-text-btn {
        display: block;
        position: relative;
        width: 100%;
        background: none;
        border: none;
        padding: 0;
        margin: 0;
        cursor: pointer;
        text-align: left;
        font-family: inherit;
        touch-action: manipulation;
    }

    .insights-text-btn .insights-text {
        display: -webkit-box;
        line-clamp: 3;
        -webkit-line-clamp: 3;
        -webkit-box-orient: vertical;
        overflow: hidden;
    }

    .collapsed-fade {
        position: absolute;
        bottom: 0;
        left: 0;
        right: 0;
        height: 1.5rem;
        background: linear-gradient(transparent, var(--paper-2));
        pointer-events: none;
    }

    .suggestions-panel .collapsed-fade {
        background: linear-gradient(transparent, var(--sugg-paper));
    }

    .insights-text :global(strong) {
        font-weight: 600;
        color: var(--ink);
    }

    .insight-footer {
        display: flex;
        align-items: center;
        gap: 0.75rem;
        margin-top: 0.7rem;
    }

    .insight-ts {
        font-size: 0.72rem;
        color: var(--mute-3);
    }

    .insight-regen,
    .insight-discuss {
        background: none;
        border: none;
        font-family: inherit;
        font-size: 0.72rem;
        color: var(--mute-2);
        cursor: pointer;
        padding: 0;
        touch-action: manipulation;
    }

    .insight-discuss,
    .insight-regen {
        margin-left: auto;
    }

    @media (hover: hover) {
        .insight-regen:hover,
        .insight-discuss:hover {
            color: var(--ink-mute);
        }
    }

    .suggestions-label {
        display: block;
        font-size: 0.7rem;
        font-weight: 600;
        text-transform: uppercase;
        letter-spacing: 0.06em;
        color: var(--sugg-mute);
        margin-bottom: 0.4rem;
    }
</style>
