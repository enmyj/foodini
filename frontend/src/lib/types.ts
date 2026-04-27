export const MEAL_ORDER = [
    "breakfast",
    "lunch",
    "snack",
    "dinner",
    "supplements",
] as const;

export type MealType = (typeof MEAL_ORDER)[number];

export type RoutePath = "/" | "/about" | "/app" | "/legal";

export type ThemePreference = "system" | "dark" | "light";

export interface CoachMessage {
    role: "user" | "model";
    text: string;
}

export interface CoachChatResponse {
    message: string;
}

export interface MacroFields {
    calories: number;
    protein: number;
    carbs: number;
    fat: number;
    fiber?: number;
}

export interface Entry extends MacroFields {
    id: string;
    date: string;
    time?: string;
    description: string;
    meal_type: MealType;
}

export interface EntryInput extends MacroFields {
    description: string;
    meal_type: MealType;
}

export interface Favorite extends EntryInput {
    id: string;
}

export const EVENT_KINDS = ["workout", "stool", "water", "feeling"] as const;
export type EventKind = (typeof EVENT_KINDS)[number];

export interface LogEvent {
    id: string;
    date: string;
    time: string;
    kind: EventKind;
    text?: string;
    num?: number;
    notes?: string;
}

export interface LogResponse {
    entries: Entry[];
    events: LogEvent[];
    spreadsheet_url?: string;
    date?: string;
    start?: string;
    end?: string;
}

export type AgentActionType =
    | "meal_added"
    | "meal_edited"
    | "event_added"
    | "event_edited"
    | "event_deleted"
    | "favorite_added";

export interface AgentAction {
    type: AgentActionType;
    entries?: Entry[];
    removed_ids?: string[];
    date?: string;
    event?: LogEvent;
    event_id?: string;
}

export interface AgentResponse {
    message: string;
    actions: AgentAction[];
}

export interface EntriesResponse {
    entries: Entry[];
}

export interface InsightResponse {
    insight?: string | null;
    generated_at?: string | null;
    triggered_by?: string | null;
}

export interface MealSuggestionResponse {
    suggestion?: string | null;
    generated_at?: string | null;
}

export interface SuggestionsResponse {
    suggestions?: string | null;
    generated_at?: string | null;
}

export interface FavoritesResponse {
    favorites: Favorite[];
}

export type LogEventInput = Omit<LogEvent, "id">;

export interface Profile {
    gender?: string | null;
    birth_year?: string | null;
    height?: string | null;
    weight?: string | null;
    notes?: string | null;
    goals?: string | null;
    dietary_restrictions?: string | null;
    nutrition_expertise?: string | null;
}

export interface PendingImage {
    file: File;
    previewUrl: string;
}

export interface InsightPanelState {
    loading: boolean;
    text: string | null;
    error: string | null;
    open: boolean;
    generatedAt: string | null;
}

export interface WeekInsightPanelState extends InsightPanelState {
    loaded: boolean;
}

export interface WeekDay {
    date: string;
    future: boolean;
    entries: Entry[];
    events: LogEvent[];
}

export interface WeekGroup {
    weekStart: string;
    weekEnd: string;
    weekTotal: number;
    days: WeekDay[];
}

export type MealEntriesMap = Partial<Record<MealType, Entry[]>>;
