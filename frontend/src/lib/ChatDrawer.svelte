<script lang="ts">
    import { createMutation } from "@tanstack/svelte-query";
    import { untrack } from "svelte";
    import { agent, confirmChat, patchEntry, deleteEntry, getActivity, putActivity, getFavorites } from "./api.ts";
    import { autosize } from "./autosize.ts";
    import ChatDrawerActivityForm from "./ChatDrawerActivityForm.svelte";
    import CoachChat from "./CoachChat.svelte";
    import { todayStr } from "./date.ts";
    import { showError } from "./toast.ts";
    import { MEAL_ORDER } from "./types.ts";
    import type {
        ActivityField,
        DrawerTab,
        Entry,
        EntryInput,
        Favorite,
        MealEntriesMap,
        MealType,
        PendingImage,
    } from "./types.ts";

    type EditableEntryField = "calories" | "protein" | "carbs" | "fat" | "fiber";

    let {
        open,
        onClose,
        onEntriesAdded,
        onEntriesEdited = null,
        date = null,
        meal = null,
        initialTab = "food",
        initialField = null,
        editEntries = null,
        editMealType = null,
        mealEntriesByMeal = {},
        yesterdayByMeal = {},
        mealIsEmpty = true,
    }: {
        open: boolean;
        onClose: () => void;
        onEntriesAdded: (entries: Entry[]) => void;
        onEntriesEdited?: ((entries: Entry[], mealType: MealType | null) => void) | null;
        date?: string | null;
        meal?: MealType | null;
        initialTab?: DrawerTab;
        initialField?: ActivityField | null;
        editEntries?: Entry[] | null;
        editMealType?: MealType | null;
        mealEntriesByMeal?: MealEntriesMap;
        yesterdayByMeal?: MealEntriesMap;
        mealIsEmpty?: boolean;
    } = $props();

    const MEALS = [...MEAL_ORDER];

    function guessMealForNow(): MealType {
        const h = new Date().getHours();
        if (h < 10) return "breakfast";
        if (h < 14) return "lunch";
        if (h < 16) return "snack";
        if (h < 21) return "dinner";
        return "snack";
    }

    // Shared
    let tab = $state<DrawerTab>("food");
    let selectedDate = $state("");
    let drawerEl = $state<HTMLDivElement | null>(null);

    // Food
    let input = $state("");
    let sending = $state(false);
    let clarifyingQuestion = $state<string | null>(null);
    let pendingImages = $state<PendingImage[]>([]);
    let selectedMeal = $state<MealType | null>(null);
    let inputEl = $state<HTMLTextAreaElement | null>(null);
    let fileInputEl = $state<HTMLInputElement | null>(null);
    let mealError = $state(false);

    // Entries — always initialised (empty [] for new meals, pre-filled for edits)
    let entries = $state<Entry[]>([]);
    // Non-meal agent actions surfaced inside the drawer so the user sees them too.
    type AgentNote =
        | { kind: "activity"; text: string }
        | { kind: "stool" }
        | { kind: "favorite"; description: string }
        | { kind: "message"; text: string };
    let agentNotes = $state<AgentNote[]>([]);
    let favorites = $state<Favorite[] | null>(null);
    let openAction = $state<"repeat" | "favs" | null>(null);
    let mealsExpanded = $state(false);
    let favSearch = $state("");
    let deletingEntryIds = $state<Set<string>>(new Set());

    let hasEntries = $derived(entries.length > 0);
    let hasContent = $derived(hasEntries || agentNotes.length > 0);
    let yesterdayMeals = $derived(
        (Object.keys(yesterdayByMeal) as MealType[]).filter((m) => (yesterdayByMeal[m]?.length ?? 0) > 0),
    );
    let filteredFavs = $derived.by(() => {
        if (!favorites || !favSearch.trim()) return favorites ?? [];
        const q = favSearch.toLowerCase();
        return favorites.filter((f) => f.description.toLowerCase().includes(q));
    });

    let activityText = $state("");
    let feelingNotes = $state("");
    let poop = $state(false);
    let poopNotes = $state("");
    let hydration = $state("");
    let activitySaving = $state(false);
    let activityLoadedFor = $state<string | null>(null);

    // Drag-to-dismiss
    let dragStartY = $state<number | null>(null);
    let dragCurrentY = 0;

    const saveActivityMutation = createMutation(() => ({
        mutationFn: ({
            date,
            payload,
        }: {
            date: string;
            payload: Parameters<typeof putActivity>[1];
        }) => putActivity(date, payload),
        onError: (err) => showError(err, "Failed to save activity."),
    }));

    const agentMutation = createMutation(() => ({
        mutationFn: ({
            message,
            date,
            images,
            meal,
            currentEntries,
        }: {
            message: string;
            date: string;
            images: File[] | null;
            meal: MealType;
            currentEntries: Entry[] | null;
        }) => agent(message, { date, meal, images, currentEntries }),
        onError: (err) =>
            showError(err, "Something went wrong. Please try again."),
    }));

    const confirmChatMutation = createMutation(() => ({
        mutationFn: ({
            entries,
            date,
        }: {
            entries: EntryInput[];
            date: string;
        }) => confirmChat(entries, date),
    }));

    const patchEntryMutation = createMutation(() => ({
        mutationFn: ({ id, entry }: { id: string; entry: Partial<Entry> }) =>
            patchEntry(id, entry),
    }));

    const deleteEntryMutation = createMutation(() => ({
        mutationFn: (id: string) => deleteEntry(id),
    }));

    function onDragStart(e: TouchEvent) {
        const target = e.target;
        const tag = target instanceof HTMLElement ? target.tagName : "";
        const touch = e.touches[0];
        if (
            tag === "TEXTAREA" ||
            tag === "INPUT" ||
            tag === "BUTTON" ||
            tag === "SELECT"
        )
            return;
        if (!touch) return;
        // Don't hijack touches that start inside a scrollable region —
        // the user is trying to scroll content, not dismiss the drawer.
        if (target instanceof HTMLElement && isInsideScrollable(target)) return;
        dragStartY = touch.clientY;
        dragCurrentY = 0;
        if (drawerEl) drawerEl.style.transition = "none";
    }

    function isInsideScrollable(el: HTMLElement): boolean {
        let node: HTMLElement | null = el;
        while (node && node !== drawerEl) {
            const style = window.getComputedStyle(node);
            const overflowY = style.overflowY;
            if (
                (overflowY === "auto" || overflowY === "scroll") &&
                node.scrollHeight > node.clientHeight
            ) {
                return true;
            }
            node = node.parentElement;
        }
        return false;
    }

    function onDragMove(e: TouchEvent) {
        if (dragStartY === null) return;
        const touch = e.touches[0];
        if (!touch) return;
        const dy = touch.clientY - dragStartY;
        if (dy < 0) return;
        dragCurrentY = dy;
        if (drawerEl) drawerEl.style.transform = `translateY(${dy}px)`;
    }

    function onDragEnd() {
        if (dragStartY === null) return;
        dragStartY = null;
        if (drawerEl) {
            drawerEl.style.transition = "";
            if (dragCurrentY > 120) {
                drawerEl.style.transform = "";
                onClose();
            } else {
                drawerEl.style.transform = "";
            }
        }
        dragCurrentY = 0;
    }

    function revokePreview(url: string): void {
        if (typeof url === "string" && url.startsWith("blob:")) {
            URL.revokeObjectURL(url);
        }
    }

    function clearPendingImages() {
        for (const img of pendingImages) {
            revokePreview(img.previewUrl);
        }
        pendingImages = [];
    }

    function entriesForMeal(mealType: MealType | null): Entry[] {
        return mealType ? [...(mealEntriesByMeal[mealType] ?? [])] : [];
    }

    function chooseMeal(mealType: MealType): void {
        const nextMeal = selectedMeal === mealType ? null : mealType;
        const useEditSeed = editEntries && nextMeal === editMealType;
        selectedMeal = nextMeal;
        entries = useEditSeed ? [...editEntries!] : entriesForMeal(nextMeal);
        clarifyingQuestion = null;
        openAction = null;
        deletingEntryIds = new Set();
        if (nextMeal) mealsExpanded = false;
    }

    // Only re-run when `open` changes — read props via untrack so edits
    // during the session (e.g. onEntriesEdited resetting parent state) don't
    // re-initialize the drawer.
    $effect(() => {
        const isOpen = open;
        untrack(() => {
            if (isOpen) {
                tab = initialTab;
                selectedDate = date || todayStr();
                selectedMeal = editMealType ?? meal ?? guessMealForNow();
                clearPendingImages();
                input = "";
                sending = false;
                clarifyingQuestion = null;
                entries = editEntries ? [...editEntries] : entriesForMeal(selectedMeal);
                mealsExpanded = false;
                openAction = null;
                favSearch = "";
                deletingEntryIds = new Set();
                if (!favorites) {
                    getFavorites().then((res) => { favorites = res.favorites ?? []; }).catch(() => {});
                }
            } else {
                tab = "food";
                selectedDate = "";
                selectedMeal = null;
                clearPendingImages();
                input = "";
                clarifyingQuestion = null;
                entries = [];
                openAction = null;
                favSearch = "";
                deletingEntryIds = new Set();
                activityText = "";
                feelingNotes = "";
                poop = false;
                poopNotes = "";
                hydration = "";
                activitySaving = false;
                activityLoadedFor = null;
            }
        });
    });

    $effect(() => {
        if (
            open &&
            tab === "activity" &&
            selectedDate &&
            selectedDate !== activityLoadedFor
        ) {
            loadActivity(selectedDate);
        }
    });

    async function loadActivity(d: string): Promise<void> {
        activityLoadedFor = d;
        try {
            const res = await getActivity(d);
            activityText = res.activity ?? "";
            feelingNotes = res.feeling_notes ?? "";
            poop = res.poop ?? false;
            poopNotes = res.poop_notes ?? "";
            hydration = res.hydration ? String(res.hydration) : "";
        } catch (err) {
            showError(err, "Failed to load activity.");
        }
    }

    async function saveActivity() {
        activitySaving = true;
        try {
            await saveActivityMutation.mutateAsync({
                date: selectedDate,
                payload: {
                    activity: activityText,
                    feeling_score: 0,
                    feeling_notes: feelingNotes,
                    poop,
                    poop_notes: poopNotes,
                    hydration: hydration ? parseFloat(hydration) : 0,
                },
            });
            onClose();
        } catch {
        } finally {
            activitySaving = false;
        }
    }

    async function onFileSelected(e: Event): Promise<void> {
        const target = e.currentTarget as HTMLInputElement;
        const files = Array.from(target.files ?? []);
        if (!files.length) return;
        pendingImages = [
            ...pendingImages,
            ...files.map((file) => ({
                file,
                previewUrl: URL.createObjectURL(file),
            })),
        ];
        setTimeout(() => inputEl?.focus(), 30);
        if (fileInputEl) fileInputEl.value = "";
    }

    function removeImage(index: number): void {
        const image = pendingImages[index];
        if (image) revokePreview(image.previewUrl);
        pendingImages = pendingImages.filter((_, i) => i !== index);
    }

    function openFilePicker(): void {
        fileInputEl?.click();
    }

    async function send(): Promise<void> {
        if (sending) return;
        if (!selectedMeal) {
            mealError = true;
            setTimeout(() => {
                mealError = false;
            }, 600);
            return;
        }
        const imgs = pendingImages.length
            ? pendingImages.map((img) => img.file)
            : null;
        const text = input.trim();
        if (!imgs && !text) return;

        input = "";
        clarifyingQuestion = null;
        clearPendingImages();
        sending = true;

        try {
            const res = await agentMutation.mutateAsync({
                message: text,
                date: selectedDate,
                images: imgs,
                meal: selectedMeal,
                currentEntries: entries.length ? [...entries] : null,
            });
            sending = false;
            let added: Entry[] = [];
            let edited: { entries: Entry[] } | null = null;
            for (const action of res.actions ?? []) {
                if (action.type === "meal_added" && action.entries?.length) {
                    added = added.concat(action.entries);
                } else if (action.type === "meal_edited" && action.entries) {
                    edited = { entries: action.entries };
                }
            }
            if (edited) {
                entries = edited.entries;
                if (onEntriesEdited) onEntriesEdited(edited.entries, selectedMeal);
            }
            if (added.length) {
                entries = [...entries, ...added];
                onEntriesAdded(added);
            }
            if (!added.length && !edited && res.message) {
                clarifyingQuestion = res.message;
            }
        } catch {
            sending = false;
        }
    }

    // --- Entry editing functions ---

    async function editInlineEntry(
        index: number,
        field: EditableEntryField,
        value: number,
    ): Promise<void> {
        const entry = entries[index];
        if (!entry) return;
        const updated: Entry = { ...entry, [field]: value };
        entries = entries.map((e, i) => i === index ? updated : e);
        try {
            const saved = await patchEntryMutation.mutateAsync({
                id: updated.id,
                entry: updated,
            });
            entries = entries.map((e) => e.id === saved.id ? saved : e);
            if (onEntriesEdited) onEntriesEdited(entries, selectedMeal);
        } catch (err) {
            showError(err, "Failed to save change.");
        }
    }

    async function deleteEntry_(index: number): Promise<void> {
        const entry = entries[index];
        if (!entry || deletingEntryIds.has(entry.id)) return;
        deletingEntryIds = new Set([...deletingEntryIds, entry.id]);
        try {
            await deleteEntryMutation.mutateAsync(entry.id);
            const nextEntries = entries.filter((e) => e.id !== entry.id);
            entries = nextEntries;
            if (onEntriesEdited) onEntriesEdited(nextEntries, selectedMeal);
        } catch (err) {
            showError(err, "Failed to delete entry.");
        } finally {
            deletingEntryIds = new Set(
                [...deletingEntryIds].filter((id) => id !== entry.id),
            );
        }
    }

    async function addFavoriteToMeal(fav: Favorite): Promise<void> {
        if (!selectedMeal) return;
        const input_: EntryInput[] = [{
            meal_type: selectedMeal,
            description: fav.description,
            calories: fav.calories,
            protein: fav.protein,
            carbs: fav.carbs,
            fat: fav.fat,
            fiber: fav.fiber ?? 0,
        }];
        let res: Awaited<ReturnType<typeof confirmChat>>;
        try {
            res = await confirmChatMutation.mutateAsync({
                entries: input_,
                date: selectedDate,
            });
        } catch (err) {
            showError(err, "Failed to add favorite.");
            return;
        }
        if (res.entries?.length) {
            entries = [...entries, ...res.entries];
            openAction = null;
            try {
                onEntriesAdded(res.entries);
            } catch (err) {
                console.error("onEntriesAdded failed:", err);
            }
        }
    }

    async function repeatYesterday(fromEntries: Entry[]): Promise<void> {
        if (!fromEntries.length || !selectedMeal) return;
        sending = true;
        const input_: EntryInput[] = fromEntries.map((e) => ({
            meal_type: selectedMeal!,
            description: e.description,
            calories: e.calories,
            protein: e.protein,
            carbs: e.carbs,
            fat: e.fat,
            fiber: e.fiber ?? 0,
        }));
        let res: Awaited<ReturnType<typeof confirmChat>>;
        try {
            res = await confirmChatMutation.mutateAsync({
                entries: input_,
                date: selectedDate,
            });
        } catch (err) {
            sending = false;
            showError(err, "Failed to repeat yesterday's meal.");
            return;
        }
        sending = false;
        if (res.entries?.length) {
            entries = [...entries, ...res.entries];
            try {
                onEntriesAdded(res.entries);
            } catch (err) {
                console.error("onEntriesAdded failed:", err);
            }
        }
    }

    function numberValueFromEvent(e: FocusEvent): number {
        const target = e.currentTarget as HTMLInputElement;
        return Number(target.value);
    }

    function onDone() {
        if (tab === "activity") {
            saveActivity();
        } else {
            onClose();
        }
    }

    let doneLabel = $derived(tab === "activity" && activitySaving ? "Saving…" : "Done");

    function onKeyDown(e: KeyboardEvent) {
        if (e.key === "Enter" && !e.shiftKey) {
            e.preventDefault();
            send();
        }
    }

</script>

{#if open}
    <!-- svelte-ignore a11y_click_events_have_key_events -->
    <div class="overlay" aria-hidden="true" onclick={onClose}></div>
    <div
        class="drawer"
        role="dialog"
        aria-label="Log"
        tabindex="-1"
        bind:this={drawerEl}
        ontouchstart={onDragStart}
        ontouchmove={onDragMove}
        ontouchend={onDragEnd}
    >
        <button class="handle" onclick={onClose} aria-label="Close drawer">
            <span class="handle-bar"></span>
        </button>

        <!-- Tab switcher + date + done -->
        <div class="drawer-top">
            <div class="tabs">
                <button
                    class="tab-btn"
                    class:active={tab === "food"}
                    onclick={() => (tab = "food")}>Food</button
                >
                <button
                    class="tab-btn"
                    class:active={tab === "activity"}
                    onclick={() => (tab = "activity")}>Activity</button
                >
                <button
                    class="tab-btn"
                    class:active={tab === "coach"}
                    onclick={() => (tab = "coach")}>Coach</button
                >
            </div>
            <div class="top-right">
                <input
                    class="date-input"
                    type="date"
                    bind:value={selectedDate}
                    max={todayStr()}
                />
                <button
                    class="done-btn"
                    onclick={onDone}
                    disabled={tab === "activity" && activitySaving}
                    >{doneLabel}</button
                >
            </div>
        </div>

        {#if tab === "food"}
            <!-- Meal header — always visible so users can switch meals -->
            <div class="meal-pills-wrap" class:shake={mealError} class:collapsed={!mealsExpanded && !!selectedMeal}>
                <button
                    type="button"
                    class="meal-toggle"
                    onclick={() => (mealsExpanded = !mealsExpanded)}
                    aria-expanded={mealsExpanded}
                >{selectedMeal ?? "meal"} <span class="meal-toggle-arrow" aria-hidden="true">{mealsExpanded ? "▾" : "▸"}</span></button>
                <div class="meal-pills">
                    {#each MEALS as m}
                        <button
                            class="meal-pill"
                            class:selected={selectedMeal === m}
                            onclick={() => chooseMeal(m)}
                            >{m}</button
                        >
                    {/each}
                </div>
            </div>

            {#if selectedMeal && (yesterdayMeals.length > 0 || (favorites && favorites.length > 0))}
                <div class="action-pills">
                    {#if yesterdayMeals.length > 0}
                        <button
                            class="action-pill"
                            class:active={openAction === "repeat"}
                            onclick={() => (openAction = openAction === "repeat" ? null : "repeat")}
                            disabled={sending}
                            >Repeat</button
                        >
                    {/if}
                    {#if favorites && favorites.length > 0}
                        <button
                            class="action-pill"
                            class:active={openAction === "favs"}
                            onclick={() => (openAction = openAction === "favs" ? null : "favs")}
                            >Favorites</button
                        >
                    {/if}
                </div>
                {#if openAction === "repeat"}
                    <div class="action-panel action-panel--equal">
                        {#each yesterdayMeals as m}
                            <button
                                class="scale-pill"
                                onclick={() => { openAction = null; repeatYesterday(yesterdayByMeal[m] ?? []); }}
                                disabled={sending}
                                >{m}</button
                            >
                        {/each}
                    </div>
                {:else if openAction === "favs"}
                    <div class="action-panel fav-panel">
                        <input
                            class="fav-search"
                            type="text"
                            placeholder="Search favorites…"
                            bind:value={favSearch}
                        />
                        <div class="fav-list">
                            {#each filteredFavs.slice(0, 8) as fav}
                                <button class="fav-item" onclick={() => addFavoriteToMeal(fav)} disabled={sending}>
                                    <span class="fav-desc">{fav.description}</span>
                                    <span class="fav-cal">{fav.calories} cal</span>
                                </button>
                            {/each}
                            {#if filteredFavs.length === 0}
                                <span class="fav-empty">No favorites found</span>
                            {/if}
                        </div>
                    </div>
                {/if}
            {/if}

            <!-- Content area -->
            <div class="content-area">
                <!-- Existing entries with inline editing -->
                {#if hasEntries}
                    <div class="result-card" class:dimmed={sending}>
                        {#each entries as entry, i}
                            <div class="card-entry" class:dimmed={deletingEntryIds.has(entry.id)}>
                                <div class="card-entry-head">
                                    <div class="card-desc">{entry.description}</div>
                                    <button
                                        class="entry-delete"
                                        class:deleting={deletingEntryIds.has(entry.id)}
                                        onclick={() => deleteEntry_(i)}
                                        disabled={deletingEntryIds.has(entry.id)}
                                        aria-label={deletingEntryIds.has(entry.id) ? "Deleting…" : "Delete entry"}
                                        >{#if deletingEntryIds.has(entry.id)}<span class="entry-spinner" aria-hidden="true"></span>{:else}✕{/if}</button
                                    >
                                </div>
                                <div class="card-macros">
                                    <span class="macro-field">
                                        <input type="number" value={entry.calories}
                                            onblur={(e: FocusEvent) => editInlineEntry(i, "calories", numberValueFromEvent(e))}
                                            disabled={sending || deletingEntryIds.has(entry.id)} />
                                        <span class="macro-label">cal</span>
                                    </span>
                                    <span class="macro-sep">·</span>
                                    <span class="macro-field">
                                        <input type="number" value={entry.protein}
                                            onblur={(e: FocusEvent) => editInlineEntry(i, "protein", numberValueFromEvent(e))}
                                            disabled={sending || deletingEntryIds.has(entry.id)} />
                                        <span class="macro-label">P</span>
                                    </span>
                                    <span class="macro-sep">·</span>
                                    <span class="macro-field">
                                        <input type="number" value={entry.carbs}
                                            onblur={(e: FocusEvent) => editInlineEntry(i, "carbs", numberValueFromEvent(e))}
                                            disabled={sending || deletingEntryIds.has(entry.id)} />
                                        <span class="macro-label">C</span>
                                    </span>
                                    <span class="macro-sep">·</span>
                                    <span class="macro-field">
                                        <input type="number" value={entry.fat}
                                            onblur={(e: FocusEvent) => editInlineEntry(i, "fat", numberValueFromEvent(e))}
                                            disabled={sending || deletingEntryIds.has(entry.id)} />
                                        <span class="macro-label">F</span>
                                    </span>
                                    <span class="macro-sep">·</span>
                                    <span class="macro-field">
                                        <input type="number" value={entry.fiber ?? 0}
                                            onblur={(e: FocusEvent) => editInlineEntry(i, "fiber", numberValueFromEvent(e))}
                                            disabled={sending || deletingEntryIds.has(entry.id)} />
                                        <span class="macro-label">Fb</span>
                                    </span>
                                </div>
                            </div>
                        {/each}
                    </div>
                {/if}

                <!-- Skeleton (only when loading with no entries yet) -->
                {#if sending && !hasEntries}
                    <div class="skeleton-card">
                        <div class="skeleton-entry">
                            <div class="sk-line" style="width: 62%"></div>
                            <div class="sk-line" style="width: 80%; margin-top: 0.4rem; opacity: 0.6"></div>
                        </div>
                        <div class="skeleton-entry">
                            <div class="sk-line" style="width: 45%"></div>
                            <div class="sk-line" style="width: 80%; margin-top: 0.4rem; opacity: 0.6"></div>
                        </div>
                    </div>
                {/if}

                {#if clarifyingQuestion}
                    <p class="clarifying">{clarifyingQuestion}</p>
                {/if}
            </div>

            <input
                bind:this={fileInputEl}
                type="file"
                accept="image/*"
                multiple
                class="file-input"
                onchange={onFileSelected}
            />

            <!-- Bottom controls -->
            {#if selectedMeal && (!sending || hasEntries)}
                {#if pendingImages.length}
                    <div class="thumb-strip">
                        {#each pendingImages as img, i}
                            <div class="thumb">
                                <img src={img.previewUrl} alt="Photo {i + 1}" />
                                <button
                                    class="thumb-remove"
                                    onclick={() => removeImage(i)}
                                    aria-label="Remove photo">✕</button
                                >
                            </div>
                        {/each}
                        <button
                            class="thumb-add"
                            onclick={openFilePicker}
                            aria-label="Add another photo"
                        >
                            <svg
                                width="18"
                                height="18"
                                viewBox="0 0 24 24"
                                fill="none"
                                stroke="currentColor"
                                stroke-width="2"
                                stroke-linecap="round"
                                stroke-linejoin="round"
                                ><line x1="12" y1="5" x2="12" y2="19" /><line
                                    x1="5"
                                    y1="12"
                                    x2="19"
                                    y2="12"
                                /></svg
                            >
                        </button>
                    </div>
                {/if}
                <div class="input-row">
                    <button
                        class="attach-btn"
                        onclick={openFilePicker}
                        disabled={sending}
                        aria-label="Attach photo"
                    >
                        <svg
                            width="20"
                            height="20"
                            viewBox="0 0 24 24"
                            fill="none"
                            stroke="currentColor"
                            stroke-width="1.75"
                            stroke-linecap="round"
                            stroke-linejoin="round"
                            ><path
                                d="M23 19a2 2 0 0 1-2 2H3a2 2 0 0 1-2-2V8a2 2 0 0 1 2-2h4l2-3h6l2 3h4a2 2 0 0 1 2 2z"
                            /><circle cx="12" cy="13" r="4" /></svg
                        >
                    </button>
                    <textarea
                        class="text-entry composer-input"
                        bind:this={inputEl}
                        use:autosize
                        bind:value={input}
                        onkeydown={onKeyDown}
                        placeholder={hasEntries ? "Add more, or “fix” to correct…" : "What did you eat?"}
                        rows="1"
                        disabled={sending}
                    ></textarea>
                </div>
                <div class="action-row">
                    <button
                        class="add-btn"
                        onclick={send}
                        disabled={sending ||
                            (!input.trim() && !pendingImages.length)}
                        >Send</button
                    >
                </div>
            {/if}
        {:else if tab === "activity"}
            <ChatDrawerActivityForm
                active={open && tab === "activity"}
                {initialField}
                bind:activityText
                bind:feelingNotes
                bind:poop
                bind:poopNotes
                bind:hydration
            />
        {:else}
            <CoachChat active={open && tab === "coach"} date={selectedDate} />
        {/if}
    </div>
{/if}

<style>
    .overlay {
        position: fixed;
        inset: 0;
        background: rgba(0, 0, 0, 0.2);
        z-index: 10;
    }

    .drawer {
        position: fixed;
        bottom: 0;
        left: 0;
        right: 0;
        max-width: 640px;
        margin: 0 auto;
        background: var(--paper);
        border-radius: var(--r-lg) var(--r-lg) 0 0;
        box-shadow: 0 -2px 16px rgba(0, 0, 0, 0.08);
        z-index: 11;
        display: flex;
        flex-direction: column;
        width: min(100%, 640px);
        height: min(78vh, 620px);
        height: min(78dvh, 620px);
        max-height: calc(100dvh - 0.5rem);
        padding: 0.75rem 1.25rem calc(1.5rem + env(safe-area-inset-bottom, 0px));
        transition: transform 0.2s ease;
        will-change: transform;
    }

    .handle {
        background: none;
        border: none;
        display: flex;
        align-items: center;
        justify-content: center;
        width: 100%;
        padding: 0.25rem 0 0.5rem;
        margin: -0.25rem 0 0.25rem;
        cursor: pointer;
        min-height: 0;
        touch-action: manipulation;
    }

    .handle-bar {
        display: block;
        width: 36px;
        height: 5px;
        background: var(--rule);
        border-radius: 3px;
        transition: background 0.15s, width 0.15s;
    }

    @media (hover: hover) {
        .handle:hover .handle-bar {
            background: var(--mute-2);
            width: 48px;
        }
    }

    /* --- Top header row (tabs + date + done) --- */
    .drawer-top {
        display: flex;
        justify-content: space-between;
        align-items: center;
        gap: 0.5rem;
        margin-bottom: 0.75rem;
    }

    .tabs {
        display: flex;
        gap: 0.25rem;
        flex-shrink: 1;
        min-width: 0;
    }

    .tab-btn {
        background: none;
        border: none;
        border-radius: var(--r-pill);
        padding: 0.4rem 0.9rem;
        font-size: var(--t-body-sm);
        font-weight: 500;
        color: var(--mute);
        cursor: pointer;
        font-family: inherit;
        min-height: 2.75rem;
    }

    @media (max-width: 480px) {
        .drawer {
            padding-left: 1rem;
            padding-right: 1rem;
        }
        .tabs {
            gap: 0.1rem;
        }
        .tab-btn {
            padding: 0.4rem 0.55rem;
            font-size: var(--t-meta);
        }
        .top-right {
            gap: 0.35rem;
        }
        .done-btn {
            padding: 0.35rem 0.65rem;
            font-size: var(--t-meta);
        }
        .date-input {
            padding: 0.3rem 0.4rem;
            font-size: 0.7rem;
        }
    }

    .tab-btn.active {
        background: var(--paper-4);
        color: var(--ink);
    }

    .top-right {
        display: flex;
        align-items: center;
        gap: 0.5rem;
    }

    .done-btn {
        background: var(--ink-2);
        color: var(--paper);
        border: none;
        border-radius: var(--r-sm);
        padding: 0.4rem 0.85rem;
        font-size: var(--t-body-sm);
        font-family: inherit;
        font-weight: 500;
        cursor: pointer;
        min-height: 2.25rem;
        white-space: nowrap;
    }

    @media (hover: hover) {
        .done-btn:not(:disabled):hover {
            background: var(--ink);
        }
    }

    .done-btn:disabled {
        opacity: 0.35;
        cursor: default;
    }

    /* --- Date input --- */
    .date-input {
        border: 1px solid var(--rule-4);
        border-radius: var(--r-sm);
        padding: 0.3rem 0.6rem;
        font-size: var(--t-meta);
        font-family: inherit;
        color: var(--ink-mute);
        font-weight: 500;
        background: var(--paper);
    }

    .date-input:focus {
        outline: none;
        border-color: var(--ink-2);
    }

    @keyframes shake {
        0%,
        100% {
            transform: translateX(0);
        }
        20% {
            transform: translateX(-6px);
        }
        40% {
            transform: translateX(6px);
        }
        60% {
            transform: translateX(-4px);
        }
        80% {
            transform: translateX(4px);
        }
    }

    .meal-pills-wrap.shake {
        animation: shake 0.5s ease;
    }

    /* --- Meal pills --- */
    .meal-pills-wrap {
        margin-bottom: 0.75rem;
    }

    .meal-pills {
        display: flex;
        gap: 0.4rem;
        flex-wrap: wrap;
    }

    .meal-toggle {
        display: inline-flex;
        align-items: center;
        gap: 0.3rem;
        background: none;
        border: 1px solid var(--rule-3);
        border-radius: var(--r-pill);
        color: var(--mute);
        font-family: inherit;
        font-size: 0.72rem;
        letter-spacing: 0.02em;
        padding: 0.2rem 0.6rem;
        cursor: pointer;
        text-transform: capitalize;
        font-weight: 500;
        min-height: 0;
        white-space: nowrap;
    }

    .meal-toggle-arrow {
        color: var(--mute-3);
        font-size: 0.7rem;
    }

    .meal-pills-wrap.collapsed .meal-pills {
        display: none;
    }
    .meal-pills-wrap.collapsed .meal-toggle {
        border-color: var(--ink-2);
        color: var(--ink-2);
        background: var(--paper-2);
    }
    .meal-pills-wrap:not(.collapsed) .meal-toggle {
        margin-bottom: 0.4rem;
    }

    .meal-pill {
        padding: 0.2rem 0.6rem;
        border: 1px solid var(--rule-3);
        border-radius: var(--r-pill);
        background: none;
        font-family: inherit;
        font-size: 0.72rem;
        letter-spacing: 0.02em;
        color: var(--ink-mute);
        cursor: pointer;
        white-space: nowrap;
        touch-action: manipulation;
        font-weight: 500;
        min-height: 0;
    }

    @media (hover: hover) {
        .meal-pill:hover:not(:disabled) {
            border-color: var(--ink-2);
            color: var(--ink-2);
        }
    }

    .meal-pill.selected {
        background: var(--ink-2);
        border-color: var(--ink-2);
        color: var(--paper);
    }

    /* --- Content area --- */
    .content-area {
        flex: 1;
        min-height: 0;
        overflow-y: auto;
        margin-bottom: 0.75rem;
        padding: 0.25rem 0;
    }

    /* --- Skeleton --- */
    @keyframes shimmer {
        0% {
            background-position: -200% 0;
        }
        100% {
            background-position: 200% 0;
        }
    }

    /* Hairline ledger: no surrounding box */
    .skeleton-card {
        border-top: 1px solid var(--rule);
        border-bottom: 1px solid var(--rule);
    }

    .skeleton-entry {
        padding: 0.75rem 0;
        border-bottom: 1px solid var(--rule);
    }

    .skeleton-entry:last-child {
        border-bottom: none;
    }

    .sk-line {
        height: 12px;
        border-radius: 6px;
        background: linear-gradient(
            90deg,
            var(--rule-2) 25%,
            var(--paper-2) 50%,
            var(--rule-2) 75%
        );
        background-size: 200% 100%;
        animation: shimmer 1.4s ease-in-out infinite;
    }

    /* --- Result ledger (hairlines, not boxes) --- */
    .result-card {
        border-top: 1px solid var(--rule);
        border-bottom: 1px solid var(--rule);
        transition: opacity 0.15s;
    }

    .result-card.dimmed {
        opacity: 0.5;
    }

    .card-entry {
        padding: 0.65rem 0;
        border-bottom: 1px solid var(--rule);
        display: flex;
        flex-direction: column;
        gap: 0.3rem;
    }

    .card-entry.dimmed {
        opacity: 0.45;
    }

    .card-entry:last-child {
        border-bottom: none;
    }

    .card-desc {
        font-size: var(--t-body-sm);
        font-weight: 500;
        color: var(--ink);
        line-height: 1.3;
    }

    .card-macros {
        display: flex;
        align-items: center;
        gap: 0.2rem;
        flex-wrap: wrap;
        font-variant-numeric: tabular-nums;
    }

    .macro-field {
        display: inline-flex;
        align-items: baseline;
        gap: 2px;
    }

    .macro-field input {
        width: 40px;
        border: none;
        border-bottom: 1px dotted var(--rule-3);
        background: transparent;
        text-align: right;
        font-family: var(--num-stack);
        font-size: var(--t-meta);
        color: var(--ink);
        padding: 0 1px 1px;
        appearance: textfield;
        -moz-appearance: textfield;
        font-variant-numeric: tabular-nums;
    }

    .macro-field input::-webkit-outer-spin-button,
    .macro-field input::-webkit-inner-spin-button {
        -webkit-appearance: none;
    }

    .macro-field input:focus {
        outline: none;
        border-bottom: 1px solid var(--ink-2);
    }

    .macro-field input:disabled {
        color: var(--mute-2);
        border-bottom-color: transparent;
    }

    .macro-label {
        font-size: 0.75rem;
        color: var(--mute-2);
        font-weight: 500;
    }

    .macro-sep {
        color: var(--mute-4);
        font-size: 0.75rem;
        margin: 0 0.1rem;
    }

    /* --- Action row (Fix / Add) --- */
    .action-row {
        display: flex;
        gap: 0.5rem;
        margin-top: 0.5rem;
    }

    .action-row > button {
        flex: 1;
    }

    /* --- Clarifying question --- */
    .clarifying {
        font-size: var(--t-body-sm);
        color: var(--ink-mute);
        margin: 0 0 0.75rem;
        line-height: 1.45;
        padding: 0.6rem 0.75rem;
        background: var(--paper-3);
        border-radius: var(--r-md);
    }

    .file-input {
        display: none;
    }

    button:focus-visible,
    input:focus-visible,
    textarea:focus-visible {
        outline: 2px solid var(--ink-2);
        outline-offset: 2px;
    }

    /* --- Thumbnail strip --- */
    .thumb-strip {
        display: flex;
        gap: 0.5rem;
        margin-bottom: 0.5rem;
        overflow-x: auto;
        padding: 2px 0;
    }

    .thumb {
        position: relative;
        flex-shrink: 0;
        width: 70px;
        height: 70px;
        border-radius: var(--r-sm);
        overflow: hidden;
        border: 1px solid var(--rule);
    }

    .thumb img {
        display: block;
        width: 100%;
        height: 100%;
        object-fit: cover;
    }

    .thumb-remove {
        position: absolute;
        top: 4px;
        right: 4px;
        width: 24px;
        height: 24px;
        border-radius: 50%;
        background: rgba(0, 0, 0, 0.6);
        color: var(--paper);
        border: none;
        font-size: 0.78rem;
        cursor: pointer;
        display: flex;
        align-items: center;
        justify-content: center;
        padding: 0;
        line-height: 1;
    }

    .thumb-add {
        flex-shrink: 0;
        width: 70px;
        height: 70px;
        border-radius: var(--r-sm);
        border: 1px dashed var(--rule-4);
        background: none;
        color: var(--mute-2);
        cursor: pointer;
        display: flex;
        align-items: center;
        justify-content: center;
        padding: 0;
    }

    @media (hover: hover) {
        .thumb-add:hover {
            border-color: var(--ink-2);
            color: var(--ink-2);
        }
    }

    /* --- Input row --- */
    .input-row {
        display: flex;
        gap: 0.5rem;
        align-items: flex-end;
    }

    .composer-input {
        flex: 1;
        min-height: 2.75rem;
    }

    .attach-btn {
        flex-shrink: 0;
        width: 2.75rem;
        height: 2.75rem;
        border-radius: 50%;
        background: none;
        border: 1px solid var(--rule);
        color: var(--mute);
        cursor: pointer;
        display: flex;
        align-items: center;
        justify-content: center;
        padding: 0;
    }

    @media (hover: hover) {
        .attach-btn:hover:not(:disabled) {
            border-color: var(--ink-2);
            color: var(--ink-2);
        }
    }
    .attach-btn:disabled {
        opacity: 0.35;
        cursor: default;
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

    /* --- Action pills --- */
    .action-pills {
        display: flex;
        gap: 0.4rem;
        margin-bottom: 0.5rem;
    }

    .action-pill {
        background: none;
        border: 1px solid var(--rule-3);
        border-radius: var(--r-pill);
        color: var(--mute);
        font-size: 0.72rem;
        padding: 0.2rem 0.65rem;
        cursor: pointer;
        touch-action: manipulation;
        font-family: inherit;
        font-weight: 500;
        letter-spacing: 0.02em;
        white-space: nowrap;
        min-height: 0;
        min-width: 0;
        transition:
            border-color 0.12s,
            color 0.12s,
            background 0.12s;
    }

    .action-pill.active {
        border-color: var(--ink-2);
        color: var(--ink-2);
        background: var(--paper-2);
    }

    @media (hover: hover) {
        .action-pill:hover:not(:disabled) {
            border-color: var(--ink-2);
            color: var(--ink-2);
        }
    }

    .action-panel {
        display: flex;
        gap: 0.4rem;
        flex-wrap: wrap;
        margin-bottom: 0.5rem;
    }

    .action-panel.fav-panel {
        flex-direction: column;
    }

    .action-panel--equal .scale-pill {
        flex: 1 1 0;
        text-align: center;
    }

    .scale-pill {
        background: none;
        border: 1px solid var(--rule-3);
        border-radius: var(--r-pill);
        padding: 0.2rem 0.65rem;
        font-size: 0.72rem;
        letter-spacing: 0.02em;
        color: var(--mute);
        cursor: pointer;
        font-family: inherit;
        font-weight: 500;
        touch-action: manipulation;
        min-height: 0;
        white-space: nowrap;
    }

    @media (hover: hover) {
        .scale-pill:hover:not(:disabled) {
            border-color: var(--ink-2);
            color: var(--ink-2);
        }
    }

    .scale-pill:disabled {
        opacity: 0.35;
        cursor: default;
    }

    .card-entry-head {
        display: flex;
        justify-content: space-between;
        align-items: flex-start;
        gap: 0.5rem;
    }

    .entry-delete {
        background: none;
        border: none;
        color: var(--mute-4);
        font-size: 0.75rem;
        cursor: pointer;
        padding: 0.1rem 0.2rem;
        line-height: 1;
        flex-shrink: 0;
        min-width: 0;
        min-height: 0;
        display: inline-flex;
        align-items: center;
        justify-content: center;
        width: 1.25rem;
        height: 1.25rem;
    }

    .entry-delete.deleting {
        opacity: 1;
        cursor: default;
    }

    .entry-spinner {
        width: 0.75rem;
        height: 0.75rem;
        border: 1.5px solid var(--rule-3);
        border-top-color: var(--ink-2);
        border-radius: 50%;
        animation: spin 0.7s linear infinite;
    }

    @keyframes spin {
        to { transform: rotate(360deg); }
    }

    @media (hover: hover) {
        .entry-delete:hover {
            color: var(--danger, #c00);
        }
    }

    .fav-search {
        width: 100%;
        border: 1px solid var(--rule);
        border-radius: var(--r-sm);
        padding: 0.4rem 0.6rem;
        font-size: var(--t-body-sm);
        font-family: inherit;
        background: var(--paper);
        color: var(--ink);
        margin-bottom: 0.35rem;
    }

    .fav-search:focus {
        outline: none;
        border-color: var(--ink-2);
    }

    .fav-list {
        max-height: 140px;
        overflow-y: auto;
        border: 1px solid var(--rule);
        border-radius: var(--r-sm);
    }

    .fav-item {
        display: flex;
        justify-content: space-between;
        align-items: center;
        width: 100%;
        background: none;
        border: none;
        border-bottom: 1px solid var(--rule);
        padding: 0.45rem 0.6rem;
        font-family: inherit;
        font-size: var(--t-meta);
        color: var(--ink);
        cursor: pointer;
        text-align: left;
    }

    .fav-item:last-child {
        border-bottom: none;
    }

    @media (hover: hover) {
        .fav-item:hover {
            background: var(--paper-4);
        }
    }

    .fav-desc {
        font-weight: 500;
    }

    .fav-cal {
        color: var(--mute-2);
        font-size: 0.72rem;
        flex-shrink: 0;
        margin-left: 0.5rem;
    }

    .fav-empty {
        display: block;
        padding: 0.6rem;
        font-size: var(--t-meta);
        color: var(--mute-2);
        text-align: center;
    }

</style>
