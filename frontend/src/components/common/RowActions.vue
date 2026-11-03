<template>
  <button
    ref="trigger"
    type="button"
    class="row-actions-trigger"
    :aria-label="t('common.more')"
    :title="t('common.more')"
    aria-haspopup="menu"
    :aria-expanded="open"
    :aria-controls="open ? menuId : undefined"
    @click.stop="toggle"
    @keydown.down.prevent="show"
  ><Icon name="more" size="sm" /></button>
  <Teleport to="body">
    <div
      v-if="open"
      :id="menuId"
      ref="menu"
      class="console-action-menu"
      role="menu"
      :aria-label="t('common.more')"
      :style="position"
      @keydown="onKeydown"
      @click.capture="prepareAction"
      @click="onSelect"
    ><slot /></div>
  </Teleport>
</template>

<script setup lang="ts">
import { nextTick, ref, useId, watch } from 'vue'
import { useEventListener } from '@vueuse/core'
import { useI18n } from 'vue-i18n'
import Icon from '@/components/icons/Icon.vue'

const { t } = useI18n()
const menuId = useId()
const open = ref(false)
const trigger = ref<HTMLButtonElement>()
const menu = ref<HTMLElement>()
const position = ref({ top: '0px', left: '0px' })
const items = () => Array.from(menu.value?.querySelectorAll<HTMLButtonElement>('button:not(:disabled)') ?? [])

function close(restoreFocus = false) {
  open.value = false
  if (restoreFocus) trigger.value?.focus()
}
async function show() {
  open.value = true
  await nextTick()
  if (!open.value || !menu.value || !trigger.value) return
  const anchor = trigger.value.getBoundingClientRect()
  const rect = menu.value.getBoundingClientRect()
  position.value = {
    top: `${Math.max(8, Math.min(anchor.bottom + 4, window.innerHeight - rect.height - 8))}px`,
    left: `${Math.max(8, Math.min(anchor.right - rect.width, window.innerWidth - rect.width - 8))}px`
  }
  menu.value.querySelectorAll('button').forEach(button => { button.setAttribute('role', 'menuitem'); button.tabIndex = -1 })
  await nextTick()
  if (open.value) items()[0]?.focus({ preventScroll: true })
}
function toggle() { if (open.value) close(); else void show() }
function prepareAction(event: MouseEvent) {
  if ((event.target as Element).closest('button:not(:disabled)')) trigger.value?.focus({ preventScroll: true })
}
function onSelect(event: MouseEvent) {
  if (!(event.target as Element).closest('button:not(:disabled)')) return
  close()
  void nextTick(() => {
    // Dialogs opened by a menu action own focus; other actions return to the trigger.
    if (document.activeElement === document.body) trigger.value?.focus({ preventScroll: true })
  })
}
function onKeydown(event: KeyboardEvent) {
  const buttons = items()
  const index = buttons.indexOf(document.activeElement as HTMLButtonElement)
  if (['ArrowDown', 'ArrowUp', 'Home', 'End'].includes(event.key)) {
    event.preventDefault()
    const target = event.key === 'Home' ? 0 : event.key === 'End' ? buttons.length - 1
      : (index + (event.key === 'ArrowDown' ? 1 : -1) + buttons.length) % buttons.length
    buttons[target]?.focus()
  } else if (event.key === 'Escape') { event.preventDefault(); event.stopPropagation(); close(true) }
  else if (event.key === 'Tab') close(true)
}
useEventListener(document, 'pointerdown', (event: PointerEvent) => {
  if (open.value && !menu.value?.contains(event.target as Node) && !trigger.value?.contains(event.target as Node)) close()
})
useEventListener(window, 'resize', () => close())
useEventListener(document, 'scroll', (event: Event) => {
  if (open.value && !(event.target instanceof Node && menu.value?.contains(event.target))) close()
}, true)
watch(open, value => { if (!value) position.value = { top: '0px', left: '0px' } })
</script>

<style scoped>
.row-actions-trigger { display: inline-flex; align-items: center; justify-content: center; width: 32px; height: 32px; border: 1px solid transparent; border-radius: 7px; color: var(--signal-muted); }
.row-actions-trigger:hover, .row-actions-trigger[aria-expanded='true'] { background: var(--signal-hover); color: var(--signal-text); }
.row-actions-trigger:focus-visible { outline: 2px solid var(--signal-accent); outline-offset: 2px; }
.console-action-menu { position: fixed; z-index: 9999; display: grid; min-width: 176px; max-width: calc(100vw - 16px); max-height: calc(100dvh - 16px); overflow: auto; padding: 4px; border: 1px solid var(--signal-line); border-radius: 10px; background: var(--signal-surface); color: var(--signal-text); box-shadow: var(--signal-popover-shadow); }
.console-action-menu :deep(button) { display: flex; align-items: center; flex-direction: row; gap: 9px; width: 100%; min-height: 34px; padding: 7px 10px; border-radius: 6px; text-align: left; font-size: 13px; color: inherit; }
.console-action-menu :deep(button:hover), .console-action-menu :deep(button:focus-visible) { background: var(--signal-hover); outline: none; }
.console-action-menu :deep(button:disabled) { cursor: not-allowed; opacity: .5; }
.console-action-menu :deep(button[data-danger]) { color: #dc2626; }
.dark .console-action-menu :deep(button[data-danger]) { color: #fca5a5; }
.console-action-menu :deep(svg) { flex-shrink: 0; width: 16px; height: 16px; }
.console-action-menu :deep(span) { font-size: inherit; }
@media (max-width: 639px) { .row-actions-trigger { width: 40px; height: 40px; } .console-action-menu :deep(button) { min-height: 42px; } }
</style>
