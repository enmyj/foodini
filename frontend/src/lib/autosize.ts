export function autosize(node: HTMLTextAreaElement) {
  let frame: number | null = null
  const proto = Object.getPrototypeOf(node)
  const valueDescriptor: PropertyDescriptor | undefined = proto ? Object.getOwnPropertyDescriptor(proto, 'value') : undefined

  function resize() {
    node.style.height = 'auto'
    node.style.height = `${node.scrollHeight}px`
  }

  function scheduleResize() {
    if (frame !== null) cancelAnimationFrame(frame)
    frame = requestAnimationFrame(() => {
      frame = null
      resize()
    })
  }

  function handleInput() {
    scheduleResize()
  }

  node.style.overflowY = 'hidden'
  node.addEventListener('input', handleInput)

  if (valueDescriptor?.get && valueDescriptor?.set) {
    Object.defineProperty(node, 'value', {
      configurable: true,
      enumerable: valueDescriptor.enumerable ?? true,
      get() {
        return valueDescriptor.get!.call(this)
      },
      set(nextValue: string) {
        valueDescriptor.set!.call(this, nextValue)
        scheduleResize()
      },
    })
  }

  scheduleResize()

  return {
    destroy() {
      if (frame !== null) cancelAnimationFrame(frame)
      node.removeEventListener('input', handleInput)
      if (valueDescriptor?.get && valueDescriptor?.set) {
        delete (node as unknown as Record<string, unknown>).value
      }
    },
  }
}
