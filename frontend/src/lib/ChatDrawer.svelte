<script>
    import { untrack } from "svelte";
    import { chat, confirmChat, editChat, patchEntry, deleteEntry, getActivity, putActivity, getFavorites } from "./api.js";
    import { autosize } from "./autosize.js";
    import { showError } from "./toast.js";

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
        yesterdayEntries = [],
        mealIsEmpty = true,
    } = $props();

    const MEALS = ["breakfast", "lunch", "snack", "dinner", "supplements"];

    function todayStr() {
        const d = new Date();
        return [
            d.getFullYear(),
            String(d.getMonth() + 1).padStart(2, "0"),
            String(d.getDate()).padStart(2, "0"),
        ].join("-");
    }

    // Shared
    let tab = $state("food");
    let selectedDate = $state("");
    let drawerEl = $state(null);

    // Food
    let input = $state("");
    let sending = $state(false);
    let clarifyingQuestion = $state(null);
    let pendingImages = $state([]);
    let selectedMeal = $state(null);
    let inputEl = $state(null);
    let fileInputEl = $state(null);
    let mealError = $state(false);
    let savedEntries = $state(null); // entries just saved, awaiting confirmation

    // Edit mode state
    let editModeEntries = $state(null);
    let activeEditMealType = $state(null);
    let editSending = $state(false);
    let editMessage = $state("");
    let scalingAll = $state(false);
    let scaleAllOpen = $state(false);
    let scalingEntry = $state(-1);
    let scaleEntryOpen = $state(-1);
    let favorites = $state(null);
    let favSearch = $state("");
    let showFavPicker = $state(false);

    let isEditMode = $derived(editModeEntries !== null && editModeEntries.length > 0);
    let started = $derived(sending || savedEntries !== null || clarifyingQuestion !== null);
    let filteredFavs = $derived.by(() => {
        if (!favorites || !favSearch.trim()) return favorites ?? [];
        const q = favSearch.toLowerCase();
        return favorites.filter((f) => f.description.toLowerCase().includes(q));
    });

    // Activity
    let activityTextareaEl = $state(null);
    let feelingNotesEl = $state(null);
    let poopNotesEl = $state(null);
    let hydrationEl = $state(null);

    let activityText = $state("");
    let feelingNotes = $state("");
    let poop = $state(false);
    let poopNotes = $state("");
    let hydration = $state("");
    let activitySaving = $state(false);
    let activityLoadedFor = $state(null);

    // Drag-to-dismiss
    let dragStartY = null;
    let dragCurrentY = 0;

    function onDragStart(e) {
        const tag = e.target.tagName;
        if (
            tag === "TEXTAREA" ||
            tag === "INPUT" ||
            tag === "BUTTON" ||
            tag === "SELECT"
        )
            return;
        dragStartY = e.touches[0].clientY;
        dragCurrentY = 0;
        if (drawerEl) drawerEl.style.transition = "none";
    }

    function onDragMove(e) {
        if (dragStartY === null) return;
        const dy = e.touches[0].clientY - dragStartY;
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

    function revokePreview(url) {
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

    // Only re-run when `open` changes — read props via untrack so edits
    // during the session (e.g. onEntriesEdited resetting parent state) don't
    // re-initialize the drawer.
    $effect(() => {
        const isOpen = open;
        untrack(() => {
            if (isOpen) {
                tab = initialTab;
                selectedDate = date || todayStr();
                selectedMeal = meal;
                clearPendingImages();
                input = "";
                sending = false;
                clarifyingQuestion = null;
                savedEntries = null;
                // Edit mode init
                editModeEntries = editEntries ? [...editEntries] : null;
                activeEditMealType = editMealType;
                editSending = false;
                editMessage = "";
                scalingAll = false;
                scaleAllOpen = false;
                scalingEntry = -1;
                scaleEntryOpen = -1;
                showFavPicker = false;
                favSearch = "";
                if (!favorites) {
                    getFavorites().then((res) => { favorites = res.favorites ?? []; }).catch(() => {});
                }
                if (initialTab === "activity" && initialField) {
                    setTimeout(() => {
                        if (initialField === "activity")
                            activityTextareaEl?.focus();
                        else if (initialField === "feeling")
                            feelingNotesEl?.focus();
                        else if (initialField === "poop") poopNotesEl?.focus();
                        else if (initialField === "hydration") hydrationEl?.focus();
                    }, 120);
                }
            } else {
                tab = "food";
                selectedDate = "";
                selectedMeal = null;
                clearPendingImages();
                input = "";
                clarifyingQuestion = null;
                savedEntries = null;
                editModeEntries = null;
                activeEditMealType = null;
                editSending = false;
                editMessage = "";
                scalingAll = false;
                scaleAllOpen = false;
                scalingEntry = -1;
                scaleEntryOpen = -1;
                showFavPicker = false;
                favSearch = "";
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

    async function loadActivity(d) {
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
            await putActivity(selectedDate, {
                activity: activityText,
                feeling_score: 0,
                feeling_notes: feelingNotes,
                poop,
                poop_notes: poopNotes,
                hydration: hydration ? parseFloat(hydration) : 0,
            });
            onClose();
        } catch (err) {
            showError(err, "Failed to save activity.");
        } finally {
            activitySaving = false;
        }
    }

    async function onFileSelected(e) {
        const files = Array.from(e.target.files ?? []);
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

    function removeImage(index) {
        revokePreview(pendingImages[index]?.previewUrl);
        pendingImages = pendingImages.filter((_, i) => i !== index);
    }

    async function send() {
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
            const res = await chat(text, selectedDate, imgs, selectedMeal);
            sending = false;
            if (res.done && res.entries?.length) {
                savedEntries = res.entries;
                onEntriesAdded(res.entries);
            } else if (!res.done) {
                clarifyingQuestion = res.message || "Need more details.";
            }
        } catch (err) {
            sending = false;
            showError(err, "Something went wrong. Please try again.");
        }
    }

    function confirmSaved() {
        onClose();
    }

    function switchToEdit() {
        editModeEntries = [...savedEntries];
        activeEditMealType = selectedMeal;
        savedEntries = null;
    }

    // --- Edit mode functions ---

    async function sendEdit() {
        if (editSending || !editMessage.trim()) return;
        const text = editMessage.trim();
        editMessage = "";
        editSending = true;
        try {
            const res = await editChat(text, editModeEntries, selectedDate, activeEditMealType);
            if (res.entries?.length) {
                editModeEntries = res.entries;
                if (onEntriesEdited) onEntriesEdited(res.entries);
            }
        } catch (err) {
            showError(err, "Failed to edit meal.");
        } finally {
            editSending = false;
        }
    }

    async function scaleAllEntries(factor) {
        if (scalingAll || !editModeEntries) return;
        scalingAll = true;
        try {
            const r1 = (v) => Math.round(v * factor);
            const r10 = (v) => Math.round(v * factor * 10) / 10;
            const updates = editModeEntries.map((e) => ({
                ...e,
                calories: r1(e.calories),
                protein: r10(e.protein),
                carbs: r10(e.carbs),
                fat: r10(e.fat),
                fiber: r10(e.fiber ?? 0),
            }));
            const saved = await Promise.all(
                updates.map((u) => patchEntry(u.id, u)),
            );
            editModeEntries = saved;
            if (onEntriesEdited) onEntriesEdited(saved);
        } catch (err) {
            showError(err, "Failed to scale meal.");
        } finally {
            scalingAll = false;
        }
    }

    async function scaleOneEntry(index, factor) {
        if (scalingEntry >= 0) return;
        scalingEntry = index;
        scaleEntryOpen = -1;
        try {
            const entry = editModeEntries[index];
            if (!entry) return;
            const r1 = (v) => Math.round(v * factor);
            const r10 = (v) => Math.round(v * factor * 10) / 10;
            const updated = {
                ...entry,
                calories: r1(entry.calories),
                protein: r10(entry.protein),
                carbs: r10(entry.carbs),
                fat: r10(entry.fat),
                fiber: r10(entry.fiber ?? 0),
            };
            const saved = await patchEntry(updated.id, updated);
            editModeEntries = editModeEntries.map((e) => e.id === saved.id ? saved : e);
            if (onEntriesEdited) onEntriesEdited(editModeEntries);
        } catch (err) {
            showError(err, "Failed to scale entry.");
        } finally {
            scalingEntry = -1;
        }
    }

    async function editInlineEntry(index, field, value) {
        const entry = editModeEntries[index];
        if (!entry) return;
        const updated = { ...entry, [field]: value };
        editModeEntries = editModeEntries.map((e, i) => i === index ? updated : e);
        try {
            const saved = await patchEntry(updated.id, updated);
            editModeEntries = editModeEntries.map((e) => e.id === saved.id ? saved : e);
            if (onEntriesEdited) onEntriesEdited(editModeEntries);
        } catch (err) {
            showError(err, "Failed to save change.");
        }
    }

    async function deleteEditEntry(index) {
        const entry = editModeEntries[index];
        if (!entry) return;
        // Optimistically remove from UI immediately.
        editModeEntries = editModeEntries.filter((_, i) => i !== index);
        if (onEntriesEdited) onEntriesEdited(editModeEntries);
        if (editModeEntries.length === 0) onClose();
        try {
            await deleteEntry(entry.id);
        } catch (err) {
            showError(err, "Failed to delete entry.");
        }
    }

    async function addFavoriteToMeal(fav) {
        showFavPicker = false;
        try {
            const entries = [{
                meal_type: activeEditMealType,
                description: fav.description,
                calories: fav.calories,
                protein: fav.protein,
                carbs: fav.carbs,
                fat: fav.fat,
                fiber: fav.fiber ?? 0,
            }];
            const res = await confirmChat(entries, selectedDate);
            if (res.entries?.length) {
                editModeEntries = [...editModeEntries, ...res.entries];
                if (onEntriesEdited) onEntriesEdited(editModeEntries);
            }
        } catch (err) {
            showError(err, "Failed to add favorite.");
        }
    }

    async function repeatYesterday() {
        if (!yesterdayEntries.length) return;
        if (isEditMode) {
            // Edit mode: add to existing entries
            editSending = true;
            try {
                const entries = yesterdayEntries.map((e) => ({
                    meal_type: activeEditMealType,
                    description: e.description,
                    calories: e.calories,
                    protein: e.protein,
                    carbs: e.carbs,
                    fat: e.fat,
                    fiber: e.fiber ?? 0,
                }));
                const res = await confirmChat(entries, selectedDate);
                if (res.entries?.length) {
                    editModeEntries = [...editModeEntries, ...res.entries];
                    if (onEntriesEdited) onEntriesEdited(editModeEntries);
                }
            } catch (err) {
                showError(err, "Failed to repeat yesterday's meal.");
            } finally {
                editSending = false;
            }
        } else {
            // New-entry mode: save and show confirmation
            sending = true;
            try {
                const entries = yesterdayEntries.map((e) => ({
                    meal_type: selectedMeal,
                    description: e.description,
                    calories: e.calories,
                    protein: e.protein,
                    carbs: e.carbs,
                    fat: e.fat,
                    fiber: e.fiber ?? 0,
                }));
                const res = await confirmChat(entries, selectedDate);
                sending = false;
                if (res.entries?.length) {
                    savedEntries = res.entries;
                    onEntriesAdded(res.entries);
                }
            } catch (err) {
                sending = false;
                showError(err, "Failed to repeat yesterday's meal.");
            }
        }
    }

    async function addFavToNewMeal(fav) {
        if (!selectedMeal) return;
        sending = true;
        try {
            const entries = [{
                meal_type: selectedMeal,
                description: fav.description,
                calories: fav.calories,
                protein: fav.protein,
                carbs: fav.carbs,
                fat: fav.fat,
                fiber: fav.fiber ?? 0,
            }];
            const res = await confirmChat(entries, selectedDate);
            sending = false;
            if (res.entries?.length) {
                savedEntries = [...(savedEntries ?? []), ...res.entries];
                onEntriesAdded(res.entries);
            }
        } catch (err) {
            sending = false;
            showError(err, "Failed to add favorite.");
        }
    }

    function onEditKeyDown(e) {
        if (e.key === "Enter" && !e.shiftKey) {
            e.preventDefault();
            sendEdit();
        }
    }

    function onKeyDown(e) {
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
        <div class="handle"></div>

        <!-- Tab switcher + date -->
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
            </div>
            <input
                class="date-input"
                type="date"
                bind:value={selectedDate}
                max={todayStr()}
            />
        </div>

        {#if tab === "food"}
            {#if isEditMode}
                <!-- ===== EDIT MODE ===== -->
                <div class="meta-locked">
                    <span class="meta-chip">{activeEditMealType}</span>
                    <div class="scale-btns">
                        <button
                            class="scale-toggle"
                            class:open={scaleAllOpen}
                            onclick={() => (scaleAllOpen = !scaleAllOpen)}
                            disabled={scalingAll}
                            aria-label="Scale entire meal"
                            title="Scale entire meal"
                            >⊕</button
                        >
                        {#if scaleAllOpen}
                            {#each [0.75, 1.25, 1.5, 2] as factor}
                                <button
                                    class="scale-pill"
                                    onclick={() => { scaleAllOpen = false; scaleAllEntries(factor); }}
                                    disabled={scalingAll}
                                    >&times;{factor}</button
                                >
                            {/each}
                        {/if}
                        <button
                            class="scale-pill fav-pill"
                            onclick={() => (showFavPicker = !showFavPicker)}
                            >+ Fav</button
                        >
                        {#if yesterdayEntries.length > 0}
                            <button
                                class="scale-pill repeat-pill"
                                onclick={repeatYesterday}
                                disabled={editSending}
                                title="Repeat yesterday's {activeEditMealType}"
                                >Repeat</button
                            >
                        {/if}
                    </div>
                </div>
                {#if showFavPicker && favorites}
                    <div class="fav-picker">
                        <input
                            class="fav-search"
                            type="text"
                            placeholder="Search favorites…"
                            bind:value={favSearch}
                        />
                        <div class="fav-list">
                            {#each filteredFavs as fav}
                                <button class="fav-item" onclick={() => addFavoriteToMeal(fav)}>
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
                <div class="content-area">
                    <div class="result-card" class:dimmed={editSending}>
                        {#each editModeEntries as entry, i}
                            <div class="card-entry">
                                <div class="card-entry-head">
                                    <div class="card-desc">{entry.description}</div>
                                    <div class="entry-actions">
                                        <button
                                            class="entry-scale-toggle"
                                            class:open={scaleEntryOpen === i}
                                            onclick={() => (scaleEntryOpen = scaleEntryOpen === i ? -1 : i)}
                                            disabled={scalingEntry >= 0}
                                            aria-label="Scale portion"
                                            title="Scale portion"
                                            >⊕</button
                                        >
                                        <button
                                            class="entry-delete"
                                            onclick={() => deleteEditEntry(i)}
                                            aria-label="Delete entry">✕</button
                                        >
                                    </div>
                                </div>
                                {#if scaleEntryOpen === i}
                                    <div class="entry-scale-opts">
                                        {#each [0.75, 1.5, 2] as factor}
                                            <button
                                                class="scale-pill"
                                                onclick={() => scaleOneEntry(i, factor)}
                                                disabled={scalingEntry >= 0}
                                                >&times;{factor}</button
                                            >
                                        {/each}
                                    </div>
                                {/if}
                                <div class="card-macros">
                                    <span class="macro-field">
                                        <input type="number" value={entry.calories}
                                            onblur={(e) => editInlineEntry(i, "calories", +e.target.value)}
                                            disabled={editSending} />
                                        <span class="macro-label">cal</span>
                                    </span>
                                    <span class="macro-sep">·</span>
                                    <span class="macro-field">
                                        <input type="number" value={entry.protein}
                                            onblur={(e) => editInlineEntry(i, "protein", +e.target.value)}
                                            disabled={editSending} />
                                        <span class="macro-label">P</span>
                                    </span>
                                    <span class="macro-sep">·</span>
                                    <span class="macro-field">
                                        <input type="number" value={entry.carbs}
                                            onblur={(e) => editInlineEntry(i, "carbs", +e.target.value)}
                                            disabled={editSending} />
                                        <span class="macro-label">C</span>
                                    </span>
                                    <span class="macro-sep">·</span>
                                    <span class="macro-field">
                                        <input type="number" value={entry.fat}
                                            onblur={(e) => editInlineEntry(i, "fat", +e.target.value)}
                                            disabled={editSending} />
                                        <span class="macro-label">F</span>
                                    </span>
                                    <span class="macro-sep">·</span>
                                    <span class="macro-field">
                                        <input type="number" value={entry.fiber ?? 0}
                                            onblur={(e) => editInlineEntry(i, "fiber", +e.target.value)}
                                            disabled={editSending} />
                                        <span class="macro-label">Fb</span>
                                    </span>
                                </div>
                            </div>
                        {/each}
                    </div>
                </div>
                <div class="input-row">
                    <textarea
                        class="text-entry composer-input"
                        use:autosize
                        bind:value={editMessage}
                        onkeydown={onEditKeyDown}
                        placeholder="Describe changes…"
                        rows="1"
                        disabled={editSending}
                    ></textarea>
                    <button
                        onclick={sendEdit}
                        disabled={editSending || !editMessage.trim()}
                        >Edit</button
                    >
                    <button class="edit-done-btn" onclick={onClose}>Done</button>
                </div>
            {:else}
            <!-- ===== NEW ENTRY MODE ===== -->
            <!-- Meal pills -->
            {#if !started}
                <div class="meal-pills-wrap" class:shake={mealError}>
                    <div class="meal-pills">
                        {#each MEALS as m}
                            <button
                                class="meal-pill"
                                class:selected={selectedMeal === m}
                                onclick={() =>
                                    (selectedMeal =
                                        selectedMeal === m ? null : m)}
                                >{m}</button
                            >
                        {/each}
                    </div>
                </div>
            {:else}
                <div class="meta-locked">
                    {#if selectedMeal}<span class="meta-chip"
                            >{selectedMeal}</span
                        >{/if}
                </div>
            {/if}

            <!-- Content area -->
            <div class="content-area">
                {#if sending}
                    <!-- Skeleton loading -->
                    <div class="skeleton-card">
                        <div class="skeleton-entry">
                            <div class="sk-line" style="width: 62%"></div>
                            <div
                                class="sk-line"
                                style="width: 80%; margin-top: 0.4rem; opacity: 0.6"
                            ></div>
                        </div>
                        <div class="skeleton-entry">
                            <div class="sk-line" style="width: 45%"></div>
                            <div
                                class="sk-line"
                                style="width: 80%; margin-top: 0.4rem; opacity: 0.6"
                            ></div>
                        </div>
                    </div>
                {:else if savedEntries}
                    <!-- Saved confirmation -->
                    <div class="result-card">
                        {#each savedEntries as entry}
                            <div class="card-entry">
                                <div class="card-desc">{entry.description}</div>
                                <div class="card-macros">
                                    <span class="macro-field"><span class="macro-val">{entry.calories}</span><span class="macro-label">cal</span></span>
                                    <span class="macro-sep">·</span>
                                    <span class="macro-field"><span class="macro-val">{entry.protein}</span><span class="macro-label">P</span></span>
                                    <span class="macro-sep">·</span>
                                    <span class="macro-field"><span class="macro-val">{entry.carbs}</span><span class="macro-label">C</span></span>
                                    <span class="macro-sep">·</span>
                                    <span class="macro-field"><span class="macro-val">{entry.fat}</span><span class="macro-label">F</span></span>
                                    <span class="macro-sep">·</span>
                                    <span class="macro-field"><span class="macro-val">{entry.fiber ?? 0}</span><span class="macro-label">Fb</span></span>
                                </div>
                            </div>
                        {/each}
                    </div>
                {:else}
                    {#if clarifyingQuestion}
                        <p class="clarifying">{clarifyingQuestion}</p>
                    {/if}
                    <!-- Quick add: favorites + repeat -->
                    {#if !started && selectedMeal && favorites}
                        <div class="quick-add">
                            {#if mealIsEmpty && yesterdayEntries.length > 0}
                                <button
                                    class="repeat-btn"
                                    onclick={repeatYesterday}
                                    >Repeat yesterday's {selectedMeal}</button
                                >
                            {/if}
                            {#if favorites.length > 0}
                                <input
                                    class="fav-search"
                                    type="text"
                                    placeholder="Search favorites…"
                                    bind:value={favSearch}
                                />
                                <div class="fav-list">
                                    {#each filteredFavs.slice(0, 8) as fav}
                                        <button class="fav-item" onclick={() => addFavToNewMeal(fav)}>
                                            <span class="fav-desc">{fav.description}</span>
                                            <span class="fav-cal">{fav.calories} cal</span>
                                        </button>
                                    {/each}
                                    {#if filteredFavs.length === 0}
                                        <span class="fav-empty">No favorites found</span>
                                    {/if}
                                </div>
                            {/if}
                        </div>
                    {/if}
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
            {#if savedEntries}
                <div class="confirm-btns">
                    <button class="confirm-done" onclick={confirmSaved}>Looks good</button>
                    <button class="confirm-edit" onclick={switchToEdit}>Edit</button>
                </div>
            {:else if !sending}
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
                            onclick={() => fileInputEl.click()}
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
                        onclick={() => fileInputEl.click()}
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
                        placeholder="What did you eat?"
                        rows="1"
                        disabled={sending}
                    ></textarea>
                    <button
                        onclick={send}
                        disabled={sending ||
                            (!input.trim() && !pendingImages.length)}
                        >Send</button
                    >
                </div>
            {/if}
            {/if}
        {:else}
            <!-- Activity form -->
            <div class="activity-form">
                <div class="activity-field">
                    <label class="field-label" for="act-activity"
                        >Activity</label
                    >
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
                                onclick={() => (poop = true)}>Yes</button
                            >
                            <button
                                class="toggle-btn"
                                class:selected={poop === false}
                                onclick={() => (poop = false)}>No</button
                            >
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
                        <label class="field-label" for="act-hydration"
                            >Water</label
                        >
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
                <button
                    class="save-activity-btn"
                    onclick={saveActivity}
                    disabled={activitySaving}
                >
                    {activitySaving ? "Saving…" : "Save"}
                </button>
            </div>
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
        width: 36px;
        height: 5px;
        background: var(--rule);
        border-radius: 3px;
        margin: 0 auto 0.75rem;
    }

    /* --- Top header row (tabs + date) --- */
    .drawer-top {
        display: flex;
        justify-content: space-between;
        align-items: center;
        margin-bottom: 0.75rem;
    }

    .tabs {
        display: flex;
        gap: 0.4rem;
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

    .tab-btn.active {
        background: var(--paper-4);
        color: var(--ink);
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

    .meal-pill {
        padding: 0.3rem 0.75rem;
        border: 1px solid var(--rule-4);
        border-radius: var(--r-pill);
        background: none;
        font-family: inherit;
        font-size: var(--t-meta);
        color: var(--ink-mute);
        cursor: pointer;
        white-space: nowrap;
        touch-action: manipulation;
        font-weight: 500;
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

    .meta-locked {
        margin-bottom: 0.5rem;
    }

    .meta-chip {
        font-size: var(--t-micro);
        text-transform: uppercase;
        letter-spacing: 0.06em;
        color: var(--mute-2);
        font-weight: 600;
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

    .macro-val {
        font-family: var(--num-stack);
        font-size: var(--t-meta);
        color: var(--ink);
        font-variant-numeric: tabular-nums;
    }

    /* --- Confirmation buttons --- */
    .confirm-btns {
        display: flex;
        gap: 0.5rem;
    }

    .confirm-done {
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
    }

    .confirm-edit {
        padding: 0.6rem 1rem;
        background: none;
        color: var(--ink-mute);
        border: 1px solid var(--rule);
        border-radius: var(--r-sm);
        cursor: pointer;
        font-size: var(--t-body-sm);
        font-family: inherit;
        font-weight: 500;
    }

    @media (hover: hover) {
        .confirm-done:hover {
            background: var(--ink);
        }
        .confirm-edit:hover {
            border-color: var(--ink-2);
            color: var(--ink-2);
        }
    }

    .edit-done-btn {
        background: none;
        color: var(--ink-mute);
        border: 1px solid var(--rule);
        font-weight: 500;
    }

    @media (hover: hover) {
        .edit-done-btn:hover {
            border-color: var(--ink-2);
            color: var(--ink-2);
        }
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

    /* --- Activity form --- */
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

    .save-activity-btn {
        width: 100%;
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
        margin-top: auto;
    }

    @media (hover: hover) {
        .save-activity-btn:not(:disabled):hover {
            background: var(--ink);
        }
    }
    .save-activity-btn:disabled {
        opacity: 0.35;
        cursor: default;
    }

    /* --- Edit mode --- */
    .scale-btns {
        display: flex;
        gap: 0.3rem;
        margin-top: 0.35rem;
        flex-wrap: wrap;
        align-items: center;
    }

    .scale-toggle {
        background: none;
        border: none;
        color: var(--mute-4);
        font-size: 1.1rem;
        line-height: 1;
        cursor: pointer;
        padding: 0.15rem 0.25rem;
        min-width: 0;
        min-height: 0;
    }

    .scale-toggle.open {
        color: var(--ink-2);
    }

    .scale-toggle:disabled {
        opacity: 0.35;
        cursor: default;
    }

    @media (hover: hover) {
        .scale-toggle:not(:disabled):hover {
            color: var(--ink-2);
        }
    }

    .scale-pill {
        background: none;
        border: 1px solid var(--rule-4);
        border-radius: var(--r-pill);
        padding: 0.2rem 0.6rem;
        font-size: var(--t-meta);
        color: var(--ink-mute);
        cursor: pointer;
        font-family: inherit;
        font-weight: 500;
        touch-action: manipulation;
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

    .fav-pill {
        margin-left: auto;
    }

    .card-entry-head {
        display: flex;
        justify-content: space-between;
        align-items: flex-start;
        gap: 0.5rem;
    }

    .entry-actions {
        display: flex;
        align-items: center;
        gap: 0.15rem;
        flex-shrink: 0;
    }

    .entry-scale-toggle {
        background: none;
        border: none;
        color: var(--mute-4);
        font-size: 0.9rem;
        line-height: 1;
        cursor: pointer;
        padding: 0.15rem 0.25rem;
        min-width: 0;
        min-height: 0;
    }

    .entry-scale-toggle.open {
        color: var(--ink-2);
    }

    .entry-scale-toggle:disabled {
        opacity: 0.35;
        cursor: default;
    }

    @media (hover: hover) {
        .entry-scale-toggle:not(:disabled):hover {
            color: var(--ink-2);
        }
    }

    .entry-scale-opts {
        display: flex;
        gap: 0.3rem;
        padding: 0.25rem 0 0.15rem;
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
    }

    @media (hover: hover) {
        .entry-delete:hover {
            color: var(--danger, #c00);
        }
    }

    .fav-picker {
        margin-bottom: 0.5rem;
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

    .quick-add {
        display: flex;
        flex-direction: column;
        gap: 0.5rem;
    }

    .repeat-btn {
        width: 100%;
        padding: 0.55rem 0.75rem;
        background: var(--paper-3);
        color: var(--ink-mute);
        border: 1px solid var(--rule);
        border-radius: var(--r-sm);
        font-size: var(--t-body-sm);
        font-family: inherit;
        font-weight: 500;
        cursor: pointer;
        text-align: left;
        touch-action: manipulation;
    }

    @media (hover: hover) {
        .repeat-btn:hover {
            border-color: var(--ink-2);
            color: var(--ink-2);
        }
    }
</style>
