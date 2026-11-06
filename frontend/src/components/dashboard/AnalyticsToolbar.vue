<template>
  <div class="analytics-toolbar">
    <div class="analytics-date"><slot name="date" /></div>
    <div class="analytics-granularity"><slot name="granularity" /></div>
    <button v-if="refresh" type="button" class="analytics-refresh" :disabled="loading" :title="t('common.refresh')" :aria-label="t('common.refresh')" @click="$emit('refresh')">
      <Icon name="refresh" size="md" :class="{ 'animate-spin': loading }" />
    </button>
  </div>
</template>

<script setup lang="ts">
import { useI18n } from 'vue-i18n'
import Icon from '@/components/icons/Icon.vue'
withDefaults(defineProps<{ refresh?: boolean; loading?: boolean }>(), { refresh: false, loading: false })
defineEmits<{ refresh: [] }>()
const { t } = useI18n()
</script>

<style scoped>
.analytics-toolbar { display: flex; align-items: center; gap: 12px; min-width: 0; width: 100%; }
.analytics-date { flex: 0 1 auto; min-width: 0; }
.analytics-toolbar .analytics-date :deep(.date-picker-trigger) { min-height: 32px; height: 32px; max-width: 100%; padding: 4px 10px; gap: 7px; border: 1px solid transparent; border-radius: 6px; background: transparent; box-shadow: none; font-size: 13px; line-height: 20px; }
.analytics-toolbar .analytics-date :deep(.date-picker-trigger:hover), .analytics-toolbar .analytics-date :deep(.date-picker-trigger-open) { border-color: transparent; background: var(--signal-hover); box-shadow: none; }
.analytics-date :deep(.date-picker-value) { min-width: 0; overflow: hidden; text-overflow: ellipsis; white-space: nowrap; }
.analytics-date :deep(.date-picker-chevron) { margin-left: 0; }
.analytics-granularity { flex: 0 0 auto; width: 76px; }
.analytics-toolbar .analytics-granularity :deep(.select-trigger) { min-height: 32px; height: 32px; padding: 4px 2px; border: 0; border-radius: 4px; background: transparent; box-shadow: none; font-size: 13px; line-height: 20px; }
.analytics-toolbar .analytics-granularity :deep(.select-trigger:hover), .analytics-toolbar .analytics-granularity :deep(.select-trigger-open) { background: var(--signal-hover); }
.analytics-granularity :deep(.select-value) { flex: 0 1 auto; }
.analytics-granularity :deep(.select-icon svg) { width: 14px; height: 14px; }
.analytics-toolbar :deep(:is(.date-picker-trigger, .select-trigger):focus-visible) { outline: 2px solid var(--signal-accent); outline-offset: 2px; }
.analytics-refresh { display: grid; place-items: center; flex: 0 0 32px; width: 32px; height: 32px; margin-left: auto; border-radius: 6px; color: var(--signal-muted); }
.analytics-refresh:hover { background: var(--signal-hover); color: var(--signal-text); }
.analytics-refresh:disabled { cursor: default; opacity: .5; }
.analytics-refresh:focus-visible { outline: 2px solid var(--signal-accent); outline-offset: 2px; }
@media (max-width: 639px) { .analytics-toolbar { gap: 9px; } .analytics-granularity { width: 80px; } .analytics-date { max-width: calc(100% - 130px); } }
</style>
