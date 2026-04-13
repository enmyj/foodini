<script>
    import { untrack } from "svelte";
    import { chat, confirmChat, getActivity, putActivity } from "./api.js";
    import { autosize } from "./autosize.js";
    import { showError } from "./toast.js";

    let {
        open,
        onClose,
        onEntriesAdded,
        date = null,
        meal = null,
        initialTab = "food",
        initialField = null,
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
    let currentEntries = $state(null);
    let clarifyingQuestion = $state(null);
    let refineInput = $state("");
    let refineNote = $state(null);
    let pendingImages = $state([]);
    let selectedMeal = $state(null);
    let inputEl = $state(null);
    let refineEl = $state(null);
    let fileInputEl = $state(null);
    let mealError = $state(false);

    let started = $derived(
        sending || currentEntries !== null || clarifyingQuestion !== null,
    );

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

    $effect(() => {
        if (open) {
            tab = initialTab;
            selectedDate = date || todayStr();
            selectedMeal = meal;
            untrack(() => clearPendingImages());
            input = "";
            sending = false;
            currentEntries = null;
            clarifyingQuestion = null;
            refineInput = "";
            refineNote = null;
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
            untrack(() => clearPendingImages());
            input = "";
            currentEntries = null;
            clarifyingQuestion = null;
            refineInput = "";
            refineNote = null;
            activityText = "";
            feelingNotes = "";
            poop = false;
            poopNotes = "";
            hydration = "";
            activitySaving = false;
            activityLoadedFor = null;
        }
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
            if (res.pending && res.entries?.length) {
                currentEntries = res.entries;
                setTimeout(() => refineEl?.focus(), 30);
            } else {
                clarifyingQuestion = res.message;
            }
        } catch (err) {
            showError(err, "Something went wrong. Please try again.");
            clarifyingQuestion = "Something went wrong. Please try again.";
        } finally {
            sending = false;
        }
    }

    async function refine() {
        if (sending || !refineInput.trim()) return;
        const text = refineInput.trim();
        refineInput = "";
        refineNote = null;
        sending = true;
        try {
            const res = await chat(text, selectedDate, null, selectedMeal);
            if (res.pending) {
                currentEntries = res.entries;
            } else {
                refineNote = res.message;
            }
        } catch (err) {
            showError(err, "Something went wrong.");
            refineNote = "Something went wrong.";
        } finally {
            sending = false;
        }
    }

    async function confirm() {
        if (!currentEntries || sending) return;
        sending = true;
        try {
            const res = await confirmChat(currentEntries, selectedDate);
            sending = false;
            onEntriesAdded(res.entries);
            onClose();
        } catch (err) {
            showError(err, "Failed to save.");
            refineNote = "Failed to save. Please try again.";
            sending = false;
        }
    }

    function onKeyDown(e) {
        if (e.key === "Enter" && !e.shiftKey) {
            e.preventDefault();
            send();
        }
    }

    function onRefineKeyDown(e) {
        if (e.key === "Enter") {
            e.preventDefault();
            refine();
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
                {#if sending && !currentEntries}
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
                {:else if currentEntries}
                    <!-- Result card -->
                    <div class="result-card" class:dimmed={sending}>
                        {#each currentEntries as entry, i}
                            <div class="card-entry">
                                <div class="card-desc">{entry.description}</div>
                                <div class="card-macros">
                                    <span class="macro-field">
                                        <input
                                            type="number"
                                            value={entry.calories}
                                            oninput={(e) =>
                                                (currentEntries[i] = {
                                                    ...currentEntries[i],
                                                    calories: +e.target.value,
                                                })}
                                            disabled={sending}
                                        />
                                        <span class="macro-label">cal</span>
                                    </span>
                                    <span class="macro-sep">·</span>
                                    <span class="macro-field">
                                        <input
                                            type="number"
                                            value={entry.protein}
                                            oninput={(e) =>
                                                (currentEntries[i] = {
                                                    ...currentEntries[i],
                                                    protein: +e.target.value,
                                                })}
                                            disabled={sending}
                                        />
                                        <span class="macro-label">P</span>
                                    </span>
                                    <span class="macro-sep">·</span>
                                    <span class="macro-field">
                                        <input
                                            type="number"
                                            value={entry.carbs}
                                            oninput={(e) =>
                                                (currentEntries[i] = {
                                                    ...currentEntries[i],
                                                    carbs: +e.target.value,
                                                })}
                                            disabled={sending}
                                        />
                                        <span class="macro-label">C</span>
                                    </span>
                                    <span class="macro-sep">·</span>
                                    <span class="macro-field">
                                        <input
                                            type="number"
                                            value={entry.fat}
                                            oninput={(e) =>
                                                (currentEntries[i] = {
                                                    ...currentEntries[i],
                                                    fat: +e.target.value,
                                                })}
                                            disabled={sending}
                                        />
                                        <span class="macro-label">F</span>
                                    </span>
                                    {#if entry.fiber}
                                        <span class="macro-sep">·</span>
                                        <span class="macro-field">
                                            <input
                                                type="number"
                                                value={entry.fiber}
                                                oninput={(e) =>
                                                    (currentEntries[i] = {
                                                        ...currentEntries[i],
                                                        fiber: +e.target.value,
                                                    })}
                                                disabled={sending}
                                            />
                                            <span class="macro-label">Fb</span>
                                        </span>
                                    {/if}
                                </div>
                            </div>
                        {/each}
                    </div>
                {:else}
                    <!-- Input mode -->
                    {#if clarifyingQuestion}
                        <p class="clarifying">{clarifyingQuestion}</p>
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
            {#if currentEntries}
                {#if refineNote}
                    <p class="refine-note">{refineNote}</p>
                {/if}
                <div class="refine-row">
                    <textarea
                        class="text-entry compact refine-input"
                        bind:this={refineEl}
                        use:autosize
                        bind:value={refineInput}
                        placeholder="Adjust… e.g. 'double the rice'"
                        onkeydown={onRefineKeyDown}
                        rows="1"
                        disabled={sending}
                    ></textarea>
                    {#if refineInput.trim()}
                        <button onclick={refine} disabled={sending}
                            >Adjust</button
                        >
                    {:else}
                        <button
                            class="save-btn"
                            onclick={confirm}
                            disabled={sending}>Save</button
                        >
                    {/if}
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

    /* --- Refine row --- */
    .refine-note {
        font-size: var(--t-meta);
        color: var(--mute);
        margin: 0 0 0.4rem;
    }

    .refine-row {
        display: flex;
        gap: 0.5rem;
        align-items: flex-end;
    }

    .refine-input {
        flex: 1;
        border: 1px solid var(--rule);
        border-radius: var(--r-sm);
        padding: 0.5rem 0.75rem;
        font-size: var(--t-body-sm);
        font-family: inherit;
        background: var(--paper);
        color: var(--ink);
    }

    .refine-row textarea:focus {
        outline: none;
        border-color: var(--ink-2);
    }

    .save-btn {
        padding: 0.6rem 1rem;
        background: var(--ink-2);
        color: var(--paper);
        border: none;
        border-radius: var(--r-sm);
        cursor: pointer;
        font-size: var(--t-body-sm);
        font-family: inherit;
        font-weight: 500;
        white-space: nowrap;
        touch-action: manipulation;
    }

    @media (hover: hover) {
        .save-btn:not(:disabled):hover {
            background: var(--ink);
        }
    }

    .save-btn:disabled {
        opacity: 0.35;
        cursor: default;
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

    @media (max-width: 700px) {
        .drawer {
            width: 100%;
            max-width: none;
            height: min(88dvh, 760px);
            max-height: calc(100dvh - 0.25rem);
            padding-left: 1rem;
            padding-right: 1rem;
        }

        .date-input,
        .macro-field input,
        .refine-input,
        .hydration-input,
        .composer-input,
        .activity-form .text-entry {
            font-size: 16px;
        }
    }
</style>
