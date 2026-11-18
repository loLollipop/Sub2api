import { describe, expect, it, vi } from 'vitest'
import { mount } from '@vue/test-utils'
import EndpointDistributionChart from '../EndpointDistributionChart.vue'

vi.mock('vue-i18n', () => ({ useI18n: () => ({ t: (key: string) => key }) }))
vi.mock('vue-chartjs', () => ({
  Doughnut: { props: ['data'], template: '<div class="chart-data">{{ JSON.stringify(data) }}</div>' },
}))
vi.mock('@/api/admin/dashboard', () => ({
  getUserBreakdown: vi.fn().mockResolvedValue({ users: [] }),
}))

const endpointStats = [
  { endpoint: '/v1/responses', requests: 2, total_tokens: 30, cost: 1, actual_cost: .15, account_cost: 4 },
  { endpoint: '/v1/messages', requests: 1, total_tokens: 10, cost: 2, actual_cost: .3, account_cost: 0 },
]

describe('EndpointDistributionChart costs', () => {
  it('shows actual customer charges and account costs in separate admin columns', () => {
    const wrapper = mount(EndpointDistributionChart, { props: { endpointStats, metric: 'actual_cost' } })
    const rows = wrapper.findAll('tbody tr')
    expect(wrapper.findAll('thead th')).toHaveLength(6)
    expect(rows[0].text()).toContain('/v1/messages')
    expect(rows[0].findAll('td')[3].text()).toBe('$0.300')
    expect(rows[0].findAll('td')[4].text()).toBe('$0.0000')
    expect(rows[1].findAll('td')[3].text()).toBe('$0.150')
    expect(rows[1].findAll('td')[4].text()).toBe('$4.00')
    expect(JSON.parse(wrapper.find('.chart-data').text()).datasets[0].data).toEqual([.3, .15])
  })

  it('hides the account cost column and nested breakdown for user callers', async () => {
    const wrapper = mount(EndpointDistributionChart, { props: { endpointStats, showAccountCost: false } })
    expect(wrapper.findAll('thead th')).toHaveLength(5)
    expect(wrapper.text()).not.toContain('admin.dashboard.accountCost')
    await wrapper.find('tbody tr').trigger('click')
    expect(wrapper.find('td[colspan]').attributes('colspan')).toBe('5')
    expect(wrapper.findComponent({ name: 'UserBreakdownSubTable' }).props('showAccountCost')).toBe(false)
  })
})
