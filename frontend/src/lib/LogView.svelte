<script>
    import { createQuery, useQueryClient } from "@tanstack/svelte-query";
    import {
        getLog,
        confirmChat,
        addFavorite,
        getFavorites,
        generateInsights,
        generateDayInsights,
        fetchStoredInsight,
        fetchStoredDayInsight,
        generateDaySuggestions,
        fetchStoredDaySuggestions,
        generateWeekSuggestions,
        fetchStoredWeekSuggestions,
        patchEntry,
    } from "./api.js";
    import EntryRow from "./EntryRow.svelte";
    import ChatDrawer from "./ChatDrawer.svelte";
    import ActivityNote from "./ActivityNote.svelte";
    import ProfilePanel from "./ProfilePanel.svelte";
    import FavoritesView from "./FavoritesView.svelte";
    import { showError } from "./toast.js";
    import ThemeToggle from "./ThemeToggle.svelte";

    const queryClient = useQueryClient();

    const MEAL_ORDER = ["breakfast", "lunch", "snack", "dinner", "supplements"];
    const DAY_ABBREV = ["Su", "Mo", "Tu", "We", "Th", "Fr", "Sa"];
    const HISTORY_STATE_KEY = "foodiniNav";

    let view = $state("day");
    let currentDate = $state(todayStr());
    let profileOpen = $state(false);
    let drawerOpen = $state(false);
    let drawerTab = $state("food");
    let activityRefreshKey = $state(0);
    let drawerDate = $state(null);
    let drawerMeal = $state(null);
    let drawerField = $state(null);
    let repeating = $state(null);
    let repeatedMeals = $state(new Set());
    let repeatPicker = $state(null);
    let scalePickerMeal = $state(null);
    let scalingMeal = $state(null);
    let longPressTimer = null;
    let dateInputEl = $state(null);

    let insightsByWeek = $state({});
    let suggestionsByWeek = $state({});
    let dayInsight = $state(null);
    let daySuggestions = $state(null);
    let collapsedMeals = $state(new Set(MEAL_ORDER));
    let historyWeeks = $state(4);
    let historyReady = false;
    let skipHistorySync = false;
    let favoritedDescs = $state(new Set());

    // --- TanStack Queries ---

    const dayQuery = createQuery(() => ({
        queryKey: ["log", currentDate],
        queryFn: () => getLog({ date: currentDate }),
        enabled: view === "day",
    }));

    const yesterdayQuery = createQuery(() => ({
        queryKey: ["log", addDays(currentDate, -1)],
        queryFn: () => getLog({ date: addDays(currentDate, -1) }),
        enabled: view === "day",
    }));

    const historyQuery = createQuery(() => ({
        queryKey: ["log", "history", historyWeeks],
        queryFn: () => getLog({ days: historyWeeks * 7 }),
        enabled: view === "history",
    }));

    const favoritesQuery = createQuery(() => ({
        queryKey: ["favorites"],
        queryFn: getFavorites,
    }));

    // --- Derived state from queries (same variable names as before) ---

    let dayData = $derived(dayQuery.data ?? null);
    let historyData = $derived(historyQuery.data ?? null);
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
        const err = view === "day" ? dayQuery.error : view === "history" ? historyQuery.error : null;
        if (!err) return "";
        if (err?.status === 401 || err?.code === "session_expired")
            return "Your session expired. Sign in again.";
        if (err?.code === "insufficient_scopes")
            return "Google permissions are missing. Re-authorize to continue.";
        return view === "day"
            ? "Could not load this day. Try reloading, or sign in again."
            : "Could not load history. Try reloading, or sign in again.";
    });
    let loadErrorAction = $derived.by(() => {
        const err = view === "day" ? dayQuery.error : view === "history" ? historyQuery.error : null;
        if (!err) return null;
        if (err?.status === 401 || err?.code === "session_expired")
            return { href: "/auth/login", label: "Sign in" };
        if (err?.code === "insufficient_scopes")
            return { href: "/auth/login?consent=1", label: "Re-authorize" };
        return null;
    });

    let yesterdayByMeal = $derived.by(() => {
        const entries = yesterdayQuery.data?.entries ?? [];
        const g = {};
        for (const e of entries) {
            (g[e.meal_type] ??= []).push(e);
        }
        for (const meal of repeatedMeals) {
            g[meal] = [];
        }
        return g;
    });

    let weekGroupsData = $derived(weekGroups(historyData, historyWeeks));

    let isToday = $derived(currentDate === todayStr());
    let hasAnyEntry = $derived((dayData?.entries?.length ?? 0) > 0);
    let showDayInsights = $derived(!isToday || hasAnyEntry);
    let showDaySuggestions = $derived(isToday);

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
            collapsedMeals = new Set(MEAL_ORDER);
            repeatedMeals = new Set();
            dayInsight = null;
            daySuggestions = null;
        }
    });

    function todayStr() {
        const d = new Date();
        return [
            d.getFullYear(),
            String(d.getMonth() + 1).padStart(2, "0"),
            String(d.getDate()).padStart(2, "0"),
        ].join("-");
    }

    function openDatePicker() {
        if (!dateInputEl) return;
        if (typeof dateInputEl.showPicker === "function") {
            try {
                dateInputEl.showPicker();
            } catch {}
        }
    }

    function addDays(dateStr, n) {
        const d = new Date(dateStr + "T12:00:00");
        d.setDate(d.getDate() + n);
        return d.toISOString().slice(0, 10);
    }

    function formatDateNav(dateStr) {
        const today = todayStr();
        if (dateStr === today) return "Today";
        if (dateStr === addDays(today, -1)) return "Yesterday";
        const d = new Date(dateStr + "T12:00:00");
        return d.toLocaleDateString("en-US", {
            weekday: "short",
            month: "short",
            day: "numeric",
        });
    }

    function getMonday(dateStr) {
        const d = new Date(dateStr + "T12:00:00");
        const day = d.getDay();
        const diff = day === 0 ? -6 : 1 - day;
        d.setDate(d.getDate() + diff);
        return d.toISOString().slice(0, 10);
    }

    function formatWeekRange(start, end) {
        const s = new Date(start + "T12:00:00");
        const e = new Date(end + "T12:00:00");
        const sm = s.toLocaleDateString("en-US", { month: "short" });
        const em = e.toLocaleDateString("en-US", { month: "short" });
        if (sm === em) return `${sm} ${s.getDate()}–${e.getDate()}`;
        return `${sm} ${s.getDate()} – ${em} ${e.getDate()}`;
    }

    function snapshotNavState() {
        return {
            view,
            currentDate,
            historyWeeks,
            profileOpen,
            drawerOpen,
            drawerTab,
            drawerDate,
            drawerMeal,
            drawerField,
        };
    }

    function normalizeNavState(state) {
        const today = todayStr();
        const nextView = state?.view === "history" ? "history" : state?.view === "favorites" ? "favorites" : "day";
        const nextDate =
            typeof state?.currentDate === "string" && state.currentDate
                ? state.currentDate
                : today;
        const nextHistoryWeeks = [4, 8, 12, 26].includes(state?.historyWeeks)
            ? state.historyWeeks
            : 4;
        const nextProfileOpen = state?.profileOpen === true;
        const nextDrawerOpen = state?.drawerOpen === true;
        const nextDrawerTab =
            state?.drawerTab === "activity" ? "activity" : "food";

        return {
            view: nextView,
            currentDate: nextDate > today ? today : nextDate,
            historyWeeks: nextHistoryWeeks,
            profileOpen: nextProfileOpen,
            drawerOpen: nextDrawerOpen,
            drawerTab: nextDrawerTab,
            drawerDate: nextDrawerOpen ? state?.drawerDate || nextDate : null,
            drawerMeal: nextDrawerOpen ? (state?.drawerMeal ?? null) : null,
            drawerField: nextDrawerOpen ? (state?.drawerField ?? null) : null,
        };
    }

    function navStateEqual(a, b) {
        return (
            a?.view === b?.view &&
            a?.currentDate === b?.currentDate &&
            a?.historyWeeks === b?.historyWeeks &&
            a?.profileOpen === b?.profileOpen &&
            a?.drawerOpen === b?.drawerOpen &&
            a?.drawerTab === b?.drawerTab &&
            a?.drawerDate === b?.drawerDate &&
            a?.drawerMeal === b?.drawerMeal &&
            a?.drawerField === b?.drawerField
        );
    }

    function shouldPushHistory(prev, next) {
        if (!prev) return false;
        if (!prev.profileOpen && next.profileOpen) return true;
        if (!prev.drawerOpen && next.drawerOpen) return true;
        if (prev.view !== next.view) return true;
        if (
            prev.currentDate !== next.currentDate &&
            prev.view === "day" &&
            next.view === "day"
        )
            return true;
        return false;
    }

    function currentHistoryNavState() {
        return normalizeNavState(window.history.state?.[HISTORY_STATE_KEY]);
    }

    function applyNavState(state) {
        const next = normalizeNavState(state);
        view = next.view;
        currentDate = next.currentDate;
        historyWeeks = next.historyWeeks;
        profileOpen = next.profileOpen;
        drawerOpen = next.drawerOpen;
        drawerTab = next.drawerTab;
        drawerDate = next.drawerDate;
        drawerMeal = next.drawerMeal;
        drawerField = next.drawerField;
    }

    function pushOrReplaceHistory(state, mode = "replace") {
        const next = normalizeNavState(state);
        const payload = { [HISTORY_STATE_KEY]: next };
        if (mode === "push") {
            window.history.pushState(payload, "");
        } else {
            window.history.replaceState(payload, "");
        }
    }

    function groupedByMeal(entries) {
        const g = {};
        for (const e of entries ?? []) {
            (g[e.meal_type] ??= []).push(e);
        }
        return g;
    }

    function totals(entries) {
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

    function weekGroups(data, numWeeks = 8) {
        if (!data) return [];
        const { entries = [], daily_logs = [] } = data;
        const byDate = {};
        for (const e of entries) {
            (byDate[e.date] ??= { entries: [], dayLog: null }).entries.push(e);
        }
        for (const l of daily_logs) {
            (byDate[l.date] ??= { entries: [], dayLog: null }).dayLog = l;
        }

        const today = todayStr();
        let monday = getMonday(addDays(today, -(numWeeks * 7 - 1)));
        const todayMonday = getMonday(today);
        const weeks = [];
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

    async function repeatMeal(targetMeal, sourceMeal = targetMeal) {
        if (repeating !== null) return;
        repeating = targetMeal;
        repeatPicker = null;
        try {
            const entries = yesterdayByMeal[sourceMeal].map((e) => ({
                ...e,
                meal_type: targetMeal,
            }));
            const res = await confirmChat(entries, currentDate ?? todayStr());
            queryClient.setQueryData(["log", currentDate], (old) => ({
                ...old,
                entries: [...(old?.entries ?? []), ...res.entries],
            }));
            queryClient.invalidateQueries({ queryKey: ["log"] });
            repeatedMeals = new Set([...repeatedMeals, targetMeal]);
            collapsedMeals = new Set(
                [...collapsedMeals].filter((m) => m !== targetMeal),
            );
        } catch (err) {
            showError(err, "Failed to repeat meal.");
        } finally {
            repeating = null;
        }
    }

    function startLongPress(meal) {
        longPressTimer = setTimeout(() => {
            longPressTimer = null;
            repeatPicker = meal;
        }, 500);
    }

    function endLongPress(meal) {
        if (longPressTimer !== null) {
            clearTimeout(longPressTimer);
            longPressTimer = null;
            repeatMeal(meal);
        }
    }

    function cancelPress() {
        if (longPressTimer !== null) {
            clearTimeout(longPressTimer);
            longPressTimer = null;
        }
    }

    function handleUpdate(updated) {
        queryClient.setQueryData(["log", currentDate], (old) => ({
            ...old,
            entries: (old?.entries ?? []).map((e) =>
                e.id === updated.id ? updated : e,
            ),
        }));
    }

    async function scaleMeal(meal, group, factor) {
        if (scalingMeal) return;
        scalingMeal = meal;
        scalePickerMeal = null;
        try {
            const r1 = (v) => Math.round(v * factor);
            const r10 = (v) => Math.round(v * factor * 10) / 10;
            const updates = group.map((e) => ({
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
            for (const s of saved) handleUpdate(s);
        } catch (err) {
            showError(err, "Failed to scale meal.");
        } finally {
            scalingMeal = null;
        }
    }

    function handleDelete(id) {
        queryClient.setQueryData(["log", currentDate], (old) => ({
            ...old,
            entries: (old?.entries ?? []).filter((e) => e.id !== id),
        }));
        queryClient.invalidateQueries({ queryKey: ["log"] });
    }

    function normalizeFavoriteKey(desc) {
        return (desc ?? "").toLowerCase().trim().replace(/\s+/g, " ");
    }

    async function handleFavoriteEntry(entry) {
        const key = normalizeFavoriteKey(entry.description);
        if (favoritedDescs.has(key)) return;
        try {
            await addFavorite(entry);
            favoritedDescs = new Set([...favoritedDescs, key]);
            queryClient.invalidateQueries({ queryKey: ["favorites"] });
        } catch (e) {
            if (e?.status === 409 || e?.code === "favorite_exists") {
                favoritedDescs = new Set([...favoritedDescs, key]);
                return;
            }
            console.error("addFavorite failed:", e);
            showError(e, "Failed to add to favorites.");
        }
    }

    function syncFavoritedDescs(favorites) {
        favoritedDescs = new Set(
            favorites.map((f) => normalizeFavoriteKey(f.description)),
        );
    }

    function openActivityDrawer(field = null) {
        drawerField = field;
        drawerTab = "activity";
        drawerDate = currentDate;
        drawerMeal = null;
        drawerOpen = true;
    }

    function closeDrawer() {
        if (drawerTab === "activity") activityRefreshKey++;
        const state = currentHistoryNavState();
        if (historyReady && state?.drawerOpen) {
            window.history.back();
            return;
        }
        drawerOpen = false;
        drawerDate = null;
        drawerMeal = null;
        drawerTab = "food";
        drawerField = null;
    }

    function closeProfile() {
        const state = currentHistoryNavState();
        if (historyReady && state?.profileOpen) {
            window.history.back();
            return;
        }
        profileOpen = false;
    }

    function onEntriesAdded(newEntries) {
        queryClient.setQueryData(["log", currentDate], (old) => ({
            ...old,
            entries: [...(old?.entries ?? []), ...newEntries],
        }));
        queryClient.invalidateQueries({ queryKey: ["log"] });
        drawerOpen = false;
        if (newEntries.length > 0) {
            const addedMeal = newEntries[0].meal_type;
            collapsedMeals = new Set(
                [...collapsedMeals].filter((m) => m !== addedMeal),
            );
        }
    }

    // --- Insights & suggestions (kept imperative — complex toggle/regenerate UI) ---

    async function fetchInsights(weekStart, weekEnd, regenerate = false) {
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
                            generatedAt: stored.generated_at,
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
                    text: res.insight,
                    error: null,
                    generatedAt: res.generated_at,
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

    function escapeHtml(str) {
        return str
            .replace(/&/g, "&amp;")
            .replace(/</g, "&lt;")
            .replace(/>/g, "&gt;")
            .replace(/"/g, "&quot;");
    }

    function renderInsight(text) {
        return text
            .split("\n")
            .map((line) => line.trim())
            .filter((line) => line.length > 0)
            .map((line) =>
                escapeHtml(line).replace(
                    /\*\*(.+?)\*\*/g,
                    "<strong>$1</strong>",
                ),
            )
            .join("\n");
    }

    function formatGeneratedAt(isoStr) {
        if (!isoStr) return "";
        const d = new Date(isoStr);
        return d.toLocaleString("en-US", {
            month: "short",
            day: "numeric",
            hour: "numeric",
            minute: "2-digit",
        });
    }

    function toggleInsights(weekStart, weekEnd) {
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

    function parseDayInsight(text) {
        const lines = text
            .split("\n")
            .map((l) => l.trim())
            .filter((l) => l.length > 0);
        const summary = lines.find((l) => !l.startsWith("•")) || lines[0] || "";
        const detail = lines.filter((l) => l.startsWith("•"));
        return { summary, detail: detail.join("\n") };
    }

    async function fetchDayInsights(date, regenerate = false) {
        dayInsight = {
            loading: true,
            text: null,
            error: null,
            open: true,
            generatedAt: null,
            detailOpen: false,
        };
        try {
            if (!regenerate) {
                const stored = await fetchStoredDayInsight(date);
                if (stored.insight) {
                    dayInsight = {
                        loading: false,
                        text: stored.insight,
                        error: null,
                        open: true,
                        generatedAt: stored.generated_at,
                        detailOpen: false,
                    };
                    return;
                }
            }
            const res = await generateDayInsights(date);
            dayInsight = {
                loading: false,
                text: res.insight,
                error: null,
                open: true,
                generatedAt: res.generated_at,
                detailOpen: false,
            };
        } catch (e) {
            dayInsight = {
                loading: false,
                text: null,
                error: e.message || "Could not load insights",
                open: true,
                generatedAt: null,
                detailOpen: false,
            };
        }
    }

    function toggleDayInsights() {
        if (
            !dayInsight ||
            (!dayInsight.loading && !dayInsight.text && !dayInsight.error)
        ) {
            fetchDayInsights(currentDate, false);
        } else {
            dayInsight = { ...dayInsight, open: !dayInsight.open };
        }
    }

    async function fetchDaySuggestions(date, regenerate = false) {
        daySuggestions = {
            loading: true,
            text: null,
            error: null,
            open: true,
            generatedAt: null,
            type: null,
        };
        try {
            if (!regenerate) {
                const stored = await fetchStoredDaySuggestions(date);
                if (stored.suggestions) {
                    daySuggestions = {
                        loading: false,
                        text: stored.suggestions,
                        error: null,
                        open: true,
                        generatedAt: stored.generated_at,
                        type: stored.type,
                    };
                    return;
                }
            }
            const res = await generateDaySuggestions(date);
            daySuggestions = {
                loading: false,
                text: res.suggestions,
                error: null,
                open: true,
                generatedAt: res.generated_at,
                type: res.type,
            };
        } catch (e) {
            daySuggestions = {
                loading: false,
                text: null,
                error: e.message || "Could not load suggestions",
                open: true,
                generatedAt: null,
                type: null,
            };
        }
    }

    function toggleDaySuggestions() {
        if (
            !daySuggestions ||
            (!daySuggestions.loading &&
                !daySuggestions.text &&
                !daySuggestions.error)
        ) {
            fetchDaySuggestions(currentDate, false);
        } else {
            daySuggestions = { ...daySuggestions, open: !daySuggestions.open };
        }
    }

    async function fetchWeekSuggestions(
        weekStart,
        weekEnd,
        regenerate = false,
    ) {
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
                            generatedAt: stored.generated_at,
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
                    text: res.suggestions,
                    error: null,
                    generatedAt: res.generated_at,
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

    function toggleWeekSuggestions(weekStart, weekEnd) {
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
            <div class="toggle">
                <button
                    class:active={view === "day"}
                    onclick={() => {
                        view = "day";
                    }}>Day</button
                >
                <button
                    class:active={view === "history"}
                    onclick={() => (view = "history")}>History</button
                >
                <button
                    class:active={view === "favorites"}
                    onclick={() => (view = "favorites")}>Favorites</button
                >
            </div>
            <div class="header-actions">
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
                <ThemeToggle />
                <button
                    class="settings-btn"
                    onclick={() => (profileOpen = true)}
                    aria-label="Profile settings"
                    title="Profile settings">⚙</button
                >
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
                        onchange={(e) => {
                            if (e.target.value) currentDate = e.target.value;
                        }}
                        bind:this={dateInputEl}
                        max={todayStr()}
                    />
                </button>
                <button
                    class="nav-arrow"
                    class:dimmed={currentDate >= todayStr()}
                    disabled={currentDate >= todayStr()}
                    onclick={() => (currentDate = addDays(currentDate, 1))}
                    aria-label="Next day">›</button
                >
            </div>
            {#if dayData?.entries}
                {@const t = totals(dayData.entries)}
                <div class="totals">
                    <span>{t.calories} cal</span>
                    <span>{t.protein}g P</span>
                    <span>{t.carbs}g C</span>
                    <span>{t.fat}g F</span>
                    <span>{t.fiber}g Fb</span>
                    <div class="totals-btns">
                        {#if showDayInsights}
                            <button
                                class="insights-btn"
                                class:active={dayInsight?.open}
                                onclick={toggleDayInsights}
                                aria-label="AI insights"
                                title="AI insights">insights</button
                            >
                        {/if}
                        {#if showDaySuggestions}
                            <button
                                class="insights-btn suggestions-btn"
                                class:active={daySuggestions?.open}
                                onclick={toggleDaySuggestions}
                                aria-label="Meal suggestions"
                                title="Meal suggestions">suggestions</button
                            >
                        {/if}
                    </div>
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
            {@const parsed = dayInsight.text
                ? parseDayInsight(dayInsight.text)
                : null}
            <div class="insights-panel day-insights-panel">
                <button
                    class="insight-close"
                    onclick={() =>
                        (dayInsight = { ...dayInsight, open: false })}
                    aria-label="Close insights">✕</button
                >
                {#if dayInsight.loading}
                    <div class="insight-skeleton">
                        <div class="isk-line" style="width: 88%"></div>
                        <div class="isk-line" style="width: 72%"></div>
                        <div class="isk-line" style="width: 80%"></div>
                    </div>
                {:else if dayInsight.error}
                    <span class="insights-err">{dayInsight.error}</span>
                {:else if parsed}
                    <!-- eslint-disable-next-line svelte/no-at-html-tags -->
                    <p class="insights-text insight-summary">
                        {@html renderInsight(parsed.summary)}
                    </p>
                    {#if parsed.detail}
                        <button
                            class="detail-toggle"
                            onclick={() =>
                                (dayInsight = {
                                    ...dayInsight,
                                    detailOpen: !dayInsight.detailOpen,
                                })}
                        >
                            {dayInsight.detailOpen ? "▾ less" : "▸ more"}
                        </button>
                        {#if dayInsight.detailOpen}
                            <!-- eslint-disable-next-line svelte/no-at-html-tags -->
                            <p class="insights-text insight-detail">
                                {@html renderInsight(parsed.detail)}
                            </p>
                        {/if}
                    {/if}
                    <div class="insight-footer">
                        {#if dayInsight.generatedAt}<span class="insight-ts"
                                >{formatGeneratedAt(
                                    dayInsight.generatedAt,
                                )}</span
                            >{/if}
                        <button
                            class="insight-regen"
                            onclick={() => fetchDayInsights(currentDate, true)}
                            >regenerate</button
                        >
                    </div>
                {/if}
            </div>
        {/if}
        {#if daySuggestions?.open}
            <div class="insights-panel day-insights-panel suggestions-panel">
                <button
                    class="insight-close"
                    onclick={() =>
                        (daySuggestions = { ...daySuggestions, open: false })}
                    aria-label="Close suggestions">✕</button
                >
                {#if daySuggestions.loading}
                    <div class="insight-skeleton">
                        <div class="isk-line" style="width: 88%"></div>
                        <div class="isk-line" style="width: 72%"></div>
                        <div class="isk-line" style="width: 80%"></div>
                    </div>
                {:else if daySuggestions.error}
                    <span class="insights-err">{daySuggestions.error}</span>
                {:else if daySuggestions.text}
                    <span class="suggestions-label"
                        >{daySuggestions.type === "next-day"
                            ? "Tomorrow"
                            : "Remaining meals"}</span
                    >
                    <!-- eslint-disable-next-line svelte/no-at-html-tags -->
                    <p class="insights-text">
                        {@html renderInsight(daySuggestions.text)}
                    </p>
                    <div class="insight-footer">
                        {#if daySuggestions.generatedAt}<span class="insight-ts"
                                >{formatGeneratedAt(
                                    daySuggestions.generatedAt,
                                )}</span
                            >{/if}
                        <button
                            class="insight-regen"
                            onclick={() =>
                                fetchDaySuggestions(currentDate, true)}
                            >regenerate</button
                        >
                    </div>
                {/if}
            </div>
        {/if}
        {#each MEAL_ORDER as meal}
            {@const group = groupedByMeal(dayData?.entries)[meal] ?? []}
            {@const collapsed = collapsedMeals.has(meal)}
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
                        {#if group.length > 0}<span class="meal-summary"
                                >· {group.reduce((s, e) => s + e.calories, 0)} cal</span
                            >{/if}
                    </button>
                    {#if group.length > 0}
                        {#if scalePickerMeal === meal}
                            <div class="scale-picker">
                                <button
                                    class="scale-opt"
                                    onclick={() => scaleMeal(meal, group, 1.5)}
                                    disabled={scalingMeal !== null}
                                    >×1.5</button
                                >
                                <button
                                    class="scale-opt"
                                    onclick={() => scaleMeal(meal, group, 2)}
                                    disabled={scalingMeal !== null}
                                    >×2</button
                                >
                                <button
                                    class="pick-cancel"
                                    onclick={() => (scalePickerMeal = null)}
                                    aria-label="Cancel scale">✕</button
                                >
                            </div>
                        {:else}
                            <button
                                class="scale-meal-btn"
                                class:spinning={scalingMeal === meal}
                                onclick={() => (scalePickerMeal = meal)}
                                disabled={scalingMeal !== null}
                                aria-label="Scale {meal} portion"
                                title="Scale {meal} portion">⊕</button
                            >
                        {/if}
                    {/if}
                    {#if yesterdayByMeal[meal]?.length && !group.length}
                        {#if repeatPicker === meal}
                            <div class="repeat-picker">
                                {#each MEAL_ORDER.filter((m) => yesterdayByMeal[m]?.length) as src}
                                    <button
                                        class="pick-btn"
                                        onclick={() => repeatMeal(meal, src)}
                                        >{src}</button
                                    >
                                {/each}
                                <button
                                    class="pick-cancel"
                                    onclick={() => (repeatPicker = null)}
                                    >✕</button
                                >
                            </div>
                        {:else}
                            <button
                                class="repeat-btn"
                                class:spinning={repeating === meal}
                                onpointerdown={() => startLongPress(meal)}
                                onpointerup={() => endLongPress(meal)}
                                onpointercancel={cancelPress}
                                oncontextmenu={(e) => e.preventDefault()}
                                disabled={repeating !== null}
                                aria-label="Repeat yesterday's {meal}"
                                title="Repeat yesterday's {meal} — hold for options"
                                >↻</button
                            >
                        {/if}
                    {/if}
                </div>
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
    {:else}
        {#each weekGroupsData as week}
            <div class="week-block">
                <div class="week-head">
                    <div class="week-meta">
                        <span class="week-range"
                            >{formatWeekRange(
                                week.weekStart,
                                week.weekEnd,
                            )}</span
                        >
                        {#if week.weekTotal > 0}
                            <span class="week-cal"
                                >{week.weekTotal.toLocaleString()} cal</span
                            >
                        {/if}
                    </div>
                    {#if week.weekTotal > 0 || week.days.some((d) => d.dayLog)}
                        <div class="week-btns">
                            <button
                                class="insights-btn"
                                class:active={insightsByWeek[week.weekStart]
                                    ?.open}
                                onclick={() =>
                                    toggleInsights(
                                        week.weekStart,
                                        week.weekEnd,
                                    )}
                                aria-label="AI insights for this week"
                                title="AI insights">insights</button
                            >
                            <button
                                class="insights-btn suggestions-btn"
                                class:active={suggestionsByWeek[week.weekStart]
                                    ?.open}
                                onclick={() =>
                                    toggleWeekSuggestions(
                                        week.weekStart,
                                        week.weekEnd,
                                    )}
                                aria-label="Meal suggestions for this week"
                                title="Meal suggestions">suggestions</button
                            >
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
                                if (!day.future) {
                                    currentDate = day.date;
                                    view = "day";
                                }
                            }}
                            disabled={day.future}
                            aria-label={day.date}
                        >
                            <span class="dc-abbrev"
                                >{DAY_ABBREV[
                                    new Date(day.date + "T12:00:00").getDay()
                                ]}</span
                            >
                            <span class="dc-num"
                                >{new Date(
                                    day.date + "T12:00:00",
                                ).getDate()}</span
                            >
                            <span class="dc-indicators">
                                {#if day.entries.length > 0}<span
                                        class="dc-food">●</span
                                    >{:else}<span class="dc-empty">○</span>{/if}
                                {#if day.dayLog?.poop}<span class="dc-poop"
                                        >💩</span
                                    >{/if}
                            </span>
                        </button>
                    {/each}
                </div>
                {#if insightsByWeek[week.weekStart]?.open}
                    {@const wi = insightsByWeek[week.weekStart]}
                    <div class="insights-panel">
                        {#if wi.loading}
                            <div class="insight-skeleton">
                                <div class="isk-line" style="width: 88%"></div>
                                <div class="isk-line" style="width: 72%"></div>
                                <div class="isk-line" style="width: 80%"></div>
                            </div>
                        {:else if wi.error}
                            <span class="insights-err">{wi.error}</span>
                        {:else if wi.text}
                            <!-- eslint-disable-next-line svelte/no-at-html-tags -->
                            <p class="insights-text">
                                {@html renderInsight(wi.text)}
                            </p>
                            <div class="insight-footer">
                                {#if wi.generatedAt}<span class="insight-ts"
                                        >{formatGeneratedAt(
                                            wi.generatedAt,
                                        )}</span
                                    >{/if}
                                <button
                                    class="insight-regen"
                                    onclick={() =>
                                        fetchInsights(
                                            week.weekStart,
                                            week.weekEnd,
                                            true,
                                        )}>regenerate</button
                                >
                            </div>
                        {/if}
                    </div>
                {/if}
                {#if suggestionsByWeek[week.weekStart]?.open}
                    {@const ws = suggestionsByWeek[week.weekStart]}
                    <div class="insights-panel suggestions-panel">
                        {#if ws.loading}
                            <div class="insight-skeleton">
                                <div class="isk-line" style="width: 88%"></div>
                                <div class="isk-line" style="width: 72%"></div>
                                <div class="isk-line" style="width: 80%"></div>
                            </div>
                        {:else if ws.error}
                            <span class="insights-err">{ws.error}</span>
                        {:else if ws.text}
                            <span class="suggestions-label"
                                >Meal ideas for next week</span
                            >
                            <!-- eslint-disable-next-line svelte/no-at-html-tags -->
                            <p class="insights-text">
                                {@html renderInsight(ws.text)}
                            </p>
                            <div class="insight-footer">
                                {#if ws.generatedAt}<span class="insight-ts"
                                        >{formatGeneratedAt(
                                            ws.generatedAt,
                                        )}</span
                                    >{/if}
                                <button
                                    class="insight-regen"
                                    onclick={() =>
                                        fetchWeekSuggestions(
                                            week.weekStart,
                                            week.weekEnd,
                                            true,
                                        )}>regenerate</button
                                >
                            </div>
                        {/if}
                    </div>
                {/if}
            </div>
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
    date={drawerDate}
    meal={drawerMeal}
    initialTab={drawerTab}
    initialField={drawerField}
/>
{#if profileOpen}
    <ProfilePanel onClose={closeProfile} />
{/if}

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

    .toggle {
        display: flex;
        gap: 1.25rem;
    }

    .toggle button {
        background: none;
        border: none;
        border-bottom: 2px solid transparent;
        padding: 0 0 0.2rem;
        font-size: 0.95rem;
        font-weight: 500;
        color: var(--mute);
        cursor: pointer;
        font-family: inherit;
    }

    .toggle button.active {
        color: var(--ink);
        border-bottom-color: var(--ink-2);
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

    .nav-arrow.dimmed,
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
        gap: 0.4rem 1rem;
        align-items: center;
        font-size: 0.78rem;
        color: var(--mute);
        padding-bottom: 0.1rem;
        padding-top: 0.3rem;
        font-variant-numeric: tabular-nums;
    }

    .totals-btns {
        display: flex;
        gap: 0.35rem;
        margin-left: auto;
    }

    @media (max-width: 480px) {
        .totals-btns {
            margin-left: 0;
        }
    }

    .meal-arrow {
        display: inline-block;
        width: 0.7rem;
        color: var(--mute-3);
        font-size: 0.7rem;
    }

    .week-btns {
        display: flex;
        gap: 0.35rem;
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

    .meal-summary {
        font-weight: 400;
        color: var(--mute-3);
        letter-spacing: 0;
        text-transform: none;
        font-size: 0.72rem;
    }

    .meal-header {
        display: flex;
        align-items: center;
        gap: 0.25rem;
        margin-bottom: 0.5rem;
    }

    .repeat-btn {
        background: none;
        border: none;
        color: var(--mute-4);
        font-size: 1rem;
        line-height: 1;
        cursor: pointer;
        padding: 0.2rem 0.3rem;
        touch-action: manipulation;
        display: flex;
        align-items: center;
    }

    @media (hover: hover) {
        .repeat-btn:hover:not(:disabled) {
            color: var(--ink-mute);
        }
    }

    .repeat-btn:disabled {
        cursor: default;
    }

    .repeat-picker {
        display: flex;
        align-items: center;
        gap: 0.25rem;
    }

    .pick-btn {
        background: none;
        border: 1px solid var(--rule-4);
        border-radius: var(--r-pill);
        padding: 0.15rem 0.55rem;
        font-size: 0.7rem;
        color: var(--ink-mute);
        cursor: pointer;
        font-family: inherit;
        font-weight: 500;
        text-transform: uppercase;
        letter-spacing: 0.04em;
        white-space: nowrap;
        touch-action: manipulation;
    }

    @media (hover: hover) {
        .pick-btn:hover {
            border-color: var(--ink-2);
            color: var(--ink-2);
        }
    }

    .pick-cancel {
        background: none;
        border: none;
        color: var(--mute-4);
        font-size: 0.75rem;
        cursor: pointer;
        padding: 0.15rem 0.2rem;
        line-height: 1;
        font-family: inherit;
        touch-action: manipulation;
    }

    @media (hover: hover) {
        .pick-cancel:hover {
            color: var(--mute);
        }
    }

    .scale-meal-btn {
        background: none;
        border: none;
        color: var(--mute-4);
        font-size: 1rem;
        line-height: 1;
        cursor: pointer;
        padding: 0.2rem 0.3rem;
        touch-action: manipulation;
        display: flex;
        align-items: center;
    }

    @media (hover: hover) {
        .scale-meal-btn:hover:not(:disabled) {
            color: var(--ink-mute);
        }
    }

    .scale-meal-btn:disabled {
        cursor: default;
        opacity: 0.4;
    }

    .scale-meal-btn.spinning {
        animation: spin 0.7s linear infinite;
        color: var(--mute);
    }

    .scale-picker {
        display: flex;
        align-items: center;
        gap: 0.25rem;
    }

    .scale-picker .scale-opt {
        background: none;
        border: 1px solid var(--rule-4);
        border-radius: var(--r-pill);
        padding: 0.15rem 0.55rem;
        font-size: 0.7rem;
        color: var(--ink-mute);
        cursor: pointer;
        font-family: inherit;
        font-weight: 600;
        letter-spacing: 0.04em;
        white-space: nowrap;
        touch-action: manipulation;
    }

    @media (hover: hover) {
        .scale-picker .scale-opt:hover:not(:disabled) {
            border-color: var(--ink-2);
            color: var(--ink-2);
        }
    }

    .scale-picker .scale-opt:disabled {
        opacity: 0.4;
        cursor: default;
    }

    .repeat-btn.spinning {
        animation: spin 0.7s linear infinite;
        color: var(--mute);
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

    /* Weekly history — hairline ledger, not a box */
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

    .insights-panel {
        position: relative;
        padding: 0.9rem 2.2rem 0.95rem 0.95rem;
        background: var(--paper-2);
        border-radius: var(--r-sm);
        margin-top: 0.5rem;
    }

    .day-insights-panel {
        margin-bottom: 1.25rem;
        min-width: 0;
    }

    .insight-close {
        position: absolute;
        top: 0.55rem;
        right: 0.55rem;
        width: 1.5rem;
        height: 1.5rem;
        background: none;
        border: none;
        border-radius: 50%;
        font-size: 0.75rem;
        color: var(--mute-3);
        cursor: pointer;
        padding: 0;
        line-height: 1;
        display: flex;
        align-items: center;
        justify-content: center;
    }

    @media (hover: hover) {
        .insight-close:hover {
            color: var(--ink-mute);
            background: var(--paper-4);
        }
    }

    @keyframes shimmer {
        0% {
            background-position: -200% 0;
        }
        100% {
            background-position: 200% 0;
        }
    }

    .insight-skeleton {
        display: flex;
        flex-direction: column;
        gap: 0.6rem;
    }

    .isk-line {
        height: 0.78rem;
        border-radius: 4px;
        background: linear-gradient(
            90deg,
            var(--rule) 25%,
            var(--paper-3) 50%,
            var(--rule) 75%
        );
        background-size: 200% 100%;
        animation: shimmer 1.4s ease-in-out infinite;
    }

    .insights-err {
        font-size: var(--t-meta);
        color: var(--danger);
    }

    .insights-text {
        font-size: var(--t-meta);
        color: var(--ink);
        line-height: 1.65;
        white-space: pre-line;
        overflow-wrap: break-word;
        word-break: break-word;
        margin: 0;
    }

    .insights-text :global(strong) {
        font-weight: 600;
        color: var(--ink);
    }

    .insight-footer {
        display: flex;
        align-items: center;
        gap: 0.75rem;
        margin-top: 0.7rem;
    }

    .insight-ts {
        font-size: 0.72rem;
        color: var(--mute-3);
    }

    .insight-regen {
        background: none;
        border: none;
        font-family: inherit;
        font-size: 0.72rem;
        color: var(--mute-2);
        cursor: pointer;
        padding: 0;
        touch-action: manipulation;
        margin-left: auto;
    }

    @media (hover: hover) {
        .insight-regen:hover {
            color: var(--ink-mute);
        }
    }

    .detail-toggle {
        background: none;
        border: none;
        font-family: inherit;
        font-size: 0.72rem;
        color: var(--mute-2);
        cursor: pointer;
        padding: 0.25rem 0 0.15rem;
        touch-action: manipulation;
    }

    @media (hover: hover) {
        .detail-toggle:hover {
            color: var(--ink-mute);
        }
    }

    .insight-summary {
        font-weight: 500;
    }

    .insight-detail {
        margin-top: 0.4rem;
    }

    .suggestions-panel {
        background: var(--sugg-paper);
    }

    .suggestions-label {
        display: block;
        font-size: 0.7rem;
        font-weight: 600;
        text-transform: uppercase;
        letter-spacing: 0.06em;
        color: var(--sugg-mute);
        margin-bottom: 0.4rem;
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

    .settings-btn {
        background: none;
        border: none;
        font-size: 1.1rem;
        color: var(--mute);
        cursor: pointer;
        padding: 0.5rem 0.5rem;
        line-height: 1;
        touch-action: manipulation;
        min-height: 2.75rem;
    }

    @media (hover: hover) {
        .settings-btn:hover {
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
