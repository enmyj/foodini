import type {
    Entry,
    FavoritesResponse,
    LogResponse,
    MealType,
} from "./types.ts";

export function updateEntryInLogCache(
    log: LogResponse | undefined,
    updated: Entry,
): LogResponse | undefined {
    if (!log) return log;
    return {
        ...log,
        entries: log.entries.map((entry) =>
            entry.id === updated.id ? updated : entry,
        ),
    };
}

export function replaceMealEntriesInLogCache(
    log: LogResponse | undefined,
    mealType: MealType | null,
    updatedEntries: Entry[],
): LogResponse | undefined {
    if (!log) return log;
    const editedIds = new Set(updatedEntries.map((entry) => entry.id));
    const kept = log.entries.filter(
        (entry) => entry.meal_type !== mealType || editedIds.has(entry.id),
    );
    const existingIds = new Set(kept.map((entry) => entry.id));
    const merged = kept.map(
        (entry) =>
            updatedEntries.find((updated) => updated.id === entry.id) ?? entry,
    );
    const newEntries = updatedEntries.filter(
        (entry) => !existingIds.has(entry.id),
    );
    return { ...log, entries: [...merged, ...newEntries] };
}

export function removeEntryFromLogCache(
    log: LogResponse | undefined,
    id: string,
): LogResponse | undefined {
    if (!log) return log;
    return {
        ...log,
        entries: log.entries.filter((entry) => entry.id !== id),
    };
}

export function appendEntriesToLogCache(
    log: LogResponse | undefined,
    newEntries: Entry[],
): LogResponse | undefined {
    if (!log) {
        return {
            entries: newEntries,
            events: [],
        };
    }
    return {
        ...log,
        entries: [...(log.entries ?? []), ...newEntries],
    };
}

export function removeFavoriteFromCache(
    data: FavoritesResponse | undefined,
    id: string,
): FavoritesResponse | undefined {
    if (!data) return data;
    return {
        ...data,
        favorites: data.favorites.filter((favorite) => favorite.id !== id),
    };
}
