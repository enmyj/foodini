const TZ = Intl.DateTimeFormat().resolvedOptions().timeZone
const SAFE_ERROR_MESSAGES = {
  session_expired: 'Your session expired. Sign in again.',
  insufficient_scopes: 'Google permissions are missing. Re-authorize to continue.',
}

async function throwResponseError(res) {
  const contentType = res.headers.get('content-type') ?? ''
  let body = null
  let text = ''

  if (contentType.includes('application/json')) {
    body = await res.json().catch(() => null)
  } else {
    text = await res.text()
  }

  const code = typeof body?.error === 'string' && body.error.trim() ? body.error.trim() : ''
  const err = new Error(SAFE_ERROR_MESSAGES[code] || `Request failed (${res.status})`)
  err.status = res.status
  err.code = code || null
  err.body = body
  err.detail = code ? text || null : body?.error || text || null
  err.userMessage = SAFE_ERROR_MESSAGES[code] || ''
  throw err
}

async function apiFetch(url, init = {}) {
  const res = await fetch(url, {
    ...init,
    headers: {
      'X-Timezone': TZ,
      ...(init.headers ?? {}),
    },
  })
  if (!res.ok) await throwResponseError(res)
  return res
}

export async function getLog({ date = null, days = null } = {}) {
  const params = days ? `?days=${days}` : date ? `?date=${date}` : ''
  return (await apiFetch(`/api/log${params}`)).json()
}

export async function chat(message, date = null, images = null, meal = null) {
  const body = { message }
  if (date) body.date = date
  if (images) body.images = images
  if (meal) body.meal = meal
  return (await apiFetch('/api/chat', {
    method: 'POST',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify(body),
  })).json()
}

export async function confirmChat(entries, date = null) {
  const body = { entries }
  if (date) body.date = date
  return (await apiFetch('/api/chat/confirm', {
    method: 'POST',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify(body),
  })).json()
}

export async function patchEntry(id, entry) {
  return (await apiFetch(`/api/entries/${id}`, {
    method: 'PATCH',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify(entry),
  })).json()
}

export async function deleteEntry(id) {
  await apiFetch(`/api/entries/${id}`, { method: 'DELETE' })
}

export async function getActivity(date) {
  return (await apiFetch(`/api/activity?date=${date}`)).json()
}

export async function putActivity(date, { activity, feeling_score, feeling_notes, poop, poop_notes, hydration }) {
  return (await apiFetch('/api/activity', {
    method: 'PUT',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify({ date, activity, feeling_score, feeling_notes, poop, poop_notes, hydration }),
  })).json()
}

export async function fetchStoredDayInsight(date) {
  return (await apiFetch(`/api/insights/day?date=${date}`)).json()
}

export async function generateDayInsights(date) {
  return (await apiFetch('/api/insights/day', {
    method: 'POST',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify({ date }),
  })).json()
}

export async function fetchStoredInsight(start, end) {
  return (await apiFetch(`/api/insights?start=${start}&end=${end}`)).json()
}

export async function generateInsights(start, end) {
  return (await apiFetch('/api/insights', {
    method: 'POST',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify({ start, end }),
  })).json()
}

export async function getProfile() {
  return (await apiFetch('/api/profile')).json()
}

export async function putProfile(profile) {
  return (await apiFetch('/api/profile', {
    method: 'PUT',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify(profile),
  })).json()
}
