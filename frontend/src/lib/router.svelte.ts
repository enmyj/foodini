/** Minimal pushState router for a handful of static routes. */
import type { RoutePath } from "./types.ts";

let _listener: (() => void) | null = null;
let _current = $state<RoutePath>(window.location.pathname as RoutePath);

function navigate(path: RoutePath) {
    if (path === _current) return;
    history.pushState(null, "", path);
    _current = path;
}

function init() {
    if (_listener) return;
    _listener = () => { _current = window.location.pathname as RoutePath; };
    window.addEventListener("popstate", _listener);
}

function getCurrent(): RoutePath {
    return _current;
}

export { navigate, init, getCurrent };
