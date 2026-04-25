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

export type DrawerTab = "food" | "activity" | "coach";

export interface CoachMessage {
    role: "user" | "model";
    text: string;
}

export interface CoachChatResponse {
    message: string;
}

export type ActivityField = "activity" | "feeling" | "poop" | "hydration";

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

export interface DailyLog {
    date: string;
    activity?: string | null;
    feeling_score?: number | null;
    feeling_notes?: string | null;
    poop?: boolean | null;
    poop_notes?: string | null;
    hydration?: number | null;
}

export interface LogResponse {
    entries: Entry[];
    daily_logs: DailyLog[];
    spreadsheet_url?: string;
}

export interface ChatParseResponse {
    done: boolean;
    entries?: Entry[];
    message?: string | null;
}

export type AgentActionType =
    | "meal_added"
    | "meal_edited"
    | "activity_updated"
    | "stool_logged"
    | "favorite_added";

export interface AgentAction {
    type: AgentActionType;
    entries?: Entry[];
    removed_ids?: string[];
    date?: string;
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

export interface ActivityPayload {
    activity: string;
    feeling_score: number;
    feeling_notes: string;
    poop: boolean;
    poop_notes: string;
    hydration: number;
}

export type ActivityResponse = Partial<DailyLog>;

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
    dayLog: DailyLog | null;
}

export interface WeekGroup {
    weekStart: string;
    weekEnd: string;
    weekTotal: number;
    days: WeekDay[];
}

export type MealEntriesMap = Partial<Record<MealType, Entry[]>>;
