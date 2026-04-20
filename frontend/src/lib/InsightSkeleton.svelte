<script lang="ts">
    let { widths = ["88%", "72%", "80%"] }: { widths?: string[] } = $props();
</script>

<div class="insight-skeleton" aria-hidden="true">
    {#each widths as width}
        <div class="isk-line" style={`width: ${width}`}></div>
    {/each}
</div>

<style>
    .insight-skeleton {
        display: flex;
        flex-direction: column;
        gap: 0.6rem;
    }

    .isk-line {
        height: 0.78rem;
        border-radius: 4px;
        background: linear-gradient(
            90deg,
            var(--rule) 25%,
            var(--paper-3) 50%,
            var(--rule) 75%
        );
        background-size: 200% 100%;
        animation: shimmer 1.4s ease-in-out infinite;
    }

    @keyframes shimmer {
        0% {
            background-position: 200% 0;
        }
        100% {
            background-position: -200% 0;
        }
    }
</style>
