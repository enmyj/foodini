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

## Data model

Activity fueling is anchored to an **activity session**, not a meal. It
needs the activity to exist first.

### Sheets schema (additive, schema bump)

New sheet: `Fueling`

| Column | Type | Notes |
|---|---|---|
| ID | string | uuid |
| Date | YYYY-MM-DD | |
| ActivityID | string | FK → Activity row (existing Activity sheet) |
| Time | HH:MM | when consumed (optional, for in-ride pacing view) |
| Description | string | "SIS Beta Fuel gel", "20 oz Skratch", "banana" |
| Carbs_g | number | dominant nutrient — required |
| Calories | number | optional, defaults to carbs*4 |
| Sodium_mg | number | optional, useful for hot rides |
| Source | enum | `gel` `drink` `chew` `bar` `whole_food` `other` |

Activity sheet gains two columns (or we compute on read):

- `DurationMin` (already there or add)
- `Intensity` enum: `easy` `moderate` `hard` `race` — drives the g/hr
  target.

Bump `CurrentSchemaVersion` and register a migration that creates the
`Fueling` sheet and adds the two activity columns.

## Backend

`internal/sheets/fueling.go` — CRUD mirroring `sheets/foodentry.go`:
`AppendFueling`, `ListFueling(date)`, `ListFuelingByActivity(id)`,
`UpdateFueling`, `DeleteFueling`.

`internal/api/api_fueling.go`:

- `POST /api/fueling` — create
- `GET  /api/fueling?date=YYYY-MM-DD` — list (often joined with activity
  in the response)
- `PATCH /api/fueling/:id`
- `DELETE /api/fueling/:id`

### Insights / coach changes

Two separate buckets when summarizing a day for the model:

```
Diet (regular meals & snacks):
  - calories, macros, food list...

Activity fueling (excluded from diet quality analysis):
  - 3 hr ride @ hard: 240 g carbs over 3:05 = 78 g/hr  [target 80–100]
  - notes: 1 gel, 2x 750ml drink mix, 1 banana
```

System-prompt addition (companion to the existing portion-sizing block):

> Activity fueling is reported separately from the regular diet. **Do
> not** critique its macro composition (high sugar, low protein, low
> fiber are expected and correct mid-effort). When evaluating fueling,
> use a target of ~60 g/hr for moderate, 80–100 g/hr for hard >2 hr,
> 90–120 g/hr for race-pace efforts >2.5 hr. Flag *under-fueling* if
> intake is below the lower bound, not over-consumption of sugar.
> Activity fueling calories *do* count toward total daily energy
> balance, but not toward "diet quality" or macro-ratio assessments.

The chat agent gets a new tool: `add_fueling { activity_id, description,
carbs_g, ... }`. The agent should infer/ask which activity it goes with
if there are multiple in the day.

## Frontend

### Logging UX

Two entry points, both important:

1. **From the day log**, on rows where the user has an Activity already
   logged: a small `+ Fuel` affordance that opens a fueling-specific
   sub-row. Pre-binds `ActivityID`.
2. **From the chat drawer**, natural language: *"on the ride I had two
   Maurten 160s and a bottle of LMNT"* → agent calls `add_fueling`
   against the most recent / current activity.

A third surface (later) is a quick **in-ride logger** — a stripped-down
view (big tap targets, "+1 gel" / "+1 bottle" buttons backed by
`add_favorite`-style remembered items) for use mid-activity. Out of
scope for v1.

### Day log rendering

Fueling rows render under their parent activity row, visually distinct
from meals (different left rail color, monospace `g/hr` chip). They are
*not* mixed into the meals stream.

Day totals show two lines:

```
Diet:    2200 kcal · 145P 240C 70F
Fueling: 480 kcal · 0P 120C 0F   (3:00 ride · 80 g/hr)
```

The macro ring / "balance" widget uses *Diet* only. The "total energy"
view sums both.

## Open questions

- **Pre-ride breakfast and post-ride recovery meal**: these are still
  diet, even though they're activity-adjacent. Don't try to be clever —
  only things explicitly logged as fueling get the separate treatment.
- **Walks / easy zone-2**: probably no fueling needed; UI shouldn't push
  it. Only show the fueling target when intensity is `moderate+` and
  duration is >60 min, or the user opts in.
- **Hydration**: deferred. Could become a sibling category later
  (sodium, fluid volume) but is its own design.
- **Apple Health / Strava import**: would auto-create activities with
  duration & intensity, removing manual entry. Out of scope for v1 —
  manual activity entry already exists.

## Phasing

**v1 (small, ships fast):**
- Schema migration: `Fueling` sheet + activity duration/intensity.
- Backend CRUD + `add_fueling` agent tool.
- Coach prompt update (separate bucket + g/hr targets).
- Day-log: render fueling rows under activity, separate totals line.

**v2:**
- In-ride quick logger view.
- Saved favorites for fuel items (Maurten 160, SIS gel, etc.).
- Adequacy summary post-ride: "you fueled at 65 g/hr; target was 80–100".

**v3:**
- Strava / Apple Health activity sync.
- Hydration tracking as sibling category.

## Sources

- [90 vs 120 g/hr — EndureIQ](https://www.endureiq.com/blog/carbohydrate-ingestion-rates-during-exercise-90-vs-120-grams-per-hour-which-is-best)
- [EF Pro Cycling — gut training](https://www.efprocycling.com/tips-recipes/tour-de-france-tips-gut-training/)
- [Precision Hydration — carbs per hour](https://www.precisionhydration.com/performance-advice/nutrition/how-much-carbohydrate-carbs-athletes-per-hour/)
- [CTS — carbs per hour on the bike](https://trainright.com/how-many-carbohydrates-per-hour-on-the-bike/)
