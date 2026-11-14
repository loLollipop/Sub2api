<template>
  <AppLayout>
    <div class="signal-admin-dashboard space-y-6">
      <PageHeader :title="t('admin.dashboard.title')" />
      <AnalyticsToolbar refresh :loading="loading || chartsLoading" @refresh="loadDashboardStats">
        <template #date><DateRangePicker v-model:start-date="startDate" v-model:end-date="endDate" @change="onDateRangeChange" /></template>
        <template #granularity><Select v-model="granularity" :aria-label="t('admin.dashboard.granularity')" :options="granularityOptions" @change="loadChartData" /></template>
      </AnalyticsToolbar>
      <!-- Loading State -->
      <div v-if="loading" class="flex items-center justify-center py-12">
        <LoadingSpinner />
      </div>

      <div v-else-if="dashboardError" class="flex flex-col items-center justify-center gap-4 py-12 text-sm text-gray-500 dark:text-gray-400">
        <p>{{ t('admin.dashboard.failedToLoad') }}</p>
        <button type="button" class="btn btn-secondary" @click="loadDashboardStats">
          {{ t('common.refresh') }}
        </button>
      </div>

      <template v-else-if="stats">
        <DashboardStatGrid>
          <DashboardStat icon="key" tone="green" :label="t('admin.dashboard.apiKeys')">{{ stats.total_api_keys }}<template #detail><span>{{ stats.active_api_keys }} {{ t('common.active') }}</span></template></DashboardStat>
          <DashboardStat icon="server" tone="violet" :label="t('admin.dashboard.accounts')">{{ stats.total_accounts }}<template #detail><span>{{ stats.normal_accounts }} {{ t('common.active') }}</span><span v-if="stats.error_accounts > 0" class="text-red-500">{{ stats.error_accounts }} {{ t('common.error') }}</span></template></DashboardStat>
          <DashboardStat icon="chart" tone="blue" :label="t('admin.dashboard.todayRequests')">{{ stats.today_requests }}<template #detail><span>{{ t('common.total') }}: <strong>{{ formatNumber(stats.total_requests) }}</strong></span></template></DashboardStat>
          <DashboardStat icon="userPlus" tone="green" :label="t('admin.dashboard.users')">{{ formatNumber(stats.total_users) }}<template #badge><span :class="{ 'stat-badge-success': stats.today_new_users > 0 }" :title="t('admin.dashboard.newUsersToday')">+{{ stats.today_new_users }}</span></template></DashboardStat>
          <DashboardStat icon="cube" :label="t('admin.dashboard.todayTokens')">{{ formatTokens(stats.today_tokens) }}<template #detail><span>{{ t('admin.dashboard.actual') }} <strong>${{ formatCost(stats.today_actual_cost) }}</strong></span><span>{{ t('admin.dashboard.accountCost') }} <strong>${{ formatCost(stats.today_account_cost) }}</strong></span><span>{{ t('admin.dashboard.standard') }} ${{ formatCost(stats.today_cost) }}</span></template></DashboardStat>
          <DashboardStat icon="database" tone="violet" :label="t('admin.dashboard.totalTokens')">{{ formatTokens(stats.total_tokens) }}<template #detail><span>{{ t('admin.dashboard.actual') }} <strong>${{ formatCost(stats.total_actual_cost) }}</strong></span><span>{{ t('admin.dashboard.accountCost') }} <strong>${{ formatCost(stats.total_account_cost) }}</strong></span><span>{{ t('admin.dashboard.standard') }} ${{ formatCost(stats.total_cost) }}</span></template></DashboardStat>
          <DashboardStat icon="bolt" tone="blue" :label="t('admin.dashboard.performance')">{{ formatTokens(stats.rpm) }} <small>RPM</small><template #detail><span><strong>{{ formatTokens(stats.tpm) }}</strong> TPM</span></template></DashboardStat>
          <DashboardStat icon="banknotes" tone="purple" :label="t('admin.dashboard.usersWithBalance')" data-testid="users-with-balance">${{ formatCost(stats.users_with_balance_total) }}<template #detail><span>{{ formatNumber(stats.users_with_balance) }} {{ t('admin.dashboard.usersWithBalanceUnit') }}</span></template></DashboardStat>
        </DashboardStatGrid>

        <nav class="admin-dashboard-shortcuts" :aria-label="t('admin.dashboard.quickActions')">
          <RouterLink v-if="canUseBatchImage" to="/batch-image"><Icon name="sparkles" size="sm" /><span>{{ t('admin.dashboard.batchImage') }}</span></RouterLink>
          <RouterLink to="/admin/groups"><Icon name="grid" size="sm" /><span>{{ t('admin.dashboard.groupPricing') }}</span></RouterLink>
        </nav>

        <!-- Charts Section -->
        <div class="admin-dashboard-charts space-y-6">
          <DashboardChartCard class="admin-trend-card" :title="t('admin.dashboard.tokenUsageTrend')" :value="chartsLoading || chartsError ? undefined : formatTokens(trendTotalTokens)">
            <p v-if="chartsError" role="alert" class="admin-chart-state">{{ t('admin.dashboard.failedToLoad') }}</p>
            <div v-else class="signal-trend admin-trend-body"><TokenUsageTrend :trend-data="trendData" :loading="chartsLoading" /></div>
          </DashboardChartCard>
          <ModelDistributionChart
            :model-stats="modelStats"
            :enable-ranking-view="true"
            :ranking-items="rankingItems"
            :ranking-total-actual-cost="rankingTotalActualCost"
            :ranking-total-requests="rankingTotalRequests"
            :ranking-total-tokens="rankingTotalTokens"
            :loading="chartsLoading"
            :ranking-loading="rankingLoading"
            :ranking-error="rankingError"
            :start-date="startDate"
            :end-date="endDate"
            @ranking-click="goToUserUsage"
          />

          <!-- User Usage Trend (Full Width) -->
          <DashboardChartCard :title="`${t('admin.dashboard.recentUsage')} (Top 12)`" :value="userTrendLoading || userTrendError ? undefined : formatTokens(userTrendTotalTokens)">
            <div class="h-64 p-4">
              <div v-if="userTrendLoading" class="flex h-full items-center justify-center">
                <LoadingSpinner size="md" />
              </div>
              <p v-else-if="userTrendError" role="alert" class="admin-chart-state">{{ t('admin.dashboard.failedToLoad') }}</p>
              <Line v-else-if="userTrendChartData" :data="userTrendChartData" :options="lineOptions" />
              <div
                v-else
                class="flex h-full items-center justify-center text-sm text-gray-500 dark:text-gray-400"
              >
                {{ t('admin.dashboard.noDataAvailable') }}
              </div>
            </div>
          </DashboardChartCard>
        </div>
      </template>
    </div>
  </AppLayout>
</template>

<script setup lang="ts">
import { ref, computed, onMounted } from 'vue'
import { useI18n } from 'vue-i18n'
import { RouterLink, useRouter } from 'vue-router'
import { useAppStore } from '@/stores/app'

const { t } = useI18n()
import { adminAPI } from '@/api/admin'
import type {
  DashboardStats,
  TrendDataPoint,
  ModelStat,
  UserUsageTrendPoint,
  UserSpendingRankingItem
} from '@/types'
import AppLayout from '@/components/layout/AppLayout.vue'
import PageHeader from '@/components/layout/PageHeader.vue'
import AnalyticsToolbar from '@/components/dashboard/AnalyticsToolbar.vue'
import LoadingSpinner from '@/components/common/LoadingSpinner.vue'
import Icon from '@/components/icons/Icon.vue'
import DashboardStat from '@/components/dashboard/DashboardStat.vue'
import DashboardStatGrid from '@/components/dashboard/DashboardStatGrid.vue'
import DashboardChartCard from '@/components/dashboard/DashboardChartCard.vue'
import DateRangePicker from '@/components/common/DateRangePicker.vue'
import Select from '@/components/common/Select.vue'
import ModelDistributionChart from '@/components/charts/ModelDistributionChart.vue'
import TokenUsageTrend from '@/components/charts/TokenUsageTrend.vue'
import { useChartPalette } from '@/composables/useChartPalette'
import { useBatchImageAccess } from '@/composables/useBatchImageAccess'

import {
  Chart as ChartJS,
  CategoryScale,
  LinearScale,
  PointElement,
  LineElement,
  Tooltip,
  Legend,
  Filler
} from 'chart.js'
import { Line } from 'vue-chartjs'

// Register Chart.js components
ChartJS.register(
  CategoryScale,
  LinearScale,
  PointElement,
  LineElement,
  Tooltip,
  Legend,
  Filler
)

const appStore = useAppStore()
const router = useRouter()
const { canUseBatchImage, refreshBatchImageAccess } = useBatchImageAccess()
const stats = ref<DashboardStats | null>(null)
const loading = ref(false)
const dashboardError = ref(false)
const chartsLoading = ref(false)
const chartsError = ref(false)
const userTrendLoading = ref(false)
const userTrendError = ref(false)
const rankingLoading = ref(false)
const rankingError = ref(false)

// Chart data
const trendData = ref<TrendDataPoint[]>([])
const modelStats = ref<ModelStat[]>([])
const userTrend = ref<UserUsageTrendPoint[]>([])
const rankingItems = ref<UserSpendingRankingItem[]>([])
const rankingTotalActualCost = ref(0)
const rankingTotalRequests = ref(0)
const rankingTotalTokens = ref(0)
let chartLoadSeq = 0
let usersTrendLoadSeq = 0
let rankingLoadSeq = 0
const rankingLimit = 12
const trendTotalTokens = computed(() => trendData.value.reduce((sum, point) => sum + point.total_tokens, 0))
const userTrendTotalTokens = computed(() => userTrend.value.reduce((sum, point) => sum + point.tokens, 0))

// Helper function to format date in local timezone
const formatLocalDate = (date: Date): string => {
  return `${date.getFullYear()}-${String(date.getMonth() + 1).padStart(2, '0')}-${String(date.getDate()).padStart(2, '0')}`
}

const getLast24HoursRangeDates = (): { start: string; end: string } => {
  const end = new Date()
  const start = new Date(end.getTime() - 24 * 60 * 60 * 1000)
  return {
    start: formatLocalDate(start),
    end: formatLocalDate(end)
  }
}

// Date range
const granularity = ref<'day' | 'hour'>('hour')
const defaultRange = getLast24HoursRangeDates()
const startDate = ref(defaultRange.start)
const endDate = ref(defaultRange.end)

// Granularity options for Select component
const granularityOptions = computed(() => [
  { value: 'day', label: t('admin.dashboard.day') },
  { value: 'hour', label: t('admin.dashboard.hour') }
])

const chartColors = useChartPalette()

// Line chart options (for user trend chart)
const lineOptions = computed(() => ({
  responsive: true,
  maintainAspectRatio: false,
  interaction: {
    intersect: false,
    mode: 'index' as const
  },
  plugins: {
    legend: {
      position: 'top' as const,
      labels: {
        color: chartColors.value.text,
        usePointStyle: true,
        pointStyle: 'circle',
        padding: 15,
        font: {
          size: 11
        }
      }
    },
    tooltip: {
      itemSort: (a: any, b: any) => {
        const aValue = typeof a?.raw === 'number' ? a.raw : Number(a?.parsed?.y ?? 0)
        const bValue = typeof b?.raw === 'number' ? b.raw : Number(b?.parsed?.y ?? 0)
        return bValue - aValue
      },
      callbacks: {
        label: (context: any) => {
          return `${context.dataset.label}: ${formatTokens(context.raw)}`
        }
      }
    }
  },
  scales: {
    x: {
      grid: {
        color: chartColors.value.grid
      },
      ticks: {
        color: chartColors.value.text,
        font: {
          size: 10
        }
      }
    },
    y: {
      grid: {
        color: chartColors.value.grid
      },
      ticks: {
        color: chartColors.value.text,
        font: {
          size: 10
        },
        callback: (value: string | number) => formatTokens(Number(value))
      }
    }
  }
}))

// User trend chart data
const userTrendChartData = computed(() => {
  if (!userTrend.value?.length) return null

  const getDisplayName = (point: UserUsageTrendPoint): string => {
    const username = point.username?.trim()
    if (username) {
      return username
    }

    const email = point.email?.trim()
    if (email) {
      return email
    }

    return t('admin.redeem.userPrefix', { id: point.user_id })
  }

  // Group by user_id to avoid merging different users with the same display name
  const userGroups = new Map<number, { name: string; data: Map<string, number> }>()
  const allDates = new Set<string>()

  userTrend.value.forEach((point) => {
    allDates.add(point.date)
    const key = point.user_id
    if (!userGroups.has(key)) {
      userGroups.set(key, { name: getDisplayName(point), data: new Map() })
    }
    userGroups.get(key)!.data.set(point.date, point.tokens)
  })

  const sortedDates = Array.from(allDates).sort()
  const colors = chartColors.value.series

  const datasets = Array.from(userGroups.values()).map((group, idx) => ({
    label: group.name,
    data: sortedDates.map((date) => group.data.get(date) || 0),
    borderColor: colors[idx % colors.length],
    backgroundColor: `${colors[idx % colors.length]}20`,
    fill: false,
    tension: 0.3
  }))

  return {
    labels: sortedDates,
    datasets
  }
})

// Format helpers
const formatTokens = (value: number | undefined): string => {
  if (value === undefined || value === null) return '0'
  if (value >= 1_000_000_000) {
    return `${(value / 1_000_000_000).toFixed(2)}B`
  } else if (value >= 1_000_000) {
    return `${(value / 1_000_000).toFixed(2)}M`
  } else if (value >= 1_000) {
    return `${(value / 1_000).toFixed(2)}K`
  }
  return value.toLocaleString()
}

const toFiniteNumber = (value: unknown): number => {
  const numberValue = Number(value)
  return Number.isFinite(numberValue) ? numberValue : 0
}

const formatNumber = (value: number | null | undefined): string => {
  return toFiniteNumber(value).toLocaleString()
}

const formatCost = (value: number | null | undefined): string => {
  const safeValue = toFiniteNumber(value)
  if (safeValue >= 1000) {
    return (safeValue / 1000).toFixed(2) + 'K'
  } else if (safeValue >= 1) {
    return safeValue.toFixed(2)
  } else if (safeValue >= 0.01) {
    return safeValue.toFixed(3)
  }
  return safeValue.toFixed(4)
}

const goToUserUsage = (item: UserSpendingRankingItem) => {
  void router.push({
    path: '/admin/usage',
    query: {
      user_id: String(item.user_id),
      start_date: startDate.value,
      end_date: endDate.value
    }
  })
}

// Date range change handler
const onDateRangeChange = (range: {
  startDate: string
  endDate: string
  preset: string | null
}) => {
  // Auto-select granularity based on date range
  const start = new Date(range.startDate)
  const end = new Date(range.endDate)
  const daysDiff = Math.ceil((end.getTime() - start.getTime()) / (1000 * 60 * 60 * 24))

  // If range is 1 day, use hourly granularity
  if (daysDiff <= 1) {
    granularity.value = 'hour'
  } else {
    granularity.value = 'day'
  }

  loadChartData()
}

// Load data
const loadDashboardSnapshot = async (includeStats: boolean) => {
  const currentSeq = ++chartLoadSeq
  if (includeStats && !stats.value) {
    loading.value = true
  }
  chartsLoading.value = true
  chartsError.value = false
  if (includeStats) dashboardError.value = false
  try {
    const response = await adminAPI.dashboard.getSnapshotV2({
      start_date: startDate.value,
      end_date: endDate.value,
      granularity: granularity.value,
      include_stats: includeStats,
      include_trend: true,
      include_model_stats: true,
      include_group_stats: false,
      include_users_trend: false
    })
    if (currentSeq !== chartLoadSeq) return
    if (includeStats && response.stats) {
      stats.value = response.stats
    }
    trendData.value = response.trend || []
    modelStats.value = response.models || []
  } catch (error) {
    if (currentSeq !== chartLoadSeq) return
    appStore.showError(t('admin.dashboard.failedToLoad'))
    chartsError.value = true
    if (includeStats) dashboardError.value = true
    console.error('Error loading dashboard snapshot:', error)
  } finally {
    if (currentSeq === chartLoadSeq) {
      loading.value = false
      chartsLoading.value = false
    }
  }
}

const loadUsersTrend = async () => {
  const currentSeq = ++usersTrendLoadSeq
  userTrendLoading.value = true
  userTrendError.value = false
  try {
    const response = await adminAPI.dashboard.getUserUsageTrend({
      start_date: startDate.value,
      end_date: endDate.value,
      granularity: granularity.value,
      limit: 12
    })
    if (currentSeq !== usersTrendLoadSeq) return
    userTrend.value = response.trend || []
  } catch (error) {
    if (currentSeq !== usersTrendLoadSeq) return
    console.error('Error loading users trend:', error)
    userTrendError.value = true
    userTrend.value = []
  } finally {
    if (currentSeq === usersTrendLoadSeq) {
      userTrendLoading.value = false
    }
  }
}

const loadUserSpendingRanking = async () => {
  const currentSeq = ++rankingLoadSeq
  rankingLoading.value = true
  rankingError.value = false
  try {
    const response = await adminAPI.dashboard.getUserSpendingRanking({
      start_date: startDate.value,
      end_date: endDate.value,
      limit: rankingLimit
    })
    if (currentSeq !== rankingLoadSeq) return
    rankingItems.value = response.ranking || []
    rankingTotalActualCost.value = response.total_actual_cost || 0
    rankingTotalRequests.value = response.total_requests || 0
    rankingTotalTokens.value = response.total_tokens || 0
  } catch (error) {
    if (currentSeq !== rankingLoadSeq) return
    console.error('Error loading user spending ranking:', error)
    rankingItems.value = []
    rankingTotalActualCost.value = 0
    rankingTotalRequests.value = 0
    rankingTotalTokens.value = 0
    rankingError.value = true
  } finally {
    if (currentSeq === rankingLoadSeq) {
      rankingLoading.value = false
    }
  }
}

const loadDashboardStats = async () => {
  await Promise.all([
    loadDashboardSnapshot(true),
    loadUsersTrend(),
    loadUserSpendingRanking()
  ])
}

const loadChartData = async () => {
  await Promise.all([
    loadDashboardSnapshot(false),
    loadUsersTrend(),
    loadUserSpendingRanking()
  ])
}

onMounted(() => {
  void refreshBatchImageAccess()
  loadDashboardStats()
})
</script>

<style scoped>
.admin-dashboard-shortcuts { display: grid; grid-template-columns: repeat(2, minmax(0, 1fr)); gap: 12px; min-width: 0; }
.admin-dashboard-shortcuts > a { display: flex; align-items: center; gap: 12px; min-width: 0; min-height: 46px; padding: 12px; border: 1px solid var(--signal-line); border-radius: 10px; background: var(--signal-surface); color: var(--signal-text); }
.admin-dashboard-shortcuts > a:hover { background: var(--signal-hover); }
.admin-dashboard-shortcuts > a:focus-visible { outline: 2px solid var(--signal-accent); outline-offset: 2px; }
.admin-dashboard-shortcuts svg { width: 16px; height: 16px; flex: 0 0 16px; color: var(--signal-muted); }
.admin-dashboard-shortcuts span { min-width: 0; overflow: hidden; text-overflow: ellipsis; white-space: nowrap; font-size: 13px; line-height: 20px; font-weight: 500; }
@media (min-width: 1024px) { .admin-dashboard-shortcuts { grid-template-columns: repeat(4, minmax(0, 1fr)); } }
.admin-trend-body { padding: 16px 20px 20px; }
.admin-trend-card .admin-trend-body :deep(.card) { padding: 0; border: 0; border-radius: 0; background: transparent; box-shadow: none; }
.admin-trend-body :deep(.card > h3) { display: none; }
.admin-trend-body :deep(.h-48) { height: 260px; }
.admin-chart-state { display: flex; align-items: center; justify-content: center; min-height: 240px; color: var(--signal-muted); font-size: 13px; }
</style>
