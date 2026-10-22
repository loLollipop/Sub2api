import { defineComponent } from 'vue'
import { flushPromises, mount } from '@vue/test-utils'
import { describe, expect, it, vi } from 'vitest'
import AvailableChannelsView from '../AvailableChannelsView.vue'

const api = vi.hoisted(() => ({ getAvailable: vi.fn(), getUserGroupRates: vi.fn(), showError: vi.fn() }))
vi.mock('@/api/channels', () => ({ default: api }))
vi.mock('@/api/groups', () => ({ default: api }))
vi.mock('@/stores/app', () => ({ useAppStore: () => api }))
vi.mock('vue-i18n', async importOriginal => ({ ...await importOriginal<typeof import('vue-i18n')>(), useI18n: () => ({ t: (key: string) => key }) }))
const Layout = defineComponent({ template: '<div><slot/><slot name="filters"/><slot name="table"/></div>' })
const Table = defineComponent({ props: ['rows', 'emptyLabel'], template: '<div><span v-if="rows.length">{{ rows[0].name }}</span><template v-else>{{ emptyLabel }}<slot name="empty-action"/></template></div>' })

describe('available channel search recovery', () => {
  it('distinguishes no matches from no data and clears search without refetching', async () => {
    api.getAvailable.mockResolvedValue([{ name: 'Primary channel', description: '', platforms: [] }])
    api.getUserGroupRates.mockResolvedValue({})
    const wrapper = mount(AvailableChannelsView, { global: { stubs: { AppLayout: Layout, TablePageLayout: Layout, AvailableChannelsTable: Table } } })
    await flushPromises()
    const search = wrapper.get('input')
    await search.setValue('missing channel')
    expect(wrapper.text()).toContain('availableChannels.emptyFiltered')
    await wrapper.findAll('button').find(button => button.text() === 'availableChannels.clearSearch')!.trigger('click')
    expect(wrapper.text()).toContain('Primary channel')
    expect(api.getAvailable).toHaveBeenCalledTimes(1)
    await search.setValue('   ')
    expect(wrapper.text()).toContain('Primary channel')
    api.getAvailable.mockResolvedValue([])
    await wrapper.get('button').trigger('click')
    await flushPromises()
    expect(wrapper.text()).toContain('availableChannels.empty')
    expect(wrapper.text()).not.toContain('availableChannels.clearSearch')
    wrapper.unmount()
  })
})
