interface DialogLayer {
  zIndex: number
  sequence: number
  onEscape: () => void
}

const layers = new Map<symbol, DialogLayer>()
let sequence = 0

function topLayer() {
  let top: [symbol, DialogLayer] | undefined
  for (const entry of layers) {
    if (!top || entry[1].zIndex > top[1].zIndex ||
      (entry[1].zIndex === top[1].zIndex && entry[1].sequence > top[1].sequence)) {
      top = entry
    }
  }
  return top
}

function handleEscape(event: KeyboardEvent) {
  if (event.key === 'Escape') topLayer()?.[1].onEscape()
}

// One listener selects the visual top layer once, even if closing it removes it
// synchronously. Every registered layer owns the shared body scroll lock.
export function createDialogLayer(onEscape: () => void) {
  const id = Symbol('dialog')
  function update(open: boolean, zIndex: number) {
    if (open) {
      const existing = layers.get(id)
      if (existing) existing.zIndex = zIndex
      else {
        if (layers.size === 0) document.addEventListener('keydown', handleEscape)
        layers.set(id, { zIndex, sequence: ++sequence, onEscape })
      }
    } else {
      layers.delete(id)
      if (layers.size === 0) document.removeEventListener('keydown', handleEscape)
    }
    document.body.classList.toggle('modal-open', layers.size > 0)
  }
  return {
    update,
    isTop: () => topLayer()?.[0] === id,
    remove: () => update(false, 0),
  }
}
