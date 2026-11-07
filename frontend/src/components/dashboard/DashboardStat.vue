<template>
  <article class="overview-stat" :class="`overview-stat--${tone}`">
    <div class="overview-stat-heading"><span class="overview-stat-icon"><Icon :name="icon" size="md" /></span><p>{{ label }}</p></div>
    <p class="overview-stat-value"><slot /><span v-if="$slots.badge" class="overview-stat-badge"><slot name="badge" /></span></p>
    <div class="overview-stat-detail"><slot name="detail" /></div>
  </article>
</template>

<script setup lang="ts">
import Icon from '@/components/icons/Icon.vue'
withDefaults(defineProps<{
  label: string
  icon: InstanceType<typeof Icon>['$props']['name']
  tone?: 'orange' | 'blue' | 'green' | 'violet' | 'purple'
}>(), { tone: 'orange' })
</script>

<style scoped>
.overview-stat { --metric-color: #c96e19; --metric-bg: #f9ead7; min-width: 0; padding: 22px 24px; }
.overview-stat--blue { --metric-color: #3177d4; --metric-bg: #eaf2ff; }
.overview-stat--green { --metric-color: #149255; --metric-bg: #e3f4e8; }
.overview-stat--violet { --metric-color: #6159d9; --metric-bg: #ebe9ff; }
.overview-stat--purple { --metric-color: #9a46c4; --metric-bg: #f2e5f7; }
.dark .overview-stat { --metric-color: #efb16c; --metric-bg: #44372a; }
.dark .overview-stat--blue { --metric-color: #8ab4ff; --metric-bg: #2c384d; }
.dark .overview-stat--green { --metric-color: #69cf98; --metric-bg: #263e32; }
.dark .overview-stat--violet { --metric-color: #b6a5ff; --metric-bg: #363149; }
.dark .overview-stat--purple { --metric-color: #cf9aee; --metric-bg: #403047; }
.overview-stat-heading { display: flex; align-items: center; gap: 10px; color: var(--signal-muted); font-size: 13px; line-height: 20px; }
.overview-stat-icon { display: grid; place-items: center; flex-shrink: 0; width: 34px; height: 34px; border-radius: 50%; background: var(--metric-bg); color: var(--metric-color); }
.overview-stat-value { margin-top: 14px; color: var(--signal-text); font-size: 28px; line-height: 36px; font-weight: 600; letter-spacing: -.6px; font-variant-numeric: tabular-nums; overflow-wrap: anywhere; }
.overview-stat-value :deep(small) { font-size: 12px; font-weight: 400; letter-spacing: 0; color: var(--signal-muted); }
.overview-stat-badge { display: inline-flex; vertical-align: middle; margin-left: 10px; padding: 2px 6px; border-radius: 5px; background: var(--signal-inset); color: var(--signal-muted); font-size: 11px; line-height: 18px; font-weight: 500; letter-spacing: 0; }
.overview-stat-badge:has(.stat-badge-success) { background: rgb(22 163 74 / .1); color: #16a34a; }
.overview-stat-detail { display: flex; flex-wrap: wrap; gap: 3px 9px; margin-top: 8px; min-height: 36px; font-size: 11px; line-height: 18px; color: var(--signal-muted); font-variant-numeric: tabular-nums; overflow-wrap: anywhere; }
.overview-stat-detail :deep(strong) { color: var(--signal-text); font-weight: 500; }
@media (max-width: 767px) { .overview-stat { padding: 18px 16px; } .overview-stat-value { font-size: 24px; line-height: 32px; } }
@media (max-width: 399px) { .overview-stat { padding: 16px 12px; } .overview-stat-heading { gap: 7px; font-size: 12px; } .overview-stat-icon { width: 28px; height: 28px; } .overview-stat-value { font-size: 22px; } }
</style>
