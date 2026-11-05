<template>
  <DashboardChartCard class="signal-charts" data-signal-charts :title="t('dashboard.tokenUsageTrend')" :aria-busy="loading">
    <template #value>
      <span class="signal-chart-total" :title="!loading && !error ? selectedTokens.toLocaleString() : undefined">{{ totalLabel }}</span>
    </template>
    <template #actions>
      <div class="signal-chart-mode" role="tablist" :aria-label="t('dashboard.tokenUsageTrend')">
        <button role="tab" :aria-selected="mode === 'total'" @click="mode = 'total'">{{ t('dashboard.totalUsage') }}</button>
        <button role="tab" :aria-selected="mode === 'breakdown'" @click="mode = 'breakdown'">{{ t('dashboard.tokenDetails') }}</button>
      </div>
    </template>
    <div v-if="error" class="signal-chart-error" role="alert">
      <span>{{ t('dashboard.chartsFailed') }}</span>
      <button class="btn btn-secondary" @click="$emit('refresh')"><Icon name="refresh" size="sm" />{{ t('common.refresh') }}</button>
    </div>
    <div v-else class="signal-trend">
      <TokenUsageTrend :trend-data="trend" :loading="loading" :mode="mode" :animation-duration="reducedMotion ? 0 : 160" />
    </div>
  </DashboardChartCard>
</template>

<script setup lang="ts">
import { computed, ref } from 'vue'
import { usePreferredReducedMotion } from '@vueuse/core'
import { useI18n } from 'vue-i18n'
import Icon from '@/components/icons/Icon.vue'
import TokenUsageTrend from '@/components/charts/TokenUsageTrend.vue'
import DashboardChartCard from '@/components/dashboard/DashboardChartCard.vue'
import type { TrendDataPoint } from '@/types'

const props = defineProps<{ loading: boolean; error?: boolean; trend: TrendDataPoint[] }>()
defineEmits(['refresh'])
const { t } = useI18n()
const preferredMotion = usePreferredReducedMotion()
const reducedMotion = computed(() => preferredMotion.value === 'reduce')
const mode = ref<'total' | 'breakdown'>('total')
const selectedTokens = computed(() => props.trend.reduce((sum, point) => sum + point.total_tokens, 0))
const formatTokens = (value: number) => {
  if (value >= 1_000_000_000) return `${(value / 1_000_000_000).toFixed(2)}B`
  if (value >= 1_000_000) return `${(value / 1_000_000).toFixed(1)}M`
  if (value >= 1000) return `${(value / 1000).toFixed(1)}K`
  return value.toString()
}
const totalLabel = computed(() => props.loading || props.error ? '-' : formatTokens(selectedTokens.value))
</script>

<style scoped>
.signal-chart-mode { display: flex; padding: 3px; border: 1px solid var(--signal-line); border-radius: 6px; background: var(--signal-inset); }
.signal-chart-mode button { min-height: 30px; padding: 4px 12px; border-radius: 4px; font-size: 12px; color: var(--signal-muted); }
.signal-chart-mode [aria-selected='true'] { background: var(--signal-raised); color: var(--signal-text); box-shadow: 0 1px 3px #00000010; }
.signal-trend { min-width: 0; max-width: 100%; overflow: hidden; padding: 16px 20px 20px; }
.signal-trend :deep(canvas) { max-width: 100%; }
.signal-trend :deep(.card) { background: transparent; border: 0; border-radius: 0; padding: 0; box-shadow: none; }
.signal-trend :deep(.card > h3) { display: none; }
.signal-trend :deep(.h-48) { height: 260px; }
.signal-chart-error { min-height: 290px; display: flex; align-items: center; justify-content: center; flex-wrap: wrap; gap: 12px; color: var(--signal-muted); }
.signal-chart-error button { gap: 8px; }
@media (max-width: 639px) {
  .signal-chart-mode button { min-height: 38px; }
  .signal-trend :deep(.h-48) { height: 220px; }
}
</style>
