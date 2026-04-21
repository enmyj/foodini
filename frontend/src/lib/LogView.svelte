<script lang="ts">
    import { createMutation, createQuery, useQueryClient } from "@tanstack/svelte-query";
    import {
        getLog,
        addFavorite,
        getFavorites,
        fetchStoredInsight,
        fetchStoredDayInsight,
        fetchStoredWeekSuggestions,
        fetchMealSuggestion,
        generateDayInsights,
        generateInsights,
        generateWeekSuggestions,
        generateMealSuggestion,
    } from "./api.ts";
    import type { ApiError } from "./api.ts";
    import EntryRow from "./EntryRow.svelte";
    import ChatDrawer from "./ChatDrawer.svelte";
    import ActivityNote from "./ActivityNote.svelte";
    import { appendEntriesToLogCache, removeEntryFromLogCache, replaceMealEntriesInLogCache, updateEntryInLogCache } from "./cache.ts";
    import { addDays, formatDateNav, getMonday, todayStr } from "./date.ts";
    import ProfilePanel from "./ProfilePanel.svelte";
    import FavoritesView from "./FavoritesView.svelte";
    import HistoryWeekBlock from "./HistoryWeekBlock.svelte";
    import InsightPanel from "./InsightPanel.svelte";
    import { queryKeys } from "./queryKeys.ts";
    import { showError } from "./toast.ts";
    import { navigate } from "./router.svelte.ts";
    import ThemeToggle from "./ThemeToggle.svelte";
    import { MEAL_ORDER } from "./types.ts";
    import type {
        ActivityField,
        Entry,
        Favorite,
        InsightPanelState,
        LogResponse,
        MealEntriesMap,
        MealType,
        WeekGroup,
        WeekInsightPanelState,
    } from "./types.ts";

    const queryClient = useQueryClient();
    const LOG_RECONCILE_DELAY_MS = 1200;
    const DAY_INSIGHT_REGEN_DELAY_MS = 900;

    type ViewMode = "day" | "favorites" | "history" | "profile";
    const DAY_ABBREV = ["Su", "Mo", "Tu", "We", "Th", "Fr", "Sa"];
    let view = $state<ViewMode>("day");
    let currentDate = $state(todayStr());
    let menuOpen = $state(false);
    let drawerOpen = $state(false);
    let drawerTab = $state<"food" | "activity">("food");
    let activityRefreshKey = $state(0);
    let drawerDate = $state<string | null>(null);
    let drawerMeal = $state<MealType | null>(null);
    let drawerField = $state<ActivityField | null>(null);
    let drawerEditEntries = $state<Entry[] | null>(null);
    let drawerEditMealType = $state<MealType | null>(null);
    let dateInputEl = $state<HTMLInputElement | null>(null);
    let mealSuggestions = $state<Record<string, InsightPanelState>>({});

    let dayInsight = $state<InsightPanelState | null>(null);
    let dayInsightExpanded = $state(false);
    let totalsOpen = $state(false);
    let insightsByWeek = $state<Record<string, WeekInsightPanelState>>({});
    let suggestionsByWeek = $state<Record<string, WeekInsightPanelState>>({});
    let collapsedMeals = $state<Set<MealType>>(new Set(MEAL_ORDER));
    let historyWeeks = $state(4);
    let favoritedDescs = $state<Set<string>>(new Set());
    let logReconcileTimer: ReturnType<typeof setTimeout> | null = null;
    let dayInsightFresh = $state(false);
    let dayInsightRegenTimer: ReturnType<typeof setTimeout> | null = null;
    let dayInsightRequestId = 0;

    const addFavoriteMutation = createMutation(() => ({
        mutationFn: (entry: Entry) => addFavorite(entry),
        onSuccess: (_data, entry) => {
            favoritedDescs = new Set([
                ...favoritedDescs,
                normalizeFavoriteKey(entry.description),
            ]);
            queryClient.invalidateQueries({ queryKey: queryKeys.favorites });
        },
        onError: (e: unknown, entry) => {
            if (
                typeof e === "object" &&
                e !== null &&
                (("status" in e && e.status === 409) ||
                    ("code" in e && e.code === "favorite_exists"))
            ) {
                favoritedDescs = new Set([
                    ...favoritedDescs,
                    normalizeFavoriteKey(entry.description),
                ]);
                return;
            }
            console.error("addFavorite failed:", e);
            showError(e, "Failed to add to favorites.");
        },
    }));

    // --- TanStack Queries ---

    const dayQuery = createQuery(() => ({
        queryKey: queryKeys.logDay(currentDate),
        queryFn: () => getLog({ date: currentDate }),
        enabled: view === "day",
    }));

    const yesterdayQuery = createQuery(() => ({
        queryKey: queryKeys.logDay(addDays(currentDate, -1)),
        queryFn: () => getLog({ date: addDays(currentDate, -1) }),
        enabled: view === "day",
    }));

    const historyQuery = createQuery(() => ({
        queryKey: queryKeys.logHistory(historyWeeks),
        queryFn: () => getLog({ days: historyWeeks * 7 }),
        enabled: view === "history",
    }));

    const favoritesQuery = createQuery(() => ({
        queryKey: queryKeys.favorites,
        queryFn: getFavorites,
    }));

    // --- Derived state from queries (same variable names as before) ---

    let dayData = $derived<LogResponse | null>(dayQuery.data ?? null);
    let historyData = $derived<LogResponse | null>(historyQuery.data ?? null);
    let loading = $derived(
        (view === "day" && dayQuery.isPending) ||
        (view === "history" && historyQuery.isPending),
    );
    let spreadsheetUrl = $derived(
        dayQuery.data?.spreadsheet_url ||
        historyQuery.data?.spreadsheet_url ||
        "",
    );

    // Derive load error from active query
    let loadError = $derived.by(() => {
        const err = (view === "day" ? dayQuery.error : view === "history" ? historyQuery.error : null) as Partial<ApiError> | null;
        if (!err) return "";
        if (err?.status === 401 || err?.code === "session_expired")
            return "Your session expired. Sign in again.";
        if (err?.code === "insufficient_scopes")
            return "Google permissions are missing. Re-authorize to continue.";
        return view === "day"
            ? "Could not load this day. Try reloading, or sign in again."
            : "Could not load history. Try reloading, or sign in again.";
    });
    let loadErrorAction = $derived.by<{ href: string; label: string } | null>(() => {
        const err = (view === "day" ? dayQuery.error : view === "history" ? historyQuery.error : null) as Partial<ApiError> | null;
        if (!err) return null;
        if (err?.status === 401 || err?.code === "session_expired")
            return { href: "/auth/login", label: "Sign in" };
        if (err?.code === "insufficient_scopes")
            return { href: "/auth/login?consent=1", label: "Re-authorize" };
        return null;
    });

    let yesterdayByMeal = $derived.by<MealEntriesMap>(() => {
        const entries = yesterdayQuery.data?.entries ?? [];
        const g: MealEntriesMap = {};
        for (const e of entries) {
            (g[e.meal_type] ??= []).push(e);
        }
        return g;
    });

    let weekGroupsData = $derived(weekGroups(historyData, historyWeeks));
    let drawerHistoryMeal = $derived<MealType | null>(
        drawerEditMealType ?? drawerMeal,
    );

    // Sync favorited descriptions from query
    $effect(() => {
        const favs = favoritesQuery.data?.favorites;
        if (favs) {
            favoritedDescs = new Set(
                favs.map((f) => normalizeFavoriteKey(f.description)),
            );
        }
    });

    // Reset UI state when date/view changes
    $effect(() => {
        if (view === "day") {
            void currentDate;
            dayInsightRequestId++;
            if (dayInsightRegenTimer) {
                clearTimeout(dayInsightRegenTimer);
                dayInsightRegenTimer = null;
            }
            collapsedMeals = new Set(MEAL_ORDER);
            dayInsight = null;
            dayInsightExpanded = false;
            dayInsightFresh = false;
            mealSuggestions = {};
        }
    });

    $effect(() => {
        return () => {
            if (logReconcileTimer) clearTimeout(logReconcileTimer);
            if (dayInsightRegenTimer) clearTimeout(dayInsightRegenTimer);
        };
    });

    function openDatePicker(): void {
        if (!dateInputEl) return;
        if (typeof dateInputEl.showPicker === "function") {
            try {
                dateInputEl.showPicker();
            } catch {}
        }
    }

    function groupedByMeal(entries: Entry[] | null | undefined): MealEntriesMap {
        const g: MealEntriesMap = {};
        for (const e of entries ?? []) {
            (g[e.meal_type] ??= []).push(e);
        }
        return g;
    }

    function totals(entries: Entry[] | null | undefined) {
        return (entries ?? []).reduce(
            (a, e) => ({
                calories: a.calories + e.calories,
                protein: a.protein + e.protein,
                carbs: a.carbs + e.carbs,
                fat: a.fat + e.fat,
                fiber: a.fiber + (e.fiber ?? 0),
            }),
            { calories: 0, protein: 0, carbs: 0, fat: 0, fiber: 0 },
        );
    }

    function weekGroups(data: LogResponse | null, numWeeks = 8): WeekGroup[] {
        if (!data) return [];
        const { entries = [], daily_logs = [] } = data;
        const byDate: Record<string, { entries: Entry[]; dayLog: LogResponse["daily_logs"][number] | null }> = {};
        for (const e of entries) {
            (byDate[e.date] ??= { entries: [], dayLog: null }).entries.push(e);
        }
        for (const l of daily_logs) {
            (byDate[l.date] ??= { entries: [], dayLog: null }).dayLog = l;
        }

        const today = todayStr();
        let monday = getMonday(addDays(today, -(numWeeks * 7 - 1)));
        const todayMonday = getMonday(today);
        const weeks: WeekGroup[] = [];
        while (monday <= todayMonday) {
            const days = Array.from({ length: 7 }, (_, i) => {
                const date = addDays(monday, i);
                const future = date > today;
                return {
                    date,
                    future,
                    ...(future
                        ? { entries: [], dayLog: null }
                        : (byDate[date] ?? { entries: [], dayLog: null })),
                };
            });
            const sunday = addDays(monday, 6);
            const weekTotal = days.reduce(
                (t, d) => d.entries.reduce((s, e) => s + e.calories, t),
                0,
            );
            weeks.push({
                weekStart: monday,
                weekEnd: sunday <= today ? sunday : today,
                days,
                weekTotal,
            });
            monday = addDays(monday, 7);
        }
        return weeks.reverse();
    }

    function handleUpdate(updated: Entry) {
        applyDayLogMutation(updated.date, (old: LogResponse | undefined) =>
            updateEntryInLogCache(old, updated),
        );
    }

    function openEditDrawer(meal: MealType, group: Entry[]) {
        drawerEditEntries = group;
        drawerEditMealType = meal;
        drawerDate = currentDate;
        drawerMeal = meal;
        drawerField = null;
        drawerTab = "food";
        drawerOpen = true;
    }

    function onEntriesEdited(updatedEntries: Entry[]) {
        const editedMeal = drawerEditMealType;
        applyDayLogMutation(updatedEntries[0]?.date ?? currentDate, (old: LogResponse | undefined) =>
            replaceMealEntriesInLogCache(old, editedMeal, updatedEntries),
        );
        if (editedMeal && (updatedEntries[0]?.date ?? currentDate) === currentDate) {
            collapsedMeals = new Set([...collapsedMeals, editedMeal]);
        }
    }

    function handleDelete(id: string) {
        applyDayLogMutation(currentDate, (old: LogResponse | undefined) =>
            removeEntryFromLogCache(old, id),
        );
    }

    function normalizeFavoriteKey(desc: string | null | undefined): string {
        return (desc ?? "").toLowerCase().trim().replace(/\s+/g, " ");
    }

    async function handleFavoriteEntry(entry: Entry): Promise<void> {
        const key = normalizeFavoriteKey(entry.description);
        if (favoritedDescs.has(key)) return;
        try {
            await addFavoriteMutation.mutateAsync(entry);
        } catch {
        }
    }

    function syncFavoritedDescs(favorites: Favorite[]) {
        favoritedDescs = new Set(
            favorites.map((f) => normalizeFavoriteKey(f.description)),
        );
    }

    function openActivityDrawer(field: ActivityField | null = null) {
        drawerField = field;
        drawerTab = "activity";
        drawerDate = currentDate;
        drawerMeal = null;
        drawerOpen = true;
    }

    function closeDrawer() {
        if (drawerTab === "activity") activityRefreshKey++;
        drawerOpen = false;
        drawerDate = null;
        drawerMeal = null;
        drawerTab = "food";
        drawerField = null;
        drawerEditEntries = null;
        drawerEditMealType = null;
    }

    function closeProfile() {
        view = "day";
    }

    function onEntriesAdded(newEntries: Entry[]) {
        const addedDate = newEntries[0]?.date ?? currentDate;
        applyDayLogMutation(addedDate, (old: LogResponse | undefined) =>
            appendEntriesToLogCache(old, newEntries),
        );
        if (addedDate === currentDate) {
            const mealsAdded = new Set(newEntries.map((e) => e.meal_type));
            collapsedMeals = new Set([...collapsedMeals, ...mealsAdded]);
        }
    }

    // --- Meal suggestions (for empty meals) ---

    async function toggleMealSuggestion(meal: MealType, date: string) {
        const key = `${date}|${meal}`;
        const cur = mealSuggestions[key];
        if (cur?.open && cur?.text) {
            mealSuggestions = { ...mealSuggestions, [key]: { ...cur, open: false } };
            return;
        }
        if (cur?.text) {
            mealSuggestions = { ...mealSuggestions, [key]: { ...cur, open: true } };
            return;
        }
        mealSuggestions = {
            ...mealSuggestions,
            [key]: { loading: true, text: null, error: null, open: true, generatedAt: null },
        };
        try {
            const stored = await fetchMealSuggestion(date, meal);
            if (stored.suggestion) {
                mealSuggestions = {
                    ...mealSuggestions,
                    [key]: { loading: false, text: stored.suggestion, error: null, open: true, generatedAt: stored.generated_at ?? null },
                };
                return;
            }
            const res = await generateMealSuggestion(date, meal);
            mealSuggestions = {
                ...mealSuggestions,
                [key]: { loading: false, text: res.suggestion ?? null, error: null, open: true, generatedAt: res.generated_at ?? null },
            };
        } catch {
            mealSuggestions = {
                ...mealSuggestions,
                [key]: { loading: false, text: null, error: "Could not load suggestion", open: true, generatedAt: null },
            };
        }
    }

    async function regenMealSuggestion(meal: MealType, date: string) {
        const key = `${date}|${meal}`;
        mealSuggestions = {
            ...mealSuggestions,
            [key]: { loading: true, text: null, error: null, open: true, generatedAt: null },
        };
        try {
            const res = await generateMealSuggestion(date, meal);
            mealSuggestions = {
                ...mealSuggestions,
                [key]: { loading: false, text: res.suggestion ?? null, error: null, open: true, generatedAt: res.generated_at ?? null },
            };
        } catch {
            mealSuggestions = {
                ...mealSuggestions,
                [key]: { loading: false, text: null, error: "Could not load suggestion", open: true, generatedAt: null },
            };
        }
    }

    // --- Day insights ---

    function applyDayLogMutation(
        date: string,
        updater: (old: LogResponse | undefined) => LogResponse | undefined,
    ): void {
        void queryClient.cancelQueries({ queryKey: queryKeys.logBase });
        if (date === currentDate) {
            queryClient.setQueryData(queryKeys.logDay(currentDate), updater);
        }
        scheduleLogReconcile();
        if (date === currentDate) {
            scheduleDayInsightRegeneration(currentDate);
        }
    }

    function scheduleLogReconcile(): void {
        if (logReconcileTimer) clearTimeout(logReconcileTimer);
        logReconcileTimer = setTimeout(() => {
            logReconcileTimer = null;
            void queryClient.invalidateQueries({ queryKey: queryKeys.logBase });
        }, LOG_RECONCILE_DELAY_MS);
    }

    function scheduleDayInsightRegeneration(date: string): void {
        if (dayInsightRegenTimer) clearTimeout(dayInsightRegenTimer);
        dayInsightRegenTimer = setTimeout(() => {
            dayInsightRegenTimer = null;
            if (view !== "day" || currentDate !== date) return;
            void fetchDayInsights(date, true, {
                open: dayInsight?.open ?? false,
            }).then(() => {
                if (!dayInsight?.open && dayInsight?.text) {
                    dayInsightFresh = true;
                }
            });
        }, DAY_INSIGHT_REGEN_DELAY_MS);
    }

    async function fetchDayInsights(
        date: string,
        regenerate = false,
        options: { open?: boolean } = {},
    ) {
        const requestId = ++dayInsightRequestId;
        const open = options.open ?? true;
        dayInsight = {
            loading: true,
            text: dayInsight?.text ?? null,
            error: null,
            open,
            generatedAt: dayInsight?.generatedAt ?? null,
        };
        try {
            if (!regenerate) {
                const stored = await fetchStoredDayInsight(date);
                if (requestId !== dayInsightRequestId) return;
                if (stored.insight) {
                    dayInsight = {
                        loading: false,
                        text: stored.insight,
                        error: null,
                        open,
                        generatedAt: stored.generated_at ?? null,
                    };
                    return;
                }
            }
            const res = await generateDayInsights(date);
            if (requestId !== dayInsightRequestId) return;
            dayInsight = {
                loading: false,
                text: res.insight ?? null,
                error: null,
                open,
                generatedAt: res.generated_at ?? null,
            };
        } catch (e: unknown) {
            if (requestId !== dayInsightRequestId) return;
            dayInsight = {
                loading: false,
                text: dayInsight?.text ?? null,
                error: e instanceof Error ? e.message : "Could not load insights",
                open,
                generatedAt: dayInsight?.generatedAt ?? null,
            };
        }
    }

    function toggleDayInsights() {
        dayInsightFresh = false;
        if (!dayInsight || (!dayInsight.loading && !dayInsight.text && !dayInsight.error)) {
            fetchDayInsights(currentDate, false);
        } else {
            dayInsight = { ...dayInsight, open: !dayInsight.open };
        }
    }

    // --- Weekly insights & suggestions ---

    async function fetchInsights(weekStart: string, weekEnd: string, regenerate = false) {
        insightsByWeek = {
            ...insightsByWeek,
            [weekStart]: {
                open: true,
                loading: true,
                text: null,
                error: null,
                generatedAt: null,
                loaded: false,
            },
        };
        try {
            if (!regenerate) {
                const stored = await fetchStoredInsight(weekStart, weekEnd);
                if (stored.insight) {
                    insightsByWeek = {
                        ...insightsByWeek,
                        [weekStart]: {
                            open: true,
                            loading: false,
                            text: stored.insight,
                            error: null,
                            generatedAt: stored.generated_at ?? null,
                            loaded: true,
                        },
                    };
                    return;
                }
            }
            const res = await generateInsights(weekStart, weekEnd);
            insightsByWeek = {
                ...insightsByWeek,
                [weekStart]: {
                    open: true,
                    loading: false,
                    text: res.insight ?? null,
                    error: null,
                    generatedAt: res.generated_at ?? null,
                    loaded: true,
                },
            };
        } catch {
            insightsByWeek = {
                ...insightsByWeek,
                [weekStart]: {
                    open: true,
                    loading: false,
                    text: null,
                    error: "Could not load insights",
                    generatedAt: null,
                    loaded: true,
                },
            };
        }
    }

    function canCloseDayInsight(insight: InsightPanelState | null): boolean {
        return Boolean(insight && !insight.loading && (insight.error || insight.text != null));
    }

    function closeDayInsight(): void {
        if (!dayInsight) return;
        dayInsight = { ...dayInsight, open: false };
    }

    function toggleInsights(weekStart: string, weekEnd: string) {
        const cur = insightsByWeek[weekStart];
        if (!cur || !cur.loaded) {
            fetchInsights(weekStart, weekEnd, false);
        } else {
            insightsByWeek = {
                ...insightsByWeek,
                [weekStart]: { ...cur, open: !cur.open },
            };
        }
    }

    async function fetchWeekSuggestions(
        weekStart: string,
        weekEnd: string,
        regenerate = false,
    ): Promise<void> {
        suggestionsByWeek = {
            ...suggestionsByWeek,
            [weekStart]: {
                open: true,
                loading: true,
                text: null,
                error: null,
                generatedAt: null,
                loaded: false,
            },
        };
        try {
            if (!regenerate) {
                const stored = await fetchStoredWeekSuggestions(
                    weekStart,
                    weekEnd,
                );
                if (stored.suggestions) {
                    suggestionsByWeek = {
                        ...suggestionsByWeek,
                        [weekStart]: {
                            open: true,
                            loading: false,
                            text: stored.suggestions,
                            error: null,
                            generatedAt: stored.generated_at ?? null,
                            loaded: true,
                        },
                    };
                    return;
                }
            }
            const res = await generateWeekSuggestions(weekStart, weekEnd);
            suggestionsByWeek = {
                ...suggestionsByWeek,
                [weekStart]: {
                    open: true,
                    loading: false,
                    text: res.suggestions ?? null,
                    error: null,
                    generatedAt: res.generated_at ?? null,
                    loaded: true,
                },
            };
        } catch {
            suggestionsByWeek = {
                ...suggestionsByWeek,
                [weekStart]: {
                    open: true,
                    loading: false,
                    text: null,
                    error: "Could not load suggestions",
                    generatedAt: null,
                    loaded: true,
                },
            };
        }
    }

    function toggleWeekSuggestions(weekStart: string, weekEnd: string) {
        const cur = suggestionsByWeek[weekStart];
        if (!cur || !cur.loaded) {
            fetchWeekSuggestions(weekStart, weekEnd, false);
        } else {
            suggestionsByWeek = {
                ...suggestionsByWeek,
                [weekStart]: { ...cur, open: !cur.open },
            };
        }
    }
</script>

<div class="wrap">
    <header>
        <div class="header-top">
            <div class="nav-left">
                <button
                    class="hamburger"
                    onclick={() => (menuOpen = !menuOpen)}
                    aria-label="Navigation menu"
                    aria-expanded={menuOpen}
                >
                    <svg width="18" height="18" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round"><line x1="3" y1="6" x2="21" y2="6" /><line x1="3" y1="12" x2="21" y2="12" /><line x1="3" y1="18" x2="21" y2="18" /></svg>
                </button>
                <span class="view-label">{view === "day" ? "Day" : view === "history" ? "History" : view === "favorites" ? "Favorites" : view === "profile" ? "Profile" : ""}</span>
                {#if menuOpen}
                    <!-- svelte-ignore a11y_click_events_have_key_events -->
                    <div class="menu-backdrop" aria-hidden="true" onclick={() => (menuOpen = false)}></div>
                    <nav class="nav-menu">
                        <button class:active={view === "day"} onclick={() => { view = "day"; menuOpen = false; }}>Day</button>
                        <button class:active={view === "history"} onclick={() => { view = "history"; menuOpen = false; }}>History</button>
                        <button class:active={view === "favorites"} onclick={() => { view = "favorites"; menuOpen = false; }}>Favorites</button>
                        <hr />
                        <button class:active={view === "profile"} onclick={() => { view = "profile"; menuOpen = false; }}>Profile</button>
                    </nav>
                {/if}
            </div>
            <div class="header-actions">
                <ThemeToggle />
                {#if spreadsheetUrl}
                    <a
                        class="sheet-link"
                        href={spreadsheetUrl}
                        target="_blank"
                        rel="noopener"
                        aria-label="Open Google Sheet"
                        title="Open Google Sheet"
                    >
                        <svg
                            width="15"
                            height="15"
                            viewBox="0 0 24 24"
                            fill="none"
                            stroke="currentColor"
                            stroke-width="2"
                            stroke-linecap="round"
                            stroke-linejoin="round"
                            ><path
                                d="M14 2H6a2 2 0 0 0-2 2v16a2 2 0 0 0 2 2h12a2 2 0 0 0 2-2V8z"
                            /><polyline points="14 2 14 8 20 8" /><line
                                x1="8"
                                y1="13"
                                x2="16"
                                y2="13"
                            /><line x1="8" y1="17" x2="16" y2="17" /><polyline
                                points="10 9 9 9 8 9"
                            /></svg
                        >
                    </a>
                {/if}
                <a
                    class="home-btn"
                    href="/"
                    onclick={(e: MouseEvent) => { e.preventDefault(); navigate("/"); }}
                    aria-label="Home"
                    title="Home"
                >
                    <svg width="15" height="15" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"><path d="M3 9l9-7 9 7v11a2 2 0 0 1-2 2H5a2 2 0 0 1-2-2z" /><polyline points="9 22 9 12 15 12 15 22" /></svg>
                </a>
                <a
                    class="signout-btn"
                    href="/auth/logout"
                    aria-label="Sign out"
                    title="Sign out"
                >
                    <svg
                        width="15"
                        height="15"
                        viewBox="0 0 24 24"
                        fill="none"
                        stroke="currentColor"
                        stroke-width="2"
                        stroke-linecap="round"
                        stroke-linejoin="round"
                        ><path
                            d="M9 21H5a2 2 0 0 1-2-2V5a2 2 0 0 1 2-2h4"
                        /><polyline points="16 17 21 12 16 7" /><line
                            x1="21"
                            y1="12"
                            x2="9"
                            y2="12"
                        /></svg
                    >
                </a>
            </div>
        </div>
        {#if view === "history"}
            <div class="week-picker">
                {#each [4, 8, 12, 26] as w}
                    <button
                        class="wp-btn"
                        class:active={historyWeeks === w}
                        onclick={() => (historyWeeks = w)}>{w}w</button
                    >
                {/each}
            </div>
        {/if}
        {#if view === "day"}
            <div class="date-nav">
                <button
                    class="nav-arrow"
                    onclick={() => (currentDate = addDays(currentDate, -1))}
                    aria-label="Previous day">‹</button
                >
                <button class="nav-date" onclick={openDatePicker}>
                    {formatDateNav(currentDate)}
                    <input
                        type="date"
                        class="date-input-hidden"
                        value={currentDate}
                        onchange={(e: Event) => {
                            const target = e.currentTarget as HTMLInputElement;
                            if (target.value) currentDate = target.value;
                        }}
                        bind:this={dateInputEl}
                    />
                </button>
                <button
                    class="nav-arrow"
                    onclick={() => (currentDate = addDays(currentDate, 1))}
                    aria-label="Next day">›</button
                >
            </div>
            {#if dayData?.entries}
                {@const t = totals(dayData.entries)}
                <div class="totals">
                    <button
                        class="totals-toggle"
                        class:active={totalsOpen}
                        onclick={() => (totalsOpen = !totalsOpen)}
                        aria-expanded={totalsOpen}
                    >{t.calories} cal <span class="totals-arrow" aria-hidden="true">{totalsOpen ? "▾" : "▸"}</span></button>
                    <div class="macros" class:open={totalsOpen}>
                        <span class="macro-cal">{t.calories} cal</span>
                        <span>{t.protein}g P</span>
                        <span>{t.carbs}g C</span>
                        <span>{t.fat}g F</span>
                        <span>{t.fiber}g Fb</span>
                    </div>
                    {#if t.calories > 0}
                        <button
                            class="insights-btn"
                            class:active={dayInsight?.open}
                            class:fresh={dayInsightFresh}
                            class:generating={dayInsight?.loading}
                            onclick={toggleDayInsights}
                            aria-label="AI insights"
                            title="AI insights">insights{#if dayInsightFresh}<span class="insight-dot"></span>{/if}</button
                        >
                    {/if}
                </div>
            {/if}
        {/if}
    </header>

    {#if loading}
        <p class="state">Loading…</p>
    {:else if loadError}
        <div class="state-block">
            <p class="state error">{loadError}</p>
            {#if loadErrorAction}
                <a class="state-link" href={loadErrorAction.href}
                    >{loadErrorAction.label}</a
                >
            {/if}
        </div>
    {:else if view === "day"}
        {#if dayInsight?.open}
            <div class="day-insights-panel">
                <InsightPanel
                    loading={dayInsight.loading}
                    error={dayInsight.error}
                    text={dayInsight.text}
                    generatedAt={dayInsight.generatedAt}
                    closeable={canCloseDayInsight(dayInsight)}
                    collapsed={!dayInsightExpanded}
                    showMoreToggle={Boolean(dayInsight.generatedAt)}
                    expanded={dayInsightExpanded}
                    onClose={closeDayInsight}
                    onToggleExpanded={() =>
                        (dayInsightExpanded = !dayInsightExpanded)}
                    onRegenerate={() => fetchDayInsights(currentDate, true)}
                />
            </div>
        {/if}
        {#each MEAL_ORDER as meal}
            {@const group = groupedByMeal(dayData?.entries)[meal] ?? []}
            {@const collapsed = collapsedMeals.has(meal)}
            {@const miKey = `${currentDate}|${meal}`}
            {@const ms = mealSuggestions[miKey]}
            <section>
                <div class="meal-header">
                    <button
                        class="meal-name"
                        onclick={() => {
                            if (collapsed) {
                                collapsedMeals = new Set(
                                    [...collapsedMeals].filter(
                                        (m) => m !== meal,
                                    ),
                                );
                            } else {
                                collapsedMeals = new Set([
                                    ...collapsedMeals,
                                    meal,
                                ]);
                            }
                        }}
                    >
                        <span class="meal-arrow" aria-hidden="true"
                            >{collapsed ? "▸" : "▾"}</span
                        >
                        {meal}
                    </button>
                    {#if group.length > 0}
                        <button
                            class="meal-action-btn"
                            onclick={() => openEditDrawer(meal, group)}
                            >Edit</button
                        >
                    {:else if meal !== "supplements"}
                        <button
                            class="meal-action-btn"
                            class:active={ms?.open}
                            onclick={() => toggleMealSuggestion(meal, currentDate)}
                            >Suggest</button
                        >
                    {/if}
                </div>
                {#if ms?.open}
                    <InsightPanel
                        loading={ms.loading}
                        error={ms.error}
                        text={ms.text}
                        generatedAt={ms.generatedAt}
                        variant="suggestion"
                        onRegenerate={() => regenMealSuggestion(meal, currentDate)}
                    />
                {/if}
                {#if !collapsed}
                    {#each group as entry (entry.id)}
                        <EntryRow
                            {entry}
                            onUpdate={handleUpdate}
                            onDelete={handleDelete}
                            onFavorite={handleFavoriteEntry}
                            isFavorited={favoritedDescs.has(
                                normalizeFavoriteKey(entry.description),
                            )}
                        />
                    {/each}
                    <button
                        class="add-row"
                        onclick={() => {
                            drawerMeal = meal;
                            drawerDate = currentDate;
                            drawerField = null;
                            drawerTab = "food";
                            drawerOpen = true;
                        }}>+ add item</button
                    >
                {/if}
            </section>
        {/each}
        <ActivityNote
            date={currentDate}
            onOpen={openActivityDrawer}
            refreshKey={activityRefreshKey}
        />
    {:else if view === "favorites"}
        <FavoritesView onLoad={syncFavoritedDescs} />
    {:else if view === "profile"}
        <ProfilePanel onClose={closeProfile} />
    {:else}
        {#each weekGroupsData as week}
            <HistoryWeekBlock
                {week}
                dayAbbrev={DAY_ABBREV}
                insightState={insightsByWeek[week.weekStart] ?? null}
                suggestionState={suggestionsByWeek[week.weekStart] ?? null}
                onOpenDay={(date) => {
                    currentDate = date;
                    view = "day";
                }}
                onToggleInsights={() =>
                    toggleInsights(week.weekStart, week.weekEnd)}
                onToggleSuggestions={() =>
                    toggleWeekSuggestions(week.weekStart, week.weekEnd)}
                onRegenerateInsights={() =>
                    fetchInsights(week.weekStart, week.weekEnd, true)}
                onRegenerateSuggestions={() =>
                    fetchWeekSuggestions(week.weekStart, week.weekEnd, true)}
            />
        {/each}
    {/if}
</div>

<button
    class="fab"
    onclick={() => {
        drawerDate = currentDate;
        drawerMeal = null;
        drawerField = null;
        drawerTab = "food";
        drawerOpen = true;
    }}
    aria-label="Add food"
>
    <svg
        width="22"
        height="22"
        viewBox="0 0 24 24"
        fill="none"
        stroke="currentColor"
        stroke-width="2.5"
        stroke-linecap="round"
        ><line x1="12" y1="4" x2="12" y2="20" /><line
            x1="4"
            y1="12"
            x2="20"
            y2="12"
        /></svg
    >
</button>
<ChatDrawer
    open={drawerOpen}
    onClose={closeDrawer}
    {onEntriesAdded}
    {onEntriesEdited}
    date={drawerDate}
    meal={drawerMeal}
    initialTab={drawerTab}
    initialField={drawerField}
    editEntries={drawerEditEntries}
    editMealType={drawerEditMealType}
    yesterdayEntries={drawerHistoryMeal ? (yesterdayByMeal[drawerHistoryMeal] ?? []) : []}
    mealIsEmpty={drawerMeal ? (groupedByMeal(dayData?.entries)[drawerMeal] ?? []).length === 0 : true}
/>

<style>
    .wrap {
        max-width: 640px;
        margin: 0 auto;
        padding: 0 1.25rem 6rem;
    }

    header {
        position: sticky;
        top: 0;
        z-index: 10;
        background: var(--paper);
        padding: 1rem 0 0.75rem;
        border-bottom: 1px solid var(--rule);
        margin-bottom: 1.25rem;
    }

    .header-top {
        display: flex;
        justify-content: space-between;
        align-items: baseline;
        margin-bottom: 0.5rem;
    }

    .nav-left {
        display: flex;
        align-items: center;
        gap: 0.5rem;
        position: relative;
    }

    .hamburger {
        background: none;
        border: none;
        color: var(--ink);
        cursor: pointer;
        padding: 0.4rem;
        display: flex;
        align-items: center;
        touch-action: manipulation;
        border-radius: var(--r-sm);
    }

    @media (hover: hover) {
        .hamburger:hover {
            background: var(--paper-4);
        }
    }

    .view-label {
        font-size: 0.95rem;
        font-weight: 500;
        color: var(--ink);
    }

    .menu-backdrop {
        position: fixed;
        inset: 0;
        z-index: 9;
    }

    .nav-menu {
        position: absolute;
        top: 100%;
        left: 0;
        margin-top: 0.35rem;
        background: var(--paper);
        border: 1px solid var(--rule);
        border-radius: var(--r-md);
        box-shadow: 0 4px 16px rgba(0, 0, 0, 0.1);
        z-index: 10;
        min-width: 160px;
        padding: 0.35rem 0;
        display: flex;
        flex-direction: column;
    }

    .nav-menu button {
        background: none;
        border: none;
        text-align: left;
        padding: 0.55rem 1rem;
        font-size: var(--t-body-sm);
        font-family: inherit;
        color: var(--mute);
        cursor: pointer;
        font-weight: 500;
    }

    .nav-menu button.active {
        color: var(--ink);
    }

    .nav-menu button:hover {
        background: var(--paper-4);
        color: var(--ink);
    }

    .nav-menu hr {
        border: none;
        border-top: 1px solid var(--rule);
        margin: 0.25rem 0;
    }

    /* Week picker */
    .week-picker {
        display: flex;
        gap: 0.35rem;
        margin: 0.4rem 0 0.1rem;
    }

    .wp-btn {
        background: none;
        border: 1px solid var(--rule-3);
        border-radius: var(--r-pill);
        color: var(--mute);
        font-size: 0.72rem;
        padding: 0.2rem 0.6rem;
        cursor: pointer;
        font-family: inherit;
        letter-spacing: 0.02em;
        transition:
            border-color 0.12s,
            color 0.12s,
            background 0.12s;
        touch-action: manipulation;
    }

    .wp-btn.active {
        border-color: var(--ink-2);
        color: var(--ink-2);
        background: var(--paper-2);
    }

    @media (hover: hover) {
        .wp-btn:not(.active):hover {
            border-color: var(--mute-2);
            color: var(--ink-mute);
        }
    }

    /* Date navigator */
    .date-nav {
        display: flex;
        align-items: center;
        justify-content: space-between;
        margin: 0.4rem 0 0.1rem;
        position: relative;
    }

    .nav-arrow {
        background: none;
        border: none;
        font-size: 1.6rem;
        color: var(--ink-mute);
        cursor: pointer;
        padding: 0.1rem 0.4rem;
        line-height: 1;
        touch-action: manipulation;
        font-family: inherit;
        min-height: 2.5rem;
        display: flex;
        align-items: center;
    }

    .nav-arrow:disabled {
        color: var(--mute-4);
        cursor: default;
    }

    .nav-date {
        position: relative;
        background: none;
        border: none;
        font-family: inherit;
        font-size: 1rem;
        font-weight: 600;
        color: var(--ink);
        cursor: pointer;
        padding: 0.2rem 0.75rem;
        touch-action: manipulation;
        flex: 1;
        text-align: center;
        border-radius: var(--r-sm);
        transition: background 0.12s;
    }

    @media (hover: hover) {
        .nav-date:hover {
            background: var(--paper-4);
        }
    }

    .date-input-hidden {
        position: absolute;
        inset: 0;
        width: 100%;
        height: 100%;
        opacity: 0;
        pointer-events: none;
    }

    @media (pointer: coarse) {
        .date-input-hidden {
            pointer-events: auto;
            cursor: pointer;
        }
    }

    .totals {
        display: flex;
        flex-wrap: wrap;
        gap: 0.4rem 0.75rem;
        align-items: center;
        font-size: 0.78rem;
        color: var(--mute);
        padding-bottom: 0.1rem;
        padding-top: 0.3rem;
        font-variant-numeric: tabular-nums;
    }

    .macros {
        display: contents;
    }

    .macros .macro-cal {
        display: none;
    }

    .totals-toggle {
        display: none;
    }

    @media (max-width: 600px) {
        .totals {
            gap: 0.3rem 0.5rem;
            font-size: 0.72rem;
        }
        .totals-toggle {
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
            touch-action: manipulation;
            order: 1;
        }
        .totals-toggle.active {
            border-color: var(--ink-2);
            color: var(--ink-2);
            background: var(--paper-2);
        }
        .totals-arrow {
            color: var(--mute-3);
            font-size: 0.7rem;
        }
        .insights-btn {
            order: 2;
            margin-left: auto;
        }
        .macros {
            display: none;
            flex-basis: 100%;
            order: 3;
        }
        .macros.open {
            display: flex;
            flex-wrap: wrap;
            gap: 0.3rem 0.6rem;
        }
        .macros .macro-cal {
            display: inline;
        }
    }

    .meal-arrow {
        display: inline-block;
        width: 0.7rem;
        color: var(--mute-3);
        font-size: 0.7rem;
    }

    section {
        margin: 1.5rem 0;
    }

    .meal-name {
        background: none;
        border: none;
        font-family: inherit;
        text-transform: uppercase;
        font-size: 0.72rem;
        color: var(--mute);
        letter-spacing: 0.08em;
        font-weight: 600;
        cursor: pointer;
        display: inline-flex;
        align-items: center;
        gap: 0.3rem;
        padding: 0.3rem 0;
        touch-action: manipulation;
    }

    @media (hover: hover) {
        .meal-name:hover {
            color: var(--ink-2);
        }
    }

    .meal-header {
        display: flex;
        align-items: center;
        gap: 0.25rem;
        margin-bottom: 0.5rem;
    }

    .meal-action-btn {
        background: none;
        border: 1px solid var(--rule-3);
        border-radius: var(--r-pill);
        color: var(--mute);
        font-size: 0.68rem;
        padding: 0.15rem 0.55rem;
        cursor: pointer;
        touch-action: manipulation;
        font-family: inherit;
        letter-spacing: 0.02em;
        white-space: nowrap;
        font-weight: 500;
        transition: border-color 0.12s, color 0.12s;
    }

    .meal-action-btn.active {
        border-color: var(--ink-2);
        color: var(--ink-2);
    }

    @media (hover: hover) {
        .meal-action-btn:hover {
            border-color: var(--ink-2);
            color: var(--ink-2);
        }
    }

    @keyframes spin {
        from {
            transform: rotate(0deg);
        }
        to {
            transform: rotate(360deg);
        }
    }

    .add-row {
        background: none;
        border: none;
        font-family: inherit;
        text-align: left;
        color: var(--mute-4);
        font-size: var(--t-meta);
        padding: 0.6rem 0;
        cursor: pointer;
        touch-action: manipulation;
        width: 100%;
    }

    @media (hover: hover) {
        .add-row:hover {
            color: var(--mute);
        }
    }

    .state {
        color: var(--mute-2);
        text-align: center;
        margin-top: 4rem;
        font-size: var(--t-body-sm);
    }

    .state-block {
        display: flex;
        flex-direction: column;
        align-items: center;
        gap: 0.75rem;
        margin-top: 4rem;
    }

    .state.error {
        color: var(--mute);
        margin-top: 0;
    }

    .state-link {
        color: var(--ink-2);
        text-decoration: underline;
        text-underline-offset: 2px;
        font-size: var(--t-body-sm);
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

    .insights-btn.fresh {
        border-color: var(--accent, var(--ink-2));
        color: var(--accent, var(--ink-2));
    }

    .insights-btn.generating {
        background-image: linear-gradient(
            100deg,
            transparent 20%,
            color-mix(in srgb, var(--ink-2) 12%, transparent) 45%,
            color-mix(in srgb, var(--ink-2) 18%, transparent) 50%,
            color-mix(in srgb, var(--ink-2) 12%, transparent) 55%,
            transparent 80%
        );
        background-size: 220% 100%;
        background-repeat: no-repeat;
        animation: insights-shimmer 1.6s linear infinite;
    }

    @keyframes insights-shimmer {
        0% { background-position: 180% 0; }
        100% { background-position: -80% 0; }
    }

    @media (prefers-reduced-motion: reduce) {
        .insights-btn.generating {
            animation: none;
            background-image: none;
        }
    }

    .insight-dot {
        display: inline-block;
        width: 0.38rem;
        height: 0.38rem;
        background: var(--accent, var(--ink-2));
        border-radius: 50%;
        margin-left: 0.3rem;
        vertical-align: middle;
        animation: dot-pulse 1.8s ease-in-out 3;
    }

    @keyframes dot-pulse {
        0%, 100% { opacity: 1; }
        50% { opacity: 0.35; }
    }

    @media (hover: hover) {
        .insights-btn:hover {
            border-color: var(--ink-2);
            color: var(--ink-2);
        }
    }

    .day-insights-panel {
        margin-bottom: 1.25rem;
    }

    /* FAB + shared actions */
    .fab {
        position: fixed;
        bottom: calc(2rem + env(safe-area-inset-bottom, 0px));
        right: 2rem;
        width: 3.5rem;
        height: 3.5rem;
        border-radius: 50%;
        background: var(--ink-2);
        color: var(--paper);
        border: none;
        cursor: pointer;
        box-shadow: 0 2px 8px rgba(0, 0, 0, 0.18);
        display: flex;
        align-items: center;
        justify-content: center;
        touch-action: manipulation;
    }

    @media (hover: hover) {
        .fab:hover {
            background: var(--ink);
        }
    }

    .header-actions {
        display: flex;
        align-items: center;
        gap: 0.25rem;
    }

    .home-btn {
        display: flex;
        align-items: center;
        color: var(--mute);
        padding: 0.5rem 0.4rem;
        text-decoration: none;
        touch-action: manipulation;
        min-height: 2.75rem;
    }

    @media (hover: hover) {
        .home-btn:hover {
            color: var(--ink-2);
        }
    }

    .sheet-link {
        display: flex;
        align-items: center;
        color: var(--mute);
        padding: 0.5rem 0.4rem;
        text-decoration: none;
        touch-action: manipulation;
        min-height: 2.75rem;
    }

    @media (hover: hover) {
        .sheet-link:hover {
            color: var(--ink-2);
        }
    }

.header-actions :global(.theme-toggle) {
        font-size: 1rem;
        color: var(--mute);
        padding: 0.5rem 0.4rem;
        min-height: 2.75rem;
    }

    .signout-btn {
        display: flex;
        align-items: center;
        color: var(--mute);
        padding: 0.5rem 0.4rem;
        text-decoration: none;
        touch-action: manipulation;
        min-height: 2.75rem;
    }

    @media (hover: hover) {
        .signout-btn:hover {
            color: var(--ink-2);
        }
    }
</style>
