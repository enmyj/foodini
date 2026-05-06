<script lang="ts">
    import { untrack } from "svelte";
    import { addEvent, deleteEvent, deleteFueling, patchEvent } from "./api.ts";
    import { showError } from "./toast.ts";
    import { EVENT_KINDS } from "./types.ts";
    import type { EventKind, FuelingEntry, LogEvent } from "./types.ts";

    let {
        date,
        time,
        editEvent,
        existingFueling = [],
        onSaved,
        onDeleted,
        onDone,
        onSaveAndFuel = null,
        onFuelingDeleted = null,
    }: {
        date: string;
        time: string;
        editEvent: LogEvent | null;
        existingFueling?: FuelingEntry[];
        onSaved: (change: { added?: LogEvent; updated?: LogEvent }) => void;
        onDeleted: (id: string) => void;
        onDone: () => void;
        onSaveAndFuel?: ((saved: LogEvent) => void) | null;
        onFuelingDeleted?: ((id: string) => void) | null;
    } = $props();

    const EVENT_KIND_LABELS: Record<EventKind, string> = {
        workout: "Activity",
        stool: "Bowel",
        water: "Water",
        feeling: "Feeling",
    };

    let kind = $state<EventKind | null>(null);
    let text = $state("");
    let waterMl = $state(250);
    let feelingScore = $state(7);
    let durationMin = $state<number | null>(null);
    let saving = $state(false);
    let deleting = $state(false);

    $effect(() => {
        const ev = editEvent;
        untrack(() => {
            if (ev) {
                kind = ev.kind;
                text = ev.text ?? "";
                if (ev.kind === "water") {
                    waterMl = Math.max(0, Math.round(ev.num ?? 250));
                } else if (ev.kind === "feeling") {
                    feelingScore = Math.max(1, Math.min(10, Math.round(ev.num ?? 7)));
                } else if (ev.kind === "workout") {
                    durationMin = ev.num && ev.num > 0 ? Math.round(ev.num) : null;
                }
            } else {
                kind = null;
                text = "";
                waterMl = 250;
                feelingScore = 7;
                durationMin = null;
            }
            saving = false;
            deleting = false;
        });
    });

    function selectKind(k: EventKind) {
        kind = k;
        text = "";
        waterMl = 250;
        feelingScore = 7;
        durationMin = null;
    }

    function nowHHMM(): string {
        const d = new Date();
        return `${String(d.getHours()).padStart(2, "0")}:${String(d.getMinutes()).padStart(2, "0")}`;
    }

    async function save(thenFuel = false) {
        if (!kind || saving) return;
        const t = time || nowHHMM();
        const trimmed = text.trim();
        const payload: {
            date: string;
            time: string;
            kind: EventKind;
            text?: string;
            num?: number;
        } = { date, time: t, kind };
        if (kind === "water") payload.num = Math.max(0, Math.round(waterMl));
        else if (kind === "feeling") {
            payload.num = Math.max(1, Math.min(10, Math.round(feelingScore)));
            if (trimmed) payload.text = trimmed;
        } else if (kind === "workout") {
            if (trimmed) payload.text = trimmed;
            if (durationMin && durationMin > 0) {
                payload.num = Math.round(durationMin);
            }
        } else {
            if (trimmed) payload.text = trimmed;
        }
        saving = true;
        try {
            let saved: LogEvent;
            if (editEvent) {
                const patch: Partial<LogEvent> = {
                    date: payload.date,
                    time: payload.time,
                    kind: payload.kind,
                    text: payload.text ?? "",
                };
                if (payload.num !== undefined) patch.num = payload.num;
                saved = await patchEvent(editEvent.id, patch);
                onSaved({ updated: saved });
            } else {
                saved = await addEvent(payload);
                onSaved({ added: saved });
            }
            if (thenFuel && onSaveAndFuel) {
                onSaveAndFuel(saved);
            } else {
                onDone();
            }
        } catch (err) {
            showError(err, "Failed to save event.");
        } finally {
            saving = false;
        }
    }

    let deletingFuelIds = $state<Set<string>>(new Set());

    async function removeFuel(f: FuelingEntry) {
        if (deletingFuelIds.has(f.id)) return;
        deletingFuelIds = new Set([...deletingFuelIds, f.id]);
        try {
            await deleteFueling(f.id);
            onFuelingDeleted?.(f.id);
        } catch (err) {
            showError(err, "Failed to delete fuel.");
        } finally {
            deletingFuelIds = new Set(
                [...deletingFuelIds].filter((id) => id !== f.id),
            );
        }
    }

    async function deleteEditing() {
        if (!editEvent || deleting || saving) return;
        deleting = true;
        try {
            await deleteEvent(editEvent.id);
            onDeleted(editEvent.id);
            onDone();
        } catch (err) {
            showError(err, "Failed to delete event.");
        } finally {
            deleting = false;
        }
    }
</script>

<div class="event-form">
    <div class="kind-bubbles">
        {#each EVENT_KINDS as k}
            <button
                class="kind-bubble"
                class:active={kind === k}
                onclick={() => selectKind(k)}
            >{EVENT_KIND_LABELS[k]}</button>
        {/each}
    </div>
    {#if kind}
        <div class="event-fields">
            {#if kind === "water"}
                <label class="field-row">
                    <span class="field-label">Amount (ml)</span>
                    <input type="number" min="0" max="5000" step="50" bind:value={waterMl} />
                </label>
                <div class="quick-pills">
                    {#each [125, 250, 500, 750, 1000] as ml}
                        <button class="quick-pill" onclick={() => (waterMl = ml)}>{ml}ml</button>
                    {/each}
                </div>
            {:else if kind === "feeling"}
                <label class="field-row">
                    <span class="field-label">Score (1-10)</span>
                    <div class="slider-wrap">
                        <input type="range" min="1" max="10" step="1" bind:value={feelingScore} />
                        <span class="slider-val">{feelingScore}</span>
                    </div>
                </label>
                <label class="field-row stacked">
                    <span class="field-label">Note (optional)</span>
                    <textarea rows="2" bind:value={text} placeholder="energy, mood, anything notable"></textarea>
                </label>
            {:else if kind === "stool"}
                <label class="field-row stacked">
                    <span class="field-label">Note (optional)</span>
                    <textarea rows="2" bind:value={text} placeholder="consistency, urgency, etc."></textarea>
                </label>
            {:else if kind === "workout"}
                <label class="field-row stacked">
                    <span class="field-label">What did you do?</span>
                    <textarea rows="3" bind:value={text} placeholder="e.g. morning ride, Z2 spin, bench 3×8"></textarea>
                </label>
                <label class="field-row">
                    <span class="field-label">Duration (min)</span>
                    <input
                        type="number"
                        min="0"
                        max="1440"
                        step="5"
                        bind:value={durationMin}
                        placeholder="optional"
                    />
                </label>
                {#if editEvent && existingFueling.length}
                    <div class="fueling-block">
                        <div class="fueling-block-label">Fuel logged</div>
                        <ul class="fueling-list">
                            {#each existingFueling as f (f.id)}
                                <li class="fueling-row" class:dimmed={deletingFuelIds.has(f.id)}>
                                    <span class="fueling-time">{f.time}</span>
                                    <span class="fueling-desc">{f.description}</span>
                                    <span class="fueling-carbs">{f.carbs_g}g</span>
                                    <button
                                        type="button"
                                        class="fueling-del"
                                        aria-label="Delete fuel"
                                        title="Delete"
                                        disabled={deletingFuelIds.has(f.id)}
                                        onclick={() => removeFuel(f)}
                                    >×</button>
                                </li>
                            {/each}
                        </ul>
                    </div>
                {/if}
            {/if}
            <div class="event-actions">
                {#if editEvent}
                    <button class="event-delete" onclick={deleteEditing} disabled={saving || deleting}>
                        {deleting ? "Deleting…" : "Delete"}
                    </button>
                {/if}
                {#if kind === "workout" && onSaveAndFuel}
                    <button
                        class="event-save-fuel"
                        onclick={() => save(true)}
                        disabled={saving || deleting}
                        title="Save activity details and open fuel chat"
                    >
                        {existingFueling.length ? "Edit fuel" : "Add fuel"}
                    </button>
                {/if}
                <button class="event-save" onclick={() => save(false)} disabled={saving || deleting}>
                    {saving ? "Saving…" : "Save"}
                </button>
            </div>
        </div>
    {/if}
</div>

<style>
    .event-form {
        display: flex;
        flex-direction: column;
        gap: 1rem;
        padding: 0.5rem 0 0.25rem;
        flex: 1;
        overflow-y: auto;
        min-height: 0;
    }

    .kind-bubbles {
        display: flex;
        gap: 0.4rem;
        flex-wrap: wrap;
    }

    .kind-bubble {
        background: none;
        border: 1px solid var(--rule-3);
        border-radius: var(--r-pill);
        color: var(--mute);
        font-size: var(--t-body-sm);
        padding: 0.4rem 0.85rem;
        cursor: pointer;
        font-family: inherit;
        font-weight: 500;
        touch-action: manipulation;
        transition: border-color 0.12s, color 0.12s, background 0.12s;
    }

    .kind-bubble.active {
        border-color: var(--ink-2);
        color: var(--ink-2);
        background: var(--paper-2);
    }

    @media (hover: hover) {
        .kind-bubble:hover {
            border-color: var(--ink-2);
            color: var(--ink-2);
        }
    }

    .event-fields {
        display: flex;
        flex-direction: column;
        gap: 0.85rem;
    }

    .field-row {
        display: flex;
        align-items: center;
        gap: 0.75rem;
    }

    .field-row.stacked {
        flex-direction: column;
        align-items: stretch;
        gap: 0.35rem;
    }

    .field-label {
        font-size: var(--t-meta);
        color: var(--mute);
        text-transform: uppercase;
        letter-spacing: 0.04em;
        font-weight: 600;
        min-width: 5.5rem;
    }

    /* font-size: 16px on focusable inputs prevents iOS Safari auto-zoom-and-stuck */
    .field-row input[type="number"] {
        border: 1px solid var(--rule);
        border-radius: var(--r-sm);
        padding: 0.45rem 0.65rem;
        font-family: inherit;
        font-size: 16px;
        color: var(--ink);
        background: var(--paper);
        font-variant-numeric: tabular-nums;
        width: 6rem;
    }

    .field-row textarea {
        border: 1px solid var(--rule);
        border-radius: var(--r-sm);
        padding: 0.5rem 0.65rem;
        font-family: inherit;
        font-size: 16px;
        background: var(--paper);
        color: var(--ink);
        resize: vertical;
    }

    .quick-pills {
        display: flex;
        gap: 0.35rem;
        flex-wrap: wrap;
    }

    .quick-pill {
        background: none;
        border: 1px solid var(--rule-3);
        border-radius: var(--r-pill);
        color: var(--mute);
        font-size: var(--t-meta);
        padding: 0.2rem 0.6rem;
        cursor: pointer;
        font-family: inherit;
        touch-action: manipulation;
    }

    @media (hover: hover) {
        .quick-pill:hover {
            border-color: var(--ink-2);
            color: var(--ink-2);
        }
    }

    .slider-wrap {
        display: flex;
        align-items: center;
        gap: 0.6rem;
        flex: 1;
    }

    .slider-wrap input[type="range"] {
        flex: 1;
    }

    .slider-val {
        font-variant-numeric: tabular-nums;
        font-weight: 600;
        color: var(--ink);
        min-width: 1.5rem;
        text-align: right;
    }

    .event-actions {
        display: flex;
        justify-content: space-between;
        align-items: center;
        gap: 0.5rem;
        margin-top: 0.5rem;
    }

    .event-actions :only-child {
        margin-left: auto;
    }

    .event-delete {
        background: none;
        color: var(--danger, #c00);
        border: 1px solid var(--rule);
        border-radius: var(--r-sm);
        padding: 0.55rem 1rem;
        font-size: var(--t-body-sm);
        font-family: inherit;
        font-weight: 500;
        cursor: pointer;
        min-height: 2.5rem;
    }

    .event-delete:disabled {
        opacity: 0.45;
        cursor: default;
    }

    @media (hover: hover) {
        .event-delete:not(:disabled):hover {
            border-color: var(--danger, #c00);
        }
    }

    .event-save {
        background: var(--ink-2);
        color: var(--paper);
        border: none;
        border-radius: var(--r-sm);
        padding: 0.55rem 1.4rem;
        font-size: var(--t-body-sm);
        font-family: inherit;
        font-weight: 500;
        cursor: pointer;
        min-height: 2.5rem;
    }

    .event-save-fuel {
        background: none;
        color: var(--ink-2);
        border: 1px solid var(--ink-2);
        border-radius: var(--r-sm);
        padding: 0.55rem 0.9rem;
        font-size: var(--t-body-sm);
        font-family: inherit;
        font-weight: 500;
        cursor: pointer;
        min-height: 2.5rem;
    }

    .event-save-fuel:disabled {
        opacity: 0.45;
        cursor: default;
    }

    .fueling-block {
        display: flex;
        flex-direction: column;
        gap: 0.4rem;
        padding: 0.6rem 0.75rem;
        background: var(--paper-2);
        border: 1px solid var(--rule-3);
        border-radius: var(--r-sm);
    }

    .fueling-block-label {
        font-size: var(--t-meta);
        color: var(--mute);
        text-transform: uppercase;
        letter-spacing: 0.04em;
        font-weight: 600;
    }

    .fueling-list {
        list-style: none;
        margin: 0;
        padding: 0;
        display: flex;
        flex-direction: column;
        gap: 0.15rem;
    }

    .fueling-row {
        display: flex;
        align-items: baseline;
        gap: 0.6rem;
        font-size: var(--t-body-sm);
    }

    .fueling-row.dimmed {
        opacity: 0.5;
    }

    .fueling-time {
        font-variant-numeric: tabular-nums;
        color: var(--mute);
        font-size: var(--t-meta);
        min-width: 3rem;
    }

    .fueling-desc {
        flex: 1;
        color: var(--ink);
    }

    .fueling-carbs {
        font-variant-numeric: tabular-nums;
        color: var(--mute);
    }

    .fueling-del {
        background: none;
        border: none;
        color: var(--mute);
        cursor: pointer;
        font-size: 1rem;
        padding: 0 0.25rem;
        font-family: inherit;
    }

    .fueling-del:disabled {
        opacity: 0.45;
        cursor: default;
    }

    .event-save:disabled {
        opacity: 0.45;
        cursor: default;
    }

    @media (hover: hover) {
        .event-save:not(:disabled):hover {
            background: var(--ink);
        }
    }

    @media (max-width: 480px) {
        .field-label {
            min-width: 4.5rem;
            font-size: 0.65rem;
        }
    }
</style>
