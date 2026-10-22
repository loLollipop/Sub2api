interface DialogLayer {
  zIndex: number
  sequence: number
  onEscape: () => void
  onTab?: (event: KeyboardEvent) => void
  onRestoreFocus?: (target: HTMLElement | null) => void
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

function handleKeydown(event: KeyboardEvent) {
  const top = topLayer()?.[1]
  if (event.key === 'Escape' && top && !event.defaultPrevented) {
    event.preventDefault()
    event.stopPropagation()
    top.onEscape()
  }
  else if (event.key === 'Tab' && !event.defaultPrevented) top?.onTab?.(event)
}

// One listener selects the visual top layer once, even if closing it removes it
// synchronously. Every registered layer owns the shared body scroll lock.
export function createDialogLayer(
  onEscape: () => void,
  handlers: Pick<DialogLayer, 'onTab' | 'onRestoreFocus'> = {},
) {
  const id = Symbol('dialog')
  function update(open: boolean, zIndex: number) {
    if (open) {
      const existing = layers.get(id)
      if (existing) existing.zIndex = zIndex
      else {
        if (layers.size === 0) document.addEventListener('keydown', handleKeydown)
        layers.set(id, { zIndex, sequence: ++sequence, onEscape, ...handlers })
      }
    } else {
      layers.delete(id)
      if (layers.size === 0) document.removeEventListener('keydown', handleKeydown)
    }
    document.body.classList.toggle('modal-open', layers.size > 0)
  }
  return {
    update,
    isTop: () => topLayer()?.[0] === id,
    restoreFocus: (target: HTMLElement | null) => {
      const top = topLayer()?.[1]
      if (top) top.onRestoreFocus?.(target)
      else if (target?.isConnected) target.focus()
    },
    remove: () => update(false, 0),
  }
}
