import { mount } from 'svelte'
import './app.css'
import App from './App.svelte'

function syncViewportHeight() {
  const vv = window.visualViewport
  const visibleH = vv?.height ?? window.innerHeight
  const offsetTop = vv?.offsetTop ?? 0
  // Bottom inset = how much the keyboard (or accessory bar) covers the
  // layout viewport. With `interactive-widget=resizes-content` this is
  // usually 0 because the layout viewport already excludes the keyboard.
  // Without it (older iOS, or fallback), this is the keyboard height and
  // we use it to push position:fixed elements above the keyboard.
  const bottomInset = Math.max(0, window.innerHeight - visibleH - offsetTop)
  document.documentElement.style.setProperty('--vvh', `${visibleH}px`)
  document.documentElement.style.setProperty('--vvb', `${bottomInset}px`)
}
syncViewportHeight()
window.visualViewport?.addEventListener('resize', syncViewportHeight)
window.visualViewport?.addEventListener('scroll', syncViewportHeight)
window.addEventListener('resize', syncViewportHeight)

const app = mount(App, {
  target: document.getElementById('app')!,
})

export default app
