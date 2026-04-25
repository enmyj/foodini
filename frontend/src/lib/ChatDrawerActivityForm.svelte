<script lang="ts">
    import { autosize } from "./autosize.ts";
    import type { ActivityField } from "./types.ts";

    let {
        active,
        initialField = null,
        activityText = $bindable(""),
        feelingNotes = $bindable(""),
        poop = $bindable(false),
        poopNotes = $bindable(""),
        hydration = $bindable(""),
    }: {
        active: boolean;
        initialField?: ActivityField | null;
        activityText?: string;
        feelingNotes?: string;
        poop?: boolean;
        poopNotes?: string;
        hydration?: string;
    } = $props();

    let activityTextareaEl = $state<HTMLTextAreaElement | null>(null);
    let feelingNotesEl = $state<HTMLTextAreaElement | null>(null);
    let poopNotesEl = $state<HTMLTextAreaElement | null>(null);
    let hydrationEl = $state<HTMLInputElement | null>(null);

    $effect(() => {
        if (!active || !initialField) return;
        setTimeout(() => {
            if (initialField === "activity") activityTextareaEl?.focus();
            else if (initialField === "feeling") feelingNotesEl?.focus();
            else if (initialField === "poop") poopNotesEl?.focus();
            else if (initialField === "hydration") hydrationEl?.focus();
        }, 120);
    });
</script>

<div class="activity-form">
    <div class="activity-field">
        <label class="field-label" for="act-activity">Activity</label>
        <textarea
            class="text-entry"
            id="act-activity"
            bind:this={activityTextareaEl}
            use:autosize
            bind:value={activityText}
            placeholder="Exercise, stress, unusual events…"
            rows="2"
        ></textarea>
    </div>
    <div class="activity-field">
        <label class="field-label" for="act-feeling">Feeling</label>
        <textarea
            class="text-entry"
            id="act-feeling"
            bind:this={feelingNotesEl}
            use:autosize
            bind:value={feelingNotes}
            placeholder="Energy, digestion, mood, sleep…"
            rows="2"
        ></textarea>
    </div>
    <div class="activity-field">
        <div class="field-header">
            <span class="field-label">Stool</span>
            <div class="toggle-group">
                <button
                    class="toggle-btn"
                    class:selected={poop === true}
                    onclick={() => (poop = true)}
                >
                    Yes
                </button>
                <button
                    class="toggle-btn"
                    class:selected={poop === false}
                    onclick={() => (poop = false)}
                >
                    No
                </button>
            </div>
        </div>
        <textarea
            class="text-entry"
            bind:this={poopNotesEl}
            use:autosize
            bind:value={poopNotes}
            placeholder="Any details…"
            rows="1"
        ></textarea>
    </div>
    <div class="activity-field">
        <div class="field-header">
            <label class="field-label" for="act-hydration">Water</label>
            <div class="hydration-inline">
                <input
                    id="act-hydration"
                    bind:this={hydrationEl}
                    class="hydration-input"
                    type="number"
                    min="0"
                    max="10"
                    step="0.1"
                    bind:value={hydration}
                    placeholder="0.0"
                />
                <span class="hydration-unit">L</span>
            </div>
        </div>
    </div>
</div>

<style>
    .activity-form {
        display: flex;
        flex-direction: column;
        gap: 1.5rem;
        min-height: 0;
        overflow-y: auto;
        flex: 1;
    }

    .activity-field {
        display: flex;
        flex-direction: column;
        gap: 0.45rem;
    }

    .field-label {
        font-size: var(--t-micro);
        text-transform: uppercase;
        letter-spacing: 0.08em;
        font-weight: 600;
        color: var(--mute-2);
    }

    .field-header {
        display: flex;
        align-items: center;
        gap: 0.75rem;
    }

    .toggle-group {
        display: flex;
        gap: 0.4rem;
        flex-shrink: 0;
    }

    .hydration-inline {
        display: flex;
        align-items: center;
        gap: 0.4rem;
        flex-shrink: 0;
    }

    .toggle-btn {
        padding: 0.3rem 0.85rem;
        border: 1px solid var(--rule-4);
        border-radius: var(--r-pill);
        background: none;
        font-family: inherit;
        font-size: var(--t-meta);
        color: var(--ink-mute);
        cursor: pointer;
        font-weight: 500;
        flex-shrink: 0;
        touch-action: manipulation;
    }

    .toggle-btn.selected {
        background: var(--ink-2);
        border-color: var(--ink-2);
        color: var(--paper);
    }

    .hydration-input {
        width: 80px;
        flex-shrink: 0;
        border: 1px solid var(--rule);
        border-radius: var(--r-sm);
        padding: 0.5rem 0.5rem;
        font-size: var(--t-body);
        font-family: var(--num-stack);
        background: var(--paper);
        color: var(--ink);
        text-align: center;
        font-variant-numeric: tabular-nums;
    }

    .hydration-input::-webkit-outer-spin-button,
    .hydration-input::-webkit-inner-spin-button {
        -webkit-appearance: none;
        margin: 0;
    }

    .hydration-input:focus {
        outline: none;
        border-color: var(--ink-2);
    }

    .hydration-unit {
        font-size: var(--t-meta);
        color: var(--mute-2);
    }
</style>
