<script lang="ts">
    import { createMutation, createQuery, useQueryClient } from "@tanstack/svelte-query";
    import {
        getLog,
        addFavorite,
        getFavorites,
        fetchStoredInsight,
        fetchStoredDayInsight,
        fetchStoredWeekSuggestions,
        fetchInsightSnapshots,
        fetchInsightByTrigger,
        generateDayInsights,
        generateInsights,
        generateWeekSuggestions,
        deleteEvent,
        patchEvent,
        patchEntry,
    } from "./api.ts";
    import type { InsightSnapshot } from "./api.ts";
    import type { ApiError } from "./api.ts";
    import EntryRow from "./EntryRow.svelte";
    import ChatDrawer from "./ChatDrawer.svelte";
    import CoachChat from "./CoachChat.svelte";
    import { EVENT_KINDS } from "./types.ts";
    import type { EventKind } from "./types.ts";
    import { appendEntriesToLogCache, removeEntryFromLogCache, replaceMealEntriesInLogCache, updateEntryInLogCache } from "./cache.ts";
    import { addDays, formatDateNav, formatTimeShort, getMonday, todayStr } from "./date.ts";
    import ProfilePanel from "./ProfilePanel.svelte";
    import FavoritesView from "./FavoritesView.svelte";
    import HistoryWeekBlock from "./HistoryWeekBlock.svelte";
    import InsightPanel from "./InsightPanel.svelte";
    import { queryKeys } from "./queryKeys.ts";
    import { showError } from "./toast.ts";
    import { navigate } from "./router.svelte.ts";
    import ThemeToggle from "./ThemeToggle.svelte";
    import SystemPromptModal from "./SystemPromptModal.svelte";
    import { MEAL_ORDER } from "./types.ts";
    import type {
        Entry,
        LogEvent,
        Favorite,
        InsightPanelState,
        LogResponse,
        MealEntriesMap,
        MealType,
        WeekGroup,
        WeekInsightPanelState,
    } from "./types.ts";

    const queryClient = useQueryClient();

    type ViewMode = "day" | "favorites" | "history" | "coach" | "profile";
    const DAY_ABBREV = ["Su", "Mo", "Tu", "We", "Th", "Fr", "Sa"];
    let view = $state<ViewMode>("day");
    let currentDate = $state(todayStr());
    let menuOpen = $state(false);
    let gearOpen = $state(false);
    let promptOpen = $state(false);
    let drawerOpen = $state(false);
    let drawerDate = $state<string | null>(null);
    let drawerMeal = $state<MealType | null>(null);
    let drawerEditEntries = $state<Entry[] | null>(null);
    let drawerEditMealType = $state<MealType | null>(null);
    let drawerEditEvent = $state<LogEvent | null>(null);
    let drawerInitialMode = $state<"meal" | "event" | null>(null);
    let dateInputEl = $state<HTMLInputElement | null>(null);
    let coachPrefill = $state("");

    let dayInsight = $state<InsightPanelState | null>(null);
    let dayInsightExpanded = $state(false);
    let totalsOpen = $state(false);
    let insightsByWeek = $state<Record<string, WeekInsightPanelState>>({});
    let suggestionsByWeek = $state<Record<string, WeekInsightPanelState>>({});
    // Both keyed by timeline rowKey (for non-snack meals: the meal name; for
    // snacks: a per-cluster id like "snack:0", since snacks split into multiple
    // rows by time-of-day clustering).
    let expandedRows = $state<Set<string>>(new Set());
    let expandedMealMacros = $state<Set<string>>(new Set());
    let insightSnapshots = $state<InsightSnapshot[]>([]);
    let openMealInsightFor = $state<string | null>(null);
    let mealInsightCache = $state<Record<string, InsightPanelState>>({});
    let historyWeeks = $state(4);
    let favoritedDescs = $state<Set<string>>(new Set());
    let dayInsightFresh = $state(false);

    const INSIGHT_FRESH_KEY = (date: string) => `insightFresh:${date}`;
    function readFresh(date: string): boolean {
        try {
            return localStorage.getItem(INSIGHT_FRESH_KEY(date)) === "1";
        } catch {
            return false;
        }
    }
    function writeFresh(date: string): void {
        try {
            localStorage.setItem(INSIGHT_FRESH_KEY(date), "1");
        } catch {}
    }
    function clearFresh(date: string): void {
        try {
            localStorage.removeItem(INSIGHT_FRESH_KEY(date));
        } catch {}
    }
    let dayInsightStale = $state(false);
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
        const hasData =
            view === "day"
                ? Boolean(dayQuery.data)
                : view === "history"
                  ? Boolean(historyQuery.data)
                  : false;
        // Keep showing the last good snapshot if a background refetch fails.
        // This is common on iOS when Safari resumes before the network is
        // fully ready; the query will usually recover on its own.
        if (hasData) return "";
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
        const hasData =
            view === "day"
                ? Boolean(dayQuery.data)
                : view === "history"
                  ? Boolean(historyQuery.data)
                  : false;
        if (hasData) return null;
        if (!err) return null;
        if (err?.status === 401 || err?.code === "session_expired")
            return { href: "/auth/login", label: "Sign in" };
        if (err?.code === "insufficient_scopes")
            return { href: "/auth/login?consent=1", label: "Re-authorize" };
        return null;
    });
    // Only surface a banner for auth-related refresh failures — those need user
    // action. Generic network/5xx failures stay quiet; the next focus or
    // online event will retry automatically.
    let refreshNotice = $derived.by(() => {
        const err = (view === "day" ? dayQuery.error : view === "history" ? historyQuery.error : null) as Partial<ApiError> | null;
        const hasData =
            view === "day"
                ? Boolean(dayQuery.data)
                : view === "history"
                  ? Boolean(historyQuery.data)
                  : false;
        if (!err || !hasData) return "";
        if (err?.status === 401 || err?.code === "session_expired")
            return "Session expired. Sign in again to refresh.";
        if (err?.code === "insufficient_scopes")
            return "Google permissions are missing. Re-authorize to refresh.";
        return "";
    });
    let refreshNoticeAction = $derived.by<{ href: string; label: string } | null>(() => {
        const err = (view === "day" ? dayQuery.error : view === "history" ? historyQuery.error : null) as Partial<ApiError> | null;
        const hasData =
            view === "day"
                ? Boolean(dayQuery.data)
                : view === "history"
                  ? Boolean(historyQuery.data)
                  : false;
        if (!err || !hasData) return null;
        if (err?.status === 401 || err?.code === "session_expired")
            return { href: "/auth/login", label: "Sign in" };
        if (err?.code === "insufficient_scopes")
            return { href: "/auth/login?consent=1", label: "Re-authorize" };
        return null;
    });

    let weekGroupsData = $derived(weekGroups(historyData, historyWeeks));

type TimelineItem =
        | {
              kind: "meal";
              meal: MealType;
              rowKey: string;
              label: string;
              entries: Entry[];
              time: string;
          }
        | { kind: "event"; event: LogEvent; time: string };

    // Snacks split into clusters: any gap >30min between entry times starts a
    // new cluster. Each cluster gets a time-of-day label. Untimed snacks land
    // in a trailing cluster labeled plainly "snack".
    function snackClusters(
        entries: Entry[],
    ): { rowKey: string; label: string; entries: Entry[]; time: string }[] {
        const timed = entries
            .filter((e) => !!e.time)
            .slice()
            .sort((a, b) => (a.time ?? "").localeCompare(b.time ?? ""));
        const untimed = entries.filter((e) => !e.time);
        const clusters: Entry[][] = [];
        let cur: Entry[] = [];
        let prevMin = -Infinity;
        for (const e of timed) {
            const [h, m] = (e.time as string).split(":");
            const min = Number(h) * 60 + Number(m);
            if (cur.length === 0 || min - prevMin <= 30) cur.push(e);
            else {
                clusters.push(cur);
                cur = [e];
            }
            prevMin = min;
        }
        if (cur.length) clusters.push(cur);
        if (untimed.length) clusters.push(untimed);
        return clusters.map((es, i) => {
            const t = es[0]?.time;
            let label = "snack";
            if (t) {
                if (t < "12:00") label = "morning snack";
                else if (t < "17:00") label = "afternoon snack";
                else label = "evening snack";
            }
            return {
                rowKey: `snack:${i}`,
                label,
                entries: es,
                time: t ?? "99:99",
            };
        });
    }

    // Unified time-sorted timeline interleaving meals (each meal type as one row,
    // anchored to its earliest entry time) and events. Snacks split by time-of-day
    // clustering. Untimed entries sort last.
    let timelineItems = $derived.by<TimelineItem[]>(() => {
        const grouped = groupedByMeal(dayData?.entries);
        const items: TimelineItem[] = [];
        for (const meal of MEAL_ORDER) {
            const entries = grouped[meal] ?? [];
            if (entries.length === 0) continue;
            if (meal === "snack") {
                for (const c of snackClusters(entries)) {
                    items.push({
                        kind: "meal",
                        meal,
                        rowKey: c.rowKey,
                        label: c.label,
                        entries: c.entries,
                        time: c.time,
                    });
                }
                continue;
            }
            let firstTime = "99:99";
            for (const e of entries) {
                const t = e.time ?? "";
                if (t && t < firstTime) firstTime = t;
            }
            items.push({
                kind: "meal",
                meal,
                rowKey: meal,
                label: meal,
                entries,
                time: firstTime,
            });
        }
        for (const ev of dayData?.events ?? []) {
            items.push({ kind: "event", event: ev, time: ev.time || "99:99" });
        }
        items.sort((a, b) => a.time.localeCompare(b.time));
        return items;
    });

    const MEAL_DOT_COLORS: Record<MealType, string> = {
        breakfast: "#d6a04c",
        lunch: "#c97a3e",
        snack: "#7a99c4",
        dinner: "#b8533f",
        supplements: "#9a7ab8",
    };

    const EVENT_DOT_COLORS: Record<EventKind, string> = {
        workout: "#7aa57a",
        stool: "#b08a5b",
        water: "#6da3c0",
        feeling: "#c08aa8",
    };

    const EVENT_KIND_LABELS: Record<EventKind, string> = {
        workout: "Workout",
        stool: "💩",
        water: "Water",
        feeling: "Feeling",
    };

    // The expanded body's primary detail line. Returns "" when the event has no
    // info beyond its kind (e.g. a logged BM with no text), so the row can show
    // a "no details" placeholder instead of repeating the label.
    function eventDetail(ev: LogEvent): string {
        switch (ev.kind) {
            case "workout":
                return ev.text || "";
            case "stool":
                return ev.text || "";
            case "water":
                return `${Math.round(ev.num ?? 0)} ml`;
            case "feeling":
                return `${ev.num ?? 0}/10${ev.text ? ` — ${ev.text}` : ""}`;
        }
    }

    function describeEvent(ev: LogEvent): string {
        switch (ev.kind) {
            case "workout":
                return ev.text || "Workout";
            case "stool":
                return ev.text || "💩";
            case "water":
                return `${Math.round(ev.num ?? 0)} ml`;
            case "feeling":
                return `${ev.num ?? 0}/10${ev.text ? ` — ${ev.text}` : ""}`;
        }
    }

    async function handleDeleteEvent(ev: LogEvent) {
        try {
            await deleteEvent(ev.id);
        } catch (e) {
            showError(e, "Failed to delete event.");
            return;
        }
        if (ev.date === currentDate) {
            queryClient.setQueryData<LogResponse>(
                queryKeys.logDay(currentDate),
                (old) =>
                    old
                        ? { ...old, events: old.events.filter((e) => e.id !== ev.id) }
                        : old,
            );
            dayInsightStale = true;
        }
        queryClient.invalidateQueries({ queryKey: queryKeys.events(ev.date) });
        queryClient.invalidateQueries({ queryKey: queryKeys.log() });
    }

    let emptyMeals = $derived<MealType[]>(
        MEAL_ORDER.filter(
            (m) => (groupedByMeal(dayData?.entries)[m]?.length ?? 0) === 0,
        ),
    );

    let nextSuggestableMeal = $derived<MealType | null>(
        emptyMeals.find((m) => m !== "supplements") ?? null,
    );

    async function retimeEvent(ev: LogEvent, newTime: string) {
        if (!newTime || newTime === ev.time) return;
        try {
            const updated = await patchEvent(ev.id, { ...ev, time: newTime });
            onEventChanged({ updated });
        } catch (e) {
            showError(e, "Failed to update time.");
        }
    }

    async function retimeMeal(meal: MealType, entries: Entry[], newTime: string) {
        if (!newTime) return;
        try {
            const updated = await Promise.all(
                entries.map((e) => patchEntry(e.id, { ...e, time: newTime })),
            );
            onEntriesEdited(updated, meal);
        } catch (e) {
            showError(e, "Failed to update time.");
        }
    }

    function suggestNextMeal() {
        if (!nextSuggestableMeal) return;
        coachPrefill = `Suggest a ${nextSuggestableMeal} for today.`;
        setView("coach");
    }

    function discussInsight(text: string | null) {
        if (!text) return;
        coachPrefill = text.trim();
        setView("coach");
    }

    let triggerEntryIds = $derived<Set<string>>(
        new Set(insightSnapshots.map((s) => s.triggered_by).filter(Boolean)),
    );

    // For each meal, the latest trigger entry whose id falls within that meal's
    // entries — that's the "verdict at this point in the day" anchor.
    function latestTriggerForMeal(entries: Entry[]): string | null {
        if (insightSnapshots.length === 0 || entries.length === 0) return null;
        const ids = new Set(entries.map((e) => e.id));
        let pickId: string | null = null;
        let pickAt = "";
        for (const snap of insightSnapshots) {
            if (!ids.has(snap.triggered_by)) continue;
            if (snap.generated_at >= pickAt) {
                pickAt = snap.generated_at;
                pickId = snap.triggered_by;
            }
        }
        return pickId;
    }

    async function toggleMealInsight(rowKey: string, entries: Entry[]) {
        if (openMealInsightFor === rowKey) {
            openMealInsightFor = null;
            return;
        }
        openMealInsightFor = rowKey;
        const triggerId = latestTriggerForMeal(entries);
        if (!triggerId) return;
        if (mealInsightCache[triggerId]?.text != null) return;
        mealInsightCache = {
            ...mealInsightCache,
            [triggerId]: {
                loading: true,
                text: null,
                error: null,
                open: true,
                generatedAt: null,
            },
        };
        try {
            const res = await fetchInsightByTrigger(triggerId);
            mealInsightCache = {
                ...mealInsightCache,
                [triggerId]: {
                    loading: false,
                    text: res.insight ?? null,
                    error: null,
                    open: true,
                    generatedAt: res.generated_at ?? null,
                },
            };
        } catch {
            mealInsightCache = {
                ...mealInsightCache,
                [triggerId]: {
                    loading: false,
                    text: null,
                    error: "Could not load insight",
                    open: true,
                    generatedAt: null,
                },
            };
        }
    }

    const NON_DAY_VIEWS: ViewMode[] = ["favorites", "history", "coach", "profile"];

    function viewFromHash(): ViewMode {
        const h = location.hash.slice(1) as ViewMode;
        return NON_DAY_VIEWS.includes(h) ? h : "day";
    }

    function setView(next: ViewMode) {
        if (next === view) return;
        if (next === "day") {
            if (history.state?.appView) {
                history.back();
                return;
            }
            view = "day";
            return;
        }
        const url = `${location.pathname}${location.search}#${next}`;
        if (view === "day") {
            history.pushState({ appView: next }, "", url);
        } else {
            history.replaceState({ appView: next }, "", url);
        }
        view = next;
    }

    // Restore view from hash on initial load (refresh / deep link).
    {
        const initial = viewFromHash();
        if (initial !== "day") {
            view = initial;
            history.replaceState({ appView: initial }, "", location.href);
        }
    }

    $effect(() => {
        const onPop = () => {
            const next = (history.state?.appView ?? viewFromHash()) as ViewMode;
            view = NON_DAY_VIEWS.includes(next) ? next : "day";
        };
        window.addEventListener("popstate", onPop);
        return () => window.removeEventListener("popstate", onPop);
    });

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
            dayInsightStale = false;
            expandedRows = new Set();
            expandedMealMacros = new Set();
            dayInsight = null;
            dayInsightExpanded = false;
            dayInsightFresh = readFresh(currentDate);
            insightSnapshots = [];
            mealInsightCache = {};
            openMealInsightFor = null;
            void loadInsightSnapshots(currentDate);
        }
    });

    async function loadInsightSnapshots(date: string): Promise<void> {
        try {
            const res = await fetchInsightSnapshots(date);
            if (date !== currentDate) return;
            insightSnapshots = res.snapshots ?? [];
        } catch {
            // best-effort; bubble simply won't appear
        }
    }

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
        const { entries = [], events = [] } = data;
        const byDate: Record<string, { entries: Entry[]; events: LogEvent[] }> = {};
        for (const e of entries) {
            (byDate[e.date] ??= { entries: [], events: [] }).entries.push(e);
        }
        for (const ev of events) {
            (byDate[ev.date] ??= { entries: [], events: [] }).events.push(ev);
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
                        ? { entries: [], events: [] }
                        : (byDate[date] ?? { entries: [], events: [] })),
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
        drawerInitialMode = "meal";
        drawerOpen = true;
    }

    function openEditEventDrawer(ev: LogEvent) {
        drawerEditEvent = ev;
        drawerDate = ev.date || currentDate;
        drawerMeal = null;
        drawerEditEntries = null;
        drawerEditMealType = null;
        drawerInitialMode = "event";
        drawerOpen = true;
    }

    function onEntriesEdited(updatedEntries: Entry[], editedMealType: MealType | null = null) {
        const mealType =
            editedMealType ??
            drawerEditMealType ??
            drawerMeal ??
            updatedEntries[0]?.meal_type ??
            null;
        applyDayLogMutation(updatedEntries[0]?.date ?? currentDate, (old: LogResponse | undefined) =>
            replaceMealEntriesInLogCache(old, mealType, updatedEntries),
        );
        if (mealType && (updatedEntries[0]?.date ?? currentDate) === currentDate) {
            // Re-collapse rows for the edited meal type. Snack cluster ids may
            // have changed if entries were retimed, so drop all "snack:*" too.
            expandedRows = new Set(
                [...expandedRows].filter(
                    (k) => k !== mealType && !k.startsWith(`${mealType}:`),
                ),
            );
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

    function onEventChanged(_change: { added?: LogEvent; updated?: LogEvent; deletedId?: string }) {
        dayInsightStale = true;
        queryClient.invalidateQueries({ queryKey: queryKeys.logDay(currentDate) });
        queryClient.invalidateQueries({ queryKey: queryKeys.events(currentDate) });
    }

    function onSwitchMeal(meal: MealType): Entry[] | null {
        const grouped = groupedByMeal(dayData?.entries);
        const list = grouped[meal] ?? null;
        drawerEditMealType = meal;
        drawerEditEntries = list ?? null;
        drawerMeal = meal;
        return list && list.length ? list : null;
    }

    function closeDrawer() {
        drawerOpen = false;
        drawerDate = null;
        drawerMeal = null;
        drawerEditEntries = null;
        drawerEditMealType = null;
        drawerEditEvent = null;
        drawerInitialMode = null;
        if (dayInsightStale && view === "day") {
            dayInsightStale = false;
            const currentLog = queryClient.getQueryData<LogResponse>(
                queryKeys.logDay(currentDate),
            );
            const hasEntries = (currentLog?.entries?.length ?? 0) > 0;
            if (hasEntries) {
                const wasOpen = dayInsight?.open ?? false;
                void fetchDayInsights(currentDate, true, { open: wasOpen }).then(() => {
                    if (!dayInsight?.open && dayInsight?.text) {
                        dayInsightFresh = true;
                        writeFresh(currentDate);
                    }
                });
            }
        }
    }

    function closeProfile() {
        setView("day");
    }

    function onEntriesAdded(newEntries: Entry[]) {
        const addedDate = newEntries[0]?.date ?? currentDate;
        applyDayLogMutation(addedDate, (old: LogResponse | undefined) =>
            appendEntriesToLogCache(old, newEntries),
        );
        if (addedDate === currentDate) {
            const mealsAdded = new Set(newEntries.map((e) => e.meal_type));
            expandedRows = new Set(
                [...expandedRows].filter(
                    (k) =>
                        !mealsAdded.has(k as MealType) &&
                        ![...mealsAdded].some((m) => k.startsWith(`${m}:`)),
                ),
            );
        }
    }

    // --- Day insights ---

    function applyDayLogMutation(
        date: string,
        updater: (old: LogResponse | undefined) => LogResponse | undefined,
    ): void {
        // All callers pass authoritative server entries, so update the cache
        // directly and skip any refetch — a reconcile here would race the
        // Sheets API's read-your-write window and overwrite good data.
        if (date === currentDate) {
            void queryClient.cancelQueries({ queryKey: queryKeys.logDay(currentDate) });
            queryClient.setQueryData(queryKeys.logDay(currentDate), updater);
            dayInsightStale = true;
        }
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
            // a regeneration creates a new snapshot anchored to the latest entry
            void loadInsightSnapshots(date);
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
        clearFresh(currentDate);
        if (!dayInsight || (!dayInsight.loading && !dayInsight.text && !dayInsight.error)) {
            fetchDayInsights(currentDate, false);
        } else {
            dayInsight = { ...dayInsight, open: !dayInsight.open };
        }
    }

    // --- Weekly insights & suggestions ---

    function canCloseDayInsight(insight: InsightPanelState | null): boolean {
        return Boolean(insight && !insight.loading && (insight.error || insight.text != null));
    }

    function closeDayInsight(): void {
        if (!dayInsight) return;
        dayInsight = { ...dayInsight, open: false };
    }

    // Loads/regenerates a per-week panel (insights or suggestions). Each panel
    // state lives in its own map keyed by weekStart; behavior is identical
    // beyond the API endpoints and error string.
    async function fetchWeekPanel(
        map: Record<string, WeekInsightPanelState>,
        setMap: (next: Record<string, WeekInsightPanelState>) => void,
        weekStart: string,
        weekEnd: string,
        regenerate: boolean,
        fetchStored: (s: string, e: string) => Promise<{ text: string | null; generatedAt: string | null }>,
        generate: (s: string, e: string) => Promise<{ text: string | null; generatedAt: string | null }>,
        errorText: string,
    ): Promise<void> {
        const set = (state: WeekInsightPanelState) =>
            setMap({ ...map, [weekStart]: state });
        set({ open: true, loading: true, text: null, error: null, generatedAt: null, loaded: false });
        try {
            if (!regenerate) {
                const stored = await fetchStored(weekStart, weekEnd);
                if (stored.text) {
                    set({ open: true, loading: false, text: stored.text, error: null, generatedAt: stored.generatedAt, loaded: true });
                    return;
                }
            }
            const res = await generate(weekStart, weekEnd);
            set({ open: true, loading: false, text: res.text, error: null, generatedAt: res.generatedAt, loaded: true });
        } catch {
            set({ open: true, loading: false, text: null, error: errorText, generatedAt: null, loaded: true });
        }
    }

    function togglePanel(
        map: Record<string, WeekInsightPanelState>,
        setMap: (next: Record<string, WeekInsightPanelState>) => void,
        weekStart: string,
        load: () => void,
    ): void {
        const cur = map[weekStart];
        if (!cur || !cur.loaded) load();
        else setMap({ ...map, [weekStart]: { ...cur, open: !cur.open } });
    }

    function fetchInsights(weekStart: string, weekEnd: string, regenerate = false): Promise<void> {
        return fetchWeekPanel(
            insightsByWeek,
            (next) => (insightsByWeek = next),
            weekStart, weekEnd, regenerate,
            async (s, e) => {
                const r = await fetchStoredInsight(s, e);
                return { text: r.insight ?? null, generatedAt: r.generated_at ?? null };
            },
            async (s, e) => {
                const r = await generateInsights(s, e);
                return { text: r.insight ?? null, generatedAt: r.generated_at ?? null };
            },
            "Could not load insights",
        );
    }

    function toggleInsights(weekStart: string, weekEnd: string) {
        togglePanel(insightsByWeek, (next) => (insightsByWeek = next), weekStart,
            () => fetchInsights(weekStart, weekEnd, false));
    }

    function fetchWeekSuggestions(weekStart: string, weekEnd: string, regenerate = false): Promise<void> {
        return fetchWeekPanel(
            suggestionsByWeek,
            (next) => (suggestionsByWeek = next),
            weekStart, weekEnd, regenerate,
            async (s, e) => {
                const r = await fetchStoredWeekSuggestions(s, e);
                return { text: r.suggestions ?? null, generatedAt: r.generated_at ?? null };
            },
            async (s, e) => {
                const r = await generateWeekSuggestions(s, e);
                return { text: r.suggestions ?? null, generatedAt: r.generated_at ?? null };
            },
            "Could not load suggestions",
        );
    }

    function toggleWeekSuggestions(weekStart: string, weekEnd: string) {
        togglePanel(suggestionsByWeek, (next) => (suggestionsByWeek = next), weekStart,
            () => fetchWeekSuggestions(weekStart, weekEnd, false));
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
                <span class="view-label">{view === "day" ? "Day" : view === "history" ? "History" : view === "favorites" ? "Favorites" : view === "coach" ? "Coach" : view === "profile" ? "Profile" : ""}</span>
                {#if menuOpen}
                    <!-- svelte-ignore a11y_click_events_have_key_events -->
                    <div class="menu-backdrop" aria-hidden="true" onclick={() => (menuOpen = false)}></div>
                    <nav class="nav-menu">
                        <button class:active={view === "day"} onclick={() => { setView("day"); menuOpen = false; }}>Day</button>
                        <button class:active={view === "history"} onclick={() => { setView("history"); menuOpen = false; }}>History</button>
                        <button class:active={view === "favorites"} onclick={() => { setView("favorites"); menuOpen = false; }}>Favorites</button>
                        <button class:active={view === "coach"} onclick={() => { setView("coach"); menuOpen = false; }}>Coach</button>
                    </nav>
                {/if}
            </div>
            <div class="header-actions">
                <ThemeToggle />
                <div class="gear-wrap">
                    <button
                        class="gear-btn"
                        onclick={() => (gearOpen = !gearOpen)}
                        aria-label="Settings"
                        aria-expanded={gearOpen}
                        title="Settings"
                    >
                        <svg width="16" height="16" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"><circle cx="12" cy="12" r="3" /><path d="M19.4 15a1.65 1.65 0 0 0 .33 1.82l.06.06a2 2 0 0 1-2.83 2.83l-.06-.06a1.65 1.65 0 0 0-1.82-.33 1.65 1.65 0 0 0-1 1.51V21a2 2 0 0 1-4 0v-.09a1.65 1.65 0 0 0-1-1.51 1.65 1.65 0 0 0-1.82.33l-.06.06a2 2 0 0 1-2.83-2.83l.06-.06a1.65 1.65 0 0 0 .33-1.82 1.65 1.65 0 0 0-1.51-1H3a2 2 0 0 1 0-4h.09a1.65 1.65 0 0 0 1.51-1 1.65 1.65 0 0 0-.33-1.82l-.06-.06a2 2 0 0 1 2.83-2.83l.06.06a1.65 1.65 0 0 0 1.82.33h0a1.65 1.65 0 0 0 1-1.51V3a2 2 0 0 1 4 0v.09a1.65 1.65 0 0 0 1 1.51h0a1.65 1.65 0 0 0 1.82-.33l.06-.06a2 2 0 0 1 2.83 2.83l-.06.06a1.65 1.65 0 0 0-.33 1.82v0a1.65 1.65 0 0 0 1.51 1H21a2 2 0 0 1 0 4h-.09a1.65 1.65 0 0 0-1.51 1z" /></svg>
                    </button>
                    {#if gearOpen}
                        <!-- svelte-ignore a11y_click_events_have_key_events -->
                        <div class="menu-backdrop" aria-hidden="true" onclick={() => (gearOpen = false)}></div>
                        <nav class="gear-menu">
                            <button class:active={view === "profile"} onclick={() => { setView("profile"); gearOpen = false; }}>Profile</button>
                            <button onclick={() => { promptOpen = true; gearOpen = false; }}>System prompt</button>
                            {#if spreadsheetUrl}
                                <a href={spreadsheetUrl} target="_blank" rel="noopener" onclick={() => (gearOpen = false)}>Open Google Sheet</a>
                            {/if}
                            <hr />
                            <a href="/" onclick={(e: MouseEvent) => { e.preventDefault(); gearOpen = false; navigate("/"); }}>Home</a>
                            <a href="/auth/logout">Sign out</a>
                        </nav>
                    {/if}
                </div>
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
            {:else}
                <button
                    class="state-link"
                    type="button"
                    onclick={() => {
                        if (view === "day") dayQuery.refetch();
                        else if (view === "history") historyQuery.refetch();
                    }}>Retry</button>
            {/if}
        </div>
    {:else if view === "day"}
        {#if refreshNotice && refreshNoticeAction}
            <div class="refresh-notice">
                <span>{refreshNotice}</span>
                <a href={refreshNoticeAction.href}>{refreshNoticeAction.label}</a>
            </div>
        {/if}
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
                    onDiscuss={() => discussInsight(dayInsight?.text ?? null)}
                />
            </div>
        {/if}
        <div class="day-timeline">
        {#each timelineItems as item (item.kind === "meal" ? `m-${item.rowKey}` : `e-${item.event.id}`)}
            {#if item.kind === "meal"}
                {@const meal = item.meal}
                {@const rowKey = item.rowKey}
                {@const label = item.label}
                {@const group = item.entries}
                {@const collapsed = !expandedRows.has(rowKey)}
                {@const mt = totals(group)}
                {@const macrosOpen = expandedMealMacros.has(rowKey)}
                {@const hasAnchor = group.some((e) => triggerEntryIds.has(e.id))}
                {@const insightOpen = openMealInsightFor === rowKey}
                {@const triggerId = insightOpen ? latestTriggerForMeal(group) : null}
                {@const mealInsight = triggerId ? mealInsightCache[triggerId] : null}
                <section class="tl-row event-row" class:expanded={!collapsed}>
                    <div class="event-head">
                        <span class="tl-dot" style="background:{MEAL_DOT_COLORS[meal]}" aria-hidden="true"></span>
                        <input
                            type="time"
                            class="meal-time-input"
                            lang="en-GB"
                            title="Tap to retime this meal"
                            value={item.time !== "99:99" ? item.time : ""}
                            onblur={(e) => {
                                const v = (e.currentTarget as HTMLInputElement).value;
                                const orig = item.time !== "99:99" ? item.time : "";
                                if (v && v !== orig) retimeMeal(meal, group, v);
                            }}
                        />
                        <button
                            class="event-toggle"
                            onclick={() => {
                                expandedRows = collapsed
                                    ? new Set([...expandedRows, rowKey])
                                    : new Set([...expandedRows].filter((k) => k !== rowKey));
                            }}
                            aria-expanded={!collapsed}
                        >
                            <span class="event-name">{label}</span>
                            <span class="event-caret" aria-hidden="true">{collapsed ? "▸" : "▾"}</span>
                        </button>
                        {#if hasAnchor}
                            <button
                                class="meal-insight-bubble"
                                class:active={insightOpen}
                                aria-label="Insight at this point"
                                title="Insight at this point in the day"
                                onclick={() => toggleMealInsight(rowKey, group)}
                                >💡</button
                            >
                        {/if}
                        <button
                            class="meal-macros-pill"
                            class:active={macrosOpen}
                            aria-expanded={macrosOpen}
                            onclick={() => {
                                const next = new Set(expandedMealMacros);
                                if (macrosOpen) next.delete(rowKey);
                                else next.add(rowKey);
                                expandedMealMacros = next;
                            }}
                        >
                            {#if macrosOpen}
                                <span>{mt.calories} cal</span>
                                <span>{mt.protein}g P</span>
                                <span>{mt.carbs}g C</span>
                                <span>{mt.fat}g F</span>
                                <span>{mt.fiber}g Fb</span>
                            {:else}
                                {mt.calories} cal
                            {/if}
                        </button>
                        <button
                            class="meal-action-btn"
                            onclick={() => openEditDrawer(meal, group)}
                            >Edit</button
                        >
                    </div>
                    {#if insightOpen && mealInsight}
                        <div class="meal-insight-panel">
                            <InsightPanel
                                loading={mealInsight.loading}
                                error={mealInsight.error}
                                text={mealInsight.text}
                                generatedAt={mealInsight.generatedAt}
                                onDiscuss={() =>
                                    discussInsight(mealInsight.text)}
                            />
                        </div>
                    {/if}
                    {#if !collapsed}
                        <div class="event-body">
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
                                    drawerEditEntries = null;
                                    drawerEditMealType = null;
                                    drawerEditEvent = null;
                                    drawerInitialMode = "meal";
                                    drawerOpen = true;
                                }}>+ add item</button
                            >
                        </div>
                    {/if}
                </section>
            {:else}
                {@const ev = item.event}
                {@const evRowKey = `event:${ev.id}`}
                {@const evCollapsed = !expandedRows.has(evRowKey)}
                <section class="tl-row tl-event" class:expanded={!evCollapsed}>
                    <div class="event-head">
                        <span class="tl-dot" style="background:{EVENT_DOT_COLORS[ev.kind]}" aria-hidden="true"></span>
                        <input
                            type="time"
                            class="meal-time-input tl-event-time"
                            lang="en-GB"
                            title="Tap to retime"
                            value={ev.time ?? ""}
                            onblur={(e) => {
                                const v = (e.currentTarget as HTMLInputElement).value;
                                if (v && v !== (ev.time ?? "")) retimeEvent(ev, v);
                            }}
                        />
                        <button
                            class="event-toggle"
                            onclick={() => {
                                expandedRows = evCollapsed
                                    ? new Set([...expandedRows, evRowKey])
                                    : new Set([...expandedRows].filter((k) => k !== evRowKey));
                            }}
                            aria-expanded={!evCollapsed}
                        >
                            <span class="event-name">{EVENT_KIND_LABELS[ev.kind]}</span>
                            <span class="event-caret" aria-hidden="true">{evCollapsed ? "▸" : "▾"}</span>
                        </button>
                        <button
                            class="meal-action-btn"
                            onclick={() => openEditEventDrawer(ev)}
                            >Edit</button
                        >
                    </div>
                    {#if !evCollapsed}
                        {@const detail = eventDetail(ev)}
                        <div class="event-body">
                            {#if detail || ev.notes}
                                {#if detail}
                                    <div class="tl-event-detail">{detail}</div>
                                {/if}
                                {#if ev.notes}
                                    <div class="tl-event-detail tl-event-notes">{ev.notes}</div>
                                {/if}
                            {:else}
                                <div class="tl-event-detail tl-event-notes">no details</div>
                            {/if}
                        </div>
                    {/if}
                </section>
            {/if}
        {/each}
        </div>

        {#if nextSuggestableMeal}
            <div class="suggest-row">
                <button class="suggest-btn" onclick={suggestNextMeal}>
                    Suggest {nextSuggestableMeal} →
                </button>
            </div>
        {/if}
    {:else if view === "favorites"}
        <FavoritesView onLoad={syncFavoritedDescs} />
    {:else if view === "coach"}
        <div class="coach-pane">
            <CoachChat
                active={view === "coach"}
                date={currentDate}
                pinnedContext={coachPrefill || null}
                onContextConsumed={() => (coachPrefill = "")}
            />
        </div>
    {:else if view === "profile"}
        <ProfilePanel onClose={closeProfile} />
    {:else}
        {#if refreshNotice && refreshNoticeAction}
            <div class="refresh-notice">
                <span>{refreshNotice}</span>
                <a href={refreshNoticeAction.href}>{refreshNoticeAction.label}</a>
            </div>
        {/if}
        {#each weekGroupsData as week}
            <HistoryWeekBlock
                {week}
                dayAbbrev={DAY_ABBREV}
                insightState={insightsByWeek[week.weekStart] ?? null}
                suggestionState={suggestionsByWeek[week.weekStart] ?? null}
                onOpenDay={(date) => {
                    currentDate = date;
                    setView("day");
                }}
                onToggleInsights={() =>
                    toggleInsights(week.weekStart, week.weekEnd)}
                onToggleSuggestions={() =>
                    toggleWeekSuggestions(week.weekStart, week.weekEnd)}
                onRegenerateInsights={() =>
                    fetchInsights(week.weekStart, week.weekEnd, true)}
                onRegenerateSuggestions={() =>
                    fetchWeekSuggestions(week.weekStart, week.weekEnd, true)}
                onDiscussInsights={() =>
                    discussInsight(
                        insightsByWeek[week.weekStart]?.text ?? null,
                    )}
                onDiscussSuggestions={() =>
                    discussInsight(
                        suggestionsByWeek[week.weekStart]?.text ?? null,
                    )}
            />
        {/each}
    {/if}
</div>

{#if promptOpen}
    <SystemPromptModal onClose={() => (promptOpen = false)} />
{/if}

{#if view === "day"}
<button
    class="fab"
    onclick={() => {
        drawerDate = currentDate;
        drawerMeal = null;
        drawerEditEntries = null;
        drawerEditMealType = null;
        drawerEditEvent = null;
        drawerInitialMode = null;
        drawerOpen = true;
    }}
    aria-label="Add"
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
{/if}
<ChatDrawer
    open={drawerOpen}
    onClose={closeDrawer}
    {onEntriesAdded}
    {onEntriesEdited}
    {onEventChanged}
    {onSwitchMeal}
    date={drawerDate}
    meal={drawerMeal}
    editEntries={drawerEditEntries}
    editMealType={drawerEditMealType}
    editEvent={drawerEditEvent}
    initialMode={drawerInitialMode}
/>

<style>
    .wrap {
        max-width: 640px;
        margin: 0 auto;
        padding: 0 1.25rem 6rem;
    }

    .coach-pane {
        display: flex;
        flex-direction: column;
        min-height: calc(100dvh - 8rem);
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

section {
        margin: 0.6rem 0;
    }

    .day-timeline {
        position: relative;
        padding-left: 1.25rem;
        margin: 0.5rem 0;
    }

    .day-timeline::before {
        content: "";
        position: absolute;
        /* Center on the dots: row left = padding 1.25rem; dot offset -1.05rem,
           width 0.65rem → dot center at 0.525rem from container left. */
        left: calc(0.525rem - 0.5px);
        top: 0.6rem;
        bottom: 0.6rem;
        width: 1px;
        background: var(--rule);
    }

    .tl-row {
        position: relative;
        margin: 0;
    }

    .tl-dot {
        position: absolute;
        left: -1.05rem;
        top: 50%;
        transform: translateY(-50%);
        width: 0.65rem;
        height: 0.65rem;
        border-radius: 50%;
        z-index: 1;
        box-shadow: 0 0 0 3px var(--paper);
    }

    .tl-event .event-head {
        gap: 0.4rem;
        flex-wrap: wrap;
    }

    .tl-event-time {
        min-width: 3.2rem;
    }

    .tl-event-detail {
        font-size: var(--t-body-sm);
        color: var(--ink);
        padding: 0.2rem 0;
    }

    .tl-event-notes {
        color: var(--ink-soft, var(--ink));
        opacity: 0.8;
    }

    .tl-row {
        padding: 0.4rem 0;
    }

    .event-head {
        position: relative;
        display: flex;
        align-items: center;
        gap: 0.4rem;
    }

    .event-toggle {
        background: none;
        border: none;
        font-family: inherit;
        cursor: pointer;
        display: inline-flex;
        align-items: center;
        gap: 0.45rem;
        padding: 0;
        flex: 1;
        min-width: 0;
        text-align: left;
        touch-action: manipulation;
        color: inherit;
    }

    .meal-time-input {
        font-size: 0.72rem;
        color: var(--mute-2);
        font-variant-numeric: tabular-nums;
        font-weight: 500;
        font-family: inherit;
        background: none;
        border: 1px solid var(--rule-3);
        border-radius: var(--r-pill);
        padding: 0.15rem 0.5rem;
        cursor: pointer;
        touch-action: manipulation;
        width: 3.2rem;
        min-width: 3.2rem;
        box-sizing: content-box;
        appearance: none;
        -webkit-appearance: none;
    }

    .meal-time-input::-webkit-calendar-picker-indicator {
        display: none;
        -webkit-appearance: none;
    }

    @media (hover: hover) {
        .meal-time-input:hover {
            border-color: var(--ink-2);
            color: var(--ink-2);
        }
    }

    .event-name {
        text-transform: uppercase;
        font-size: 0.72rem;
        color: var(--ink);
        letter-spacing: 0.08em;
        font-weight: 600;
    }

    .event-caret {
        color: var(--mute-3);
        font-size: 0.7rem;
    }

    @media (hover: hover) {
        .event-toggle:hover .event-name {
            color: var(--ink-2);
        }
    }

    .event-body {
        padding: 0.25rem 0 0.5rem;
    }

    .meal-insight-bubble {
        background: none;
        border: 1px solid var(--rule-3);
        border-radius: 50%;
        width: 1.65rem;
        height: 1.65rem;
        font-size: 0.85rem;
        line-height: 1;
        padding: 0;
        cursor: pointer;
        display: inline-flex;
        align-items: center;
        justify-content: center;
        flex-shrink: 0;
        transition: border-color 0.12s, background 0.12s;
    }

    .meal-insight-bubble.active {
        border-color: var(--ink-2);
        background: var(--paper-2);
    }

    @media (hover: hover) {
        .meal-insight-bubble:hover {
            border-color: var(--ink-2);
        }
    }

    .meal-insight-panel {
        margin: 0.4rem 0 0.2rem;
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

    @media (hover: hover) {
        .meal-action-btn:hover {
            border-color: var(--ink-2);
            color: var(--ink-2);
        }
    }

    .meal-macros-pill {
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
        display: inline-flex;
        gap: 0.5rem;
        transition: border-color 0.12s, color 0.12s;
    }

    .meal-macros-pill.active {
        border-color: var(--ink-2);
        color: var(--ink-2);
    }

    @media (hover: hover) {
        .meal-macros-pill:hover {
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
        background: none;
        border: none;
        padding: 0;
        font-family: inherit;
        cursor: pointer;
    }

    .refresh-notice {
        display: flex;
        align-items: center;
        justify-content: center;
        gap: 0.65rem;
        color: var(--mute);
        font-size: var(--t-meta);
        margin: -0.15rem 0 1rem;
        text-align: center;
    }

    .refresh-notice a {
        color: var(--ink-2);
        text-decoration: underline;
        text-underline-offset: 2px;
        white-space: nowrap;
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
        margin-left: auto;
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

    .suggest-row {
        display: flex;
        justify-content: center;
        margin-top: 1.25rem;
    }

    .suggest-btn {
        background: none;
        border: 1px solid var(--rule-3);
        border-radius: var(--r-pill);
        color: var(--mute);
        font-family: inherit;
        font-size: 0.78rem;
        letter-spacing: 0.02em;
        padding: 0.4rem 0.9rem;
        cursor: pointer;
        text-transform: capitalize;
        touch-action: manipulation;
        transition: border-color 0.12s, color 0.12s, background 0.12s;
    }

    @media (hover: hover) {
        .suggest-btn:hover {
            border-color: var(--ink-2);
            color: var(--ink-2);
            background: var(--paper-2);
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

    .header-actions :global(.theme-toggle) {
        font-size: 1rem;
        color: var(--mute);
        padding: 0.5rem 0.4rem;
        min-height: 2.75rem;
    }

    .gear-wrap {
        position: relative;
        display: flex;
        align-items: center;
    }

    .gear-btn {
        display: flex;
        align-items: center;
        background: none;
        border: none;
        color: var(--mute);
        padding: 0.5rem 0.4rem;
        cursor: pointer;
        touch-action: manipulation;
        min-height: 2.75rem;
        font-family: inherit;
    }

    @media (hover: hover) {
        .gear-btn:hover {
            color: var(--ink-2);
        }
    }

    .gear-menu {
        position: absolute;
        top: 100%;
        right: 0;
        margin-top: 0.35rem;
        background: var(--paper);
        border: 1px solid var(--rule);
        border-radius: var(--r-md);
        box-shadow: 0 4px 16px rgba(0, 0, 0, 0.1);
        z-index: 10;
        min-width: 180px;
        padding: 0.35rem 0;
        display: flex;
        flex-direction: column;
    }

    .gear-menu button,
    .gear-menu a {
        background: none;
        border: none;
        text-align: left;
        padding: 0.55rem 1rem;
        font-size: var(--t-body-sm);
        font-family: inherit;
        color: var(--mute);
        cursor: pointer;
        font-weight: 500;
        text-decoration: none;
        display: block;
    }

    .gear-menu button.active {
        color: var(--ink);
    }

    .gear-menu button:hover,
    .gear-menu a:hover {
        background: var(--paper-4);
        color: var(--ink);
    }

    .gear-menu hr {
        border: none;
        border-top: 1px solid var(--rule);
        margin: 0.25rem 0;
    }
</style>
