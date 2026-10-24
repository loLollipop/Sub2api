<template>
  <Teleport to="body">
    <Transition name="modal">
      <div
        v-if="show"
        class="modal-overlay"
        :style="zIndexStyle"
        :aria-labelledby="dialogId"
        role="dialog"
        :aria-modal="isTopDialog || undefined"
        :aria-hidden="!isTopDialog || undefined"
        @click.self="handleClose"
      >
        <!-- Modal panel -->
        <div
          ref="dialogRef"
          :class="['modal-content', widthClasses]"
          tabindex="-1"
          @focusin="rememberFocus"
          @click.stop
        >
          <!-- Header -->
          <div class="modal-header">
            <h3 :id="dialogId" class="modal-title">
              {{ title }}
            </h3>
            <button
              v-if="showCloseButton"
              @click="emit('close')"
              class="-mr-2 rounded-xl p-2 text-gray-400 transition-colors hover:bg-gray-100 hover:text-gray-600 focus:outline-none focus-visible:ring-2 focus-visible:ring-primary-500/30 focus-visible:ring-offset-2 dark:text-dark-500 dark:hover:bg-dark-700 dark:hover:text-dark-300 dark:focus-visible:ring-offset-dark-900"
              aria-label="Close modal"
            >
              <Icon name="x" size="md" />
            </button>
          </div>

          <!-- Body -->
          <div ref="modalBodyRef" class="modal-body">
            <slot></slot>
          </div>

          <!-- Footer -->
          <div v-if="$slots.footer" class="modal-footer">
            <slot name="footer"></slot>
          </div>
        </div>
      </div>
    </Transition>
  </Teleport>
</template>

<script lang="ts">
let dialogIdCounter = 0
</script>

<script setup lang="ts">
import { computed, watch, onBeforeUnmount, ref, nextTick } from 'vue'
import Icon from '@/components/icons/Icon.vue'
import { createDialogLayer } from './dialogStack'

// 生成唯一ID以避免多个对话框时ID冲突
const dialogId = `modal-title-${++dialogIdCounter}`

// 焦点管理
const dialogRef = ref<HTMLElement | null>(null)
const modalBodyRef = ref<HTMLElement | null>(null)
const isTopDialog = ref(false)
let previousActiveElement: HTMLElement | null = null
let lastFocusedElement: HTMLElement | null = null

type DialogWidth = 'narrow' | 'normal' | 'wide' | 'extra-wide' | 'full'

interface Props {
  show: boolean
  title: string
  width?: DialogWidth
  closeOnEscape?: boolean
  closeOnClickOutside?: boolean
  showCloseButton?: boolean
  zIndex?: number
}

interface Emits {
  (e: 'close'): void
}

const props = withDefaults(defineProps<Props>(), {
  width: 'normal',
  closeOnEscape: true,
  closeOnClickOutside: false,
  showCloseButton: true,
  zIndex: 50
})

const emit = defineEmits<Emits>()

// Custom z-index style (overrides the default z-50 from CSS)
const zIndexStyle = computed(() => {
  return props.zIndex !== 50 ? { zIndex: props.zIndex } : undefined
})

const widthClasses = computed(() => {
  // Width guidance: narrow=confirm/short prompts, normal=standard forms,
  // wide=multi-section forms or rich content, extra-wide=analytics/tables,
  // full=full-screen or very dense layouts.
  const widths: Record<DialogWidth, string> = {
    narrow: 'max-w-md',
    normal: 'max-w-lg',
    wide: 'w-full sm:max-w-2xl md:max-w-3xl lg:max-w-4xl',
    'extra-wide': 'w-full sm:max-w-3xl md:max-w-4xl lg:max-w-5xl xl:max-w-6xl',
    full: 'w-full sm:max-w-4xl md:max-w-5xl lg:max-w-6xl xl:max-w-7xl'
  }
  return widths[props.width]
})

const handleClose = () => {
  if (props.closeOnClickOutside) {
    emit('close')
  }
}

function isAvailable(element: HTMLElement) {
  if (!element.isConnected || element.matches(':disabled, input[type="hidden"]')) return false
  for (let ancestor: HTMLElement | null = element; ancestor; ancestor = ancestor.parentElement) {
    if (ancestor.hidden || ancestor.hasAttribute('inert')) return false
    // Closed details hide their contents without setting display:none on them.
    // Only the first summary and its descendants remain in the tab order.
    if (ancestor instanceof HTMLDetailsElement && !ancestor.open && ancestor !== element) {
      const summary = Array.from(ancestor.children).find(child => child.tagName === 'SUMMARY')
      if (!summary?.contains(element)) return false
    }
    const style = getComputedStyle(ancestor)
    if (style.display === 'none' || style.visibility === 'hidden' || style.visibility === 'collapse') {
      return false
    }
  }
  return true
}

function focusableElements() {
  const elements = dialogRef.value?.querySelectorAll<HTMLElement>(
    'button, a[href], area[href], input, select, textarea, details > summary:first-of-type, [tabindex], [contenteditable="true"]'
  )
  return Array.from(elements ?? [])
    .filter(element => element.tabIndex >= 0 && isAvailable(element))
    .sort((a, b) => (a.tabIndex || Infinity) - (b.tabIndex || Infinity))
}

function rememberFocus(event: FocusEvent) {
  if (event.target instanceof HTMLElement) lastFocusedElement = event.target
}

function focusDialog(target: HTMLElement | null = null) {
  if (target && dialogRef.value?.contains(target) && isAvailable(target)) target.focus()
  else (focusableElements()[0] ?? dialogRef.value)?.focus()
}

function handleTab(event: KeyboardEvent) {
  if (event.altKey || event.ctrlKey || event.metaKey || event.isComposing) return
  const elements = focusableElements()
  // Select moves its search input into a body Teleport. Let it receive focus and
  // handle Tab first, then resume the dialog order from its original trigger.
  const active = dialogRef.value?.contains(document.activeElement)
    ? document.activeElement
    : lastFocusedElement
  const index = elements.findIndex(element => element === active)
  const nextIndex = index < 0
    ? (event.shiftKey ? elements.length - 1 : 0)
    : (index + (event.shiftKey ? -1 : 1) + elements.length) % elements.length
  event.preventDefault()
  ;(elements[nextIndex] ?? dialogRef.value)?.focus()
}

const dialogLayer = createDialogLayer(
  () => {
    if (props.show && props.closeOnEscape) emit('close')
  },
  { onTab: handleTab, onRestoreFocus: focusDialog, onTopChange: top => { isTopDialog.value = top } }
)

let opened = false
let focusRequest = 0
watch(() => [props.show, props.zIndex] as const, ([show, zIndex]) => {
  const wasTop = dialogLayer.isTop()
  const opening = show && !opened
  const closing = !show && opened
  opened = show
  if (opening) {
    previousActiveElement = document.activeElement instanceof HTMLElement ? document.activeElement : null
    lastFocusedElement = null
  }
  dialogLayer.update(show, zIndex)
  if (opening) {
    const request = ++focusRequest
    nextTick(() => {
      if (request !== focusRequest || !props.show || !dialogLayer.isTop()) return
      if (modalBodyRef.value) modalBodyRef.value.scrollTop = 0
      focusDialog()
    })
  } else if (closing) {
    ++focusRequest
    if (wasTop) dialogLayer.restoreFocus(previousActiveElement)
    previousActiveElement = null
  }
}, { immediate: true, flush: 'sync' })

onBeforeUnmount(() => {
  ++focusRequest
  const wasTop = dialogLayer.isTop()
  dialogLayer.remove()
  if (wasTop) dialogLayer.restoreFocus(previousActiveElement)
})
</script>
