# Code Review — Agentic refactor (commits `f545c5a..1531982`)

## Overview
Big, cohesive refactor: replaced the multi-step `chat → confirm/edit` flow with a single tool-calling agent (`/api/agent`), promoted Coach to its own view, added `Events` as a first-class log type, and reworked the day view as a unified time-sorted timeline. Net +~2.2k LoC across ~30 files.

## High-impact: dead code from the migration

The old chat path is no longer in the user flow but is still wired up. `frontend/src/lib/api.ts` exports `chat()` and `editChat()` — neither is imported anywhere except `api.ts` itself. Backend mirrors:

- `internal/api/api_chat.go:28` `Chat`, `:245` `EditChat` — handlers unreferenced from frontend.
- `internal/gemini/gemini.go:280` `EditEntries`, `ParseEntries`, plus the per-user `conversations sync.Map` and `ClearConversation` — only called from `api_chat.go`.
- `router.go:107–109` routes `/api/chat` and `/api/chat/edit`.

Only `confirmChat` is still used (by `FavoritesView.svelte`). That's a single small endpoint — could be renamed `POST /api/entries` and the rest of `api_chat.go` + the conversation store + the old `responseSchema`/`systemPrompt` deleted. ~600 LoC of removable surface area.

## Correctness / risk

- **`api_agent.go:349`** — `editMeal` swallows `DeleteFood` errors (`_ = ex.svc.DeleteFood…`) but still records the IDs as removed in the agent action. A failed delete leaves the row in the sheet while the UI shows it gone. Either fail the tool call or omit the ID from `removed`.
- **`api_agent.go:502`** — `deleteEvent` mutates `ex.todaysEvents` *before* calling `svc.DeleteEvent`. If the API call fails, in-memory state diverges from the sheet for the rest of the agent loop. Flip the order.
- **`agent.go:357,391`** — `AgentStart`/`AgentContinue` append the user/tool turn to `sess.history` *before* the model call. If `agentGenerate` errors, the next call replays a history that ends on a user turn with no model reply. Either roll back on error or reset the session in the handler's error path (handler currently does this only for the iteration cap, `api_agent.go:185`).
- **`api_chat.go:233` (still live via `ConfirmChat`)** — loops `AppendFood` per entry: N Sheets writes for one favorites-log click. `AppendFoods` already exists; one call would do.

## Quality / simplification

- **`ChatDrawer.svelte` (1788 lines)** does drawer chrome, image picker, drag-to-close, agent chat, inline macro editing, AND the event form. The `mode === "event"` form (~250 lines of state + markup) is logically separate and could move to a sibling component the drawer composes — would shrink the file ~30%.
- **`LogView.svelte` (2011 lines)** has three near-identical fetch/store/toggle blocks for day insights, week insights, week suggestions (`fetchDayInsights`/`fetchInsights`/`fetchWeekSuggestions`, lines 609–825). Differ only in the API + state map. One generic `fetchPanelState(state, fetcher, regenFetcher)` would dedupe ~150 lines.
- **`api_agent.go:573–593`** — `groupByMeal` + `convertMealMap` together do `entries → map[string][]Entry`. The intermediate `sheets.FoodEntry → gemini.Entry` conversion is also done inline at `:116–127`, `:258–262`, `:311–315`. One helper.
- **`AgentEvent.Num float64`** is a true tagged-union value (minutes / ml / 1–10 score) — the comment exists at the sheets level (`sheets.go:42`) but worth repeating on `gemini.AgentEvent` since the agent prompt references it.
- **`ChatMsg`** in `ChatDrawer.svelte:27` uses `role` as a tag but leaves `text/previewUrls/action` all optional. A real tagged union (`{role: "user", text?, previewUrls} | {role: "action", action} | {role: "agent", text}`) would let TS catch missing field branches in `applyAgentAction`.

## Tests

- `api_agent_test.go` is 83 lines and exercises `resolveLogMealTime` + `defaultMealTime` only — none of the actual `agentExecutor.execute` paths (logMeal/editMeal/event tools) are covered. The pairing logic in `editMeal` (`claimByDesc`, the duplicate-description case it explicitly guards) is the kind of thing a test would lock in.

## Minor

- `router.go:35` skipper exempts `/api/chat` and `/api/agent` from the 1MB global limit, then re-applies 20MB per-route. Once `/api/chat` is dead, drop it from the skipper.
- `agentTools()` is rebuilt on every call (`api_agent.go:144,176` indirectly via `AgentStart`/`AgentContinue`). Cheap, but `var agentToolsOnce = sync.OnceValue(...)` would make it a one-time allocation.
- `api_agent.go:298` `claimByDesc` is O(n·m) per meal edit. Fine at meal scale; not worth changing.

Net: the agent migration is solid; the highest-leverage cleanup is deleting the now-unused old chat path and breaking up the two huge Svelte files.
