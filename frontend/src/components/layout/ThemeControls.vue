<template>
  <div class="top-theme-controls" role="group" :aria-label="labels.theme">
    <button type="button" :title="labels.light" :aria-label="labels.light" :aria-pressed="selected === 'light'" @click="selectTheme('light')"><Icon name="sun" size="sm" /></button>
    <button type="button" :title="labels.dark" :aria-label="labels.dark" :aria-pressed="selected === 'dark'" @click="selectTheme('dark')"><Icon name="moon" size="sm" /></button>
    <button type="button" :title="labels.system" :aria-label="labels.system" :aria-pressed="selected === 'system'" @click="selectTheme('system')">
      <svg width="16" height="16" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="1.5" aria-hidden="true"><rect x="3" y="4" width="18" height="13" rx="2" /><path d="M8 21h8m-4-4v4" /></svg>
    </button>
  </div>
</template>

<script setup lang="ts">
import { computed, onBeforeUnmount, onMounted, ref } from 'vue'
import { useI18n } from 'vue-i18n'
import Icon from '@/components/icons/Icon.vue'

type Theme = 'light' | 'dark' | 'system'
const { locale } = useI18n()
const saved = localStorage.getItem('theme')
const selected = ref<Theme>(saved === 'light' || saved === 'dark' ? saved : 'system')
const systemTheme = window.matchMedia('(prefers-color-scheme: dark)')
const labels = computed(() => locale.value.startsWith('zh')
  ? { theme: '外观', light: '浅色', dark: '深色', system: '跟随系统' }
  : { theme: 'Appearance', light: 'Light', dark: 'Dark', system: 'System' })
function applyTheme() {
  document.documentElement.classList.toggle('dark', selected.value === 'dark' || (selected.value === 'system' && systemTheme.matches))
}
function selectTheme(theme: Theme) {
  selected.value = theme
  localStorage.setItem('theme', theme)
  applyTheme()
}
function syncPreference(event: StorageEvent) {
  if (event.key !== 'theme' && event.key !== null) return
  const theme = localStorage.getItem('theme')
  selected.value = theme === 'light' || theme === 'dark' ? theme : 'system'
  applyTheme()
}
onMounted(() => {
  applyTheme()
  systemTheme.addEventListener('change', applyTheme)
  window.addEventListener('storage', syncPreference)
})
onBeforeUnmount(() => {
  systemTheme.removeEventListener('change', applyTheme)
  window.removeEventListener('storage', syncPreference)
})
</script>
