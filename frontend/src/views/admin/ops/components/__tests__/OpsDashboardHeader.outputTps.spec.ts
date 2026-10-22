import { beforeEach, describe, expect, it, vi } from 'vitest'
import { shallowMount } from '@vue/test-utils'
import type { OpsDashboardOverview } from '@/api/admin/ops'
import OpsDashboardHeader from '../OpsDashboardHeader.vue'

vi.mock('@/api', () => ({ adminAPI: { groups: { getAll: vi.fn().mockResolvedValue([]) } } }))
vi.mock('@/stores', () => ({ useAdminSettingsStore: () => ({ opsRealtimeMonitoringEnabled: false }) }))
vi.mock('vue-i18n', async (importOriginal) => ({
  ...await importOriginal<typeof import('vue-i18n')>(),
  useI18n: () => ({ t: (key: string, args?: { count?: string }) => args?.count != null ? `${key}: ${args.count}` : key }),
}))

describe('Ops per-request output TPS', () => {
  beforeEach(() => vi.clearAllMocks())

  it('opens health diagnosis with an accessible button instead of requiring hover', async () => {
    const wrapper = shallowMount(OpsDashboardHeader, {
      props: {
        overview: { tps: { current: 0, peak: 0, avg: 0 } } as OpsDashboardOverview,
        platform: '', groupId: null, timeRange: '1h', queryMode: 'auto', loading: false, lastUpdated: null,
      },
    })
    const trigger = wrapper.get('[aria-label="admin.ops.diagnosis.title"]')
    expect(trigger.element.tagName).toBe('BUTTON')
    await trigger.trigger('click')
    const dialog = wrapper.findAllComponents({ name: 'BaseDialog' }).find(dialog => dialog.props('title') === 'admin.ops.diagnosis.title')!
    expect(dialog.props('show')).toBe(true)
    dialog.vm.$emit('close')
    await wrapper.vm.$nextTick()
    expect(dialog.props('show')).toBe(false)
    wrapper.unmount()
  })

  it.each([
    { stats: { p5: 0, p10: 12.34, p50: 45.67, avg: 89.01, sample_count: 4 }, expected: ['0.0', '12.3', '45.7', '89.0'], samples: '4' },
    { stats: { p5: null, p10: null, p50: null, avg: null, sample_count: 0 }, expected: ['—', '—', '—', '—'], samples: '0' },
    { stats: null, expected: ['—', '—', '—', '—'], samples: '—' },
  ])('shows null and zero distinctly without substituting aggregate throughput', ({ stats, expected, samples }) => {
    const wrapper = shallowMount(OpsDashboardHeader, {
      props: {
        overview: { output_tps: stats, tps: { current: 999, peak: 999, avg: 999 } } as OpsDashboardOverview,
        platform: '', groupId: null, timeRange: '1h', queryMode: 'auto', loading: false, lastUpdated: null,
      },
    })
    const card = wrapper.find('[data-testid="output-tps-card"]')
    for (const [i, metric] of ['p5', 'p10', 'p50', 'avg'].entries()) {
      expect(card.find(`[data-testid="output-tps-${metric}"]`).text()).toBe(expected[i])
    }
    expect(card.find('[data-testid="output-tps-samples"]').text()).toContain(samples)
    expect(card.text()).not.toContain('999')
    wrapper.unmount()
  })
})
