import { flushPromises, mount } from '@vue/test-utils'
import { afterEach, describe, expect, it, vi } from 'vitest'
import { defineComponent } from 'vue'
import OpsThroughputTrendChart from '@/views/admin/ops/components/OpsThroughputTrendChart.vue'
import OpsSwitchRateTrendChart from '@/views/admin/ops/components/OpsSwitchRateTrendChart.vue'
import OpsErrorTrendChart from '@/views/admin/ops/components/OpsErrorTrendChart.vue'
import OpsErrorDistributionChart from '@/views/admin/ops/components/OpsErrorDistributionChart.vue'
import OpsLatencyChart from '@/views/admin/ops/components/OpsLatencyChart.vue'
import MonitorTrendChart from '@/features/channel-monitor-v2/MonitorTrendChart.vue'
import AccountStatsModal from '@/components/account/AccountStatsModal.vue'

vi.mock('vue-i18n', async () => ({
  ...await vi.importActual<typeof import('vue-i18n')>('vue-i18n'),
  useI18n: () => ({ t: (key: string) => key, locale: { value: 'en' } })
}))

vi.mock('vue-chartjs', () => {
  const chart = defineComponent({
    name: 'ThemeChartStub',
    props: ['data', 'options'],
    template: '<div />'
  })
  return { Line: chart, Bar: chart, Doughnut: chart }
})

vi.mock('@/api/admin', () => ({
  adminAPI: { accounts: { getStats: vi.fn().mockResolvedValue({
    summary: { total_cost: 1, total_user_cost: 2, total_standard_cost: 3,
      total_requests: 3, total_tokens: 1000, avg_daily_cost: 1,
      avg_daily_user_cost: 2, avg_daily_requests: 3, avg_daily_tokens: 1000,
      avg_duration_ms: 100, actual_days_used: 1, days: 30 }, models: [], endpoints: [],
    history: [{ label: 'Day 1', actual_cost: 1, user_cost: 2, requests: 3 }]
  }) } }
}))

const points = [{ bucket_start: '2026-11-10T00:00:00Z', request_count: 10,
  token_consumed: 1000, qps: 1, tps: 100, switch_count: 1 }]
const cases = [
  { name: 'throughput', component: OpsThroughputTrendChart, props: { points, loading: false, timeRange: '1h' } },
  { name: 'switch rate', component: OpsSwitchRateTrendChart, props: { points, loading: false, timeRange: '1h' } },
  { name: 'error trend', component: OpsErrorTrendChart, props: { points: [{ bucket_start: points[0].bucket_start,
    error_count_total: 2, error_count_sla: 1, business_limited_count: 1,
    upstream_error_count_excl_429_529: 1, upstream_429_count: 0, upstream_529_count: 0 }], loading: false, timeRange: '1h' } },
  { name: 'error distribution', component: OpsErrorDistributionChart, props: {
    data: { total: 2, items: [{ status_code: 400, total: 2, sla: 2, business_limited: 0 }] }, loading: false } },
  { name: 'latency', component: OpsLatencyChart, props: { loading: false,
    latencyData: { start_time: '', end_time: '', platform: '', total_requests: 3, buckets: [{ range: '0-100ms', count: 3 }] } } },
  { name: 'monitor', component: MonitorTrendChart, props: { loading: false, coverage: null,
    trend: [{ bucket_start: points[0].bucket_start, metrics: { error_rate: 0.1, cache_rate: 0.5, ttft: { p50_ms: 100 } }, health: {} }] } },
  { name: 'account stats', component: AccountStatsModal, props: { show: true, account: { id: 1, name: 'Demo', platform: 'openai', type: 'apikey', credentials: {} } } }
]

afterEach(() => { document.documentElement.classList.remove('dark') })

describe('chart theme switching without remounting', () => {
  it.each(cases)('$name refreshes its tooltip and chart colors in both directions', async ({ component, props }) => {
    document.documentElement.classList.remove('dark')
    const wrapper = mount(component as Parameters<typeof mount>[0], {
      props,
      global: { stubs: {
        HelpTooltip: true, EmptyState: true, LoadingSpinner: true, Icon: true,
        ModelDistributionChart: true, EndpointDistributionChart: true,
        BaseDialog: { template: '<div><slot /><slot name="footer" /></div>' }
      } }
    })
    await flushPromises()
    const chart = wrapper.findComponent({ name: 'ThemeChartStub' })
    expect(chart.exists()).toBe(true)
    const beforeData = JSON.stringify(chart.props('data'))
    expect(chart.props('options').plugins.tooltip).toMatchObject({ backgroundColor: '#ffffff', bodyColor: '#5d6573' })

    document.documentElement.classList.add('dark')
    await flushPromises()
    expect(chart.props('options').plugins.tooltip).toMatchObject({ backgroundColor: '#1d2027', bodyColor: '#a4abb8' })
    expect(JSON.stringify(chart.props('data'))).not.toBe(beforeData)

    document.documentElement.classList.remove('dark')
    await flushPromises()
    expect(chart.props('options').plugins.tooltip.backgroundColor).toBe('#ffffff')
    expect(JSON.stringify(chart.props('data'))).toBe(beforeData)
    wrapper.unmount()
  })
})
