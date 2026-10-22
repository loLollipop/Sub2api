<template>
  <div class="relative" ref="containerRef">
    <button
      ref="triggerRef"
      type="button"
      :aria-expanded="isOpen"
      :aria-controls="isOpen ? panelId : undefined"
      @click="toggle"
      :class="['date-picker-trigger', isOpen && 'date-picker-trigger-open']"
    >
      <span class="date-picker-icon">
        <Icon name="calendar" size="sm" />
      </span>
      <span class="date-picker-value">
        {{ displayValue }}
      </span>
      <span class="date-picker-chevron">
        <Icon
          name="chevronDown"
          size="sm"
          :class="['transition-transform duration-200', isOpen && 'rotate-180']"
        />
      </span>
    </button>

    <Transition name="date-picker-dropdown">
      <div v-if="isOpen" :id="panelId" class="date-picker-dropdown" role="group" :aria-label="t('dates.selectDateRange')">
        <!-- Quick presets -->
        <div class="date-picker-presets">
          <button
            v-for="preset in presets"
            :key="preset.value"
            @click="selectPreset(preset)"
            :class="['date-picker-preset', isPresetActive(preset) && 'date-picker-preset-active']"
          >
            {{ t(preset.labelKey) }}
          </button>
        </div>

        <div class="date-picker-divider"></div>

        <!-- Custom date range inputs -->
        <div class="date-picker-custom" :class="includeTime && 'date-picker-custom-time'">
          <div class="date-picker-field">
            <label class="date-picker-label">{{ t('dates.startDate') }}</label>
            <input
              :type="includeTime ? 'datetime-local' : 'date'"
              :step="includeTime ? 1 : undefined"
              :aria-label="t('dates.startDate')"
              v-model="localStartDate"
              :max="localEndDate || maximumDate"
              class="date-picker-input"
              @change="onDateChange"
            />
          </div>
          <div class="date-picker-separator">
            <Icon name="arrowRight" size="sm" class="text-gray-400" />
          </div>
          <div class="date-picker-field">
            <label class="date-picker-label">{{ t('dates.endDate') }}</label>
            <input
              :type="includeTime ? 'datetime-local' : 'date'"
              :step="includeTime ? 1 : undefined"
              :aria-label="t('dates.endDate')"
              v-model="localEndDate"
              :min="localStartDate"
              :max="maximumDate"
              class="date-picker-input"
              @change="onDateChange"
            />
          </div>
        </div>

        <!-- Apply button -->
        <div class="date-picker-actions">
          <button @click="apply" :disabled="!validRange" class="date-picker-apply disabled:cursor-not-allowed disabled:opacity-50">
            {{ t('dates.apply') }}
          </button>
        </div>
      </div>
    </Transition>
  </div>
</template>

<script setup lang="ts">
import { ref, computed, watch, onMounted, onUnmounted, useId } from 'vue'
import { useI18n } from 'vue-i18n'
import Icon from '@/components/icons/Icon.vue'

interface DatePreset {
  labelKey: string
  value: string
  getRange: () => { start: string; end: string }
}

interface Props {
  startDate: string
  endDate: string
  includeTime?: boolean
}

interface Emits {
  (e: 'update:startDate', value: string): void
  (e: 'update:endDate', value: string): void
  (e: 'change', range: { startDate: string; endDate: string; preset: string | null }): void
}

const props = withDefaults(defineProps<Props>(), { includeTime: false })
const emit = defineEmits<Emits>()

const { t, locale } = useI18n()

const isOpen = ref(false)
const panelId = `date-range-${useId()}`
const triggerRef = ref<HTMLButtonElement | null>(null)
const containerRef = ref<HTMLElement | null>(null)
const normalizeInput = (value: string, end = false): string => {
  if (!props.includeTime || !value) return value
  if (!value.includes('T')) return value + (end ? 'T23:59:59' : 'T00:00:00')
  return value.length === 16 ? value + ':00' : value.slice(0, 19)
}
const localStartDate = ref(normalizeInput(props.startDate))
const localEndDate = ref(normalizeInput(props.endDate, true))
const activePreset = ref<string | null>('last24Hours')
const appliedStartDate = ref(localStartDate.value)
const appliedEndDate = ref(localEndDate.value)
const appliedPreset = ref<string | null>('last24Hours')
const maximumDate = computed(() => normalizeInput(tomorrow.value, true))
const validRange = computed(() => {
  const start = new Date(localStartDate.value).getTime()
  const end = new Date(localEndDate.value).getTime()
  return Number.isFinite(start) && Number.isFinite(end) && start <= end
})

// Tomorrow's date - used for max date to handle timezone differences
// When user is in a timezone behind the server, "today" on server might be "tomorrow" locally
const tomorrow = computed(() => {
  const d = new Date()
  d.setDate(d.getDate() + 1)
  return formatDateToString(d)
})

// Helper function to format date to YYYY-MM-DD using local timezone
const formatDateToString = (date: Date): string => {
  const year = date.getFullYear()
  const month = String(date.getMonth() + 1).padStart(2, '0')
  const day = String(date.getDate()).padStart(2, '0')
  return `${year}-${month}-${day}`
}

const formatTimestamp = (date: Date): string => {
  const time = [date.getHours(), date.getMinutes(), date.getSeconds()]
    .map(value => String(value).padStart(2, '0')).join(':')
  return `${formatDateToString(date)}T${time}`
}

const presets: DatePreset[] = [
  {
    labelKey: 'dates.today',
    value: 'today',
    getRange: () => {
      const t = formatDateToString(new Date())
      return { start: t, end: t }
    }
  },
  {
    labelKey: 'dates.yesterday',
    value: 'yesterday',
    getRange: () => {
      const d = new Date()
      d.setDate(d.getDate() - 1)
      const yesterday = formatDateToString(d)
      return { start: yesterday, end: yesterday }
    }
  },
  {
    labelKey: 'dates.last24Hours',
    value: 'last24Hours',
    getRange: () => {
      const end = new Date()
      const start = new Date(end.getTime() - 24 * 60 * 60 * 1000)
      return {
        start: props.includeTime ? formatTimestamp(start) : formatDateToString(start),
        end: props.includeTime ? formatTimestamp(end) : formatDateToString(end)
      }
    }
  },
  {
    labelKey: 'dates.last7Days',
    value: '7days',
    getRange: () => {
      const end = formatDateToString(new Date())
      const d = new Date()
      d.setDate(d.getDate() - 6)
      const start = formatDateToString(d)
      return { start, end }
    }
  },
  {
    labelKey: 'dates.last14Days',
    value: '14days',
    getRange: () => {
      const end = formatDateToString(new Date())
      const d = new Date()
      d.setDate(d.getDate() - 13)
      const start = formatDateToString(d)
      return { start, end }
    }
  },
  {
    labelKey: 'dates.last30Days',
    value: '30days',
    getRange: () => {
      const end = formatDateToString(new Date())
      const d = new Date()
      d.setDate(d.getDate() - 29)
      const start = formatDateToString(d)
      return { start, end }
    }
  },
  {
    labelKey: 'dates.thisMonth',
    value: 'thisMonth',
    getRange: () => {
      const now = new Date()
      const start = formatDateToString(new Date(now.getFullYear(), now.getMonth(), 1))
      return { start, end: formatDateToString(new Date()) }
    }
  },
  {
    labelKey: 'dates.lastMonth',
    value: 'lastMonth',
    getRange: () => {
      const now = new Date()
      const start = formatDateToString(new Date(now.getFullYear(), now.getMonth() - 1, 1))
      const end = formatDateToString(new Date(now.getFullYear(), now.getMonth(), 0))
      return { start, end }
    }
  }
]

const displayValue = computed(() => {
  if (appliedPreset.value) {
    const preset = presets.find((p) => p.value === appliedPreset.value)
    if (preset) return t(preset.labelKey)
  }

  if (appliedStartDate.value && appliedEndDate.value) {
    if (appliedStartDate.value === appliedEndDate.value) {
      return formatDate(appliedStartDate.value)
    }
    return `${formatDate(appliedStartDate.value)} - ${formatDate(appliedEndDate.value)}`
  }

  return t('dates.selectDateRange')
})

const formatDate = (dateStr: string): string => {
  const date = new Date(dateStr.includes('T') ? dateStr : dateStr + 'T00:00:00')
  const dateLocale = locale.value === 'zh' ? 'zh-CN' : 'en-US'
  if (props.includeTime) {
    return date.toLocaleString(dateLocale, {
      year: 'numeric', month: '2-digit', day: '2-digit',
      hour: '2-digit', minute: '2-digit', second: '2-digit', hour12: false,
    })
  }
  return date.toLocaleDateString(dateLocale, { month: 'short', day: 'numeric' })
}

const isPresetActive = (preset: DatePreset): boolean => {
  return activePreset.value === preset.value
}

const selectPreset = (preset: DatePreset) => {
  const range = preset.getRange()
  localStartDate.value = normalizeInput(range.start)
  localEndDate.value = normalizeInput(range.end, true)
  activePreset.value = preset.value
}

const findMatchingPreset = (start: string, end: string): string | null => {
  return presets.find((preset) => {
    const range = preset.getRange()
    return normalizeInput(range.start) === start && normalizeInput(range.end, true) === end
  })?.value ?? null
}

const onDateChange = () => {
  activePreset.value = findMatchingPreset(localStartDate.value, localEndDate.value)
}

const resetDraft = () => {
  localStartDate.value = appliedStartDate.value
  localEndDate.value = appliedEndDate.value
  activePreset.value = appliedPreset.value
}

const close = () => {
  isOpen.value = false
  resetDraft()
}

const toggle = () => {
  if (isOpen.value) close()
  else {
    resetDraft()
    isOpen.value = true
  }
}

const apply = () => {
  if (!validRange.value) return
  localStartDate.value = normalizeInput(localStartDate.value)
  localEndDate.value = normalizeInput(localEndDate.value, true)
  appliedStartDate.value = localStartDate.value
  appliedEndDate.value = localEndDate.value
  appliedPreset.value = activePreset.value
  emit('update:startDate', localStartDate.value)
  emit('update:endDate', localEndDate.value)
  emit('change', {
    startDate: localStartDate.value,
    endDate: localEndDate.value,
    preset: activePreset.value
  })
  close()
  triggerRef.value?.focus()
}

// refreshActivePreset re-applies the currently active quick preset so an
// external refresh button can slide a relative window (e.g. 近24小时) to
// "now". Returns the recomputed range, or null when the user set a custom
// range — a custom range must stay exactly where the user put it.
const refreshActivePreset = (): { startDate: string; endDate: string } | null => {
  if (!appliedPreset.value) return null
  const preset = presets.find((p) => p.value === appliedPreset.value)
  if (!preset) return null
  const range = preset.getRange()
  appliedStartDate.value = normalizeInput(range.start)
  appliedEndDate.value = normalizeInput(range.end, true)
  if (!isOpen.value) resetDraft()
  return { startDate: appliedStartDate.value, endDate: appliedEndDate.value }
}

defineExpose({ refreshActivePreset })

const handleClickOutside = (event: MouseEvent) => {
  if (containerRef.value && !containerRef.value.contains(event.target as Node)) {
    close()
  }
}

const handleEscape = (event: KeyboardEvent) => {
  if (event.key === 'Escape' && isOpen.value) {
    close()
    triggerRef.value?.focus()
  }
}

// Preserve a selected preset when the parent echoes our applied range. A
// relative preset's clock may have advanced since selection. External changes
// still reset/re-detect the preset, and manual input calls onDateChange above.
watch(
  () => [props.startDate, props.endDate, props.includeTime] as const,
  ([start, end]) => {
    const normalizedStart = normalizeInput(start)
    const normalizedEnd = normalizeInput(end, true)
    const unchanged = normalizedStart === appliedStartDate.value && normalizedEnd === appliedEndDate.value
    appliedStartDate.value = normalizedStart
    appliedEndDate.value = normalizedEnd
    if (!unchanged) appliedPreset.value = findMatchingPreset(normalizedStart, normalizedEnd)
    if (!unchanged || !isOpen.value) resetDraft()
  }
)

onMounted(() => {
  document.addEventListener('click', handleClickOutside)
  document.addEventListener('keydown', handleEscape)
  // Initialize active preset detection
  appliedPreset.value = findMatchingPreset(appliedStartDate.value, appliedEndDate.value)
  resetDraft()
})

onUnmounted(() => {
  document.removeEventListener('click', handleClickOutside)
  document.removeEventListener('keydown', handleEscape)
})
</script>

<style scoped>
.date-picker-trigger {
  @apply flex items-center gap-2;
  @apply rounded-lg px-3 py-2 text-sm;
  @apply bg-white dark:bg-dark-800;
  @apply border border-gray-200 dark:border-dark-600;
  @apply text-gray-700 dark:text-gray-300;
  @apply transition-all duration-200;
  @apply focus:border-primary-500 focus:outline-none focus:ring-2 focus:ring-primary-500/30;
  @apply hover:border-gray-300 dark:hover:border-dark-500;
  @apply cursor-pointer;
}

.date-picker-trigger-open {
  @apply border-primary-500 ring-2 ring-primary-500/30;
}

.date-picker-icon {
  @apply text-gray-400 dark:text-dark-400;
}

.date-picker-value {
  @apply font-medium;
}

.date-picker-chevron {
  @apply text-gray-400 dark:text-dark-400;
}

.date-picker-dropdown {
  @apply absolute left-0 z-[100] mt-2;
  @apply bg-white dark:bg-dark-800;
  @apply rounded-xl;
  @apply border border-gray-200 dark:border-dark-700;
  @apply shadow-lg shadow-black/10 dark:shadow-black/30;
  @apply overflow-hidden;
  @apply min-w-[320px];
}

.date-picker-presets {
  @apply grid grid-cols-2 gap-1 p-2;
}

.date-picker-preset {
  @apply rounded-md px-3 py-1.5 text-xs font-medium;
  @apply text-gray-600 dark:text-gray-400;
  @apply hover:bg-gray-100 dark:hover:bg-dark-700;
  @apply transition-colors duration-150;
}

.date-picker-preset-active {
  @apply bg-primary-100 dark:bg-primary-900/30;
  @apply text-primary-700 dark:text-primary-300;
}

.date-picker-divider {
  @apply border-t border-gray-100 dark:border-dark-700;
}

.date-picker-custom {
  @apply flex items-end gap-2 p-3;
}

.date-picker-custom-time {
  @apply flex-col items-stretch;
}

.date-picker-custom-time .date-picker-separator {
  @apply hidden;
}

.date-picker-field {
  @apply flex-1;
}

.date-picker-label {
  @apply mb-1 block text-xs font-medium text-gray-500 dark:text-gray-400;
}

.date-picker-input {
  @apply w-full rounded-md px-2 py-1.5 text-sm;
  @apply bg-gray-50 dark:bg-dark-700;
  @apply border border-gray-200 dark:border-dark-600;
  @apply text-gray-900 dark:text-gray-100;
  @apply focus:border-primary-500 focus:outline-none focus:ring-2 focus:ring-primary-500/30;
}

.date-picker-input::-webkit-calendar-picker-indicator {
  @apply cursor-pointer opacity-60 hover:opacity-100;
  filter: invert(0.5);
}

.dark .date-picker-input::-webkit-calendar-picker-indicator {
  filter: none;
}

.date-picker-separator {
  @apply flex items-center justify-center pb-1;
}

.date-picker-actions {
  @apply flex justify-end p-2 pt-0;
}

.date-picker-apply {
  @apply rounded-lg px-4 py-1.5 text-sm font-medium;
  @apply bg-primary-600 text-white;
  @apply hover:bg-primary-700;
  @apply transition-colors duration-150;
}

/* Dropdown animation */
.date-picker-dropdown-enter-active,
.date-picker-dropdown-leave-active {
  transition: all 0.2s ease;
}

.date-picker-dropdown-enter-from,
.date-picker-dropdown-leave-to {
  opacity: 0;
  transform: translateY(-8px);
}
</style>
