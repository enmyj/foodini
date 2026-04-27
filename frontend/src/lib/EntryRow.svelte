<script lang="ts">
    import { createMutation } from "@tanstack/svelte-query";
    import { patchEntry, deleteEntry } from "./api.ts";
    import { autosize } from "./autosize.ts";
    import { showError } from "./toast.ts";
    import { MEAL_ORDER } from "./types.ts";
    import type { Entry, Favorite, MealType } from "./types.ts";

    let {
        entry,
        onUpdate,
        onDelete,
        onFavorite = null,
        isFavorited = false,
    }: {
        entry: Entry;
        onUpdate: (entry: Entry) => void;
        onDelete: (id: string) => void;
        onFavorite?: ((entry: Entry) => Promise<Favorite | void>) | null;
        isFavorited?: boolean;
    } = $props();

    const MEALS = [...MEAL_ORDER];

    let modalOpen = $state(false);
    let editDesc = $state("");
    let editMeal = $state<MealType>("breakfast");
    let editCal = $state(0);
    let editProtein = $state(0);
    let editCarbs = $state(0);
    let editFat = $state(0);
    let editFiber = $state(0);
    let editTime = $state("");
    let saving = $state(false);
    let deleting = $state(false);
    let favoriting = $state(false);
    let pendingDelete = $state(false);
    let deleteTimer = $state<ReturnType<typeof setTimeout> | null>(null);

    const saveMutation = createMutation(() => ({
        mutationFn: (updated: Entry) => patchEntry(updated.id, updated),
        onError: (err) => showError(err, "Failed to save entry."),
    }));

    const deleteMutation = createMutation(() => ({
        mutationFn: (id: string) => deleteEntry(id),
        onError: (err) => showError(err, "Failed to delete entry."),
    }));

    function openModal() {
        editDesc = entry.description;
        editMeal = entry.meal_type;
        editCal = entry.calories;
        editProtein = entry.protein;
        editCarbs = entry.carbs;
        editFat = entry.fat;
        editFiber = entry.fiber ?? 0;
        editTime = entry.time ?? "";
        modalOpen = true;
    }

    async function save() {
        if (saving || saveMutation.isPending) return;
        saving = true;
        try {
            const updated = {
                ...entry,
                description: editDesc,
                meal_type: editMeal,
                calories: editCal,
                protein: editProtein,
                carbs: editCarbs,
                fat: editFat,
                fiber: editFiber,
                time: editTime,
            };
            const saved = await saveMutation.mutateAsync(updated);
            onUpdate(saved);
            modalOpen = false;
        } catch {
        } finally {
            saving = false;
        }
    }

    function handleDelete() {
        if (deleting) return;
        if (!pendingDelete) {
            pendingDelete = true;
            deleteTimer = setTimeout(() => {
                pendingDelete = false;
            }, 2500);
            return;
        }
        if (deleteTimer) clearTimeout(deleteTimer);
        pendingDelete = false;
        doDelete();
    }

    async function doDelete() {
        deleting = true;
        try {
            await deleteMutation.mutateAsync(entry.id);
            onDelete(entry.id);
        } catch {
            deleting = false;
        }
    }

    function onKeyDown(e: KeyboardEvent) {
        if (e.key === "Escape") modalOpen = false;
        if (e.key === "Enter" && (e.metaKey || e.ctrlKey)) save();
    }

    async function handleFavorite() {
        if (!onFavorite || favoriting) return;
        favoriting = true;
        try {
            await onFavorite(entry);
        } finally {
            favoriting = false;
        }
    }
</script>

<div class="row" class:fading={deleting}>
    <!-- svelte-ignore a11y_click_events_have_key_events -->
    <div class="main" role="button" tabindex="0" onclick={openModal}>
        <span class="desc">{entry.description}</span>
        <span class="macros"
            >{entry.calories} cal · {entry.protein}g P · {entry.carbs}g C · {entry.fat}g
            F{entry.fiber ? ` · ${entry.fiber}g Fb` : ""}</span
        >
    </div>
    {#if onFavorite}
        <button
            class="fav"
            class:starred={isFavorited}
            onclick={handleFavorite}
            disabled={favoriting || isFavorited}
            aria-label={isFavorited ? "Already in favorites" : "Add to favorites"}
            title={isFavorited ? "Already in favorites" : "Add to favorites"}
            >{isFavorited ? "★" : "☆"}</button
        >
    {/if}
</div>

{#if modalOpen}
    <!-- svelte-ignore a11y_click_events_have_key_events -->
    <div
        class="overlay"
        aria-hidden="true"
        onclick={() => (modalOpen = false)}
    ></div>
    <div
        class="modal"
        role="dialog"
        aria-label="Edit entry"
        tabindex="-1"
        onkeydown={onKeyDown}
    >
        <h3>Edit entry</h3>

        <label class="field">
            <span class="label">Description</span>
            <textarea
                class="text-entry"
                use:autosize
                bind:value={editDesc}
                rows="2"
                disabled={saving}
            ></textarea>
        </label>

        <div class="row-fields">
            <label class="field">
                <span class="label">Meal</span>
                <select bind:value={editMeal} disabled={saving}>
                    {#each MEALS as m}
                        <option value={m}>{m}</option>
                    {/each}
                </select>
            </label>
            <label class="field">
                <span class="label">Time</span>
                <input type="time" bind:value={editTime} disabled={saving} />
            </label>
        </div>

        <div class="num-grid">
            <label class="field">
                <span class="label">Calories</span>
                <input type="number" bind:value={editCal} disabled={saving} />
            </label>
            <label class="field">
                <span class="label">Protein (g)</span>
                <input
                    type="number"
                    bind:value={editProtein}
                    disabled={saving}
                />
            </label>
            <label class="field">
                <span class="label">Carbs (g)</span>
                <input type="number" bind:value={editCarbs} disabled={saving} />
            </label>
            <label class="field">
                <span class="label">Fat (g)</span>
                <input type="number" bind:value={editFat} disabled={saving} />
            </label>
            <label class="field">
                <span class="label">Fiber (g)</span>
                <input type="number" bind:value={editFiber} disabled={saving} />
            </label>
        </div>

        <div class="actions">
            <button class="save-btn" onclick={save} disabled={saving}
                >{saving ? "Saving…" : "Save"}</button
            >
            <button
                class="cancel-btn"
                onclick={() => (modalOpen = false)}
                disabled={saving}>Cancel</button
            >
        </div>
        <button
            class="modal-delete"
            class:confirm={pendingDelete}
            onclick={handleDelete}
            disabled={deleting}
        >{pendingDelete ? "Tap again to confirm" : "Delete entry"}</button>
    </div>
{/if}

<style>
    .row {
        display: flex;
        align-items: center;
        padding: 0.75rem 0;
        border-bottom: 1px solid var(--rule);
        gap: 0.5rem;
    }

    .row.fading {
        opacity: 0.4;
    }

    .main {
        flex: 1;
        min-width: 0;
        cursor: pointer;
        display: flex;
        flex-direction: column;
        gap: 0.25rem;
    }

    .main:hover .desc {
        color: var(--ink-mute);
    }

    .desc {
        font-size: 0.95rem;
        line-height: 1.4;
        color: var(--ink);
    }

    .macros {
        font-size: 0.78rem;
        color: var(--mute);
        line-height: 1.3;
        font-variant-numeric: tabular-nums;
    }

    .fav {
        background: none;
        border: none;
        color: var(--mute-4);
        font-size: 1.1rem;
        line-height: 1;
        cursor: pointer;
        padding: 0;
        flex-shrink: 0;
        min-width: 2.25rem;
        min-height: 2.75rem;
        display: flex;
        align-items: center;
        justify-content: center;
        touch-action: manipulation;
    }

    .fav.starred {
        color: var(--ink-2);
    }

    @media (hover: hover) {
        .fav:not(.starred):hover {
            color: var(--ink-2);
        }
    }

    .fav:disabled {
        opacity: 1;
        cursor: default;
    }

    .fav:not(.starred):disabled {
        opacity: 0.35;
    }

    .modal-delete {
        width: 100%;
        margin-top: 1.25rem;
        padding: 0.5rem 1rem;
        background: none;
        border: 1px solid var(--rule);
        border-radius: var(--r-sm);
        color: var(--mute);
        font-size: var(--t-body-sm);
        font-family: inherit;
        cursor: pointer;
        touch-action: manipulation;
    }

    .modal-delete.confirm {
        border-color: var(--danger);
        color: var(--danger);
        font-weight: 500;
    }

    @media (hover: hover) {
        .modal-delete:hover:not(:disabled) {
            border-color: var(--danger);
            color: var(--danger);
        }
    }

    .modal-delete:disabled {
        opacity: 0.35;
        cursor: default;
    }

    /* Modal */
    .overlay {
        position: fixed;
        inset: 0;
        background: rgba(0, 0, 0, 0.25);
        z-index: 30;
    }

    .modal {
        position: fixed;
        top: 50%;
        left: 50%;
        transform: translate(-50%, -50%);
        background: var(--paper);
        border-radius: var(--r-md);
        width: min(92vw, 420px);
        max-height: 85vh;
        overflow-y: auto;
        z-index: 31;
        padding: 1.5rem;
        box-shadow: 0 4px 24px rgba(0, 0, 0, 0.12);
    }

    .modal h3 {
        font-size: 0.95rem;
        font-weight: 600;
        color: var(--ink);
        margin-bottom: 1.25rem;
        text-transform: none;
        letter-spacing: 0;
    }

    .field {
        display: flex;
        flex-direction: column;
        gap: 0.3rem;
        margin-bottom: 1rem;
    }

    .label {
        font-size: var(--t-micro);
        font-weight: 600;
        text-transform: uppercase;
        letter-spacing: 0.06em;
        color: var(--mute);
    }

    textarea,
    input,
    select {
        border: 1px solid var(--rule);
        border-radius: var(--r-sm);
        padding: 0.5rem 0.6rem;
        font-family: inherit;
        font-size: var(--t-body);
        background: var(--paper);
        color: var(--ink);
        outline: none;
        width: 100%;
        box-sizing: border-box;
    }

    textarea:focus,
    input:focus,
    select:focus {
        border-color: var(--ink-2);
    }

    select {
        appearance: none;
        background-image: url("data:image/svg+xml,%3Csvg xmlns='http://www.w3.org/2000/svg' width='12' height='8' viewBox='0 0 12 8'%3E%3Cpath d='M1 1l5 5 5-5' stroke='%23888' stroke-width='1.5' fill='none' stroke-linecap='round'/%3E%3C/svg%3E");
        background-repeat: no-repeat;
        background-position: right 0.6rem center;
        padding-right: 2rem;
        cursor: pointer;
    }

    .num-grid {
        display: grid;
        grid-template-columns: repeat(3, 1fr);
        gap: 0.75rem;
        margin-bottom: 1rem;
    }

    .row-fields {
        display: grid;
        grid-template-columns: 1fr 1fr;
        gap: 0.75rem;
        margin-bottom: 1rem;
    }

    .row-fields .field {
        margin-bottom: 0;
    }

    .num-grid .field {
        margin-bottom: 0;
    }

    .actions {
        display: flex;
        gap: 0.5rem;
        margin-top: 0.25rem;
    }

    .save-btn {
        flex: 1;
        padding: 0.6rem 1rem;
        background: var(--ink-2);
        color: var(--paper);
        border: none;
        border-radius: var(--r-sm);
        cursor: pointer;
        font-size: var(--t-body-sm);
        font-family: inherit;
        font-weight: 500;
        touch-action: manipulation;
    }

    @media (hover: hover) {
        .save-btn:hover:not(:disabled) {
            background: var(--ink);
        }
    }

    .save-btn:disabled {
        opacity: 0.5;
        cursor: default;
    }

    .cancel-btn {
        padding: 0.6rem 1rem;
        background: none;
        color: var(--mute);
        border: 1px solid var(--rule);
        border-radius: var(--r-sm);
        cursor: pointer;
        font-size: var(--t-body-sm);
        font-family: inherit;
        touch-action: manipulation;
    }

    @media (hover: hover) {
        .cancel-btn:hover:not(:disabled) {
            border-color: var(--mute);
        }
    }
</style>
