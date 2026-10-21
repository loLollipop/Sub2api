<template>
  <div ref="containerRef" class="consumer-navigation" @focusout="handleFocusOut">
    <nav class="consumer-nav" :aria-label="t('nav.myAccount')">
      <router-link
        v-for="item in primaryUserNavItems" :key="item.path" :to="item.path"
        class="consumer-nav-link" :class="{ 'consumer-nav-link-active': isActive(item.path) }"
        :aria-current="isActive(item.path) ? 'page' : undefined"
        :data-tour="item.path === '/keys' ? 'sidebar-my-keys' : undefined"
        @click="handleNavigation(item.path)"
      >
        <component :is="item.icon" class="h-4 w-4 shrink-0" aria-hidden="true" />
        <span>{{ item.label }}</span>
      </router-link>
      <div class="consumer-more">
        <button
          ref="triggerRef" type="button" class="consumer-nav-link consumer-more-trigger"
          :class="{ 'consumer-nav-link-active': moreActive }" :aria-expanded="open"
          :aria-controls="menuId"
          @click="open = !open" @keydown.down.prevent="openAndFocus(0)" @keydown.up.prevent="openAndFocus(-1)"
        >
          <Icon name="grid" size="sm" aria-hidden="true" />{{ t('common.more') }}
          <Icon name="chevronDown" size="xs" :class="{ 'rotate-180': open }" aria-hidden="true" />
        </button>
        <div v-if="open" :id="menuId" ref="menuRef" class="consumer-more-menu" @keydown="handleMenuKeydown">
          <router-link
            v-for="item in moreUserNavItems" :key="item.path" :to="item.path"
            class="consumer-more-link" :class="{ 'consumer-more-link-active': isActive(item.path) }"
            :aria-current="isActive(item.path) ? 'page' : undefined" @click="handleNavigation(item.path)"
          >
            <span v-if="item.iconSvg" class="consumer-custom-icon h-4 w-4 shrink-0" aria-hidden="true" v-html="sanitizeSvg(item.iconSvg)"></span>
            <component v-else :is="item.icon" class="h-4 w-4 shrink-0" aria-hidden="true" />
            <span>{{ item.label }}</span>
          </router-link>
          <button type="button" class="consumer-more-link consumer-theme-toggle" @click="toggleTheme">
            <Icon :name="isDark ? 'sun' : 'moon'" size="sm" aria-hidden="true" />
            {{ isDark ? t('nav.lightMode') : t('nav.darkMode') }}
          </button>
        </div>
      </div>
    </nav>
  </div>
</template>

<script setup lang="ts">
import { computed, nextTick, onBeforeUnmount, onMounted, ref, useId, watch } from 'vue'
import { useMutationObserver } from '@vueuse/core'
import { useRoute } from 'vue-router'
import { useI18n } from 'vue-i18n'
import { useOnboardingStore } from '@/stores'
import { useConsoleNavigation } from '@/composables/useConsoleNavigation'
import { sanitizeSvg } from '@/utils/sanitize'
import Icon from '@/components/icons/Icon.vue'

const { t } = useI18n()
const route = useRoute()
const onboardingStore = useOnboardingStore()
const { primaryUserNavItems, moreUserNavItems } = useConsoleNavigation()
const open = ref(false)
const containerRef = ref<HTMLElement | null>(null)
const menuRef = ref<HTMLElement | null>(null)
const triggerRef = ref<HTMLButtonElement | null>(null)
const menuId = useId()
const isDark = ref(document.documentElement.classList.contains('dark'))
useMutationObserver(document.documentElement, () => {
  isDark.value = document.documentElement.classList.contains('dark')
}, { attributes: true, attributeFilter: ['class'] })
const moreActive = computed(() => moreUserNavItems.value.some(item => isActive(item.path)))

function isActive(path: string) {
  return route.path === path || route.path.startsWith(path + '/')
}
function close(restoreFocus = false) {
  open.value = false
  if (restoreFocus) triggerRef.value?.focus()
}
function handleNavigation(path: string) {
  close()
  if (path === '/keys' && onboardingStore.isCurrentStep('[data-tour="sidebar-my-keys"]')) {
    onboardingStore.nextStep(500)
  }
}
function toggleTheme() {
  const dark = !document.documentElement.classList.contains('dark')
  document.documentElement.classList.toggle('dark', dark)
  localStorage.setItem('theme', dark ? 'dark' : 'light')
  isDark.value = dark
}
function menuItems() {
  return Array.from(menuRef.value?.querySelectorAll<HTMLElement>('a, button') ?? [])
}
async function openAndFocus(index: number) {
  open.value = true
  await nextTick()
  menuItems().at(index)?.focus()
}
function handleMenuKeydown(event: KeyboardEvent) {
  const items = menuItems()
  const index = items.indexOf(document.activeElement as HTMLElement)
  let target: number
  switch (event.key) {
    case 'ArrowDown': target = (index + 1) % items.length; break
    case 'ArrowUp': target = (index - 1 + items.length) % items.length; break
    case 'Home': target = 0; break
    case 'End': target = items.length - 1; break
    default: return
  }
  event.preventDefault()
  items[target]?.focus()
}
function handlePointerDown(event: PointerEvent) {
  if (open.value && !containerRef.value?.contains(event.target as Node)) close()
}
function handleKeydown(event: KeyboardEvent) {
  if (event.key === 'Escape' && open.value) {
    event.preventDefault()
    close(true)
  }
}
function handleFocusOut(event: FocusEvent) {
  if (event.relatedTarget && !containerRef.value?.contains(event.relatedTarget as Node)) close()
}
watch(() => route.fullPath, () => close())
onMounted(() => {
  document.addEventListener('pointerdown', handlePointerDown)
  document.addEventListener('keydown', handleKeydown)
})
onBeforeUnmount(() => {
  document.removeEventListener('pointerdown', handlePointerDown)
  document.removeEventListener('keydown', handleKeydown)
})
</script>
