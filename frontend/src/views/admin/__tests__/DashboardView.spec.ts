import { beforeEach, describe, expect, it, vi } from 'vitest'
import { flushPromises, mount, RouterLinkStub } from '@vue/test-utils'
import { createPinia, setActivePinia } from 'pinia'

import type { DashboardStats } from '@/types'
import DashboardView from '../DashboardView.vue'
import DashboardChartCard from '@/components/dashboard/DashboardChartCard.vue'

const { getSnapshotV2, getUserUsageTrend, getUserSpendingRanking } = vi.hoisted(() => ({
  getSnapshotV2: vi.fn(),
  getUserUsageTrend: vi.fn(),
  getUserSpendingRanking: vi.fn()
}))

vi.mock('@/api/admin', () => ({
  adminAPI: {
    dashboard: {
      getSnapshotV2,
      getUserUsageTrend,
      getUserSpendingRanking
    }
  }
}))

vi.mock('@/stores/app', () => ({
  useAppStore: () => ({
    showError: vi.fn()
  })
}))
vi.mock('vue-chartjs', () => ({ Line: { name: 'Line', props: ['data', 'options'], template: '<canvas />' } }))

vi.mock('vue-router', async original => ({
  ...await original<typeof import('vue-router')>(),
  useRouter: () => ({
    push: vi.fn()
  })
}))

vi.mock('vue-i18n', async () => {
  const actual = await vi.importActual<typeof import('vue-i18n')>('vue-i18n')
  return {
    ...actual,
    useI18n: () => ({
      t: (key: string) => key
    })
  }
})

const formatLocalDate = (date: Date): string => {
  const year = date.getFullYear()
  const month = String(date.getMonth() + 1).padStart(2, '0')
  const day = String(date.getDate()).padStart(2, '0')
  return `${year}-${month}-${day}`
}

const createDashboardStats = (): DashboardStats => ({
  total_users: 0,
  today_new_users: 0,
  active_users: 0,
  hourly_active_users: 0,
  users_with_balance: 0,
  users_with_balance_total: 0,
  stats_updated_at: '',
  stats_stale: false,
  total_api_keys: 0,
  active_api_keys: 0,
  total_accounts: 0,
  normal_accounts: 0,
  error_accounts: 0,
  ratelimit_accounts: 0,
  overload_accounts: 0,
  total_requests: 0,
  total_input_tokens: 0,
  total_output_tokens: 0,
  total_cache_creation_tokens: 0,
  total_cache_read_tokens: 0,
  total_tokens: 0,
  total_cost: 0,
  total_actual_cost: 0,
  today_requests: 0,
  today_input_tokens: 0,
  today_output_tokens: 0,
  today_cache_creation_tokens: 0,
  today_cache_read_tokens: 0,
  today_tokens: 0,
  today_cost: 0,
  today_actual_cost: 0,
  average_duration_ms: 0,
  uptime: 0,
  rpm: 0,
  tpm: 0
})

const mountDashboard = () => mount(DashboardView, {
  global: { stubs: {
    AppLayout: { template: '<div><slot /></div>' },
    LoadingSpinner: true, Icon: true, DateRangePicker: true, Select: true,
    ModelDistributionChart: true, TokenUsageTrend: true, Line: true, RouterLink: RouterLinkStub,
  } },
})

describe('admin DashboardView', () => {
  beforeEach(() => {
    setActivePinia(createPinia())

    getSnapshotV2.mockReset()
    getUserUsageTrend.mockReset()
    getUserSpendingRanking.mockReset()

    getSnapshotV2.mockResolvedValue({
      stats: createDashboardStats(),
      trend: [],
      models: []
    })
    getUserUsageTrend.mockResolvedValue({
      trend: [],
      start_date: '',
      end_date: '',
      granularity: 'hour'
    })
    getUserSpendingRanking.mockResolvedValue({
      ranking: [],
      total_actual_cost: 0,
      total_requests: 0,
      total_tokens: 0,
      start_date: '',
      end_date: ''
    })
  })

  it('uses last 24 hours as default dashboard range and keeps one full-refresh control', async () => {
    const wrapper = mountDashboard()

    await flushPromises()

    const now = new Date()
    const yesterday = new Date(now.getTime() - 24 * 60 * 60 * 1000)

    expect(getSnapshotV2).toHaveBeenCalledTimes(1)
    expect(getSnapshotV2).toHaveBeenCalledWith(expect.objectContaining({
      start_date: formatLocalDate(yesterday),
      end_date: formatLocalDate(now),
      granularity: 'hour'
    }))
    expect(wrapper.findAll('button[aria-label="common.refresh"]')).toHaveLength(1)
    await wrapper.get('button[aria-label="common.refresh"]').trigger('click')
    await flushPromises()
    expect(getSnapshotV2).toHaveBeenCalledTimes(2)
    expect(getUserUsageTrend).toHaveBeenCalledTimes(2)
    expect(getUserSpendingRanking).toHaveBeenCalledTimes(2)
    wrapper.unmount()
  })

  it.each([
    [0, 0, '$0.0000'],
    [1234, 12345.67, '$12.35K'],
    [2, 12.3456, '$12.35'],
  ])('shows positive-balance user totals and count (%s users)', async (count, total, formatted) => {
    getSnapshotV2.mockResolvedValueOnce({
      stats: { ...createDashboardStats(), users_with_balance: count, users_with_balance_total: total, average_duration_ms: 9876 },
      trend: [], models: [],
    })
    const wrapper = mountDashboard()
    await flushPromises()
    const card = wrapper.get('[data-testid="users-with-balance"]')
    expect(card.get('.overview-stat-heading').text()).toBe('admin.dashboard.usersWithBalance')
    expect(card.get('.overview-stat-value').text()).toBe(formatted)
    expect(card.get('.overview-stat-detail').text()).toBe(`${count.toLocaleString()} admin.dashboard.usersWithBalanceUnit`)
    expect(wrapper.text()).not.toContain('admin.dashboard.avgResponse')
    expect(wrapper.findAll('.overview-stat')).toHaveLength(8)
    wrapper.unmount()
  })

  it('shows total users with today growth, labeled costs, and selected-range chart sums in source order', async () => {
    getSnapshotV2.mockResolvedValueOnce({
      stats: { ...createDashboardStats(), total_users: 12345, today_new_users: 7, total_tokens: 1000000 },
      trend: [{ total_tokens: 100 }, { total_tokens: 200 }], models: [],
    })
    getUserUsageTrend.mockResolvedValueOnce({ trend: [{ tokens: 400 }, { tokens: 500 }] })
    const wrapper = mountDashboard()
    await flushPromises()
    const users = wrapper.findAll('.overview-stat')[3]
    expect(users.get('.overview-stat-value').text()).toContain('12,345')
    expect(users.get('.overview-stat-badge').text()).toBe('+7')
    for (const index of [4, 5]) {
      const detail = wrapper.findAll('.overview-stat')[index].get('.overview-stat-detail').text()
      for (const key of ['actual', 'accountCost', 'standard']) expect(detail).toContain(`admin.dashboard.${key}`)
    }
    const cards = wrapper.findAllComponents(DashboardChartCard)
    expect(cards.map(card => card.get('.dashboard-chart-value').text())).toEqual(['300', '900'])
    const children = wrapper.get('.admin-dashboard-charts').element.children
    expect(children[0].classList.contains('admin-trend-card')).toBe(true)
    expect(children[1].tagName.toLowerCase()).toBe('model-distribution-chart-stub')
    expect(children[2].textContent).toContain('Top 12')

    let resolveSnapshot!: (value: unknown) => void
    getSnapshotV2.mockImplementationOnce(() => new Promise(resolve => { resolveSnapshot = resolve }))
    wrapper.getComponent({ name: 'Select' }).vm.$emit('change')
    await flushPromises()
    expect(cards[0].find('.dashboard-chart-value').exists()).toBe(false)
    resolveSnapshot({ trend: [{ total_tokens: 600 }], models: [] })
    await flushPromises()
    expect(cards[0].get('.dashboard-chart-value').text()).toBe('600')

    const consoleError = vi.spyOn(console, 'error').mockImplementation(() => {})
    getSnapshotV2.mockRejectedValueOnce(new Error('Chart unavailable'))
    getUserUsageTrend.mockRejectedValueOnce(new Error('Users unavailable'))
    wrapper.getComponent({ name: 'Select' }).vm.$emit('change')
    await flushPromises()
    expect(cards.every(card => !card.find('.dashboard-chart-value').exists())).toBe(true)
    expect(wrapper.findAll('.admin-dashboard-charts [role="alert"]')).toHaveLength(2)
    consoleError.mockRestore()
    wrapper.unmount()
  })
})
