# Minimalist Redesign

Date: 2026-03-06

## Goal

Replace the Vite default dark template and inconsistent blue accent with a clean,
editorial/typographic light UI. No dark mode. No fancy CSS.

## Palette

| Token       | Value     | Usage                                    |
|-------------|-----------|------------------------------------------|
| Background  | `#fafaf9` | Page background (warm off-white)         |
| Text        | `#1c1c1c` | Primary text                             |
| Muted       | `#888`    | Section labels, secondary info           |
| Accent      | `#2d2d2d` | Buttons, FAB, active states, inputs      |
| Border      | `#e8e8e6` | Dividers, row separators                 |

## Typography

- Font stack: `-apple-system, BlinkMacSystemFont, 'Segoe UI', sans-serif`
- Section labels: `0.7rem`, uppercase, `letter-spacing: 0.08em`, muted
- Entry descriptions: `0.95rem`, `line-height: 1.5`
- Macros: `0.8rem`, muted, right-aligned

## Components

### app.css
Complete replacement of Vite default. Light base only — no dark mode, no color-scheme.
Box-sizing reset, zero margin/padding on body.

### App.svelte (login/loading shell)
- Background: `#fafaf9`
- Wordmark centered, `1.5rem`, weight 500
- Sign-in link: small pill border using accent color, no fill

### LogView.svelte (main view)
- Header: sticky, white bg, hairline bottom border
- Today/Week toggle: plain text, underline active state (no pill/box)
- Totals: single quiet line below toggle, `0.8rem`, muted
- Meal sections: small-caps label, hairline rows
- Week view: same hairline row style

### EntryRow.svelte
- Row padding: `0.65rem 0`
- Description: `0.95rem`
- Macros: `0.8rem`, muted, flex right
- Edit inputs: minimal border, no box-shadow

### ChatDrawer.svelte
- White background, `border-radius: 16px 16px 0 0`
- User message: accent (`#2d2d2d`) fill
- Send button: accent fill, no blue

### ActivityNote.svelte
- Section label consistent with meal labels
- Textarea: subtle border, accent focus ring

## What Does Not Change

- All Svelte component logic and structure
- Inline editing behavior
- Layout (max-width 640px centered column)
- FAB position and shape (only color changes to accent)
