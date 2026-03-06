# Food Tracker — Design Doc
**Date:** 2026-03-05

## Overview

A frictionless food and activity tracker built entirely on Google infrastructure. Users log meals via natural language (Gemini handles extraction + macro estimation), data lives in their own Google Sheets, and auth uses standard Google OAuth. The app owner incurs zero per-user costs — Gemini and Sheets calls run under each user's own Google credentials.

## Goals

- Log food by talking naturally ("I had oatmeal and coffee for breakfast")
- Gemini estimates macros; asks one clarifying question at a time if quantities are unclear
- Free-text daily activity note ("went for a run, stressful day")
- View today's log grouped by meal slot, with weekly summary available
- Inline editing + natural language correction for wrong entries
- Multi-user via Google OAuth — no third-party auth services

## Non-Goals

- No calorie tracking beyond what Gemini estimates
- No meal planning, recipes, or food database search
- No mobile app (responsive web is sufficient)
- No admin dashboard or user management

## Architecture

### Stack
- **Backend:** Go (single binary, serves embedded Svelte frontend)
- **Frontend:** Svelte
- **Auth:** Google OAuth 2.0 ("Sign in with Google") — no Auth0/WorkOS
- **Storage:** Google Sheets (one spreadsheet per user, in their own Drive)
- **LLM:** Google Gemini via Generative Language API (OAuth, user's own quota)

### OAuth Scopes
```
openid, email, profile
https://www.googleapis.com/auth/spreadsheets
https://www.googleapis.com/auth/drive.file
https://www.googleapis.com/auth/generative-language
```

### Session
Encrypted HttpOnly cookie containing the user's Google refresh token. No database. Token refresh handled transparently by middleware on every request.

### App owner infrastructure
- One Google Cloud project with OAuth client credentials (free)
- Cookie signing key (env var)
- Deploy anywhere Go runs (single binary)

## Google Sheets Structure

One spreadsheet per user, auto-created on first login.

**Sheet: Food**
| Column | Type | Notes |
|--------|------|-------|
| id | string | UUID, for stable row references |
| date | date | YYYY-MM-DD |
| time | time | HH:MM |
| meal_type | enum | breakfast, snack, lunch, snack, dinner, snack |
| description | string | Free text from Gemini |
| calories | number | Gemini estimate |
| protein | number | grams |
| carbs | number | grams |
| fat | number | grams |

**Sheet: Activity**
| Column | Type | Notes |
|--------|------|-------|
| date | date | YYYY-MM-DD |
| notes | string | Free text, one row per day |

## Frontend Components

- **LogView** — main screen. Today's entries grouped by meal slot, macro totals at top. Week toggle shows 7-day summary; tap a day to drill in.
- **EntryRow** — inline-editable row. Click any field to edit, save on blur.
- **ChatDrawer** — slides up from floating action button. Text input, Gemini responses, typing indicator. Closes on confirm; log refreshes.
- **ActivityNote** — bottom of today's view. Free-text daily note, inline editable.

## Backend Modules

- `auth` — OAuth2 login/callback, token refresh middleware
- `sheets` — create spreadsheet on first login; CRUD for Food and Activity rows
- `gemini` — multi-turn conversation handler; structured JSON extraction; system prompt enforces one clarifying question at a time
- `api` — REST handlers

## Data Flow

### Add food entry
1. User types in ChatDrawer → `POST /api/chat`
2. Go appends to in-memory conversation (keyed by session)
3. Gemini called with full conversation + system prompt
4. If clarification needed: response returned, drawer stays open
5. If confident: Gemini returns `{meal_type, description, calories, protein, carbs, fat}`
6. Go writes row to Sheets → returns entry → drawer closes, log refreshes

### Inline edit
`PATCH /api/entries/:id` → Go updates cell(s) in Sheets

### Load log
- `GET /api/log?date=today` → Food + Activity filtered by date, grouped by meal slot
- `GET /api/log?week=true` → Last 7 days, aggregated daily totals

### Natural language correction
Same chat flow; system prompt includes the existing entry. Gemini returns updated values. `PATCH` replaces the row.

### Auth
`/auth/login` → Google OAuth consent → `/auth/callback` → store refresh token in encrypted cookie → redirect to app

## Error Handling

- Token refresh failure → clear cookie, redirect to login
- Sheets API error → toast notification; chat drawer stays open (user's text preserved)
- Gemini parse failure → show raw response in drawer, user can retry or correct inline after
- No optimistic UI — wait for server confirmation before updating log (prevents phantom entries)

## Testing

- Go unit tests: Gemini JSON parsing/prompt logic, Sheets row mapping
- Primary QA: manual end-to-end (personal tool, no SLA)
