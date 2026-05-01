<script lang="ts">
    import { coachChatStream } from "./api.ts";
    import { renderInsight } from "./insight.ts";
    import { showError } from "./toast.ts";
    import type { CoachMessage } from "./types.ts";

    let {
        active,
        date,
        pinnedContext = null,
        onContextConsumed = null,
    }: {
        active: boolean;
        date: string;
        pinnedContext?: string | null;
        onContextConsumed?: (() => void) | null;
    } = $props();

    let messages = $state<CoachMessage[]>([]);
    let input = $state("");
    let pinned = $state<string | null>(null);
    let pinnedExpanded = $state(false);

    $effect(() => {
        if (pinnedContext) {
            pinned = pinnedContext;
            pinnedExpanded = false;
            onContextConsumed?.();
            focusInput();
        }
    });
    let sending = $state(false);
    let streaming = $state(false);
    let weeks = $state(1);
    let inputEl = $state<HTMLTextAreaElement | null>(null);
    let scrollEl = $state<HTMLDivElement | null>(null);
    let prevLen = 0;

    function focusInput() {
        setTimeout(() => {
            inputEl?.focus();
            scrollInputIntoView();
        }, 60);
    }

    function scrollInputIntoView() {
        setTimeout(() => {
            inputEl?.scrollIntoView({ block: "end", behavior: "smooth" });
        }, 280);
    }

    $effect(() => {
        if (active) {
            setTimeout(() => inputEl?.focus(), 120);
        }
    });

    $effect(() => {
        const len = messages.length;
        const last = messages[len - 1];
        if (scrollEl && len > prevLen && last?.role === "user") {
            queueMicrotask(() => {
                const items = scrollEl?.querySelectorAll(".msg");
                const el = items?.[items.length - 1] as HTMLElement | undefined;
                el?.scrollIntoView({ block: "start", behavior: "smooth" });
            });
        }
        prevLen = len;
    });

    async function send(): Promise<void> {
        if (sending) return;
        const typed = input.trim();
        const ctx = pinned?.trim() ?? "";
        if (!typed && !ctx) return;
        const text = ctx && typed ? `${ctx}\n\n${typed}` : ctx || typed;
        const next: CoachMessage[] = [...messages, { role: "user", text }];
        messages = next;
        input = "";
        pinned = null;
        pinnedExpanded = false;
        sending = true;
        let started = false;
        try {
            for await (const chunk of coachChatStream(next, date, weeks * 7)) {
                if (!started) {
                    messages = [...next, { role: "model", text: chunk }];
                    started = true;
                    streaming = true;
                } else {
                    const last = messages[messages.length - 1];
                    if (last) {
                        messages = [
                            ...messages.slice(0, -1),
                            { role: "model", text: last.text + chunk },
                        ];
                    }
                }
            }
            if (!started) {
                messages = next.slice(0, -1);
                input = text;
            }
        } catch (err) {
            if (!started) {
                messages = next.slice(0, -1);
                input = text;
            }
            showError(err, "Coach is unavailable. Try again.");
        } finally {
            sending = false;
            streaming = false;
        }
    }

    function onKeyDown(e: KeyboardEvent) {
        if (e.key === "Enter" && !e.shiftKey) {
            e.preventDefault();
            send();
        }
    }
</script>

<div class="coach">
    <div class="range-picker">
        <span class="range-label">Context:</span>
        {#each [1, 2, 3, 4] as w}
            <button
                class="range-btn"
                class:active={weeks === w}
                onclick={() => (weeks = w)}
                disabled={sending}
                >{w}w</button
            >
        {/each}
    </div>
    <div class="messages" bind:this={scrollEl}>
        {#if messages.length === 0}
            <p class="empty">
                Ask your coach about the last {weeks * 7} days — patterns, gaps, swaps, ideas.
            </p>
        {/if}
        {#each messages as msg, i (i)}
            <div class="msg" class:user={msg.role === "user"} class:model={msg.role === "model"}>
                <!-- eslint-disable-next-line svelte/no-at-html-tags -->
                <div class="bubble">{@html renderInsight(msg.text)}</div>
            </div>
        {/each}
        {#if sending && !streaming}
            <div class="msg model">
                <div class="bubble typing">
                    <span></span><span></span><span></span>
                </div>
            </div>
        {/if}
    </div>

    {#if pinned}
        <div class="pinned" class:expanded={pinnedExpanded}>
            <button
                class="pinned-remove"
                onclick={() => (pinned = null)}
                aria-label="Remove context"
                type="button"
            >✕</button>
            <div class="pinned-body">
                <!-- eslint-disable-next-line svelte/no-at-html-tags -->
                <div class="pinned-text">{@html renderInsight(pinned)}</div>
                {#if pinned.length > 140}
                    <button
                        class="pinned-toggle"
                        onclick={() => (pinnedExpanded = !pinnedExpanded)}
                        type="button"
                    >{pinnedExpanded ? "Show less" : "Show more"}</button>
                {/if}
            </div>
        </div>
    {/if}

    <div class="input-row">
        <textarea
            class="text-entry composer-input"
            bind:this={inputEl}
            bind:value={input}
            onkeydown={onKeyDown}
            onfocus={scrollInputIntoView}
            placeholder={pinned ? "Add details (optional)…" : `Ask about your last ${weeks * 7} days…`}
            rows="1"
            disabled={sending}
        ></textarea>
        <button onclick={send} disabled={sending || (!input.trim() && !pinned)}>Send</button>
    </div>
</div>

<style>
    .coach {
        display: flex;
        flex-direction: column;
        flex: 1;
        min-height: 0;
        gap: 0.5rem;
    }

    .range-picker {
        display: flex;
        align-items: center;
        gap: 0.35rem;
        padding: 0.1rem 0;
    }

    .range-label {
        font-size: 0.72rem;
        color: var(--mute);
        margin-right: 0.15rem;
    }

    .range-btn {
        background: none;
        border: 1px solid var(--rule-3);
        border-radius: var(--r-pill);
        color: var(--mute);
        font-family: inherit;
        font-size: 0.72rem;
        letter-spacing: 0.02em;
        padding: 0.2rem 0.6rem;
        cursor: pointer;
        touch-action: manipulation;
        min-height: 0;
        transition: border-color 0.12s, color 0.12s, background 0.12s;
    }

    .range-btn.active {
        border-color: var(--ink-2);
        color: var(--ink-2);
        background: var(--paper-2);
    }

    .range-btn:disabled {
        opacity: 0.5;
        cursor: default;
    }

    @media (hover: hover) {
        .range-btn:not(.active):not(:disabled):hover {
            border-color: var(--mute-2);
            color: var(--ink-mute);
        }
    }

    .messages {
        flex: 1;
        min-height: 0;
        overflow-y: auto;
        overscroll-behavior: contain;
        display: flex;
        flex-direction: column;
        gap: 0.5rem;
        padding: 0.25rem 0;
    }

    .empty {
        font-size: var(--t-meta);
        color: var(--mute-2);
        text-align: center;
        margin: 1rem 0;
        line-height: 1.5;
    }

    .msg {
        display: flex;
        max-width: 100%;
    }

    .msg.user {
        justify-content: flex-end;
    }

    .msg.model {
        justify-content: flex-start;
    }

    .bubble {
        max-width: 85%;
        padding: 0.55rem 0.8rem;
        border-radius: var(--r-md);
        font-size: var(--t-body-sm);
        line-height: 1.5;
        white-space: pre-line;
        overflow-wrap: break-word;
        word-break: break-word;
    }

    .msg.user .bubble {
        background: var(--ink-2);
        color: var(--paper);
        border-bottom-right-radius: var(--r-sm);
    }

    .msg.model .bubble {
        background: var(--paper-2);
        color: var(--ink);
        border-bottom-left-radius: var(--r-sm);
    }

    .bubble :global(strong) {
        font-weight: 600;
    }

    .typing {
        display: inline-flex;
        gap: 4px;
        align-items: center;
    }

    .typing span {
        width: 6px;
        height: 6px;
        border-radius: 50%;
        background: var(--mute-3);
        animation: bounce 1.2s infinite ease-in-out;
    }

    .typing span:nth-child(2) {
        animation-delay: 0.15s;
    }
    .typing span:nth-child(3) {
        animation-delay: 0.3s;
    }

    @keyframes bounce {
        0%, 80%, 100% { transform: translateY(0); opacity: 0.5; }
        40% { transform: translateY(-4px); opacity: 1; }
    }

    .pinned {
        position: relative;
        padding: 0.5rem 2rem 0.5rem 0.7rem;
        border-left: 3px solid var(--ink-2);
        background: var(--paper-2);
        border-radius: 0 var(--r-sm) var(--r-sm) 0;
        font-size: var(--t-meta);
        color: var(--mute);
        line-height: 1.45;
    }

    .pinned-body {
        display: flex;
        flex-direction: column;
        gap: 0.25rem;
    }

    .pinned-text {
        white-space: pre-line;
        overflow-wrap: break-word;
        word-break: break-word;
        display: -webkit-box;
        -webkit-line-clamp: 3;
        line-clamp: 3;
        -webkit-box-orient: vertical;
        overflow: hidden;
    }

    .pinned.expanded .pinned-text {
        -webkit-line-clamp: unset;
        line-clamp: unset;
        overflow: visible;
        max-height: 40vh;
        overflow-y: auto;
    }

    .pinned-toggle {
        align-self: flex-start;
        background: none;
        border: none;
        color: var(--ink-2);
        font-family: inherit;
        font-size: 0.7rem;
        cursor: pointer;
        padding: 0;
        min-height: 0;
    }

    .pinned-remove {
        position: absolute;
        top: 0.25rem;
        right: 0.35rem;
        background: none;
        border: none;
        color: var(--mute-3);
        font-size: 0.85rem;
        cursor: pointer;
        padding: 0.2rem 0.35rem;
        line-height: 1;
        min-height: 0;
    }

    @media (hover: hover) {
        .pinned-remove:hover {
            color: var(--ink-2);
        }
    }

    .input-row {
        display: flex;
        gap: 0.5rem;
        align-items: flex-end;
    }

    .composer-input {
        flex: 1;
        min-height: 2.75rem;
    }

    textarea {
        flex: 1;
        border: 1px solid var(--rule);
        border-radius: var(--r-sm);
        padding: 0.5rem 0.75rem;
        font-size: var(--t-body);
        resize: none;
        font-family: inherit;
        background: var(--paper);
        color: var(--ink);
    }

    textarea:focus {
        outline: none;
        border-color: var(--ink-2);
    }

    button {
        padding: 0.6rem 1rem;
        background: var(--ink-2);
        color: var(--paper);
        border: none;
        border-radius: var(--r-sm);
        cursor: pointer;
        font-size: var(--t-body-sm);
        font-family: inherit;
        white-space: nowrap;
        min-height: 2.75rem;
    }

    button:disabled {
        opacity: 0.35;
        cursor: default;
    }
</style>
