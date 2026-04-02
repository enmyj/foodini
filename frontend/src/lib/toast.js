import { writable } from 'svelte/store'

export const toasts = writable([])

let nextToastID = 1

export function getErrorMessage(err, fallback = 'Something went wrong.') {
  if (err?.status === 401 || err?.code === 'session_expired') {
    return 'Your session expired. Sign in again.'
  }
  if (err?.code === 'insufficient_scopes') {
    return 'Google permissions are missing. Re-authorize to continue.'
  }
  if (typeof err?.userMessage === 'string' && err.userMessage.trim()) {
    return err.userMessage.trim()
  }
  return fallback
}

export function showToast(message, { tone = 'error', duration = 5000 } = {}) {
  const id = nextToastID++
  toasts.update(items => [...items, { id, message, tone }])
  if (duration > 0) {
    setTimeout(() => dismissToast(id), duration)
  }
  return id
}

export function showError(err, fallback) {
  return showToast(getErrorMessage(err, fallback), { tone: 'error' })
}

export function dismissToast(id) {
  toasts.update(items => items.filter(item => item.id !== id))
}
