<script setup lang="ts">
import Icon from '@/components/icons/Icon.vue'
import { onBeforeUnmount, onMounted, onUpdated, ref, useId, useTemplateRef, nextTick } from 'vue'

const props = withDefaults(defineProps<{
  content?: string
  trigger?: 'hover' | 'click'
  widthClass?: string
}>(), {
  trigger: 'hover',
  widthClass: 'w-64',
})

const show = ref(false)
const tooltipId = useId()
const triggerRef = useTemplateRef<HTMLElement>('trigger')
const tooltipRef = useTemplateRef<HTMLElement>('tooltip')
const tooltipStyle = ref<Record<string, string>>({ top: '0px', left: '0px' })
const contentMaxHeight = ref('none')
const placement = ref<'top' | 'bottom'>('top')
const hasInteractiveTrigger = ref(false)

function updateTriggerAccessibility() {
  const target = triggerRef.value?.querySelector<HTMLElement>('button, a[href], input, select, textarea, [tabindex]')
  hasInteractiveTrigger.value = !!target
  if (target) {
    const descriptions = (target.getAttribute('aria-describedby') || '').split(/\s+/).filter(id => id && id !== tooltipId)
    if (show.value) descriptions.push(tooltipId)
    if (descriptions.length) target.setAttribute('aria-describedby', descriptions.join(' '))
    else target.removeAttribute('aria-describedby')
  }
}

function focusTrigger() {
  const target = hasInteractiveTrigger.value
    ? triggerRef.value?.querySelector<HTMLElement>('button, a[href], input, select, textarea, [tabindex]')
    : triggerRef.value
  target?.focus()
}

function dismissTooltip() {
  if (isInside(tooltipRef.value, document.activeElement)) focusTrigger()
  closeTooltip()
}

function openTooltip() {
  show.value = true
  nextTick(updatePosition)
}

function closeTooltip() {
  show.value = false
}

function onFocus() {
  if (props.trigger === 'hover') openTooltip()
}

function onBlur(event: FocusEvent) {
  if (props.trigger !== 'hover' || isInside(tooltipRef.value, event.relatedTarget)) return
  if (!isInside(triggerRef.value, event.relatedTarget)) closeTooltip()
}

function onTriggerKeydown(event: KeyboardEvent) {
  if (event.target !== event.currentTarget || event.isComposing) return
  if (event.key === 'Enter' || event.key === ' ') {
    event.preventDefault()
    event.stopPropagation()
    if (show.value) closeTooltip()
    else openTooltip()
  }
}

function onEnter() {
  if (props.trigger !== 'hover') return
  openTooltip()
}

function isInside(container: HTMLElement | null, target: EventTarget | null): boolean {
  return target instanceof Node && !!container?.contains(target)
}

// 悬停模式下指针在触发图标与提示框之间往返时保持打开，便于选中提示里的文字。
function onLeave(event: MouseEvent) {
  if (props.trigger !== 'hover') return
  if (isInside(tooltipRef.value, event.relatedTarget)) return
  if (isInside(triggerRef.value, document.activeElement)) return
  closeTooltip()
}

function onTooltipLeave(event: MouseEvent) {
  if (props.trigger !== 'hover') return
  if (isInside(triggerRef.value, event.relatedTarget)) return
  if (isInside(triggerRef.value, document.activeElement)) return
  closeTooltip()
}

function onClick(event: MouseEvent) {
  if (props.trigger !== 'click') return
  event.stopPropagation()
  if (show.value) {
    closeTooltip()
    return
  }
  openTooltip()
}

function onDocumentClick(event: MouseEvent) {
  if (props.trigger !== 'click' || !show.value) return
  const target = event.target as Node | null
  if (!target) return
  if (triggerRef.value?.contains(target) || tooltipRef.value?.contains(target)) return
  closeTooltip()
}

function onDocumentKeydown(event: KeyboardEvent) {
  if (show.value && event.key === 'Escape') {
    event.preventDefault()
    event.stopPropagation()
    dismissTooltip()
  }
}

function onViewportChange() {
  if (!show.value) return
  updatePosition()
}

function updatePosition() {
  const el = triggerRef.value
  const tooltip = tooltipRef.value
  if (!el || !tooltip) return
  const rect = el.getBoundingClientRect()
  const padding = 8
  const gap = 8
  const viewportWidth = window.innerWidth
  const viewportHeight = window.innerHeight
  const maxWidth = Math.max(0, viewportWidth - padding * 2)
  tooltip.style.maxWidth = `${maxWidth}px`
  const width = Math.min(tooltip.getBoundingClientRect().width, maxWidth)
  const content = tooltip.firstElementChild as HTMLElement | null
  const height = content?.scrollHeight || tooltip.getBoundingClientRect().height
  const above = Math.max(0, rect.top - gap - padding)
  const below = Math.max(0, viewportHeight - rect.bottom - gap - padding)
  placement.value = height <= above || above >= below ? 'top' : 'bottom'
  const availableHeight = Math.min(
    Math.max(0, viewportHeight - padding * 2),
    placement.value === 'top' ? above : below
  )
  contentMaxHeight.value = `${availableHeight}px`
  const renderedHeight = Math.min(height, availableHeight)
  const top = placement.value === 'top' ? rect.top - gap - renderedHeight : rect.bottom + gap
  tooltipStyle.value = {
    top: `${Math.max(padding, Math.min(top, viewportHeight - padding - renderedHeight))}px`,
    left: `${Math.max(padding, Math.min(rect.left + rect.width / 2 - width / 2, viewportWidth - padding - width))}px`,
    maxWidth: `${maxWidth}px`,
  }
}

onMounted(() => {
  updateTriggerAccessibility()
  document.addEventListener('click', onDocumentClick, true)
  document.addEventListener('keydown', onDocumentKeydown, true)
  window.addEventListener('resize', onViewportChange)
  window.addEventListener('scroll', onViewportChange, true)
})

onUpdated(updateTriggerAccessibility)

onBeforeUnmount(() => {
  document.removeEventListener('click', onDocumentClick, true)
  document.removeEventListener('keydown', onDocumentKeydown, true)
  window.removeEventListener('resize', onViewportChange)
  window.removeEventListener('scroll', onViewportChange, true)
})
</script>

<template>
  <div
    ref="trigger"
    class="group relative ml-1 inline-flex items-center rounded align-middle focus-visible:outline focus-visible:outline-2 focus-visible:outline-primary-500"
    :role="hasInteractiveTrigger ? undefined : 'button'"
    :tabindex="hasInteractiveTrigger ? undefined : 0"
    :aria-label="hasInteractiveTrigger ? undefined : (content || 'Help')"
    :aria-describedby="!hasInteractiveTrigger && show ? tooltipId : undefined"
    :aria-expanded="hasInteractiveTrigger ? undefined : show"
    @focusin="onFocus"
    @focusout="onBlur"
    @keydown="onTriggerKeydown"
    @keydown.esc="onDocumentKeydown"
    @mouseenter="onEnter"
    @mouseleave="onLeave"
    @click="onClick"
  >
    <!-- Trigger Icon -->
    <slot name="trigger">
      <Icon name="infoCircleSimple" size="none" class="h-4 w-4 cursor-help text-gray-400 transition-colors hover:text-primary-600 dark:text-gray-500 dark:hover:text-primary-400" :stroke-width="2" />
    </slot>

    <!-- Teleport to body to escape modal overflow clipping -->
    <Teleport to="body">
      <!-- before: 伪元素向下延伸一段透明区域，盖住提示框与触发图标之间的空隙，让指针能连续移入提示框。 -->
      <div
        ref="tooltip"
        :id="tooltipId"
        v-show="show"
        role="tooltip"
        :class="[
          'fixed z-[99999] rounded-lg bg-gray-900 text-xs leading-relaxed text-white shadow-xl ring-1 ring-white/10 selection:bg-primary-200 selection:text-gray-900 before:absolute before:inset-x-0 before:h-3 dark:bg-gray-800 dark:selection:bg-primary-200 dark:selection:text-gray-900',
          placement === 'top' ? 'before:top-full' : 'before:bottom-full',
          props.widthClass,
        ]"
        :style="tooltipStyle"
        @mouseleave="onTooltipLeave"
        @focusout="onBlur"
        @keydown.esc="onDocumentKeydown"
      >
        <div class="overflow-y-auto break-words rounded-lg p-3" :style="{ maxHeight: contentMaxHeight }">
          <button
            v-if="props.trigger === 'click'"
            type="button"
            class="absolute right-1.5 top-1.5 rounded p-1 text-gray-300 transition-colors hover:bg-white/10 hover:text-white"
            aria-label="Close"
            @click.stop="dismissTooltip"
          >
            <Icon name="x" size="none" class="h-3.5 w-3.5" :stroke-width="2" />
          </button>
          <slot>{{ content }}</slot>
        </div>
        <div
          class="pointer-events-none absolute left-1/2 h-2 w-2 -translate-x-1/2 rotate-45 bg-gray-900 dark:bg-gray-800"
          :class="placement === 'top' ? '-bottom-1' : '-top-1'"
        ></div>
      </div>
    </Teleport>
  </div>
</template>
