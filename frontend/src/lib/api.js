const TZ = Intl.DateTimeFormat().resolvedOptions().timeZone
const SAFE_ERROR_MESSAGES = {
  session_expired: 'Your session expired. Sign in again.',
  insufficient_scopes: 'Google permissions are missing. Re-authorize to continue.',
  upload_too_large: 'Photos are too large for one request. Try fewer photos and send again.',
  favorite_exists: 'That favorite already exists.',
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
  if (images?.length) {
    const body = new FormData()
    body.append('message', message ?? '')
    if (date) body.append('date', date)
    if (meal) body.append('meal', meal)
    for (const image of images) {
      body.append('images', image)
    }
    return (await apiFetch('/api/chat', {
      method: 'POST',
      body,
    })).json()
  }

  const body = { message }
  if (date) body.date = date
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

export async function editChat(message, entries, date = null, mealType = null) {
  const body = { message, entries }
  if (date) body.date = date
  if (mealType) body.meal_type = mealType
  return (await apiFetch('/api/chat/edit', {
    method: 'POST',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify(body),
  })).json()
}

export async function fetchMealSuggestion(date, meal) {
  return (await apiFetch(`/api/suggestions/meal?date=${date}&meal=${meal}`)).json()
}

export async function streamMealSuggestion(date, meal, onChunk) {
  return streamInsight('/api/suggestions/meal?stream=1', { date, meal }, onChunk)
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

export async function streamDayInsights(date, onChunk) {
  return streamInsight('/api/insights/day?stream=1', { date }, onChunk)
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

export async function streamInsights(start, end, onChunk) {
  return streamInsight('/api/insights?stream=1', { start, end }, onChunk)
}

export async function fetchStoredDaySuggestions(date) {
  return (await apiFetch(`/api/suggestions/day?date=${date}`)).json()
}

export async function generateDaySuggestions(date) {
  return (await apiFetch('/api/suggestions/day', {
    method: 'POST',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify({ date }),
  })).json()
}

export async function fetchStoredWeekSuggestions(start, end) {
  return (await apiFetch(`/api/suggestions/week?start=${start}&end=${end}`)).json()
}

export async function generateWeekSuggestions(start, end) {
  return (await apiFetch('/api/suggestions/week', {
    method: 'POST',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify({ start, end }),
  })).json()
}

export async function streamWeekSuggestions(start, end, onChunk) {
  return streamInsight('/api/suggestions/week?stream=1', { start, end }, onChunk)
}

export async function getFavorites() {
  return (await apiFetch('/api/favorites')).json()
}

export async function addFavorite(entry) {
  return (await apiFetch('/api/favorites', {
    method: 'POST',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify({
      description: entry.description,
      meal_type: entry.meal_type,
      calories: entry.calories,
      protein: entry.protein,
      carbs: entry.carbs,
      fat: entry.fat,
      fiber: entry.fiber ?? 0,
    }),
  })).json()
}

export async function deleteFavorite(id) {
  await apiFetch(`/api/favorites/${id}`, { method: 'DELETE' })
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

/**
 * Generic SSE streaming helper for insight/suggestion endpoints.
 * Sends a POST, reads SSE events, calls onChunk(text) for each chunk,
 * and resolves with { text, generated_at } when done.
 */
async function streamInsight(url, body, onChunk) {
  const res = await fetch(url, {
    method: 'POST',
    headers: { 'Content-Type': 'application/json', 'X-Timezone': TZ },
    body: JSON.stringify(body),
  })
  if (!res.ok) await throwResponseError(res)

  const reader = res.body.getReader()
  const decoder = new TextDecoder()
  let buf = ''
  let result = null

  for (;;) {
    const { done, value } = await reader.read()
    if (done) break
    buf += decoder.decode(value, { stream: true })
    const lines = buf.split('\n')
    buf = lines.pop()
    for (const line of lines) {
      if (!line.startsWith('data: ')) continue
      const json = line.slice(6)
      try {
        const evt = JSON.parse(json)
        if (evt.error) throw new Error(evt.error)
        if (evt.done) {
          result = { text: evt.text, generated_at: evt.generated_at }
        } else if (evt.chunk) {
          onChunk(evt.chunk)
        }
      } catch (e) {
        if (e.message && !e.message.startsWith('Unexpected')) throw e
      }
    }
  }

  if (!result) throw new Error('Stream ended without completion')
  return result
}
