const TZ = Intl.DateTimeFormat().resolvedOptions().timeZone

export async function getLog({ date = null, days = null } = {}) {
  const params = days ? `?days=${days}` : date ? `?date=${date}` : ''
  const res = await fetch(`/api/log${params}`, { headers: { 'X-Timezone': TZ } })
  if (!res.ok) throw new Error(await res.text())
  return res.json()
}

export async function chat(message, date = null, image = null, meal = null) {
  const body = { message }
  if (date) body.date = date
  if (image) body.image = image
  if (meal) body.meal = meal
  const res = await fetch('/api/chat', {
    method: 'POST',
    headers: { 'Content-Type': 'application/json', 'X-Timezone': TZ },
    body: JSON.stringify(body),
  })
  if (!res.ok) throw new Error(await res.text())
  return res.json()
}

export async function confirmChat(entries, date = null) {
  const body = { entries }
  if (date) body.date = date
  const res = await fetch('/api/chat/confirm', {
    method: 'POST',
    headers: { 'Content-Type': 'application/json', 'X-Timezone': TZ },
    body: JSON.stringify(body),
  })
  if (!res.ok) throw new Error(await res.text())
  return res.json()
}

export async function patchEntry(id, entry) {
  const res = await fetch(`/api/entries/${id}`, {
    method: 'PATCH',
    headers: { 'Content-Type': 'application/json', 'X-Timezone': TZ },
    body: JSON.stringify(entry),
  })
  if (!res.ok) throw new Error(await res.text())
  return res.json()
}

export async function deleteEntry(id) {
  const res = await fetch(`/api/entries/${id}`, { method: 'DELETE', headers: { 'X-Timezone': TZ } })
  if (!res.ok) throw new Error(await res.text())
}

export async function getActivity(date) {
  const res = await fetch(`/api/activity?date=${date}`, { headers: { 'X-Timezone': TZ } })
  if (!res.ok) throw new Error(await res.text())
  return res.json()
}

export async function putActivity(date, { activity, feeling_score, feeling_notes, poop, poop_notes, hydration }) {
  const res = await fetch('/api/activity', {
    method: 'PUT',
    headers: { 'Content-Type': 'application/json', 'X-Timezone': TZ },
    body: JSON.stringify({ date, activity, feeling_score, feeling_notes, poop, poop_notes, hydration }),
  })
  if (!res.ok) throw new Error(await res.text())
  return res.json()
}

export async function getInsights(start, end) {
  const res = await fetch('/api/insights', {
    method: 'POST',
    headers: { 'Content-Type': 'application/json', 'X-Timezone': TZ },
    body: JSON.stringify({ start, end }),
  })
  if (!res.ok) throw new Error(await res.text())
  return res.json()
}

export async function getProfile() {
  const res = await fetch('/api/profile', { headers: { 'X-Timezone': TZ } })
  if (!res.ok) throw new Error(await res.text())
  return res.json()
}

export async function putProfile(profile) {
  const res = await fetch('/api/profile', {
    method: 'PUT',
    headers: { 'Content-Type': 'application/json', 'X-Timezone': TZ },
    body: JSON.stringify(profile),
  })
  if (!res.ok) throw new Error(await res.text())
  return res.json()
}
