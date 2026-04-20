export interface ApiError extends Error {
  status: number
  code: string | null
  body: unknown
  detail: string | null
  userMessage: string
}

const TZ = Intl.DateTimeFormat().resolvedOptions().timeZone
const SAFE_ERROR_MESSAGES: Record<string, string> = {
  session_expired: 'Your session expired. Sign in again.',
  insufficient_scopes: 'Google permissions are missing. Re-authorize to continue.',
  upload_too_large: 'Photos are too large for one request. Try fewer photos and send again.',
  favorite_exists: 'That favorite already exists.',
}

async function throwResponseError(res: Response): Promise<never> {
  const contentType = res.headers.get('content-type') ?? ''
  let body: unknown = null
  let text = ''

  if (contentType.includes('application/json')) {
    body = await res.json().catch(() => null)
  } else {
    text = await res.text()
  }

  const jsonBody = body as Record<string, unknown> | null
  const code = typeof jsonBody?.error === 'string' && (jsonBody.error as string).trim() ? (jsonBody.error as string).trim() : ''
  const err = new Error(SAFE_ERROR_MESSAGES[code] || `Request failed (${res.status})`) as ApiError
  err.status = res.status
  err.code = code || null
  err.body = body
  err.detail = code ? text || null : (jsonBody?.error as string) || text || null
  err.userMessage = SAFE_ERROR_MESSAGES[code] || ''
  throw err
}

async function apiFetch(url: string, init: RequestInit = {}): Promise<Response> {
  const headers: Record<string, string> = {
    'X-Timezone': TZ,
    ...((init.headers as Record<string, string>) ?? {}),
  }
  const res = await fetch(url, {
    ...init,
    headers,
  })
  if (!res.ok) await throwResponseError(res)
  return res
}

export async function getLog({ date = null, days = null }: { date?: string | null; days?: number | null } = {}) {
  const params = days ? `?days=${days}` : date ? `?date=${date}` : ''
  return (await apiFetch(`/api/log${params}`)).json()
}

export async function chat(message: string | null, date: string | null = null, images: File[] | null = null, meal: string | null = null) {
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

  const body: Record<string, unknown> = { message }
  if (date) body.date = date
  if (meal) body.meal = meal
  return (await apiFetch('/api/chat', {
    method: 'POST',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify(body),
  })).json()
}

export async function confirmChat(entries: unknown[], date: string | null = null) {
  const body: Record<string, unknown> = { entries }
  if (date) body.date = date
  return (await apiFetch('/api/chat/confirm', {
    method: 'POST',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify(body),
  })).json()
}

export async function editChat(message: string, entries: unknown[], date: string | null = null, mealType: string | null = null) {
  const body: Record<string, unknown> = { message, entries }
  if (date) body.date = date
  if (mealType) body.meal_type = mealType
  return (await apiFetch('/api/chat/edit', {
    method: 'POST',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify(body),
  })).json()
}

export async function fetchMealSuggestion(date: string, meal: string) {
  return (await apiFetch(`/api/suggestions/meal?date=${date}&meal=${meal}`)).json()
}

export async function generateMealSuggestion(date: string, meal: string) {
  return (await apiFetch('/api/suggestions/meal', {
    method: 'POST',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify({ date, meal }),
  })).json()
}

export async function patchEntry(id: string, entry: Record<string, unknown>) {
  return (await apiFetch(`/api/entries/${id}`, {
    method: 'PATCH',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify(entry),
  })).json()
}

export async function deleteEntry(id: string) {
  await apiFetch(`/api/entries/${id}`, { method: 'DELETE' })
}

export async function getActivity(date: string) {
  return (await apiFetch(`/api/activity?date=${date}`)).json()
}

export async function putActivity(date: string, { activity, feeling_score, feeling_notes, poop, poop_notes, hydration }: { activity: string; feeling_score: number; feeling_notes: string; poop: boolean; poop_notes: string; hydration: number }) {
  return (await apiFetch('/api/activity', {
    method: 'PUT',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify({ date, activity, feeling_score, feeling_notes, poop, poop_notes, hydration }),
  })).json()
}

export async function fetchStoredDayInsight(date: string) {
  return (await apiFetch(`/api/insights/day?date=${date}`)).json()
}

export async function generateDayInsights(date: string) {
  return (await apiFetch('/api/insights/day', {
    method: 'POST',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify({ date }),
  })).json()
}

export async function fetchStoredInsight(start: string, end: string) {
  return (await apiFetch(`/api/insights?start=${start}&end=${end}`)).json()
}

export async function generateInsights(start: string, end: string) {
  return (await apiFetch('/api/insights', {
    method: 'POST',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify({ start, end }),
  })).json()
}

export async function fetchStoredDaySuggestions(date: string) {
  return (await apiFetch(`/api/suggestions/day?date=${date}`)).json()
}

export async function generateDaySuggestions(date: string) {
  return (await apiFetch('/api/suggestions/day', {
    method: 'POST',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify({ date }),
  })).json()
}

export async function fetchStoredWeekSuggestions(start: string, end: string) {
  return (await apiFetch(`/api/suggestions/week?start=${start}&end=${end}`)).json()
}

export async function generateWeekSuggestions(start: string, end: string) {
  return (await apiFetch('/api/suggestions/week', {
    method: 'POST',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify({ start, end }),
  })).json()
}

export async function getFavorites() {
  return (await apiFetch('/api/favorites')).json()
}

export async function addFavorite(entry: { description: string; meal_type: string; calories: number; protein: number; carbs: number; fat: number; fiber?: number }) {
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

export async function deleteFavorite(id: string) {
  await apiFetch(`/api/favorites/${id}`, { method: 'DELETE' })
}

export async function getProfile() {
  return (await apiFetch('/api/profile')).json()
}

export async function putProfile(profile: Record<string, unknown>) {
  return (await apiFetch('/api/profile', {
    method: 'PUT',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify(profile),
  })).json()
}
