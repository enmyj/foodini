import { QueryClient } from '@tanstack/svelte-query'

export const queryClient = new QueryClient({
  defaultOptions: {
    queries: {
      staleTime: 5 * 60_000,
      // 'always' (vs true) refetches on focus regardless of staleTime — needed
      // so a mobile pocket trip under staleTime still pulls fresh state when
      // the tab resumes. Fires on visibilitychange, which iOS Safari emits
      // reliably on resume.
      refetchOnWindowFocus: "always",
      refetchOnReconnect: true,
      // Retry network/5xx errors twice — covers the iOS-Safari case where a tab
      // resumed from background fires a fetch before the network is ready. Skip
      // 4xx so auth errors surface immediately.
      retry: (failureCount, error) => {
        if (failureCount >= 2) return false;
        const status = (error as { status?: number } | null)?.status;
        if (typeof status === 'number' && status >= 400 && status < 500) return false;
        return true;
      },
      retryDelay: (attempt) => Math.min(1000 * 2 ** attempt, 4000),
    },
  },
})
