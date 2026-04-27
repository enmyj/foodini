<script lang="ts">
    import { createMutation } from "@tanstack/svelte-query";
    import { untrack } from "svelte";
    import { addEvent, agent, deleteEntry, deleteEvent, patchEntry, patchEvent } from "./api.ts";
    import { autosize } from "./autosize.ts";
    import { todayStr } from "./date.ts";
    import { showError } from "./toast.ts";
    import { EVENT_KINDS } from "./types.ts";
    import type {
        AgentAction,
        Entry,
        EventKind,
        LogEvent,
        MealType,
        PendingImage,
    } from "./types.ts";

    type DrawerMode = "meal" | "event";

    type EditableEntryField =
        | "calories"
        | "protein"
        | "carbs"
        | "fat"
        | "fiber";

    interface ChatMsg {
        role: "user" | "agent" | "action";
        text?: string;
        previewUrls?: string[];
        action?: AgentAction;
    }

    let {
        open,
        onClose,
        onEntriesAdded,
        onEntriesEdited = null,
        onEventChanged = null,
        date = null,
        meal = null,
        editEntries = null,
        editMealType = null,
        editEvent = null,
        initialMode = null,
    }: {
        open: boolean;
        onClose: () => void;
        onEntriesAdded: (entries: Entry[]) => void;
        onEntriesEdited?:
            | ((entries: Entry[], mealType: MealType | null) => void)
            | null;
        onEventChanged?: ((change: { added?: LogEvent; updated?: LogEvent; deletedId?: string }) => void) | null;
        date?: string | null;
        meal?: MealType | null;
        editEntries?: Entry[] | null;
        editMealType?: MealType | null;
        editEvent?: LogEvent | null;
        initialMode?: DrawerMode | null;
    } = $props();

    let mode = $state<DrawerMode | null>(null);
    let entryTime = $state("");
    let eventKind = $state<EventKind | null>(null);
    let eventText = $state("");
    let eventWaterMl = $state(250);
    let eventFeelingScore = $state(7);
    let eventSaving = $state(false);
    let eventDeleting = $state(false);

    const EVENT_KIND_LABELS: Record<EventKind, string> = {
        workout: "Activity",
        stool: "Stool",
        water: "Water",
        feeling: "Feeling",
    };

    function nowHHMM(): string {
        const d = new Date();
        return `${String(d.getHours()).padStart(2, "0")}:${String(d.getMinutes()).padStart(2, "0")}`;
    }

    function resetEventForm() {
        eventKind = null;
        eventText = "";
        eventWaterMl = 250;
        eventFeelingScore = 7;
        eventSaving = false;
        eventDeleting = false;
    }

    async function onEntryTimeBlur() {
        const newTime = entryTime;
        if (!newTime || !entries.length || mode !== "meal") return;
        const needsUpdate = entries.some((e) => (e.time ?? "") !== newTime);
        if (!needsUpdate) return;
        try {
            const updated = await Promise.all(
                entries.map((e) =>
                    (e.time ?? "") === newTime
                        ? Promise.resolve(e)
                        : patchEntryMutation.mutateAsync({
                              id: e.id,
                              entry: { ...e, time: newTime },
                          }),
                ),
            );
            entries = updated;
            if (onEntriesEdited) onEntriesEdited(updated, mealType);
        } catch (err) {
            showError(err, "Failed to update time.");
        }
    }

    async function deleteEditingEvent() {
        if (!editEvent || eventDeleting || eventSaving) return;
        eventDeleting = true;
        try {
            await deleteEvent(editEvent.id);
            if (onEventChanged) onEventChanged({ deletedId: editEvent.id });
            onClose();
        } catch (err) {
            showError(err, "Failed to delete event.");
        } finally {
            eventDeleting = false;
        }
    }

    function selectEventKind(kind: EventKind) {
        eventKind = kind;
        eventText = "";
        eventWaterMl = 250;
        eventFeelingScore = 7;
    }

    async function saveEvent() {
        if (!eventKind || eventSaving) return;
        const time = entryTime || nowHHMM();
        const text = eventText.trim();
        const payload: {
            date: string;
            time: string;
            kind: EventKind;
            text?: string;
            num?: number;
        } = { date: selectedDate, time, kind: eventKind };
        if (eventKind === "water") payload.num = Math.max(0, Math.round(eventWaterMl));
        else if (eventKind === "feeling") {
            payload.num = Math.max(1, Math.min(10, Math.round(eventFeelingScore)));
            if (text) payload.text = text;
        } else {
            if (text) payload.text = text;
        }
        eventSaving = true;
        try {
            if (editEvent) {
                const patch: Partial<LogEvent> = {
                    date: payload.date,
                    time: payload.time,
                    kind: payload.kind,
                    text: payload.text ?? "",
                };
                if (payload.num !== undefined) patch.num = payload.num;
                const updated = await patchEvent(editEvent.id, patch);
                if (onEventChanged) onEventChanged({ updated });
            } else {
                const saved = await addEvent(payload);
                if (onEventChanged) onEventChanged({ added: saved });
            }
            onClose();
        } catch (err) {
            showError(err, "Failed to save event.");
        } finally {
            eventSaving = false;
        }
    }

    let selectedDate = $state("");
    let drawerEl = $state<HTMLDivElement | null>(null);
    let scrollEl = $state<HTMLDivElement | null>(null);
    let inputEl = $state<HTMLTextAreaElement | null>(null);
    let fileInputEl = $state<HTMLInputElement | null>(null);

    let input = $state("");
    let sending = $state(false);
    let pendingImages = $state<PendingImage[]>([]);

    let messages = $state<ChatMsg[]>([]);
    let entries = $state<Entry[]>([]);
    let mealType = $state<MealType | null>(null);
    let firstSend = $state(true);
    let deletingEntryIds = $state<Set<string>>(new Set());

    let dragStartY = $state<number | null>(null);
    let dragCurrentY = 0;

    const agentMutation = createMutation(() => ({
        mutationFn: ({
            message,
            date,
            images,
            meal,
            time,
            currentEntries,
            reset,
        }: {
            message: string;
            date: string;
            images: File[] | null;
            meal: MealType | null;
            time: string | null;
            currentEntries: Entry[] | null;
            reset: boolean;
        }) =>
            agent(message, {
                date,
                meal,
                time,
                images,
                currentEntries,
                reset,
            }),
        onError: (err) =>
            showError(err, "Something went wrong. Please try again."),
    }));

    const patchEntryMutation = createMutation(() => ({
        mutationFn: ({ id, entry }: { id: string; entry: Partial<Entry> }) =>
            patchEntry(id, entry),
    }));

    const deleteEntryMutation = createMutation(() => ({
        mutationFn: (id: string) => deleteEntry(id),
    }));

    $effect(() => {
        const isOpen = open;
        untrack(() => {
            if (isOpen) {
                selectedDate = date || todayStr();
                mealType = editMealType ?? meal ?? null;
                entries = editEntries ? [...editEntries] : [];
                messages = [];
                input = "";
                sending = false;
                clearPendingImages();
                firstSend = true;
                deletingEntryIds = new Set();
                if (editEvent) mode = "event";
                else if (editEntries || meal) mode = "meal";
                else if (initialMode) mode = initialMode;
                else mode = null;
                resetEventForm();
                if (editEvent) {
                    eventKind = editEvent.kind;
                    eventText = editEvent.text ?? "";
                    if (editEvent.kind === "water") {
                        eventWaterMl = Math.max(0, Math.round(editEvent.num ?? 250));
                    } else if (editEvent.kind === "feeling") {
                        eventFeelingScore = Math.max(1, Math.min(10, Math.round(editEvent.num ?? 7)));
                    }
                }
                entryTime = editEvent?.time || editEntries?.[0]?.time || nowHHMM();
                if (mode === "meal") setTimeout(() => inputEl?.focus(), 60);
            } else {
                selectedDate = "";
                mealType = null;
                entries = [];
                messages = [];
                input = "";
                clearPendingImages();
                deletingEntryIds = new Set();
                mode = null;
                resetEventForm();
            }
        });
    });

    function pickMode(next: DrawerMode) {
        mode = next;
        if (next === "meal") setTimeout(() => inputEl?.focus(), 60);
    }

    $effect(() => {
        const len = messages.length;
        if (!len || !scrollEl) return;
        queueMicrotask(() => {
            scrollEl?.scrollTo({
                top: scrollEl.scrollHeight,
                behavior: "smooth",
            });
        });
    });

    function revokePreview(url: string): void {
        if (typeof url === "string" && url.startsWith("blob:")) {
            URL.revokeObjectURL(url);
        }
    }

    function clearPendingImages() {
        for (const img of pendingImages) revokePreview(img.previewUrl);
        pendingImages = [];
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
        if (target instanceof HTMLElement && isInsideScrollable(target)) return;
        dragStartY = touch.clientY;
        dragCurrentY = 0;
        if (drawerEl) drawerEl.style.transition = "none";
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

    function onFileSelected(e: Event): void {
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
        const text = input.trim();
        const sentImages = pendingImages;
        const imgs = sentImages.length ? sentImages.map((i) => i.file) : null;
        if (!text && !imgs) return;

        const previewUrls = sentImages.map((i) => i.previewUrl);
        pendingImages = [];

        const userMsg: ChatMsg = { role: "user", previewUrls };
        if (text) userMsg.text = text;
        messages = [...messages, userMsg];
        input = "";
        sending = true;

        await sendAgent(text, imgs, sentImages);
        sending = false;
    }

    async function sendAgent(
        text: string,
        imgs: File[] | null,
        sentImages: PendingImage[],
    ): Promise<void> {
        try {
            const res = await agentMutation.mutateAsync({
                message: text,
                date: selectedDate,
                images: imgs,
                meal: mealType,
                time: entryTime || null,
                currentEntries: entries.length ? [...entries] : null,
                reset: firstSend,
            });
            firstSend = false;
            for (const action of res.actions ?? []) {
                applyAgentAction(action);
                messages = [...messages, { role: "action", action }];
            }
            if (res.message) {
                messages = [...messages, { role: "agent", text: res.message }];
            }
        } catch {
            // mutation onError already surfaced toast
        } finally {
            for (const img of sentImages) revokePreview(img.previewUrl);
        }
    }

    function applyAgentAction(action: AgentAction) {
        if (action.type === "meal_added" && action.entries?.length) {
            const addedMeal = action.entries[0]?.meal_type ?? null;
            if (mealType === null && addedMeal) {
                mealType = addedMeal;
                entries = [...action.entries];
            } else if (mealType && addedMeal === mealType) {
                entries = [...entries, ...action.entries];
            }
            onEntriesAdded(action.entries);
        } else if (action.type === "meal_edited" && action.entries) {
            const editedMeal = action.entries[0]?.meal_type ?? mealType;
            if (mealType && editedMeal === mealType) {
                entries = action.entries;
            }
            if (onEntriesEdited)
                onEntriesEdited(action.entries, editedMeal ?? null);
        } else if (action.type === "event_added" && action.event) {
            if (onEventChanged) onEventChanged({ added: action.event });
        } else if (action.type === "event_edited" && action.event) {
            if (onEventChanged) onEventChanged({ updated: action.event });
        } else if (action.type === "event_deleted" && action.event_id) {
            if (onEventChanged) onEventChanged({ deletedId: action.event_id });
        }
    }

    async function editInlineEntry(
        index: number,
        field: EditableEntryField,
        value: number,
    ): Promise<void> {
        const entry = entries[index];
        if (!entry) return;
        const updated: Entry = { ...entry, [field]: value };
        entries = entries.map((e, i) => (i === index ? updated : e));
        try {
            const saved = await patchEntryMutation.mutateAsync({
                id: updated.id,
                entry: updated,
            });
            entries = entries.map((e) => (e.id === saved.id ? saved : e));
            if (onEntriesEdited) onEntriesEdited(entries, mealType);
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
            if (onEntriesEdited) onEntriesEdited(nextEntries, mealType);
        } catch (err) {
            showError(err, "Failed to delete entry.");
        } finally {
            deletingEntryIds = new Set(
                [...deletingEntryIds].filter((id) => id !== entry.id),
            );
        }
    }

    function numberValueFromEvent(e: FocusEvent): number {
        const target = e.currentTarget as HTMLInputElement;
        return Number(target.value);
    }

    function actionLabel(a: AgentAction): string {
        switch (a.type) {
            case "meal_added": {
                const n = a.entries?.length ?? 0;
                const cal = (a.entries ?? []).reduce(
                    (s, e) => s + (e.calories ?? 0),
                    0,
                );
                const m = a.entries?.[0]?.meal_type ?? "";
                return `Added ${n} item${n === 1 ? "" : "s"}${m ? ` to ${m}` : ""} (${cal} cal)`;
            }
            case "meal_edited": {
                const n = a.entries?.length ?? 0;
                const cal = (a.entries ?? []).reduce(
                    (s, e) => s + (e.calories ?? 0),
                    0,
                );
                const m = a.entries?.[0]?.meal_type ?? "";
                return `Updated ${m || "meal"} — ${n} item${n === 1 ? "" : "s"} (${cal} cal)`;
            }
            case "event_added":
            case "event_edited": {
                const ev = a.event;
                if (!ev) return a.type === "event_added" ? "Event added" : "Event updated";
                const verb = a.type === "event_added" ? "Logged" : "Updated";
                switch (ev.kind) {
                    case "workout":
                        return `${verb} workout: ${ev.text ?? ""}`.trim();
                    case "stool":
                        return `${verb} bowel movement${ev.text ? `: ${ev.text}` : ""}`;
                    case "water":
                        return `${verb} water: ${Math.round(ev.num ?? 0)}ml`;
                    case "feeling":
                        return `${verb} feeling: ${ev.num ?? 0}/10${ev.text ? ` — ${ev.text}` : ""}`;
                }
                return `${verb} event`;
            }
            case "event_deleted":
                return "Event deleted";
            case "favorite_added":
                return "Saved to favorites";
            default:
                return "";
        }
    }

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

        <div class="drawer-top">
            <div class="top-left">
                <input
                    class="date-input"
                    type="date"
                    bind:value={selectedDate}
                    max={todayStr()}
                />
                <input
                    class="date-input time-input"
                    type="time"
                    lang="en-GB"
                    bind:value={entryTime}
                    onblur={onEntryTimeBlur}
                    title="Time"
                />
                {#if mealType}
                    <span class="meal-tag">{mealType}</span>
                {/if}
            </div>
            <div class="top-right">
                <button class="done-btn" onclick={onClose}>Done</button>
            </div>
        </div>

        {#if mode === null}
            <div class="mode-picker">
                <p class="mode-prompt">What would you like to log?</p>
                <div class="mode-bubbles">
                    <button class="mode-bubble" onclick={() => pickMode("meal")}>
                        <span class="mode-bubble-emoji">🍽️</span>
                        <span class="mode-bubble-label">Meal</span>
                        <span class="mode-bubble-hint">food, drink</span>
                    </button>
                    <button class="mode-bubble" onclick={() => pickMode("event")}>
                        <span class="mode-bubble-emoji">📋</span>
                        <span class="mode-bubble-label">Event</span>
                        <span class="mode-bubble-hint">activity, water, stool, feeling</span>
                    </button>
                </div>
            </div>
        {:else if mode === "event"}
            <div class="event-form">
                <div class="kind-bubbles">
                    {#each EVENT_KINDS as k}
                        <button
                            class="kind-bubble"
                            class:active={eventKind === k}
                            onclick={() => selectEventKind(k)}
                        >{EVENT_KIND_LABELS[k]}</button>
                    {/each}
                </div>
                {#if eventKind}
                    <div class="event-fields">
                        {#if eventKind === "water"}
                            <label class="field-row">
                                <span class="field-label">Amount (ml)</span>
                                <input type="number" min="0" max="5000" step="50" bind:value={eventWaterMl} />
                            </label>
                            <div class="quick-pills">
                                {#each [125, 250, 500, 750, 1000] as ml}
                                    <button class="quick-pill" onclick={() => (eventWaterMl = ml)}>{ml}ml</button>
                                {/each}
                            </div>
                        {:else if eventKind === "feeling"}
                            <label class="field-row">
                                <span class="field-label">Score (1-10)</span>
                                <div class="slider-wrap">
                                    <input type="range" min="1" max="10" step="1" bind:value={eventFeelingScore} />
                                    <span class="slider-val">{eventFeelingScore}</span>
                                </div>
                            </label>
                            <label class="field-row stacked">
                                <span class="field-label">Note (optional)</span>
                                <textarea rows="2" bind:value={eventText} placeholder="energy, mood, anything notable"></textarea>
                            </label>
                        {:else if eventKind === "stool"}
                            <label class="field-row stacked">
                                <span class="field-label">Note (optional)</span>
                                <textarea rows="2" bind:value={eventText} placeholder="consistency, urgency, etc."></textarea>
                            </label>
                        {:else if eventKind === "workout"}
                            <label class="field-row stacked">
                                <span class="field-label">What did you do?</span>
                                <textarea rows="3" bind:value={eventText} placeholder="e.g. 30min run, bench 3×8 @135"></textarea>
                            </label>
                        {/if}
                        <div class="event-actions">
                            {#if editEvent}
                                <button class="event-delete" onclick={deleteEditingEvent} disabled={eventSaving || eventDeleting}>
                                    {eventDeleting ? "Deleting…" : "Delete"}
                                </button>
                            {/if}
                            <button class="event-save" onclick={saveEvent} disabled={eventSaving || eventDeleting}>
                                {eventSaving ? "Saving…" : "Save"}
                            </button>
                        </div>
                    </div>
                {/if}
            </div>
        {/if}

        {#snippet entriesCard()}
            <div class="result-card" class:dimmed={sending}>
                {#each entries as entry, i}
                    <div
                        class="card-entry"
                        class:dimmed={deletingEntryIds.has(entry.id)}
                    >
                        <div class="card-entry-head">
                            <div class="card-desc">{entry.description}</div>
                            <button
                                class="entry-delete"
                                class:deleting={deletingEntryIds.has(entry.id)}
                                onclick={() => deleteEntry_(i)}
                                disabled={deletingEntryIds.has(entry.id)}
                                aria-label={deletingEntryIds.has(entry.id)
                                    ? "Deleting…"
                                    : "Delete entry"}
                                >{#if deletingEntryIds.has(entry.id)}<span
                                        class="entry-spinner"
                                        aria-hidden="true"
                                    ></span>{:else}✕{/if}</button
                            >
                        </div>
                        <div class="card-macros">
                            <span class="macro-field">
                                <input
                                    type="number"
                                    value={entry.calories}
                                    onblur={(e: FocusEvent) =>
                                        editInlineEntry(
                                            i,
                                            "calories",
                                            numberValueFromEvent(e),
                                        )}
                                    disabled={sending ||
                                        deletingEntryIds.has(entry.id)}
                                />
                                <span class="macro-label">cal</span>
                            </span>
                            <span class="macro-sep">·</span>
                            <span class="macro-field">
                                <input
                                    type="number"
                                    value={entry.protein}
                                    onblur={(e: FocusEvent) =>
                                        editInlineEntry(
                                            i,
                                            "protein",
                                            numberValueFromEvent(e),
                                        )}
                                    disabled={sending ||
                                        deletingEntryIds.has(entry.id)}
                                />
                                <span class="macro-label">P</span>
                            </span>
                            <span class="macro-sep">·</span>
                            <span class="macro-field">
                                <input
                                    type="number"
                                    value={entry.carbs}
                                    onblur={(e: FocusEvent) =>
                                        editInlineEntry(
                                            i,
                                            "carbs",
                                            numberValueFromEvent(e),
                                        )}
                                    disabled={sending ||
                                        deletingEntryIds.has(entry.id)}
                                />
                                <span class="macro-label">C</span>
                            </span>
                            <span class="macro-sep">·</span>
                            <span class="macro-field">
                                <input
                                    type="number"
                                    value={entry.fat}
                                    onblur={(e: FocusEvent) =>
                                        editInlineEntry(
                                            i,
                                            "fat",
                                            numberValueFromEvent(e),
                                        )}
                                    disabled={sending ||
                                        deletingEntryIds.has(entry.id)}
                                />
                                <span class="macro-label">F</span>
                            </span>
                            <span class="macro-sep">·</span>
                            <span class="macro-field">
                                <input
                                    type="number"
                                    value={entry.fiber ?? 0}
                                    onblur={(e: FocusEvent) =>
                                        editInlineEntry(
                                            i,
                                            "fiber",
                                            numberValueFromEvent(e),
                                        )}
                                    disabled={sending ||
                                        deletingEntryIds.has(entry.id)}
                                />
                                <span class="macro-label">Fb</span>
                            </span>
                        </div>
                    </div>
                {/each}
            </div>
        {/snippet}

        {#if mode === "meal"}
        <div class="messages" bind:this={scrollEl}>
            {#if messages.length === 0}
                <p class="empty">
                    {#if entries.length > 0}
                        Tweak this meal, scale it, or add more.
                    {:else}
                        What did you eat?
                    {/if}
                </p>
            {/if}
            {#each messages as msg, i (i)}
                {#if msg.role === "user"}
                    <div class="msg user">
                        <div class="bubble">
                            {#if msg.previewUrls?.length}
                                <div class="msg-thumbs">
                                    {#each msg.previewUrls as url}
                                        <img
                                            class="msg-thumb"
                                            src={url}
                                            alt=""
                                        />
                                    {/each}
                                </div>
                            {/if}
                            {#if msg.text}
                                <div class="msg-text">{msg.text}</div>
                            {/if}
                        </div>
                    </div>
                {:else if msg.role === "agent"}
                    <div class="msg model">
                        <div class="bubble">{msg.text}</div>
                    </div>
                {:else if msg.role === "action" && msg.action}
                    <div class="msg action">
                        <div class="action-bubble">
                            {actionLabel(msg.action)}
                        </div>
                    </div>
                {/if}
            {/each}
            {#if sending}
                <div class="msg model">
                    <div class="bubble typing">
                        <span></span><span></span><span></span>
                    </div>
                </div>
            {/if}
            {#if entries.length > 0}
                {@render entriesCard()}
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

        <div class="composer">
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
                    placeholder={entries.length
                        ? "Tweak this meal, or add more…"
                        : "What did you eat?"}
                    rows="1"
                    disabled={sending}
                ></textarea>
                <button
                    class="send-btn"
                    onclick={send}
                    disabled={sending ||
                        (!input.trim() && !pendingImages.length)}
                    >Send</button
                >
            </div>
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
        height: min(82vh, 720px);
        height: min(82dvh, 720px);
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

    .drawer-top {
        display: flex;
        justify-content: space-between;
        align-items: center;
        gap: 0.5rem;
        margin-bottom: 0.5rem;
        flex-wrap: wrap;
    }

    .top-left {
        display: flex;
        align-items: center;
        gap: 0.35rem;
        flex-wrap: wrap;
        min-width: 0;
    }

    .meal-tag {
        font-size: 0.7rem;
        text-transform: capitalize;
        background: var(--paper-3);
        color: var(--ink);
        padding: 0.2rem 0.55rem;
        border-radius: var(--r-pill);
        font-weight: 500;
        letter-spacing: 0.02em;
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

    .done-btn:disabled {
        opacity: 0.35;
        cursor: default;
    }

    @media (hover: hover) {
        .done-btn:not(:disabled):hover {
            background: var(--ink);
        }
    }

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

    @media (max-width: 480px) {
        .drawer {
            padding-left: 1rem;
            padding-right: 1rem;
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

    /* --- Mode picker --- */
    .mode-picker {
        display: flex;
        flex-direction: column;
        align-items: center;
        gap: 1rem;
        padding: 2rem 0.5rem;
        flex: 1;
        justify-content: center;
    }

    .mode-prompt {
        font-size: var(--t-body);
        color: var(--mute);
        margin: 0;
    }

    .mode-bubbles {
        display: flex;
        gap: 1rem;
        flex-wrap: wrap;
        justify-content: center;
    }

    .mode-bubble {
        display: flex;
        flex-direction: column;
        align-items: center;
        gap: 0.25rem;
        background: var(--paper);
        border: 1px solid var(--rule);
        border-radius: var(--r-md);
        padding: 1.1rem 1.4rem;
        cursor: pointer;
        font-family: inherit;
        color: var(--ink);
        min-width: 9rem;
        transition: border-color 0.12s, background 0.12s, transform 0.08s;
        touch-action: manipulation;
    }

    .mode-bubble:active {
        transform: scale(0.98);
    }

    @media (hover: hover) {
        .mode-bubble:hover {
            border-color: var(--ink-2);
            background: var(--paper-2);
        }
    }

    .mode-bubble-emoji {
        font-size: 1.75rem;
        line-height: 1;
    }

    .mode-bubble-label {
        font-size: var(--t-body-sm);
        font-weight: 600;
    }

    .mode-bubble-hint {
        font-size: var(--t-meta);
        color: var(--mute-2);
    }

    /* --- Event form --- */
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

    .field-row input[type="number"] {
        border: 1px solid var(--rule);
        border-radius: var(--r-sm);
        padding: 0.45rem 0.65rem;
        font-family: inherit;
        font-size: var(--t-body-sm);
        color: var(--ink);
        background: var(--paper);
        font-variant-numeric: tabular-nums;
    }

    .field-row input[type="number"] {
        width: 6rem;
    }

    .field-row textarea {
        border: 1px solid var(--rule);
        border-radius: var(--r-sm);
        padding: 0.5rem 0.65rem;
        font-family: inherit;
        font-size: var(--t-body-sm);
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

    /* --- Result ledger --- */
    .result-card {
        border-top: 1px solid var(--rule);
        border-bottom: 1px solid var(--rule);
        margin-bottom: 0.5rem;
        transition: opacity 0.15s;
    }

    .result-card.dimmed {
        opacity: 0.5;
    }

    .card-entry {
        padding: 0.55rem 0;
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

    .card-entry-head {
        display: flex;
        justify-content: space-between;
        align-items: flex-start;
        gap: 0.5rem;
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
        to {
            transform: rotate(360deg);
        }
    }

    @media (hover: hover) {
        .entry-delete:hover {
            color: var(--danger, #c00);
        }
    }

    /* --- Messages --- */
    .messages {
        flex: 1;
        min-height: 0;
        overflow-y: auto;
        display: flex;
        flex-direction: column;
        gap: 0.5rem;
        padding: 0.25rem 0;
        margin-bottom: 0.5rem;
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

    .msg.action {
        justify-content: center;
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

    .msg-thumbs {
        display: flex;
        gap: 0.3rem;
        margin-bottom: 0.3rem;
        flex-wrap: wrap;
    }

    .msg-thumb {
        width: 60px;
        height: 60px;
        border-radius: var(--r-sm);
        object-fit: cover;
        display: block;
    }

    .msg-text {
        white-space: pre-line;
    }

    .action-bubble {
        font-size: var(--t-meta);
        color: var(--mute);
        background: var(--paper-3);
        padding: 0.3rem 0.7rem;
        border-radius: var(--r-pill);
        font-style: italic;
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
        0%,
        80%,
        100% {
            transform: translateY(0);
            opacity: 0.5;
        }
        40% {
            transform: translateY(-4px);
            opacity: 1;
        }
    }

    /* --- Composer --- */
    .composer {
        display: flex;
        flex-direction: column;
        gap: 0.5rem;
    }

    .file-input {
        display: none;
    }

    .thumb-strip {
        display: flex;
        gap: 0.5rem;
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

    .send-btn {
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

    .send-btn:disabled {
        opacity: 0.35;
        cursor: default;
    }

    button:focus-visible,
    input:focus-visible,
    textarea:focus-visible {
        outline: 2px solid var(--ink-2);
        outline-offset: 2px;
    }
</style>
