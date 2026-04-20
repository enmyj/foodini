<script>
    import { createQuery, useQueryClient } from "@tanstack/svelte-query";
    import {
        getLog,
        addFavorite,
        getFavorites,
        fetchStoredInsight,
        fetchStoredDayInsight,
        fetchStoredWeekSuggestions,
        fetchMealSuggestion,
        streamDayInsights,
        streamInsights,
        streamWeekSuggestions,
        streamMealSuggestion,
    } from "./api.js";
    import EntryRow from "./EntryRow.svelte";
    import ChatDrawer from "./ChatDrawer.svelte";
    import ActivityNote from "./ActivityNote.svelte";
    import ProfilePanel from "./ProfilePanel.svelte";
    import FavoritesView from "./FavoritesView.svelte";
    import { showError } from "./toast.js";
    import { navigate } from "./router.svelte.js";
    import ThemeToggle from "./ThemeToggle.svelte";

    const queryClient = useQueryClient();

    const MEAL_ORDER = ["breakfast", "lunch", "snack", "dinner", "supplements"];
    const DAY_ABBREV = ["Su", "Mo", "Tu", "We", "Th", "Fr", "Sa"];
    let view = $state("day");
    let currentDate = $state(todayStr());
    let menuOpen = $state(false);
    let drawerOpen = $state(false);
    let drawerTab = $state("food");
    let activityRefreshKey = $state(0);
    let drawerDate = $state(null);
    let drawerMeal = $state(null);
    let drawerField = $state(null);
    let drawerEditEntries = $state(null);
    let drawerEditMealType = $state(null);
    let dateInputEl = $state(null);
    let mealSuggestions = $state({});

    let dayInsight = $state(null);
    let dayInsightExpanded = $state(false);
    let insightsByWeek = $state({});
    let suggestionsByWeek = $state({});
    let collapsedMeals = $state(new Set(MEAL_ORDER));
    let historyWeeks = $state(4);
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
        return g;
    });

    let weekGroupsData = $derived(weekGroups(historyData, historyWeeks));

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
            dayInsight = null;
            dayInsightExpanded = false;
            mealSuggestions = {};
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

    function handleUpdate(updated) {
        queryClient.setQueryData(["log", currentDate], (old) => ({
            ...old,
            entries: (old?.entries ?? []).map((e) =>
                e.id === updated.id ? updated : e,
            ),
        }));
    }

    function openEditDrawer(meal, group) {
        drawerEditEntries = group;
        drawerEditMealType = meal;
        drawerDate = currentDate;
        drawerMeal = meal;
        drawerField = null;
        drawerTab = "food";
        drawerOpen = true;
    }

    function onEntriesEdited(updatedEntries) {
        queryClient.setQueryData(["log", currentDate], (old) => {
            const editedIds = new Set(updatedEntries.map((e) => e.id));
            const oldMealType = drawerEditMealType;
            // Remove old entries for this meal that aren't in the updated set
            const kept = (old?.entries ?? []).filter(
                (e) => e.meal_type !== oldMealType || editedIds.has(e.id),
            );
            // Update existing and add new
            const existingIds = new Set(kept.map((e) => e.id));
            const updated = kept.map((e) =>
                editedIds.has(e.id)
                    ? updatedEntries.find((u) => u.id === e.id)
                    : e,
            );
            const newEntries = updatedEntries.filter(
                (e) => !existingIds.has(e.id),
            );
            return { ...old, entries: [...updated, ...newEntries] };
        });
        queryClient.invalidateQueries({ queryKey: ["log"] });
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

    function onEntriesAdded(newEntries) {
        queryClient.setQueryData(["log", currentDate], (old) => ({
            ...old,
            entries: [...(old?.entries ?? []), ...newEntries],
        }));
        queryClient.invalidateQueries({ queryKey: ["log"] });
        if (newEntries.length > 0) {
            const addedMeal = newEntries[0].meal_type;
            collapsedMeals = new Set(
                [...collapsedMeals].filter((m) => m !== addedMeal),
            );
            // Kick off day-level insights (streamed, auto-opens).
            fetchDayInsights(currentDate, true);
        }
    }

    // --- Meal suggestions (for empty meals) ---

    async function toggleMealSuggestion(meal, date) {
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
            [key]: { loading: false, text: "", error: null, open: true, generatedAt: null },
        };
        try {
            const stored = await fetchMealSuggestion(date, meal);
            if (stored.suggestion) {
                mealSuggestions = {
                    ...mealSuggestions,
                    [key]: { loading: false, text: stored.suggestion, error: null, open: true, generatedAt: stored.generated_at },
                };
                return;
            }
            const res = await streamMealSuggestion(date, meal, (chunk) => {
                mealSuggestions = {
                    ...mealSuggestions,
                    [key]: { ...mealSuggestions[key], text: (mealSuggestions[key]?.text ?? "") + chunk },
                };
            });
            mealSuggestions = {
                ...mealSuggestions,
                [key]: { loading: false, text: res.text, error: null, open: true, generatedAt: res.generated_at },
            };
        } catch {
            mealSuggestions = {
                ...mealSuggestions,
                [key]: { loading: false, text: null, error: "Could not load suggestion", open: true, generatedAt: null },
            };
        }
    }

    async function regenMealSuggestion(meal, date) {
        const key = `${date}|${meal}`;
        mealSuggestions = {
            ...mealSuggestions,
            [key]: { loading: false, text: "", error: null, open: true, generatedAt: null },
        };
        try {
            const res = await streamMealSuggestion(date, meal, (chunk) => {
                mealSuggestions = {
                    ...mealSuggestions,
                    [key]: { ...mealSuggestions[key], text: (mealSuggestions[key]?.text ?? "") + chunk },
                };
            });
            mealSuggestions = {
                ...mealSuggestions,
                [key]: { loading: false, text: res.text, error: null, open: true, generatedAt: res.generated_at },
            };
        } catch {
            mealSuggestions = {
                ...mealSuggestions,
                [key]: { loading: false, text: null, error: "Could not load suggestion", open: true, generatedAt: null },
            };
        }
    }

    // --- Day insights ---

    async function fetchDayInsights(date, regenerate = false) {
        dayInsight = { loading: true, text: null, error: null, open: true, generatedAt: null };
        try {
            if (!regenerate) {
                const stored = await fetchStoredDayInsight(date);
                if (stored.insight) {
                    dayInsight = { loading: false, text: stored.insight, error: null, open: true, generatedAt: stored.generated_at };
                    return;
                }
            }
            dayInsight = { loading: false, text: "", error: null, open: true, generatedAt: null };
            const res = await streamDayInsights(date, (chunk) => {
                dayInsight = { ...dayInsight, text: (dayInsight.text ?? "") + chunk };
            });
            dayInsight = { loading: false, text: res.text, error: null, open: true, generatedAt: res.generated_at };
        } catch (e) {
            dayInsight = { loading: false, text: null, error: e.message || "Could not load insights", open: true, generatedAt: null };
        }
    }

    function toggleDayInsights() {
        if (!dayInsight || (!dayInsight.loading && !dayInsight.text && !dayInsight.error)) {
            fetchDayInsights(currentDate, false);
        } else {
            dayInsight = { ...dayInsight, open: !dayInsight.open };
        }
    }

    // --- Weekly insights & suggestions ---

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
            insightsByWeek = {
                ...insightsByWeek,
                [weekStart]: { open: true, loading: false, text: "", error: null, generatedAt: null, loaded: false },
            };
            const res = await streamInsights(weekStart, weekEnd, (chunk) => {
                insightsByWeek = {
                    ...insightsByWeek,
                    [weekStart]: { ...insightsByWeek[weekStart], text: (insightsByWeek[weekStart]?.text ?? "") + chunk },
                };
            });
            insightsByWeek = {
                ...insightsByWeek,
                [weekStart]: {
                    open: true,
                    loading: false,
                    text: res.text,
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
            suggestionsByWeek = {
                ...suggestionsByWeek,
                [weekStart]: { open: true, loading: false, text: "", error: null, generatedAt: null, loaded: false },
            };
            const res = await streamWeekSuggestions(weekStart, weekEnd, (chunk) => {
                suggestionsByWeek = {
                    ...suggestionsByWeek,
                    [weekStart]: { ...suggestionsByWeek[weekStart], text: (suggestionsByWeek[weekStart]?.text ?? "") + chunk },
                };
            });
            suggestionsByWeek = {
                ...suggestionsByWeek,
                [weekStart]: {
                    open: true,
                    loading: false,
                    text: res.text,
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
                    onclick={(e) => { e.preventDefault(); navigate("/"); }}
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
                        onchange={(e) => {
                            if (e.target.value) currentDate = e.target.value;
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
                    <span>{t.calories} cal</span>
                    <span>{t.protein}g P</span>
                    <span>{t.carbs}g C</span>
                    <span>{t.fat}g F</span>
                    <span>{t.fiber}g Fb</span>
                    {#if t.calories > 0}
                        <button
                            class="insights-btn"
                            class:active={dayInsight?.open}
                            onclick={toggleDayInsights}
                            aria-label="AI insights"
                            title="AI insights">insights</button
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
            <div class="insights-panel day-insights-panel">
                {#if !dayInsight.loading}
                    <button
                        class="insight-close"
                        onclick={() => (dayInsight = { ...dayInsight, open: false })}
                        aria-label="Close insights">✕</button
                    >
                {/if}
                {#if dayInsight.loading}
                    <div class="insight-skeleton">
                        <div class="isk-line" style="width: 88%"></div>
                        <div class="isk-line" style="width: 72%"></div>
                        <div class="isk-line" style="width: 80%"></div>
                    </div>
                {:else if dayInsight.error}
                    <span class="insights-err">{dayInsight.error}</span>
                {:else if dayInsight.text != null}
                    <!-- eslint-disable-next-line svelte/no-at-html-tags -->
                    <p class="insights-text" class:collapsed-text={!dayInsightExpanded}>{@html renderInsight(dayInsight.text)}</p>
                    {#if dayInsight.generatedAt}
                        <button class="insight-more" onclick={() => (dayInsightExpanded = !dayInsightExpanded)}>{dayInsightExpanded ? "less" : "more"}</button>
                    {/if}
                    {#if dayInsight.generatedAt && dayInsightExpanded}
                        <div class="insight-footer">
                            <span class="insight-ts">{formatGeneratedAt(dayInsight.generatedAt)}</span>
                            <button class="insight-regen" onclick={() => fetchDayInsights(currentDate, true)}>regenerate</button>
                        </div>
                    {/if}
                {/if}
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
                    <div class="insights-panel suggestions-panel">
                        {#if ms.error}
                            <span class="insights-err">{ms.error}</span>
                        {:else if ms.text != null}
                            <!-- eslint-disable-next-line svelte/no-at-html-tags -->
                            <p class="insights-text">{@html renderInsight(ms.text)}</p>
                            {#if ms.generatedAt}
                                <div class="insight-footer">
                                    <span class="insight-ts">{formatGeneratedAt(ms.generatedAt)}</span>
                                    <button class="insight-regen" onclick={() => regenMealSuggestion(meal, currentDate)}>regenerate</button>
                                </div>
                            {/if}
                        {/if}
                    </div>
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
                        {:else if wi.text != null}
                            <!-- eslint-disable-next-line svelte/no-at-html-tags -->
                            <p class="insights-text">
                                {@html renderInsight(wi.text)}
                            </p>
                            {#if wi.generatedAt}
                                <div class="insight-footer">
                                    <span class="insight-ts"
                                        >{formatGeneratedAt(
                                            wi.generatedAt,
                                        )}</span
                                    >
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
                        {:else if ws.text != null}
                            <span class="suggestions-label"
                                >Meal ideas for next week</span
                            >
                            <!-- eslint-disable-next-line svelte/no-at-html-tags -->
                            <p class="insights-text">
                                {@html renderInsight(ws.text)}
                            </p>
                            {#if ws.generatedAt}
                                <div class="insight-footer">
                                    <span class="insight-ts"
                                        >{formatGeneratedAt(
                                            ws.generatedAt,
                                        )}</span
                                    >
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
    {onEntriesEdited}
    date={drawerDate}
    meal={drawerMeal}
    initialTab={drawerTab}
    initialField={drawerField}
    editEntries={drawerEditEntries}
    editMealType={drawerEditMealType}
    yesterdayEntries={(drawerEditMealType || drawerMeal) ? (yesterdayByMeal[drawerEditMealType || drawerMeal] ?? []) : []}
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
        gap: 0.4rem 0.75rem;
        align-items: center;
        font-size: 0.78rem;
        color: var(--mute);
        padding-bottom: 0.1rem;
        padding-top: 0.3rem;
        font-variant-numeric: tabular-nums;
    }

    @media (max-width: 380px) {
        .totals {
            gap: 0.3rem 0.5rem;
            font-size: 0.72rem;
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

    @keyframes shimmer {
        0% {
            background-position: 200% 0;
        }
        100% {
            background-position: -200% 0;
        }
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

    .insights-text.collapsed-text {
        display: -webkit-box;
        -webkit-line-clamp: 3;
        -webkit-box-orient: vertical;
        overflow: hidden;
    }

    .insight-more {
        background: none;
        border: none;
        font-family: inherit;
        font-size: 0.72rem;
        color: var(--mute-2);
        cursor: pointer;
        padding: 0.25rem 0 0;
        touch-action: manipulation;
    }

    @media (hover: hover) {
        .insight-more:hover {
            color: var(--ink-mute);
        }
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
