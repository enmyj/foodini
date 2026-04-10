<script>
    import { createQuery, createMutation, useQueryClient } from "@tanstack/svelte-query";
    import { getFavorites, deleteFavorite, confirmChat } from "./api.js";
    import { showError } from "./toast.js";

    const MEALS = ["breakfast", "lunch", "snack", "dinner", "supplements"];

    let { onLoad = null } = $props();

    const queryClient = useQueryClient();

    const favoritesQuery = createQuery(() => ({
        queryKey: ["favorites"],
        queryFn: getFavorites,
    }));

    const deleteMutation = createMutation(() => ({
        mutationFn: (id) => deleteFavorite(id),
        onSuccess: (_data, id) => {
            queryClient.setQueryData(["favorites"], (old) => ({
                ...old,
                favorites: (old?.favorites ?? []).filter((f) => f.id !== id),
            }));
        },
        onError: (err) => showError(err, "Failed to delete favorite."),
    }));

    let favorites = $derived(favoritesQuery.data?.favorites ?? []);
    let loading = $derived(favoritesQuery.isPending);

    // Notify parent when favorites load
    $effect(() => {
        if (favoritesQuery.isSuccess) {
            onLoad?.(favorites);
        }
    });

    let search = $state("");

    // Modal state for adding a favorite to a day
    let addModal = $state(null); // { fav }
    let addDate = $state(todayStr());
    let addMeal = $state("breakfast");
    let adding = $state(false);

    let filtered = $derived(
        search.trim()
            ? favorites.filter((f) =>
                  f.description
                      .toLowerCase()
                      .includes(search.trim().toLowerCase()),
              )
            : favorites,
    );

    function todayStr() {
        const d = new Date();
        return [
            d.getFullYear(),
            String(d.getMonth() + 1).padStart(2, "0"),
            String(d.getDate()).padStart(2, "0"),
        ].join("-");
    }

    function handleDelete(fav) {
        deleteMutation.mutate(fav.id);
    }

    function openAddModal(fav) {
        addModal = { fav };
        addDate = todayStr();
        addMeal = fav.meal_type || "breakfast";
    }

    async function confirmAdd() {
        if (adding || !addModal) return;
        adding = true;
        try {
            const entry = {
                ...addModal.fav,
                meal_type: addMeal,
            };
            await confirmChat([entry], addDate);
            queryClient.invalidateQueries({ queryKey: ["log"] });
            addModal = null;
        } catch (e) {
            console.error("confirmChat (from favorites) failed:", e);
            showError(e, "Failed to add to log.");
        } finally {
            adding = false;
        }
    }

    function onKeyDown(e) {
        if (e.key === "Escape") addModal = null;
        if (e.key === "Enter" && (e.metaKey || e.ctrlKey)) confirmAdd();
    }
</script>

<div class="favs-wrap">
    <div class="search-row">
        <input
            class="search"
            type="search"
            placeholder="Search favorites…"
            bind:value={search}
        />
    </div>

    {#if loading}
        <p class="state">Loading…</p>
    {:else if favorites.length === 0}
        <p class="state">No favorites yet. Star an entry from the Day view.</p>
    {:else if filtered.length === 0}
        <p class="state">No matches.</p>
    {:else}
        {#each filtered as fav (fav.id)}
            <div class="fav-row">
                <!-- svelte-ignore a11y_click_events_have_key_events -->
                <div
                    class="fav-main"
                    role="button"
                    tabindex="0"
                    onclick={() => openAddModal(fav)}
                >
                    <span class="fav-desc">{fav.description}</span>
                    <span class="fav-macros"
                        >{fav.calories} cal · {fav.protein}g P · {fav.carbs}g C
                        · {fav.fat}g F{fav.fiber
                            ? ` · ${fav.fiber}g Fb`
                            : ""}</span
                    >
                    <span class="fav-meal">{fav.meal_type}</span>
                </div>
                <button
                    class="fav-del"
                    onclick={() => handleDelete(fav)}
                    aria-label="Remove from favorites">×</button
                >
            </div>
        {/each}
    {/if}
</div>

{#if addModal}
    <!-- svelte-ignore a11y_click_events_have_key_events -->
    <div
        class="overlay"
        aria-hidden="true"
        onclick={() => (addModal = null)}
    ></div>
    <div
        class="modal"
        role="dialog"
        aria-label="Add to log"
        tabindex="-1"
        onkeydown={onKeyDown}
    >
        <h3>Add to log</h3>
        <p class="modal-desc">{addModal.fav.description}</p>

        <label class="field">
            <span class="label">Date</span>
            <input type="date" bind:value={addDate} max={todayStr()} />
        </label>

        <label class="field">
            <span class="label">Meal</span>
            <select bind:value={addMeal}>
                {#each MEALS as m}
                    <option value={m}>{m}</option>
                {/each}
            </select>
        </label>

        <div class="actions">
            <button class="save-btn" onclick={confirmAdd} disabled={adding}>
                {adding ? "Adding…" : "Add to log"}
            </button>
            <button
                class="cancel-btn"
                onclick={() => (addModal = null)}
                disabled={adding}>Cancel</button
            >
        </div>
    </div>
{/if}

<style>
    .favs-wrap {
        padding-bottom: 2rem;
    }

    .search-row {
        padding: 0.75rem 0 0.5rem;
        position: sticky;
        top: 0;
        background: var(--paper);
        z-index: 5;
    }

    .search {
        width: 100%;
        box-sizing: border-box;
        border: 1px solid var(--rule);
        border-radius: var(--r-sm);
        padding: 0.5rem 0.75rem;
        font-family: inherit;
        font-size: var(--t-body);
        background: var(--paper);
        color: var(--ink);
        outline: none;
    }

    .search:focus {
        border-color: var(--ink-2);
    }

    .state {
        color: var(--mute);
        font-size: var(--t-body-sm);
        padding: 1.5rem 0;
        text-align: center;
    }

    .fav-row {
        display: flex;
        align-items: center;
        padding: 0.75rem 0;
        border-bottom: 1px solid var(--rule);
        gap: 0.5rem;
    }

    .fav-main {
        flex: 1;
        min-width: 0;
        cursor: pointer;
        display: flex;
        flex-direction: column;
        gap: 0.2rem;
    }

    .fav-main:hover .fav-desc {
        color: var(--ink-mute);
    }

    .fav-desc {
        font-size: 0.95rem;
        line-height: 1.4;
        color: var(--ink);
    }

    .fav-macros {
        font-size: 0.78rem;
        color: var(--mute);
        font-variant-numeric: tabular-nums;
    }

    .fav-meal {
        font-size: 0.72rem;
        color: var(--mute-4);
        text-transform: capitalize;
    }

    .fav-del {
        background: none;
        border: none;
        color: var(--mute-4);
        font-size: 1.1rem;
        line-height: 1;
        cursor: pointer;
        padding: 0;
        flex-shrink: 0;
        min-width: 2.75rem;
        min-height: 2.75rem;
        display: flex;
        align-items: center;
        justify-content: center;
        touch-action: manipulation;
    }

    @media (hover: hover) {
        .fav-del:hover {
            color: var(--danger);
        }
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
        width: min(92vw, 380px);
        z-index: 31;
        padding: 1.5rem;
        box-shadow: 0 4px 24px rgba(0, 0, 0, 0.12);
    }

    .modal h3 {
        font-size: 0.95rem;
        font-weight: 600;
        color: var(--ink);
        margin-bottom: 0.5rem;
        text-transform: none;
        letter-spacing: 0;
    }

    .modal-desc {
        font-size: 0.85rem;
        color: var(--mute);
        margin-bottom: 1.25rem;
        line-height: 1.4;
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

    input[type="date"],
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

    input[type="date"]:focus,
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
