import { describe, expect, it, vi } from 'vitest'
import { mount } from '@vue/test-utils'
import ModelPlazaContent from '../ModelPlazaContent.vue'
import PlazaFilterBar from '../PlazaFilterBar.vue'
import PlazaCatalogCard from '../PlazaCatalogCard.vue'
import Pagination from '@/components/common/Pagination.vue'
import type { ModelPlazaGroup, PlazaModel } from '@/api/modelPlaza'

vi.mock('vue-i18n', async () => ({
  ...await vi.importActual<typeof import('vue-i18n')>('vue-i18n'),
  useI18n: () => ({ t: (key: string) => key })
}))
vi.mock('@/stores/auth', () => ({ useAuthStore: () => ({ isAuthenticated: true }) }))

function model(name: string, platform = 'openai'): PlazaModel {
  return { name, platform, pricing: null, official_pricing: null }
}

function group(id: number, platform: string, rate: number, models: PlazaModel[]): ModelPlazaGroup {
  return {
    id, name: `group-${id}`, platform, description: '', subscription_type: 'standard',
    rate_multiplier: rate, peak_rate_enabled: false, peak_start: '', peak_end: '',
    peak_rate_multiplier: 1, is_exclusive: false, image_rate_independent: false,
    image_rate_multiplier: 1, long_context_pricing_enabled: true, models
  }
}

function mountContent(groups: ModelPlazaGroup[]) {
  return mount(ModelPlazaContent, {
    props: { response: { description: '', groups }, loading: false },
    global: { stubs: { Icon: true, Select: true, Pagination: true, PlazaFilterBar: true, PlazaCatalogCard: true } }
  })
}

describe('ModelPlazaContent catalog', () => {
  it('treats whitespace search as unfiltered when there are no models', async () => {
    const wrapper = mountContent([])
    await wrapper.get('input[type="search"]').setValue('   ')
    expect(wrapper.text()).toContain('modelPlaza.empty')
    expect(wrapper.text()).not.toContain('modelPlaza.noSearchResult')
    expect(wrapper.find('[data-testid="plaza-empty-clear-filters"]').exists()).toBe(false)
  })

  it('clears search, billing and all three facets together', async () => {
    const wrapper = mountContent([group(1, 'openai', 0, [model('gpt')]), group(2, 'kimi', 2, [model('moonshot', 'kimi')])])
    const filter = wrapper.findComponent(PlazaFilterBar)
    filter.vm.$emit('update:platform', 'openai')
    filter.vm.$emit('update:groupId', 1)
    filter.vm.$emit('update:rate', 0)
    await wrapper.get('.plaza-billing-filters button:last-child').trigger('click')
    await wrapper.get('input[type="search"]').setValue('missing')
    expect(wrapper.text()).toContain('modelPlaza.noSearchResult')
    await wrapper.get('[data-testid="plaza-empty-clear-filters"]').trigger('click')
    expect(filter.props()).toMatchObject({ platform: 'all', groupId: 'all', rate: 'all', search: '' })
    expect(wrapper.findAllComponents(PlazaCatalogCard)).toHaveLength(2)
    expect(wrapper.get('.plaza-billing-filters button:first-child').attributes('aria-checked')).toBe('true')
    filter.vm.$emit('update:groupId', 999)
    await wrapper.vm.$nextTick()
    expect(wrapper.text()).toContain('modelPlaza.noSearchResult')
    filter.vm.$emit('reset')
    await wrapper.vm.$nextTick()
    expect(filter.props('groupId')).toBe('all')
  })

  it('merges duplicate offers while preserving vendor identity and effective rate order', async () => {
    const zero = group(3, 'openai', 9, [model('shared')])
    zero.user_rate_multiplier = 0
    const wrapper = mountContent([
      group(1, 'openai', 2, [model('shared')]),
      group(2, 'openai', 0.5, [model('shared')]), zero,
      group(4, 'anthropic', 1, [model('shared', 'anthropic')])
    ])
    const entries = wrapper.findAllComponents(PlazaCatalogCard).map(card => card.props('entry'))
    expect(entries).toHaveLength(2)
    expect(entries.find(entry => entry.platform === 'openai').offers.map((offer: { group: ModelPlazaGroup }) => offer.group.id)).toEqual([3, 2, 1])
    const filter = wrapper.findComponent(PlazaFilterBar)
    expect(filter.props('rates')).toEqual([0, 0.5, 1, 2])
    filter.vm.$emit('update:rate', 0)
    await wrapper.vm.$nextTick()
    expect(wrapper.findComponent(PlazaCatalogCard).props('entry').offers[0].group.id).toBe(3)
    expect(wrapper.findAllComponents(PlazaCatalogCard)).toHaveLength(1)
  })

  it('filters actual vendors inside composite groups and searches provider names', async () => {
    const wrapper = mountContent([group(1, 'composite', 1, [model('one'), model('two', 'anthropic')])])
    const filter = wrapper.findComponent(PlazaFilterBar)
    expect(filter.props('platforms')).toEqual(['anthropic', 'openai'])
    expect(filter.props('groups')[0].platforms).toEqual(['openai', 'anthropic'])
    await wrapper.get('input[type="search"]').setValue('Anthropic')
    expect(wrapper.findAllComponents(PlazaCatalogCard).map(card => card.props('entry').name)).toEqual(['two'])
    await wrapper.get('input[type="search"]').setValue('')
    filter.vm.$emit('update:platform', 'openai')
    await wrapper.vm.$nextTick()
    expect(wrapper.findAllComponents(PlazaCatalogCard).map(card => card.props('entry').name)).toEqual(['one'])
  })

  it('paginates large catalogs and resets the page when searching or refreshing', async () => {
    const data = [group(1, 'openai', 1, Array.from({ length: 30 }, (_, i) => model(`model-${i}`)))]
    const wrapper = mountContent(data)
    expect(wrapper.findAllComponents(PlazaCatalogCard)).toHaveLength(24)
    const scrollIntoView = vi.fn()
    Object.defineProperty(wrapper.get('.plaza-catalog-toolbar').element, 'scrollIntoView', { value: scrollIntoView })
    wrapper.findComponent(Pagination).vm.$emit('update:page', 2)
    await wrapper.vm.$nextTick()
    expect(wrapper.findAllComponents(PlazaCatalogCard)).toHaveLength(6)
    expect(scrollIntoView).toHaveBeenCalledOnce()
    await wrapper.get('input[type="search"]').setValue('model-29')
    expect(wrapper.findAllComponents(PlazaCatalogCard)).toHaveLength(1)
    expect(wrapper.findComponent(PlazaCatalogCard).props('entry').name).toBe('model-29')
    expect(wrapper.findComponent(Pagination).exists()).toBe(false)
    await wrapper.get('input[type="search"]').setValue('')
    wrapper.findComponent(Pagination).vm.$emit('update:page', 2)
    await wrapper.vm.$nextTick()
    await wrapper.setProps({ response: { description: '', groups: [group(2, 'openai', 1, [model('new')])] } })
    expect(wrapper.findComponent(PlazaCatalogCard).props('entry').name).toBe('new')
  })

  it('switches card/list layouts without losing the active search', async () => {
    const wrapper = mountContent([group(1, 'openai', 1, [model('gpt'), model('other')])])
    await wrapper.get('input[type="search"]').setValue('gpt')
    await wrapper.get('[aria-label="modelPlaza.list.listView"]').trigger('click')
    expect(wrapper.findComponent(PlazaCatalogCard).props('view')).toBe('list')
    expect(wrapper.findAllComponents(PlazaCatalogCard)).toHaveLength(1)
    expect((wrapper.get('input[type="search"]').element as HTMLInputElement).value).toBe('gpt')
  })

  it('changes billing filters using the radio-group keyboard and keeps one tab stop', async () => {
    const video = model('video')
    video.pricing = {
      billing_mode: 'video', input_price: null, output_price: null,
      cache_write_price: null, cache_read_price: null, image_input_price: null,
      image_output_price: null, per_request_price: 0.37, intervals: []
    }
    const wrapper = mountContent([group(1, 'openai', 1, [model('gpt'), video])])
    const radios = wrapper.findAll('.plaza-billing-filters [role="radio"]')
    expect(radios[0].attributes('tabindex')).toBe('0')
    await radios[0].trigger('keydown', { key: 'ArrowDown' })
    expect(radios[1].attributes('aria-checked')).toBe('true')
    expect(wrapper.findAllComponents(PlazaCatalogCard).map(card => card.props('entry').name)).toEqual(['gpt'])
    await radios[1].trigger('keydown', { key: 'End' })
    expect(radios.at(-1)?.attributes('aria-checked')).toBe('true')
    expect(wrapper.findAllComponents(PlazaCatalogCard).map(card => card.props('entry').name)).toEqual(['video'])
    expect(radios.filter(radio => radio.attributes('tabindex') === '0')).toHaveLength(1)
    await radios.at(-1)!.trigger('keydown', { key: 'Home' })
    expect(wrapper.findAllComponents(PlazaCatalogCard)).toHaveLength(2)
    expect(radios[0].attributes('aria-checked')).toBe('true')
  })
})
