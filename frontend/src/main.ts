import { mount } from 'svelte'
import './app.css'
import App from './App.svelte'

function syncViewportHeight() {
  const vv = window.visualViewport
  const visibleH = vv?.height ?? window.innerHeight
  document.documentElement.style.setProperty('--vvh', `${visibleH}px`)
}
syncViewportHeight()
window.visualViewport?.addEventListener('resize', syncViewportHeight)
window.addEventListener('resize', syncViewportHeight)

const app = mount(App, {
  target: document.getElementById('app')!,
})

export default app
