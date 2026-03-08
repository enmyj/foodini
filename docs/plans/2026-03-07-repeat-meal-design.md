# Repeat Meal Feature — Design

**Date:** 2026-03-07

## Problem

Users who eat the same thing for a meal every day (e.g. the same breakfast) must re-describe it via chat every time. A one-tap "repeat from yesterday" shortcut eliminates that friction.

## Scope

All meals (breakfast, snack, lunch, dinner) in the Today view. Button only appears if yesterday had entries for that meal.

## Approach

Frontend-only — no new backend endpoints. Reuse the existing `POST /api/chat/confirm` endpoint, which accepts an array of FoodEntry objects and a target date, assigns fresh UUIDs, and saves them to the sheet.

## Data Flow

1. When Today view loads, fire a background fetch for yesterday's log: `GET /api/log?date={yesterday}`.
2. Store result as `yesterdayByMeal` — a `meal_type → entries[]` map.
3. If the background fetch fails, `yesterdayByMeal` stays empty — repeat buttons simply don't appear. No error shown.
4. `repeatMeal(meal)` calls `confirmChat(yesterdayByMeal[meal], todayDate)`.
5. Backend assigns new UUIDs and today's date; returns saved entries.
6. Returned entries are merged into `data.entries` (same path as normal chat confirm).

## UI

Each meal section `<h3>` becomes a flex row:
- Left: meal name — still clickable, opens chat drawer as before.
- Right: small `↻` icon button — only rendered if `yesterdayByMeal[meal]?.length > 0`.

Button states:
- Default: `color: #bbb`
- Hover: `color: #555`
- In-flight: spinning CSS animation, non-interactive

## Files Changed

- `frontend/src/lib/LogView.svelte` — add `yesterdayByMeal` state, background fetch, `repeatMeal()`, updated meal header markup and CSS.
- `frontend/src/lib/api.js` — no changes needed (reuses `getLog` and `confirmChat`).
- Backend — no changes.
