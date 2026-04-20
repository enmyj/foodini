/** Minimal pushState router for a handful of static routes. */

let _listener: (() => void) | null = null;
let _current = $state(window.location.pathname);

function navigate(path: string) {
    if (path === _current) return;
    history.pushState(null, "", path);
    _current = path;
}

function init() {
    if (_listener) return;
    _listener = () => { _current = window.location.pathname; };
    window.addEventListener("popstate", _listener);
}

function getCurrent() {
    return _current;
}

export { navigate, init, getCurrent };
