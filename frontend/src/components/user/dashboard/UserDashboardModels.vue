<template>
  <DashboardChartCard class="signal-model-card" :title="t('dashboard.modelDistribution')" :aria-label="t('dashboard.modelDistribution')" :aria-busy="loading">
    <template #actions><span class="signal-model-period">{{ startDate }} / {{ endDate }}</span></template>
    <div class="signal-model-body">
      <div v-if="loading" class="signal-model-state"><LoadingSpinner size="sm" /></div>
      <p v-else-if="error" class="signal-model-state">{{ t('dashboard.chartsFailed') }}</p>
      <div v-else-if="modelData" class="signal-model-summary">
        <div class="signal-model-ring"><Doughnut :data="modelData" :options="options" /></div>
        <div class="signal-model-table">
          <table class="signal-model-legend">
            <thead><tr><th>{{ t('dashboard.model') }}</th><th>{{ t('dashboard.tokens') }}</th><th>{{ t('dashboard.share') }}</th></tr></thead>
            <tbody>
              <tr v-for="(model, index) in rankedModels" :key="model.model">
                <td><span class="signal-model-name"><span class="signal-model-dot" :style="{ background: colors[index % colors.length] }" /><span class="signal-model-label" :title="model.model">{{ model.model }}</span></span></td>
                <td :title="model.total_tokens.toLocaleString()">{{ formatTokensK(model.total_tokens) }}</td>
                <td>{{ (model.total_tokens / totalTokens * 100).toFixed(1) }}%</td>
              </tr>
            </tbody>
          </table>
        </div>
      </div>
      <p v-else class="signal-model-state">{{ t('dashboard.noDataAvailable') }}</p>
    </div>
  </DashboardChartCard>
</template>

<script setup lang="ts">
import { computed } from 'vue'
import { usePreferredReducedMotion } from '@vueuse/core'
import { useI18n } from 'vue-i18n'
import { Chart as ChartJS, ArcElement, Tooltip, type ChartOptions } from 'chart.js'
import { Doughnut } from 'vue-chartjs'
import LoadingSpinner from '@/components/common/LoadingSpinner.vue'
import DashboardChartCard from '@/components/dashboard/DashboardChartCard.vue'
import type { ModelStat } from '@/types'
import { formatTokensK } from '@/utils/format'

ChartJS.register(ArcElement, Tooltip)
const props = defineProps<{ models: ModelStat[]; startDate: string; endDate: string; loading: boolean; error?: boolean }>()
const { t } = useI18n()
const motion = usePreferredReducedMotion()
const colors = ['#6486a5', '#659b87', '#9685af', '#b29968', '#ae8394', '#8796a5']
const rankedModels = computed(() => props.models.filter(model => Number.isFinite(model.total_tokens) && model.total_tokens > 0).slice().sort((a, b) => b.total_tokens - a.total_tokens))
const totalTokens = computed(() => rankedModels.value.reduce((sum, model) => sum + model.total_tokens, 0))
const modelData = computed(() => totalTokens.value > 0 ? {
  labels: rankedModels.value.map(model => model.model),
  datasets: [{ data: rankedModels.value.map(model => model.total_tokens), backgroundColor: rankedModels.value.map((_, index) => colors[index % colors.length]), borderWidth: 0 }],
} : null)
const options = computed<ChartOptions<'doughnut'>>(() => ({
  responsive: true,
  maintainAspectRatio: false,
  cutout: '74%',
  animation: { duration: motion.value === 'reduce' ? 0 : 160 },
  plugins: { legend: { display: false }, tooltip: { callbacks: {
    label: context => `${context.label}: ${formatTokensK(context.parsed)} tokens`,
  } } },
}))
</script>

<style scoped>
.signal-model-body { padding: 20px; }
.signal-model-period { font-size: 12px; color: var(--signal-muted); font-variant-numeric: tabular-nums; overflow-wrap: anywhere; }
.signal-model-summary { display: flex; flex-direction: column; align-items: stretch; gap: 24px; }
.signal-model-ring { width: 160px; height: 160px; flex-shrink: 0; align-self: center; }
.signal-model-table { min-width: 0; max-height: 288px; overflow-y: auto; flex: 1; }
.signal-model-legend { width: 100%; table-layout: fixed; border-collapse: separate; border-spacing: 0; font-size: 12px; color: var(--signal-muted); font-variant-numeric: tabular-nums; }
.signal-model-legend th, .signal-model-legend td { padding: 9px 5px; border-bottom: 1px solid var(--signal-line); text-align: right; white-space: nowrap; }
.signal-model-legend th { background: var(--signal-inset); font-weight: 500; }
.signal-model-legend :is(th, td):first-child { width: 50%; padding-left: 8px; text-align: left; }
.signal-model-legend th:first-child { border-radius: 6px 0 0 6px; }
.signal-model-legend th:last-child { border-radius: 0 6px 6px 0; }
.signal-model-legend tbody tr:last-child td { border-bottom: 0; }
.signal-model-name { display: flex; align-items: center; gap: 6px; min-width: 0; }
.signal-model-dot { width: 7px; height: 7px; border-radius: 50%; flex: 0 0 auto; }
.signal-model-label { flex: 1; min-width: 0; overflow: hidden; text-overflow: ellipsis; white-space: nowrap; color: var(--signal-text); }
.signal-model-state { min-height: 120px; display: grid; place-items: center; color: var(--signal-muted); font-size: 12px; }
@media (min-width: 640px) { .signal-model-summary { flex-direction: row; align-items: flex-start; } .signal-model-ring { align-self: flex-start; } }
</style>
