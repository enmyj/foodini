import { mount } from 'svelte'
import './app.css'
import App from './App.svelte'

function syncViewportHeight() {
  const vv = window.visualViewport
  const h = vv?.height ?? window.innerHeight
  document.documentElement.style.setProperty('--vvh', `${h}px`)
}
syncViewportHeight()
window.visualViewport?.addEventListener('resize', syncViewportHeight)
window.visualViewport?.addEventListener('scroll', syncViewportHeight)
window.addEventListener('resize', syncViewportHeight)

const app = mount(App, {
  target: document.getElementById('app')!,
})

export default app
