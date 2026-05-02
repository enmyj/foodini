import { mount } from 'svelte'
import './app.css'
import App from './App.svelte'

function syncViewportHeight() {
  const vv = window.visualViewport
  const visibleH = vv?.height ?? window.innerHeight
  const offsetTop = vv?.offsetTop ?? 0
  document.documentElement.style.setProperty('--vvh', `${visibleH}px`)
  document.documentElement.style.setProperty('--vvt', `${offsetTop}px`)
}
syncViewportHeight()
window.visualViewport?.addEventListener('resize', syncViewportHeight)
window.visualViewport?.addEventListener('scroll', syncViewportHeight)
window.addEventListener('resize', syncViewportHeight)

const app = mount(App, {
  target: document.getElementById('app')!,
})

export default app
