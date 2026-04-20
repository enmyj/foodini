<script lang="ts">
    import { getTheme, cycleTheme } from "./theme.svelte.ts";
    import type { ThemePreference } from "./types.ts";

    let theme = $state<ThemePreference>(getTheme());
</script>

<button
    class="theme-toggle"
    onclick={() => (theme = cycleTheme())}
    aria-label="Toggle theme"
    title={theme === 'system' ? 'Theme: auto' : theme === 'dark' ? 'Theme: dark' : 'Theme: light'}
>{theme === 'system' ? '◑' : theme === 'dark' ? '●' : '○'}</button>

<style>
    .theme-toggle {
        display: inline-flex;
        align-items: center;
        justify-content: center;
        background: none;
        border: none;
        margin: 0;
        color: inherit;
        font: inherit;
        cursor: pointer;
        padding: 0;
        touch-action: manipulation;
    }

    @media (hover: hover) {
        .theme-toggle:hover {
            color: var(--ink-2);
        }
    }
</style>
