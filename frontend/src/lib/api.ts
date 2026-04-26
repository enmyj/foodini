import type {
    ActivityPayload,
    ActivityResponse,
    AgentResponse,
    ChatParseResponse,
    CoachChatResponse,
    CoachMessage,
    EntriesResponse,
    Entry,
    EntryInput,
    Favorite,
    FavoritesResponse,
    InsightResponse,
    LogResponse,
    MealSuggestionResponse,
    MealType,
    Profile,
    SuggestionsResponse,
} from "./types.ts";

export interface ApiError extends Error {
    status: number;
    code: string | null;
    body: unknown;
    detail: string | null;
    userMessage: string;
}

const TZ = Intl.DateTimeFormat().resolvedOptions().timeZone;
const SAFE_ERROR_MESSAGES: Record<string, string> = {
    session_expired: "Your session expired. Sign in again.",
    insufficient_scopes:
        "Google permissions are missing. Re-authorize to continue.",
    upload_too_large:
        "Photos are too large for one request. Try fewer photos and send again.",
    favorite_exists: "That favorite already exists.",
};

function isRecord(value: unknown): value is Record<string, unknown> {
    return typeof value === "object" && value !== null;
}

async function throwResponseError(res: Response): Promise<never> {
    const contentType = res.headers.get("content-type") ?? "";
    let body: unknown = null;
    let text = "";

    if (contentType.includes("application/json")) {
        body = await res.json().catch(() => null);
    } else {
        text = await res.text();
    }

    const jsonBody = isRecord(body) ? body : null;
    const rawError = jsonBody?.error;
    const code =
        typeof rawError === "string" && rawError.trim() ? rawError.trim() : "";
    const err = new Error(
        SAFE_ERROR_MESSAGES[code] || `Request failed (${res.status})`,
    ) as ApiError;
    err.status = res.status;
    err.code = code || null;
    err.body = body;
    err.detail =
        code && text
            ? text
            : typeof rawError === "string" && rawError
              ? rawError
              : text || null;
    err.userMessage = SAFE_ERROR_MESSAGES[code] || "";
    throw err;
}

async function apiFetch(url: string, init: RequestInit = {}): Promise<Response> {
    const headers = new Headers(init.headers);
    headers.set("X-Timezone", TZ);

    const res = await fetch(url, {
        ...init,
        headers,
    });
    if (!res.ok) await throwResponseError(res);
    return res;
}

async function apiFetchJson<T>(
    url: string,
    init: RequestInit = {},
): Promise<T> {
    const res = await apiFetch(url, init);
    return res.json() as Promise<T>;
}

interface GetLogOptions {
    date?: string | null;
    days?: number | null;
}

export async function getLog(
    { date = null, days = null }: GetLogOptions = {},
): Promise<LogResponse> {
    const params = days ? `?days=${days}` : date ? `?date=${date}` : "";
    return apiFetchJson<LogResponse>(`/api/log${params}`);
}

export async function chat(
    message: string | null,
    date: string | null = null,
    images: File[] | null = null,
    meal: MealType | null = null,
): Promise<ChatParseResponse> {
    if (images?.length) {
        const body = new FormData();
        body.append("message", message ?? "");
        if (date) body.append("date", date);
        if (meal) body.append("meal", meal);
        for (const image of images) {
            body.append("images", image);
        }
        return apiFetchJson<ChatParseResponse>("/api/chat", {
            method: "POST",
            body,
        });
    }

    const body: { message: string | null; date?: string; meal?: MealType } = {
        message,
    };
    if (date) body.date = date;
    if (meal) body.meal = meal;
    return apiFetchJson<ChatParseResponse>("/api/chat", {
        method: "POST",
        headers: { "Content-Type": "application/json" },
        body: JSON.stringify(body),
    });
}

export async function agent(
    message: string | null,
    options: {
        date?: string | null;
        meal?: MealType | null;
        currentEntries?: Entry[] | null;
        images?: File[] | null;
        reset?: boolean;
    } = {},
): Promise<AgentResponse> {
    const { date = null, meal = null, currentEntries = null, images = null, reset = false } = options;
    if (images?.length) {
        const body = new FormData();
        body.append("message", message ?? "");
        if (date) body.append("date", date);
        if (meal) body.append("meal", meal);
        if (reset) body.append("reset", "true");
        if (currentEntries) body.append("current_entries", JSON.stringify(currentEntries));
        for (const image of images) body.append("images", image);
        return apiFetchJson<AgentResponse>("/api/agent", { method: "POST", body });
    }
    const body: {
        message: string | null;
        date?: string;
        meal?: MealType;
        current_entries?: Entry[];
        reset?: boolean;
    } = { message };
    if (date) body.date = date;
    if (meal) body.meal = meal;
    if (currentEntries) body.current_entries = currentEntries;
    if (reset) body.reset = true;
    return apiFetchJson<AgentResponse>("/api/agent", {
        method: "POST",
        headers: { "Content-Type": "application/json" },
        body: JSON.stringify(body),
    });
}

export async function confirmChat(
    entries: EntryInput[],
    date: string | null = null,
): Promise<EntriesResponse> {
    const body: { entries: EntryInput[]; date?: string } = { entries };
    if (date) body.date = date;
    return apiFetchJson<EntriesResponse>("/api/chat/confirm", {
        method: "POST",
        headers: { "Content-Type": "application/json" },
        body: JSON.stringify(body),
    });
}

export async function editChat(
    message: string,
    entries: Entry[],
    date: string | null = null,
    mealType: MealType | null = null,
): Promise<EntriesResponse> {
    const body: {
        message: string;
        entries: Entry[];
        date?: string;
        meal_type?: MealType;
    } = { message, entries };
    if (date) body.date = date;
    if (mealType) body.meal_type = mealType;
    return apiFetchJson<EntriesResponse>("/api/chat/edit", {
        method: "POST",
        headers: { "Content-Type": "application/json" },
        body: JSON.stringify(body),
    });
}

export async function coachChat(
    messages: CoachMessage[],
    date: string | null = null,
    days: number | null = null,
): Promise<CoachChatResponse> {
    const body: { messages: CoachMessage[]; date?: string; days?: number } = { messages };
    if (date) body.date = date;
    if (days) body.days = days;
    return apiFetchJson<CoachChatResponse>("/api/coach/chat", {
        method: "POST",
        headers: { "Content-Type": "application/json" },
        body: JSON.stringify(body),
    });
}

export async function* coachChatStream(
    messages: CoachMessage[],
    date: string | null = null,
    days: number | null = null,
): AsyncGenerator<string, void, void> {
    const body: { messages: CoachMessage[]; date?: string; days?: number } = { messages };
    if (date) body.date = date;
    if (days) body.days = days;
    const res = await apiFetch("/api/coach/chat", {
        method: "POST",
        headers: { "Content-Type": "application/json", Accept: "text/event-stream" },
        body: JSON.stringify(body),
    });
    if (!res.body) return;
    const reader = res.body.getReader();
    const decoder = new TextDecoder();
    let buf = "";
    while (true) {
        const { value, done } = await reader.read();
        if (done) break;
        buf += decoder.decode(value, { stream: true });
        let idx: number;
        while ((idx = buf.indexOf("\n\n")) !== -1) {
            const raw = buf.slice(0, idx);
            buf = buf.slice(idx + 2);
            let event = "";
            const dataLines: string[] = [];
            for (const line of raw.split("\n")) {
                if (line.startsWith("event:")) event = line.slice(6).trim();
                else if (line.startsWith("data:")) dataLines.push(line.slice(5).trimStart());
            }
            if (!dataLines.length) continue;
            const data = dataLines.join("\n");
            if (event === "done") return;
            if (event === "error") {
                let msg = "stream error";
                try {
                    const parsed = JSON.parse(data) as { error?: string };
                    if (parsed.error) msg = parsed.error;
                } catch {}
                throw new Error(msg);
            }
            try {
                const parsed = JSON.parse(data) as { text?: string };
                if (parsed.text) yield parsed.text;
            } catch {}
        }
    }
}

export async function fetchMealSuggestion(
    date: string,
    meal: MealType,
): Promise<MealSuggestionResponse> {
    return apiFetchJson<MealSuggestionResponse>(
        `/api/suggestions/meal?date=${date}&meal=${meal}`,
    );
}

export async function generateMealSuggestion(
    date: string,
    meal: MealType,
): Promise<MealSuggestionResponse> {
    return apiFetchJson<MealSuggestionResponse>("/api/suggestions/meal", {
        method: "POST",
        headers: { "Content-Type": "application/json" },
        body: JSON.stringify({ date, meal }),
    });
}

export async function patchEntry(
    id: string,
    entry: Partial<Entry>,
): Promise<Entry> {
    return apiFetchJson<Entry>(`/api/entries/${id}`, {
        method: "PATCH",
        headers: { "Content-Type": "application/json" },
        body: JSON.stringify(entry),
    });
}

export async function deleteEntry(id: string): Promise<void> {
    await apiFetch(`/api/entries/${id}`, { method: "DELETE" });
}

export async function getActivity(date: string): Promise<ActivityResponse> {
    return apiFetchJson<ActivityResponse>(`/api/activity?date=${date}`);
}

export async function putActivity(
    date: string,
    payload: ActivityPayload,
): Promise<ActivityResponse> {
    return apiFetchJson<ActivityResponse>("/api/activity", {
        method: "PUT",
        headers: { "Content-Type": "application/json" },
        body: JSON.stringify({ date, ...payload }),
    });
}

export async function fetchStoredDayInsight(
    date: string,
): Promise<InsightResponse> {
    return apiFetchJson<InsightResponse>(`/api/insights/day?date=${date}`);
}

export async function generateDayInsights(
    date: string,
): Promise<InsightResponse> {
    return apiFetchJson<InsightResponse>("/api/insights/day", {
        method: "POST",
        headers: { "Content-Type": "application/json" },
        body: JSON.stringify({ date }),
    });
}

export interface InsightSnapshot {
    triggered_by: string;
    generated_at: string;
}

export async function fetchInsightSnapshots(
    date: string,
): Promise<{ snapshots: InsightSnapshot[] }> {
    return apiFetchJson<{ snapshots: InsightSnapshot[] }>(
        `/api/insights/snapshots?date=${date}`,
    );
}

export async function fetchInsightByTrigger(
    id: string,
): Promise<InsightResponse> {
    return apiFetchJson<InsightResponse>(
        `/api/insights/by-trigger?id=${encodeURIComponent(id)}`,
    );
}

export async function fetchStoredInsight(
    start: string,
    end: string,
): Promise<InsightResponse> {
    return apiFetchJson<InsightResponse>(`/api/insights?start=${start}&end=${end}`);
}

export async function generateInsights(
    start: string,
    end: string,
): Promise<InsightResponse> {
    return apiFetchJson<InsightResponse>("/api/insights", {
        method: "POST",
        headers: { "Content-Type": "application/json" },
        body: JSON.stringify({ start, end }),
    });
}

export async function fetchStoredDaySuggestions(
    date: string,
): Promise<SuggestionsResponse> {
    return apiFetchJson<SuggestionsResponse>(`/api/suggestions/day?date=${date}`);
}

export async function generateDaySuggestions(
    date: string,
): Promise<SuggestionsResponse> {
    return apiFetchJson<SuggestionsResponse>("/api/suggestions/day", {
        method: "POST",
        headers: { "Content-Type": "application/json" },
        body: JSON.stringify({ date }),
    });
}

export async function fetchStoredWeekSuggestions(
    start: string,
    end: string,
): Promise<SuggestionsResponse> {
    return apiFetchJson<SuggestionsResponse>(
        `/api/suggestions/week?start=${start}&end=${end}`,
    );
}

export async function generateWeekSuggestions(
    start: string,
    end: string,
): Promise<SuggestionsResponse> {
    return apiFetchJson<SuggestionsResponse>("/api/suggestions/week", {
        method: "POST",
        headers: { "Content-Type": "application/json" },
        body: JSON.stringify({ start, end }),
    });
}

export async function getFavorites(): Promise<FavoritesResponse> {
    return apiFetchJson<FavoritesResponse>("/api/favorites");
}

export async function addFavorite(entry: EntryInput): Promise<Favorite> {
    return apiFetchJson<Favorite>("/api/favorites", {
        method: "POST",
        headers: { "Content-Type": "application/json" },
        body: JSON.stringify({
            description: entry.description,
            meal_type: entry.meal_type,
            calories: entry.calories,
            protein: entry.protein,
            carbs: entry.carbs,
            fat: entry.fat,
            fiber: entry.fiber ?? 0,
        }),
    });
}

export async function deleteFavorite(id: string): Promise<void> {
    await apiFetch(`/api/favorites/${id}`, { method: "DELETE" });
}

export async function getProfile(): Promise<Profile> {
    return apiFetchJson<Profile>("/api/profile");
}

export async function putProfile(profile: Profile): Promise<Profile> {
    return apiFetchJson<Profile>("/api/profile", {
        method: "PUT",
        headers: { "Content-Type": "application/json" },
        body: JSON.stringify(profile),
    });
}
