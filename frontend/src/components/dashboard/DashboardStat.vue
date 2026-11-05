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
  tone?: 'orange' | 'blue' | 'green' | 'violet'
}>(), { tone: 'orange' })
</script>

<style scoped>
.overview-stat { --metric-color: var(--signal-accent, #ea580c); min-width: 0; padding: 22px 24px; }
.overview-stat--blue { --metric-color: #3b82f6; }
.overview-stat--green { --metric-color: #16a085; }
.overview-stat--violet { --metric-color: #8b5cf6; }
.overview-stat-heading { display: flex; align-items: center; gap: 10px; color: var(--signal-muted); font-size: 13px; line-height: 20px; }
.overview-stat-icon { display: grid; place-items: center; flex-shrink: 0; width: 34px; height: 34px; border-radius: 50%; background: color-mix(in srgb, var(--metric-color) 10%, transparent); color: var(--metric-color); }
.overview-stat-value { margin-top: 14px; color: var(--signal-text); font-size: 28px; line-height: 36px; font-weight: 600; letter-spacing: -.6px; font-variant-numeric: tabular-nums; overflow-wrap: anywhere; }
.overview-stat-value :deep(small) { font-size: 12px; font-weight: 400; letter-spacing: 0; color: var(--signal-muted); }
.overview-stat-badge { display: inline-flex; vertical-align: middle; margin-left: 10px; padding: 2px 6px; border-radius: 5px; background: var(--signal-inset); color: var(--signal-muted); font-size: 11px; line-height: 18px; font-weight: 500; letter-spacing: 0; }
.overview-stat-badge:has(.stat-badge-success) { background: rgb(22 163 74 / .1); color: #16a34a; }
.overview-stat-detail { display: flex; flex-wrap: wrap; gap: 3px 9px; margin-top: 8px; min-height: 36px; font-size: 11px; line-height: 18px; color: var(--signal-muted); font-variant-numeric: tabular-nums; overflow-wrap: anywhere; }
.overview-stat-detail :deep(strong) { color: var(--signal-text); font-weight: 500; }
@media (max-width: 767px) { .overview-stat { padding: 18px 16px; } .overview-stat-value { font-size: 24px; line-height: 32px; } }
@media (max-width: 399px) { .overview-stat { padding: 16px 12px; } .overview-stat-heading { gap: 7px; font-size: 12px; } .overview-stat-icon { width: 28px; height: 28px; } .overview-stat-value { font-size: 22px; } }
</style>
