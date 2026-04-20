export const queryKeys = {
    logBase: ["log"] as const,
    logDay: (date: string) => ["log", date] as const,
    logHistory: (weeks: number) => ["log", "history", weeks] as const,
    favorites: ["favorites"] as const,
    profile: ["profile"] as const,
    activity: (date: string, refreshKey = 0) =>
        ["activity", date, refreshKey] as const,
};
