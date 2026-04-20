const STORAGE_KEY = "foodini-theme";

let current = $state(load());

function load() {
    if (typeof localStorage === "undefined") return "system";
    return localStorage.getItem(STORAGE_KEY) || "system";
}

function apply(pref: string) {
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

export function getTheme() {
    return current;
}

export function cycleTheme() {
    const order = ["system", "dark", "light"];
    const next = order[(order.indexOf(current) + 1) % order.length];
    current = next;
    localStorage.setItem(STORAGE_KEY, next);
    apply(next);
    return next;
}
