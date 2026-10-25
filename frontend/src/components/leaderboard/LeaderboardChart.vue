<template>
  <div class="ranking-chart">
    <Bar :data="chartData" :options="options" role="img" :aria-label="t(share ? 'leaderboard.shareChartLabel' : 'leaderboard.chartLabel')" />
  </div>
</template>

<script setup lang="ts">
import { computed, ref } from 'vue'
import { useI18n } from 'vue-i18n'
import { useMutationObserver, usePreferredReducedMotion } from '@vueuse/core'
import { Bar } from 'vue-chartjs'
import { Chart as ChartJS, BarElement, CategoryScale, LinearScale, Tooltip, type ChartOptions } from 'chart.js'
import type { LeaderboardResponse } from '@/api/leaderboard'
import { providerLabel } from '@/components/modelPlaza/modelPlazaProviders'
import { bucketLabels, compactTokens, providerOf, PROVIDER_COLORS, SERIES_COLORS, rankModels, trendBuckets } from './leaderboardData'

ChartJS.register(BarElement, CategoryScale, LinearScale, Tooltip)
const props = defineProps<{ data: LeaderboardResponse; share?: boolean }>()
const { t, locale } = useI18n()
const dark = ref(document.documentElement.classList.contains('dark'))
useMutationObserver(document.documentElement, () => { dark.value = document.documentElement.classList.contains('dark') }, { attributes: true, attributeFilter: ['class'] })
const reducedMotion = usePreferredReducedMotion()
const buckets = computed(() => trendBuckets(props.data))
const series = computed(() => {
  if (props.share) return [...new Set(props.data.models.filter(m => m.total_tokens > 0).map(m => providerOf(m.model)))]
    .map(provider => ({ key: provider, label: provider === 'other' ? t('leaderboard.other') : providerLabel(provider), color: PROVIDER_COLORS[provider] || '#94a3b8' }))
  const models = rankModels(props.data.models).slice(0, 8)
  const entries = models.map((m, i) => ({ key: m.model, label: m.model, color: SERIES_COLORS[i] }))
  if (props.data.models.filter(m => m.total_tokens > 0).length > 8) entries.push({ key: '__other__', label: t('leaderboard.otherModels'), color: SERIES_COLORS[8] })
  return entries
})
const chartData = computed(() => {
  const top = new Set(series.value.map(s => s.key))
  const values = buckets.value.map(bucket => {
    const totals = new Map<string, number>()
    for (const [model, tokens] of bucket.models) {
      const key = props.share ? providerOf(model) : top.has(model) ? model : '__other__'
      totals.set(key, (totals.get(key) || 0) + tokens)
    }
    return totals
  })
  const labels = bucketLabels(props.data, buckets.value.map(bucket => bucket.key), locale.value)
  return {
    labels,
    datasets: series.value.map(s => ({
      label: s.label,
      data: values.map(totals => {
        const tokens = totals.get(s.key) || 0
        const total = [...totals.values()].reduce((a, b) => a + b, 0)
        return props.share ? total > 0 ? tokens / total * 100 : 0 : tokens
      }),
      backgroundColor: s.color, borderWidth: 0, borderRadius: 2,
      maxBarThickness: 72, categoryPercentage: .76, barPercentage: .92,
    })),
  }
})
const options = computed<ChartOptions<'bar'>>(() => ({
  responsive: true, maintainAspectRatio: false,
  animation: reducedMotion.value === 'reduce' ? false : { duration: 280 },
  interaction: { intersect: false, mode: 'index' },
  plugins: {
    legend: { display: false },
    tooltip: {
      backgroundColor: dark.value ? '#252b35' : '#1f2937', padding: 12,
      callbacks: { label: context => `${context.dataset.label}: ${props.share ? `${Number(context.raw).toFixed(1)}%` : compactTokens(Number(context.raw))}` },
    },
  },
  scales: {
    x: { stacked: true, grid: { display: false }, border: { display: false }, ticks: { color: dark.value ? '#a7aeb9' : '#6b7280', font: { size: 11 }, maxRotation: 0, maxTicksLimit: 12 } },
    y: { stacked: true, beginAtZero: true, ...(props.share ? { max: 100 } : {}), border: { display: false },
      grid: { color: dark.value ? '#ffffff0e' : '#1f293710' },
      ticks: { color: dark.value ? '#a7aeb9' : '#6b7280', font: { size: 11 }, maxTicksLimit: 5, callback: value => props.share ? `${value}%` : compactTokens(Number(value)) },
    },
  },
}))
</script>

<style scoped>
.ranking-chart { height: 264px; min-width: 0; }
@media (max-width: 639px) { .ranking-chart { height: 210px; } }
</style>
