# Design: Client-Side Timezone

## Overview

The backend currently uses `time.Now()` (server time, UTC) to determine "today" and to record food entry timestamps. If the server is UTC and the user is in US/Eastern, entries logged after midnight UTC but before midnight local time are stamped to the wrong date. Fix by sending the user's IANA timezone from the client on every request.

## Frontend

`api.js` reads the timezone once at module level:

```js
const TZ = Intl.DateTimeFormat().resolvedOptions().timeZone
```

Every `fetch` call adds an `X-Timezone` header:

```js
headers: { 'Content-Type': 'application/json', 'X-Timezone': TZ }
```

GET requests (no body) also get the header added.

## Backend

New helper in `api.go`:

```go
func localNow(r *http.Request) time.Time {
    tz := r.Header.Get("X-Timezone")
    if tz != "" {
        if loc, err := time.LoadLocation(tz); err == nil {
            return time.Now().In(loc)
        }
    }
    return time.Now() // fallback: server time
}
```

All `time.Now()` calls in `api.go` that determine "today" or record entry time replace with `localNow(r)`:

- `GetLog`: `today := sheets.DateString(localNow(r))`
- `GetLog` (range): `start := sheets.DateString(localNow(r).AddDate(...))`
- `Chat`: `targetDate = sheets.DateString(localNow(r))`
- `ConfirmChat`: `targetDate = sheets.DateString(localNow(r))` and `now := localNow(r)` for the time field
- `GetActivity`: default date
- `PutActivity`: default date

No changes to the `sheets` package — timezone logic lives entirely in the API layer.
