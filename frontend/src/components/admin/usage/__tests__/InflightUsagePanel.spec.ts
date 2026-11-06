import { describe, expect, it, vi } from 'vitest'
import { mount } from '@vue/test-utils'
import InflightUsagePanel from '../InflightUsagePanel.vue'

vi.mock('vue-i18n', () => ({
  useI18n: () => ({ t: (key: string) => key }),
}))

const base = { open: false, total: 0, shown: 0 }

describe('InflightUsagePanel', () => {
  it('starts as a compact disclosure and labels its region when opened', async () => {
    const wrapper = mount(InflightUsagePanel, { props: base })
    const toggle = wrapper.get('[data-testid="inflight-toggle"]')
    expect(toggle.attributes('aria-expanded')).toBe('false')
    expect(wrapper.find('[role="region"]').exists()).toBe(false)
    await toggle.trigger('click')
    expect(wrapper.emitted('toggle')).toHaveLength(1)
    await wrapper.setProps({ open: true })
    const region = wrapper.get('[role="region"]')
    expect(region.attributes('aria-labelledby')).toBe(toggle.attributes('id'))
    expect(region.attributes('id')).toBe(toggle.attributes('aria-controls'))
  })

  it('does not show an empty state until loading has succeeded', async () => {
    const wrapper = mount(InflightUsagePanel, { props: { ...base, open: true, loading: true } })
    expect(wrapper.find('[role="status"]').exists()).toBe(true)
    expect(wrapper.find('[data-testid="inflight-empty"]').exists()).toBe(false)
    await wrapper.setProps({ loading: false, loaded: true })
    expect(wrapper.get('[data-testid="inflight-empty"]').text()).toContain('usage.inflightEmpty')
  })

  it('shows a recoverable failure rather than claiming there are no requests', async () => {
    const wrapper = mount(InflightUsagePanel, { props: { ...base, open: true, error: 'Load failed' } })
    expect(wrapper.get('[role="alert"]').text()).toBe('Load failed')
    expect(wrapper.find('[data-testid="inflight-empty"]').exists()).toBe(false)
    await wrapper.get('[data-testid="inflight-refresh"]').trigger('click')
    expect(wrapper.emitted('refresh')).toHaveLength(1)
  })

  it('preserves loaded rows during pagination and prevents duplicate load-more requests', async () => {
    const wrapper = mount(InflightUsagePanel, {
      props: { open: true, loaded: true, total: 20, shown: 10, hasMore: true },
      slots: { default: '<div data-test="rows">running rows</div>' },
    })
    expect(wrapper.get('[data-test="rows"]').text()).toBe('running rows')
    await wrapper.get('[data-testid="inflight-more"]').trigger('click')
    expect(wrapper.emitted('more')).toHaveLength(1)
    await wrapper.setProps({ loading: true })
    expect(wrapper.get('[data-testid="inflight-more"]').attributes('disabled')).toBeDefined()
    expect(wrapper.find('[data-test="rows"]').exists()).toBe(true)
    expect(wrapper.get('[data-testid="inflight-refresh"]').attributes('disabled')).toBeDefined()
  })
})
