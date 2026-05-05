# Activity Fueling — Design Note

Date: 2026-05-04

## Problem

Endurance athletes (cyclists especially) consume carbs *during* hard
activity — gels, drink mix, chews, bananas — at rates that look insane in
a normal-diet context. Current research supports **60–90 g/hr for hard
efforts >2.5 hr**, with **90–120 g/hr** for racing/Tour-level work using a
2:1 glucose:fructose blend. Trained guts can sustain this without GI
issues after a few weeks of "gut training".

This fuel is qualitatively different from regular eating:

- It's metabolized in near-real-time by working muscle, not stored.
- "100 g of pure sugar" is *correct nutrition* mid-ride, but reading it
  back as part of a daily diet would make the day's macros and the
  coach's advice both look wrong.
- It distorts insights: a 4-hour ride with proper fueling adds ~1500
  carb-only kcal that aren't representative of how the user actually
  eats. The coach should not nag the user for "too much sugar" or
  "imbalanced macros" on those days.

## Goal

Let the user log mid-activity fuel as a distinct category that:

1. Is excluded from "diet" insights / macro-balance / sugar warnings.
2. Is still visible in the daily log so the user can see what they ate.
3. Gets its own simple analysis: "did you fuel adequately for the
   duration & intensity?" (target g/hr × hours).
4. Gets counted in total-energy-balance views (it *is* calories
   consumed) but separated in macro-quality views.

## How it fits this codebase

This app has no separate Activity sheet — workouts already live in the
`Events` sheet as rows with `kind = "workout"` (`internal/sheets/sheets.go:34`).
A workout row's columns are:

- `Text` — description ("morning ride", "Z2 spin")
- `Num` — reserved for duration in minutes, but **not currently
  populated by the UI**. `EventForm.svelte:168-172` only renders a
  free-text "What did you do?" textarea, so duration today is buried in
  the description ("30min run") rather than structured. The agent's
  `log_event` tool is the only writer that uses `num` for workouts, and
  even then sporadically.
- `Notes` — free notes (also unused by the workout form today).

Fueling is anchored to a workout event, so the FK target is the event
ID, not a hypothetical Activity row. We reuse existing workout events
instead of introducing a parallel concept.

### Workouts need structured duration before fueling pays off

The whole point of the fueling feature is the g/hr ratio: `sum(carbs_g)
/ duration_hours`. Without structured duration, the coach can't
evaluate adequacy and the day-log can't render the `g/hr` chip. So
**v1 must promote duration to a real input** on the workout form:

- Add a number input ("Duration (min)") in the `workout` branch of
  `EventForm.svelte`, bound to the existing `num` field. Optional —
  but if present, used everywhere fueling math runs.
- Backfill is out of scope. Past workout rows with `num=0` simply
  don't get a g/hr target and render fueling as a flat list under
  them.

`Intensity` stays **not** a column on events. The Events schema is
deliberately narrow (`id|date|time|kind|text|num|notes`) and adding a
typed enum would force another schema bump with thin payoff. Intensity
is inferred for the g/hr target via a keyword scan of `text` + `notes`
("race", "hard", "easy", "z2"). If the heuristic is wrong often, revisit
in v2.

### Strava / Apple Health integration

Tabled. The app has no secret-storage path today (Google OAuth refresh
tokens live in an encrypted cookie, but a Strava client_secret + per-user
refresh token would need server-side storage — KMS, a managed secret
store, or a database). Worth revisiting once that infrastructure
exists; until then, manual workout entry with a duration field is the
only path.

## Data model

### Sheets schema (additive, schema bump 12 → 13)

New sheet: `Fueling`

| Col | Header        | Notes |
|-----|---------------|-------|
| A   | id            | uuid |
| B   | date          | YYYY-MM-DD |
| C   | time          | HH:MM (optional, for in-ride pacing) |
| D   | event_id      | FK → row in `Events` sheet where `kind=workout` |
| E   | description   | "SIS Beta Fuel gel", "20 oz Skratch", "banana" |
| F   | carbs_g       | required — the dominant nutrient |
| G   | calories      | optional, defaults to `carbs_g * 4` |
| H   | sodium_mg     | optional, useful for hot rides |
| I   | source        | enum: `gel` `drink` `chew` `bar` `whole_food` `other` |

### Migration

Bump `CurrentSchemaVersion` from 12 to 13 in
`internal/sheets/sheets.go`. Register a step in `runMigrations`
(`internal/api/api.go:317`) that calls a new
`sheets.MigrateV12toV13(ctx, ts, spreadsheetID)` which:

1. Adds a `Fueling` sheet via `BatchUpdate` `AddSheet` request.
2. Writes the header row.
3. Updates `Meta!A2` to `13`.

`CreateSpreadsheet` also gains the `Fueling` sheet + headers so newly
created spreadsheets land at v13 directly.

## Backend

### `internal/sheets/sheets.go` (extend, no new file)

Following the existing single-file convention. Add:

- `const fuelingSheet = "Fueling"`
- `FuelSource` constants: `FuelSourceGel`, `FuelSourceDrink`,
  `FuelSourceChew`, `FuelSourceBar`, `FuelSourceWholeFood`,
  `FuelSourceOther`.
- `type FuelingEntry struct { ID, Date, Time, EventID, Description string;
  CarbsG, Calories, SodiumMg int; Source string }` with `ToRow` /
  `FuelingEntryFromRow` mirroring `FoodEntry`.
- Methods on `*Service`:
  - `AppendFueling(ctx, FuelingEntry) error`
  - `GetFuelingByDate(ctx, date) ([]FuelingEntry, error)`
  - `GetFuelingByEventID(ctx, eventID) ([]FuelingEntry, error)`
  - `GetFuelingByDateRange(ctx, start, end) ([]FuelingEntry, error)`
  - `UpdateFueling(ctx, id, FuelingEntry) error`
  - `DeleteFueling(ctx, id) error`

Mirror the patterns already used for `FoodEntry` and `Event` (range
`A:I`, `BatchUpdate` for delete, header skip on row 0, return `nil, nil`
on `googleapi.Error` 400 for "sheet doesn't exist yet" defensiveness).

### `internal/api/api_fueling.go` (new file)

Routes added to the `apiGroup` in `router.go` (next to `/events`):

- `POST   /api/fueling`         → `PostFueling`
- `GET    /api/fueling?date=…`  → `GetFueling` (also accepts
  `event_id=…` for "fueling for one ride")
- `PATCH  /api/fueling/:id`     → `PatchFueling`
- `DELETE /api/fueling/:id`     → `DeleteFueling`

Same shape as `api_events.go`: pull session, decode body, default
date/time from `LocalNow(r)`, generate UUID if missing, call sheets,
`cacheInvalidate`, return JSON. `event_id` is required on POST and must
correspond to a row in `Events` with `kind=workout` (validated by a
single `GetEventsByDate` lookup or by trusting the client and letting
the row dangle — v1 trusts the client).

### `/api/log` is the primary read path

`GetLog` (`internal/api/api_log.go:15`) already bundles `entries` +
`events` into a single `LogResponse` per day/range. **Add `fueling` to
that response** so the day view doesn't need a second round-trip:

```go
"entries": entries,
"events":  events,
"fueling": fueling, // new: []sheets.FuelingEntry for the same date(s)
```

The standalone `GET /api/fueling` endpoint stays useful for the agent
tool flow ("show me what I've fueled with on this ride") and for the
deferred in-ride logger, but day rendering reads from `/api/log`.

### Coach / insights changes

Two separate buckets when summarizing a day for the model
(`internal/gemini/gemini.go` — the `dayInsightsSystemPrompt` block and
the day-summary builder that feeds it):

```
Diet (regular meals & snacks):
  - calories, macros, food list...

Activity fueling (excluded from diet quality analysis):
  - 90 min ride: 90 g carbs over 1:30 = 60 g/hr  [target 60–90]
  - notes: 1 gel, 1 bottle drink mix
```

Add a paragraph to the model-facing prompts in
`internal/gemini/gemini.go`: `dayInsightsSystemPrompt` (single day),
`insightsSystemPrompt` (weekly), and the meal-suggestion prompts
(`mealSuggestionsSystemPrompt`, `weekMealSuggestionsSystemPrompt`) so
suggestions don't include "more carb gels" as a meal idea. The exported
`InsightsSystemPrompt` constant is only the settings-page display copy
— mirror the addition there too so users can see how the model is
guided. Sample copy (companion to the existing profile context block):

> Activity fueling is reported separately from the regular diet. **Do
> not** critique its macro composition (high sugar, low protein, low
> fiber are expected and correct mid-effort). When evaluating fueling,
> use a target of ~60 g/hr for moderate, 80–100 g/hr for hard >2 hr,
> 90–120 g/hr for race-pace efforts >2.5 hr. Flag *under-fueling* if
> intake is below the lower bound, not over-consumption of sugar.
> Activity fueling calories *do* count toward total daily energy
> balance, but not toward "diet quality" or macro-ratio assessments.

The day-summary serializer should include a `fueling:` section grouped
by workout event ID, with computed g/hr (`sum(carbs_g) / (event.num /
60)`).

The agent (`internal/gemini/agent.go`) gets one new tool declaration in
`agentTools()`:

- `add_fueling { event_id, description, carbs_g, calories?, sodium_mg?,
  source?, time? }`

Routed in `internal/api/api_agent.go` next to the existing `log_event`
handler, calling `svc.AppendFueling`. The agent prompt
(`agentSystemPrompt` in `agent.go:24`) gains one bullet describing when
to call it, and the rule that if the day has multiple workout events the
agent must ask which one (or use the most recent if it's the only one
"in progress").

`AgentSession` (`agent.go:248`) currently exposes `TodaysEvents` to the
model — add a parallel `TodaysFueling []AgentFueling` field so the model
can see what's already logged when the user says "and another gel".

## Frontend

### Logging UX

Two entry points:

1. **From the day log**, on workout `EventForm` rows
   (`frontend/src/lib/EventForm.svelte`): a `+ Fuel` affordance opens a
   fueling sub-form. Pre-binds `event_id`. Reuses existing form
   primitives.
2. **From the chat drawer** (`ChatDrawer.svelte`), natural language:
   *"on the ride I had two Maurten 160s and a bottle of LMNT"* → agent
   calls `add_fueling` against the most recent workout event.

A third surface (later) is a quick **in-ride logger** — a stripped-down
view (big tap targets, "+1 gel" / "+1 bottle" buttons backed by the
existing favorites system) for use mid-activity. **Out of scope for
v1.**

### Day log rendering (`LogView.svelte`)

Fueling rows render under their parent workout event row, visually
distinct from meals (different left rail, monospace `g/hr` chip). They
are **not** mixed into the meals stream and **not** included in the
day's macro ring.

Day totals show two lines (in the existing summary block):

```
Diet:    2200 kcal · 145P 240C 70F
Fueling: 480 kcal · 0P 120C 0F   (90 min ride · 60 g/hr)
```

The macro ring / "balance" widget uses *Diet* only. The "total energy"
view sums both.

Day fueling rows arrive in the existing `LogResponse` (under a new
`fueling` field — see `frontend/src/lib/types.ts` for the shape, and
`cache.ts` `appendEntriesToLogCache` already initializes `events: []` —
add `fueling: []` there too). Mutations (`addFueling`, `updateFueling`,
`deleteFueling`) need cache helpers in `cache.ts` parallel to the entry
helpers (`updateEntryInLogCache`, `removeEntryFromLogCache`,
`appendEntriesToLogCache`). `queryKeys.ts` currently has `events:
(date)` — add `fueling: (date)` and a `fuelingByEvent: (eventID)` key
for the standalone fetch path used by the agent / in-ride logger.

## Open questions

- **Pre-ride breakfast and post-ride recovery meal**: these are still
  diet, even though they're activity-adjacent. Don't try to be clever —
  only things explicitly logged as fueling get the separate treatment.
- **Walks / easy zone-2**: probably no fueling needed; UI shouldn't push
  it. Only show the `+ Fuel` affordance when the workout's `num`
  (duration_min) is >60, or the user opts in.
- **Intensity**: deliberately *not* a column on events for v1. The g/hr
  target uses duration + a keyword scan of the workout `Notes` ("race",
  "hard", "z2", "easy"). If users frequently override the target, add
  an explicit `intensity` event-notes convention or a small migration
  later.
- **Hydration**: `water` events already exist
  (`EventKindWater`, `Num=ml`). Sodium tracked here (per-fuel-row) is
  *enough* for v1; broader hydration analysis is its own design.
- **Apple Health / Strava import**: blocked on secret storage (no
  server-side store for per-user refresh tokens / app client secrets
  yet). Revisit once that infra exists.

## Phasing

**v1 (small, ships fast):**
- Schema migration v12 → v13: add `Fueling` sheet.
- `sheets.FuelingEntry` + CRUD methods.
- `/api/fueling` REST endpoints + `fueling` field on `LogResponse`.
- **Duration input on workout `EventForm`** — prerequisite for g/hr.
- `add_fueling` agent tool + prompt update.
- Coach prompt update (separate bucket + g/hr targets).
- Day-log: render fueling rows under workout, separate totals line.

**v2:**
- In-ride quick logger view.
- Saved favorites for fuel items (Maurten 160, SIS gel, etc.) — extend
  the existing `Favorites` sheet or branch a `FuelFavorites` sibling.
- Adequacy summary post-ride: "you fueled at 65 g/hr; target was
  80–100".

**v3:**
- First-class hydration analysis (combining `water` events + fuel
  sodium).
- Strava / Apple Health workout sync — blocked on a secret-storage
  story (no server-side place for app client secrets + per-user
  refresh tokens today).

## Sources

- [90 vs 120 g/hr — EndureIQ](https://www.endureiq.com/blog/carbohydrate-ingestion-rates-during-exercise-90-vs-120-grams-per-hour-which-is-best)
- [EF Pro Cycling — gut training](https://www.efprocycling.com/tips-recipes/tour-de-france-tips-gut-training/)
- [Precision Hydration — carbs per hour](https://www.precisionhydration.com/performance-advice/nutrition/how-much-carbohydrate-carbs-athletes-per-hour/)
- [CTS — carbs per hour on the bike](https://trainright.com/how-many-carbohydrates-per-hour-on-the-bike/)
