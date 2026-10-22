/** Standard radio-group navigation, skipping disabled choices. */
export function plazaRadioKeyboard(event: KeyboardEvent) {
  if (event.isComposing || !['ArrowLeft', 'ArrowRight', 'ArrowUp', 'ArrowDown', 'Home', 'End'].includes(event.key)) return
  const group = event.currentTarget as HTMLElement
  const buttons = [...group.querySelectorAll<HTMLButtonElement>('button[role="radio"]:not(:disabled)')]
  const current = buttons.indexOf(event.target as HTMLButtonElement)
  if (current < 0 || !buttons.length) return
  event.preventDefault()
  const next = event.key === 'Home' ? 0 : event.key === 'End' ? buttons.length - 1 :
    (current + (['ArrowLeft', 'ArrowUp'].includes(event.key) ? -1 : 1) + buttons.length) % buttons.length
  buttons[next].focus()
  buttons[next].click()
}
