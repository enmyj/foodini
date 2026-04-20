import type { ThemePreference } from "./types.ts";

const STORAGE_KEY = "foodini-theme";
const THEME_ORDER: ThemePreference[] = ["system", "dark", "light"];

let current = $state<ThemePreference>(load());

function isThemePreference(value: string | null): value is ThemePreference {
    return value === "system" || value === "dark" || value === "light";
}

function load(): ThemePreference {
    if (typeof localStorage === "undefined") return "system";
    const stored = localStorage.getItem(STORAGE_KEY);
    return isThemePreference(stored) ? stored : "system";
}

function apply(pref: ThemePreference) {
    const root = document.documentElement;
    root.classList.remove("light", "dark");
    if (pref === "light" || pref === "dark") {
        root.classList.add(pref);
    }
    // "system" = no class, CSS media query takes over
}

export function initTheme() {
    apply(current);
}

export function getTheme(): ThemePreference {
    return current;
}

export function cycleTheme(): ThemePreference {
    const next = THEME_ORDER[(THEME_ORDER.indexOf(current) + 1) % THEME_ORDER.length]!;
    current = next;
    localStorage.setItem(STORAGE_KEY, next);
    apply(next);
    return next;
}
