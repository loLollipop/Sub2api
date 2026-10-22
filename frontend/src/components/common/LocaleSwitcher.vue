<template>
  <div class="relative" ref="dropdownRef" @keydown.esc.stop.prevent="closeDropdown(true)" @focusout="handleFocusOut">
    <button
      ref="triggerRef"
      type="button"
      @click="toggleDropdown"
      @keydown.down.prevent="openAndFocus(0)"
      @keydown.up.prevent="openAndFocus(-1)"
      :disabled="switching"
      class="flex items-center gap-1.5 rounded-lg px-2 py-1.5 text-sm font-medium text-gray-600 transition-colors hover:bg-gray-100 dark:text-gray-300 dark:hover:bg-dark-700"
      :title="`${languageLabel} · ${currentLocale?.name}`"
      :aria-label="languageLabel"
      :aria-expanded="isOpen"
      :aria-controls="isOpen ? optionsId : undefined"
    >
      <svg v-if="compact" width="18" height="18" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="1.75" stroke-linecap="round" stroke-linejoin="round" aria-hidden="true"><path d="m5 8 6 6m-7 0 6-6 2-3M2 5h12M7 2h1m14 20-5-12-5 12m2-4h6" /></svg>
      <template v-else>
        <span class="text-base">{{ currentLocale?.flag }}</span>
        <span class="hidden sm:inline">{{ currentLocale?.code.toUpperCase() }}</span>
        <Icon
          name="chevronDown"
          size="xs"
          class="text-gray-400 transition-transform duration-200"
          :class="{ 'rotate-180': isOpen }"
        />
      </template>
    </button>

    <transition name="dropdown">
      <div
        v-if="isOpen"
        :id="optionsId"
        ref="optionsRef"
        role="group"
        :aria-label="languageLabel"
        class="locale-options absolute right-0 z-50 mt-1 w-32 overflow-hidden rounded-lg border border-gray-200 bg-white shadow-lg dark:border-dark-700 dark:bg-dark-800"
        @keydown="handleOptionsKeydown"
      >
        <button
          v-for="locale in availableLocales"
          :key="locale.code"
          type="button"
          :disabled="switching"
          :aria-pressed="locale.code === currentLocaleCode"
          @click="selectLocale(locale.code)"
          class="locale-option flex w-full items-center gap-2 px-3 py-2 text-sm text-gray-700 transition-colors hover:bg-gray-100 dark:text-gray-200 dark:hover:bg-dark-700"
          :class="{
            'bg-primary-50 text-primary-600 dark:bg-primary-900/20 dark:text-primary-400':
              locale.code === currentLocaleCode
          }"
        >
          <span class="text-base">{{ locale.flag }}</span>
          <span>{{ locale.name }}</span>
          <Icon v-if="locale.code === currentLocaleCode" name="check" size="sm" class="ml-auto text-primary-500" />
        </button>
      </div>
    </transition>
  </div>
</template>

<script setup lang="ts">
import { ref, computed, nextTick, onMounted, onBeforeUnmount, useId } from 'vue'
import { useI18n } from 'vue-i18n'
import Icon from '@/components/icons/Icon.vue'
import { setLocale, availableLocales } from '@/i18n'

const { locale } = useI18n()
defineProps<{ compact?: boolean }>()

const isOpen = ref(false)
const dropdownRef = ref<HTMLElement | null>(null)
const triggerRef = ref<HTMLButtonElement | null>(null)
const optionsRef = ref<HTMLElement | null>(null)
const optionsId = useId()
const switching = ref(false)

const currentLocaleCode = computed(() => locale.value)
const currentLocale = computed(() => availableLocales.find((l) => l.code === locale.value))
const languageLabel = computed(() => locale.value.startsWith('zh') ? '切换语言' : 'Language')

function toggleDropdown() {
  isOpen.value = !isOpen.value
}

function closeDropdown(restoreFocus = false) {
  isOpen.value = false
  if (restoreFocus) triggerRef.value?.focus()
}

function options() {
  return Array.from(optionsRef.value?.querySelectorAll<HTMLButtonElement>('button:not(:disabled)') ?? [])
}

async function openAndFocus(index: number) {
  isOpen.value = true
  await nextTick()
  options().at(index)?.focus()
}

function handleOptionsKeydown(event: KeyboardEvent) {
  const items = options()
  if (!items.length) return
  const index = items.indexOf(document.activeElement as HTMLButtonElement)
  const target = event.key === 'ArrowDown' ? (index + 1) % items.length : event.key === 'ArrowUp' ? (index - 1 + items.length) % items.length : event.key === 'Home' ? 0 : event.key === 'End' ? items.length - 1 : -1
  if (target < 0) return
  event.preventDefault()
  items[target]?.focus()
}

function handleFocusOut(event: FocusEvent) {
  if (!(event.relatedTarget instanceof Node) || !dropdownRef.value?.contains(event.relatedTarget)) closeDropdown()
}

async function selectLocale(code: string) {
  if (switching.value || code === currentLocaleCode.value) {
    closeDropdown(true)
    return
  }
  switching.value = true
  try {
    await setLocale(code)
    isOpen.value = false
    // 分组名称等内容由后端按请求语言返回，整页刷新让已加载的数据换成新语言
    window.location.reload()
  } finally {
    switching.value = false
  }
}

function handleClickOutside(event: MouseEvent) {
  if (event.target instanceof Node && !dropdownRef.value?.contains(event.target)) {
    closeDropdown()
  }
}

onMounted(() => {
  document.addEventListener('click', handleClickOutside)
})

onBeforeUnmount(() => {
  document.removeEventListener('click', handleClickOutside)
})
</script>

<style scoped>
.dropdown-enter-active,
.dropdown-leave-active {
  transition: all 0.15s ease;
}

.dropdown-enter-from,
.dropdown-leave-to {
  opacity: 0;
  transform: scale(0.95) translateY(-4px);
}
</style>
