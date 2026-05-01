<script lang="ts">
    import { formatWeekRange } from "./date.ts";
    import InsightPanel from "./InsightPanel.svelte";
    import type { WeekGroup, WeekInsightPanelState } from "./types.ts";

    let {
        week,
        dayAbbrev,
        insightState = null,
        suggestionState = null,
        onOpenDay,
        onToggleInsights,
        onToggleSuggestions,
        onRegenerateInsights,
        onRegenerateSuggestions,
        onDiscussInsights = null,
        onDiscussSuggestions = null,
    }: {
        week: WeekGroup;
        dayAbbrev: string[];
        insightState?: WeekInsightPanelState | null;
        suggestionState?: WeekInsightPanelState | null;
        onOpenDay: (date: string) => void;
        onToggleInsights: () => void;
        onToggleSuggestions: () => void;
        onRegenerateInsights: () => void;
        onRegenerateSuggestions: () => void;
        onDiscussInsights?: (() => void) | null;
        onDiscussSuggestions?: (() => void) | null;
    } = $props();
</script>

<div class="week-block">
    <div class="week-head">
        <div class="week-meta">
            <span class="week-range"
                >{formatWeekRange(week.weekStart, week.weekEnd)}</span
            >
            {#if week.weekTotal > 0}
                <span class="week-cal"
                    >{week.weekTotal.toLocaleString()} cal</span
                >
            {/if}
        </div>
        {#if week.weekTotal > 0 || week.days.some((day) => day.events.length > 0)}
            <div class="week-btns">
                <button
                    class="insights-btn"
                    class:active={insightState?.open}
                    onclick={onToggleInsights}
                    aria-label="AI insights for this week"
                    title="AI insights"
                >
                    insights
                </button>
                <button
                    class="insights-btn suggestions-btn"
                    class:active={suggestionState?.open}
                    onclick={onToggleSuggestions}
                    aria-label="Meal suggestions for this week"
                    title="Meal suggestions"
                >
                    suggestions
                </button>
            </div>
        {/if}
    </div>
    <div class="week-grid">
        {#each week.days as day}
            <button
                class="day-cell"
                class:future={day.future}
                class:has-food={day.entries.length > 0}
                onclick={() => {
                    if (!day.future) onOpenDay(day.date);
                }}
                disabled={day.future}
                aria-label={day.date}
            >
                <span class="dc-abbrev"
                    >{dayAbbrev[new Date(day.date + "T12:00:00").getDay()]}</span
                >
                <span class="dc-num"
                    >{new Date(day.date + "T12:00:00").getDate()}</span
                >
                <span class="dc-indicators">
                    {#if day.entries.length > 0}
                        <span class="dc-food">●</span>
                    {:else}
                        <span class="dc-empty">○</span>
                    {/if}
                    {#if day.events.some((e) => e.kind === 'stool')}
                        <span class="dc-poop">💩</span>
                    {/if}
                </span>
            </button>
        {/each}
    </div>
    {#if insightState?.open}
        <InsightPanel
            loading={insightState.loading}
            error={insightState.error}
            text={insightState.text}
            generatedAt={insightState.generatedAt}
            onRegenerate={onRegenerateInsights}
            onDiscuss={onDiscussInsights}
        />
    {/if}
    {#if suggestionState?.open}
        <InsightPanel
            loading={suggestionState.loading}
            error={suggestionState.error}
            text={suggestionState.text}
            generatedAt={suggestionState.generatedAt}
            label="Meal ideas for next week"
            variant="suggestion"
            onRegenerate={onRegenerateSuggestions}
            onDiscuss={onDiscussSuggestions}
        />
    {/if}
</div>

<style>
    .week-block {
        border-top: 1px solid var(--rule);
        margin-bottom: 1.25rem;
    }

    .week-block:last-of-type {
        border-bottom: 1px solid var(--rule);
    }

    .week-head {
        display: flex;
        justify-content: space-between;
        align-items: center;
        padding: 0.65rem 0;
    }

    .week-meta {
        display: flex;
        flex-direction: column;
        gap: 0.05rem;
    }

    .week-range {
        font-size: var(--t-body-sm);
        font-weight: 600;
        color: var(--ink);
    }

    .week-cal {
        font-size: 0.72rem;
        color: var(--mute-2);
    }

    .week-btns {
        display: flex;
        gap: 0.35rem;
    }

    .insights-btn {
        background: none;
        border: 1px solid var(--rule-3);
        border-radius: var(--r-pill);
        color: var(--mute);
        font-size: 0.72rem;
        padding: 0.2rem 0.65rem;
        cursor: pointer;
        touch-action: manipulation;
        font-family: inherit;
        letter-spacing: 0.02em;
        white-space: nowrap;
        transition:
            border-color 0.12s,
            color 0.12s,
            background 0.12s;
    }

    .insights-btn.active {
        border-color: var(--ink-2);
        color: var(--ink-2);
        background: var(--paper-2);
    }

    @media (hover: hover) {
        .insights-btn:hover {
            border-color: var(--ink-2);
            color: var(--ink-2);
        }
    }

    .suggestions-btn {
        border-color: var(--sugg-rule);
        color: var(--sugg-mute);
    }

    .suggestions-btn.active {
        border-color: var(--sugg-mute);
        color: var(--sugg-ink);
        background: var(--sugg-paper);
    }

    @media (hover: hover) {
        .suggestions-btn:hover {
            border-color: var(--sugg-mute);
            color: var(--sugg-ink);
        }
    }

    .week-grid {
        display: grid;
        grid-template-columns: repeat(7, 1fr);
        padding: 0.4rem 0 0.5rem;
        gap: 0;
    }

    .day-cell {
        background: none;
        border: none;
        display: flex;
        flex-direction: column;
        align-items: center;
        gap: 0.1rem;
        padding: 0.4rem 0.1rem;
        cursor: pointer;
        border-radius: var(--r-sm);
        touch-action: manipulation;
        font-family: inherit;
    }

    @media (hover: hover) {
        .day-cell:not(.future):hover {
            background: var(--paper-4);
        }
    }

    .day-cell.future {
        opacity: 0.2;
        cursor: default;
    }

    .dc-abbrev {
        font-size: 0.62rem;
        color: var(--mute-2);
        text-transform: uppercase;
        letter-spacing: 0.03em;
        font-weight: 500;
        line-height: 1;
    }

    .dc-num {
        font-size: var(--t-meta);
        font-weight: 500;
        color: var(--ink);
        line-height: 1.2;
    }

    .day-cell.has-food .dc-num {
        color: var(--ink);
    }

    .dc-indicators {
        display: flex;
        gap: 0.1rem;
        align-items: center;
        min-height: 0.9rem;
    }

    .dc-food {
        font-size: 0.4rem;
        color: var(--ink-2);
        line-height: 1;
    }

    .dc-empty {
        font-size: 0.4rem;
        color: var(--mute-4);
        line-height: 1;
    }

    .dc-poop {
        font-size: 0.5rem;
        line-height: 1;
    }
</style>
