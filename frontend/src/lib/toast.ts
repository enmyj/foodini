import { writable } from 'svelte/store'
import type { ApiError } from './api.ts'

export type ToastTone = 'error' | 'info'

export interface Toast {
  id: number
  message: string
  tone: ToastTone
}

export const toasts = writable<Toast[]>([])

let nextToastID = 1

export function getErrorMessage(err: unknown, fallback = 'Something went wrong.') {
  const e = err as Partial<ApiError> | undefined
  if (e?.status === 401 || e?.code === 'session_expired') {
    return 'Your session expired. Sign in again.'
  }
  if (e?.code === 'insufficient_scopes') {
    return 'Google permissions are missing. Re-authorize to continue.'
  }
  if (typeof e?.userMessage === 'string' && e.userMessage.trim()) {
    return e.userMessage.trim()
  }
  return fallback
}

export function showToast(message: string, { tone = 'error', duration = 5000 }: { tone?: ToastTone; duration?: number } = {}) {
  const id = nextToastID++
  toasts.update(items => [...items, { id, message, tone }])
  if (duration > 0) {
    setTimeout(() => dismissToast(id), duration)
  }
  return id
}

export function showError(err: unknown, fallback?: string) {
  return showToast(getErrorMessage(err, fallback), { tone: 'error' })
}

export function dismissToast(id: number) {
  toasts.update(items => items.filter(item => item.id !== id))
}
