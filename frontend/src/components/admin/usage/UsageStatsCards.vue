<template>
  <div class="usage-metrics">
    <div class="usage-metric" data-testid="usage-metric">
      <div class="usage-stat-icon rounded-lg bg-blue-100 p-2 dark:bg-blue-900/30 text-blue-600">
        <Icon name="document" size="md" />
      </div>
      <div>
        <p class="usage-metric-label">{{ t('usage.totalRequests') }}</p>
        <p class="usage-metric-value">{{ stats?.total_requests?.toLocaleString() || '0' }}</p>
        <p class="usage-metric-note">{{ t('usage.inSelectedRange') }}</p>
      </div>
    </div>
    <div class="usage-metric" data-testid="usage-metric">
      <div class="usage-stat-icon rounded-lg bg-amber-100 p-2 dark:bg-amber-900/30 text-amber-600"><svg class="h-5 w-5" fill="none" stroke="currentColor" viewBox="0 0 24 24"><path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="m21 7.5-9-5.25L3 7.5m18 0-9 5.25m9-5.25v9l-9 5.25M3 7.5l9 5.25M3 7.5v9l9 5.25m0-9v9" /></svg></div>
      <div>
        <p class="usage-metric-label">{{ t('usage.totalTokens') }}</p>
        <p class="usage-metric-value">{{ formatTokens(stats?.total_tokens || 0) }}</p>
        <p class="flex flex-wrap items-center gap-x-1 text-xs text-gray-500">
          <span>{{ t('usage.in') }}: {{ formatTokens(stats?.total_input_tokens || 0) }}</span>
          <span>/</span>
          <span>{{ t('usage.out') }}: {{ formatTokens(stats?.total_output_tokens || 0) }}</span>
          <span>/</span>
          <span class="group relative inline-flex cursor-help items-center gap-0.5" tabindex="0">
            <span>{{ cacheLabel() }}: {{ formatTokens(stats?.total_cache_tokens || 0) }}</span>
            <svg
              class="h-3.5 w-3.5 text-gray-400"
              fill="none"
              stroke="currentColor"
              viewBox="0 0 24 24"
            >
              <path
                stroke-linecap="round"
                stroke-linejoin="round"
                stroke-width="2"
                d="M13 16h-1v-4h-1m1-4h.01M21 12a9 9 0 11-18 0 9 9 0 0118 0z"
              />
            </svg>
            <span
              class="pointer-events-none absolute left-1/2 top-full z-30 mt-2 hidden w-56 -translate-x-1/2 rounded-lg border border-gray-200 bg-white p-3 text-left text-xs text-gray-700 shadow-lg group-hover:block group-focus:block dark:border-dark-600 dark:bg-dark-800 dark:text-dark-200"
            >
              <span class="mb-2 block font-medium text-gray-900 dark:text-white">
                {{ cacheDetailLabel() }}
              </span>
              <span class="flex items-center justify-between gap-3">
                <span>{{ t('usage.cacheCreationTokensLabel') }}</span>
                <span class="tabular-nums">
                  {{ formatTokens(stats?.total_cache_creation_tokens || 0) }}
                </span>
              </span>
              <span class="mt-1 flex items-center justify-between gap-3">
                <span>{{ t('usage.cacheReadTokensLabel') }}</span>
                <span class="tabular-nums">
                  {{ formatTokens(stats?.total_cache_read_tokens || 0) }}
                </span>
              </span>
            </span>
          </span>
        </p>
      </div>
    </div>
    <div class="usage-metric" data-testid="usage-metric">
      <div class="usage-stat-icon rounded-lg bg-green-100 p-2 dark:bg-green-900/30 text-green-600">
        <Icon name="dollar" size="md" />
      </div>
      <div class="min-w-0 flex-1">
        <p class="usage-metric-label">{{ t('usage.totalCost') }}</p>
        <p class="usage-metric-value usage-metric-cost">
          ${{ (stats?.total_actual_cost || 0).toFixed(4) }}
        </p>
        <p class="usage-metric-note">
          <template v-if="showAccountCost && totalAccountCost != null">
            <span class="text-orange-500">{{ t('usage.accountCost') }} ${{ totalAccountCost.toFixed(4) }}</span>
            <span> · </span>
          </template>
          <span>
            {{ t('usage.standardCost') }}
            <span :class="{ 'line-through': strikeStandardCost }">${{ (stats?.total_cost || 0).toFixed(4) }}</span>
          </span>
        </p>
      </div>
    </div>
    <div class="usage-metric" data-testid="usage-metric">
      <div class="usage-stat-icon rounded-lg bg-purple-100 p-2 dark:bg-purple-900/30 text-purple-600">
        <Icon name="clock" size="md" />
      </div>
      <div><p class="usage-metric-label">{{ t('usage.avgDuration') }}</p><p class="usage-metric-value">{{ formatDuration(stats?.average_duration_ms || 0) }}</p></div>
    </div>
  </div>
</template>

<script setup lang="ts">
import { computed } from 'vue'
import { useI18n } from 'vue-i18n'
import type { AdminUsageStatsResponse } from '@/api/admin/usage'
import type { UsageStatsResponse } from '@/types'
import Icon from '@/components/icons/Icon.vue'

const props = withDefaults(defineProps<{
  stats: (AdminUsageStatsResponse | UsageStatsResponse) | null
  showAccountCost?: boolean
  strikeStandardCost?: boolean
}>(), {
  showAccountCost: true,
  strikeStandardCost: false,
})

const { t } = useI18n()

const totalAccountCost = computed(() => {
  const stats = props.stats as (AdminUsageStatsResponse & { total_account_cost?: number }) | null
  return stats?.total_account_cost ?? null
})
const showAccountCost = computed(() => props.showAccountCost)
const strikeStandardCost = computed(() => props.strikeStandardCost)

const formatDuration = (ms: number) =>
  ms < 1000 ? `${ms.toFixed(0)}ms` : `${(ms / 1000).toFixed(2)}s`

const formatTokens = (value: number) => {
  if (value >= 1e9) return (value / 1e9).toFixed(2) + 'B'
  if (value >= 1e6) return (value / 1e6).toFixed(2) + 'M'
  if (value >= 1e3) return (value / 1e3).toFixed(2) + 'K'
  return value.toLocaleString()
}

const cacheLabel = () => t('usage.cacheTotal')
const cacheDetailLabel = () => t('usage.cacheBreakdown')
</script>

<style scoped>
.usage-metrics { display: grid; grid-template-columns: repeat(4, minmax(0, 1fr)); min-width: 0; border: 1px solid var(--signal-line); border-radius: 12px; background: var(--signal-surface); }
.usage-metric { display: flex; align-items: flex-start; gap: 12px; min-width: 0; padding: 22px 20px; border-right: 1px solid var(--signal-line); }
.usage-metric:last-child { border-right: 0; }
.usage-metric > div:last-child { min-width: 0; overflow-wrap: anywhere; }
.usage-stat-icon { flex-shrink: 0; margin-top: 2px; padding: 3px; border-radius: 50%; background: var(--signal-inset); box-shadow: inset 0 0 0 1px var(--signal-line); color: var(--signal-muted); }
.usage-metric-label { color: var(--signal-muted); font-size: 12px; line-height: 20px; font-weight: 500; }
.usage-metric-value { margin: 3px 0; color: var(--signal-text); font-size: 28px; line-height: 34px; font-weight: 600; font-variant-numeric: tabular-nums; letter-spacing: -.5px; }
.usage-metric-cost { color: var(--signal-amber); }
.usage-metric-note { color: var(--signal-muted); font-size: 12px; line-height: 20px; }
@media (max-width: 1023px) {
  .usage-metrics { grid-template-columns: repeat(2, minmax(0, 1fr)); }
  .usage-metric:nth-child(2) { border-right: 0; }
  .usage-metric:nth-child(-n+2) { border-bottom: 1px solid var(--signal-line); }
}
@media (max-width: 639px) {
  .usage-metric { gap: 8px; padding: 16px 12px; }
  .usage-metric-value { font-size: 23px; line-height: 30px; }
  .usage-stat-icon :deep(svg) { width: 16px; height: 16px; }
}
</style>
