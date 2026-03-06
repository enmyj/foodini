export async function getLog({ date = null, week = false } = {}) {
  const params = week ? '?week=true' : date ? `?date=${date}` : ''
  const res = await fetch(`/api/log${params}`)
  if (!res.ok) throw new Error(await res.text())
  return res.json()
}

export async function chat(message) {
  const res = await fetch('/api/chat', {
    method: 'POST',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify({ message }),
  })
  if (!res.ok) throw new Error(await res.text())
  return res.json()
}

export async function patchEntry(id, entry) {
  const res = await fetch(`/api/entries/${id}`, {
    method: 'PATCH',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify(entry),
  })
  if (!res.ok) throw new Error(await res.text())
  return res.json()
}

export async function getActivity(date) {
  const res = await fetch(`/api/activity?date=${date}`)
  if (!res.ok) throw new Error(await res.text())
  return res.json()
}

export async function putActivity(date, notes) {
  const res = await fetch('/api/activity', {
    method: 'PUT',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify({ date, notes }),
  })
  if (!res.ok) throw new Error(await res.text())
  return res.json()
}
