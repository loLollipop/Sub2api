import { describe, expect, it, vi } from 'vitest'
import { mount } from '@vue/test-utils'
import ModelPlazaContent from '../ModelPlazaContent.vue'
import PlazaFilterBar from '../PlazaFilterBar.vue'
import PlazaCatalogCard from '../PlazaCatalogCard.vue'
import Pagination from '@/components/common/Pagination.vue'
import Select from '@/components/common/Select.vue'
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

  it('clears search, billing, group and brand together', async () => {
    const wrapper = mountContent([group(1, 'openai', 0, [model('gpt')]), group(2, 'kimi', 2, [model('moonshot', 'kimi')])])
    const filter = wrapper.findComponent(PlazaFilterBar)
    filter.vm.$emit('update:platform', 'openai')
    filter.vm.$emit('update:groupId', 1)
    await wrapper.get('.plaza-billing-filters button:last-child').trigger('click')
    await wrapper.get('input[type="search"]').setValue('missing')
    expect(wrapper.text()).toContain('modelPlaza.noSearchResult')
    await wrapper.get('[data-testid="plaza-empty-clear-filters"]').trigger('click')
    expect(filter.props()).toMatchObject({ platform: 'all', groupId: 'all', search: '' })
    expect(wrapper.findAllComponents(PlazaCatalogCard)).toHaveLength(2)
    expect(wrapper.get('.plaza-billing-filters button:first-child').attributes('aria-checked')).toBe('true')
    filter.vm.$emit('update:groupId', 999)
    await wrapper.vm.$nextTick()
    expect(wrapper.text()).toContain('modelPlaza.noSearchResult')
    filter.vm.$emit('reset')
    await wrapper.vm.$nextTick()
    expect(filter.props('groupId')).toBe('all')
  })

  it('keeps same-name models separate by group and preserves zero custom rates', async () => {
    const zero = group(3, 'openai', 9, [model('shared')])
    zero.user_rate_multiplier = 0
    const wrapper = mountContent([
      group(1, 'openai', 2, [model('shared')]),
      group(2, 'openai', 0.5, [model('shared')]), zero,
      group(4, 'anthropic', 1, [model('shared', 'anthropic')])
    ])
    const entries = wrapper.findAllComponents(PlazaCatalogCard).map(card => card.props('entry'))
    expect(entries).toHaveLength(4)
    expect(entries.map(entry => entry.offers[0].group.id)).toEqual([3, 2, 4, 1])
    expect(new Set(entries.map(entry => entry.key)).size).toBe(4)
    expect(entries.every(entry => entry.offers.length === 1)).toBe(true)
    expect(wrapper.findAll('.plaza-catalog-section').map(section => section.attributes('data-group-id'))).toEqual(['3', '2', '4', '1'])
    const filter = wrapper.findComponent(PlazaFilterBar)
    expect(filter.props('groups').map((option: { rate: number }) => option.rate)).toEqual([2, 0.5, 0, 1])
    expect(wrapper.get('[data-group-id="3"] .plaza-section-rate').text()).toBe('0x')
    // A legacy rate event cannot leave an invisible constraint on the catalog.
    filter.vm.$emit('update:rate', 0)
    await wrapper.vm.$nextTick()
    expect(wrapper.findAllComponents(PlazaCatalogCard)).toHaveLength(4)
    expect(filter.props()).not.toHaveProperty('rate')
    expect(filter.props()).not.toHaveProperty('rates')
  })

  it('filters actual vendors inside composite groups and searches provider names', async () => {
    const wrapper = mountContent([group(1, 'composite', 1, [model('gpt-4.1'), model('claude-sonnet-4', 'anthropic')])])
    const filter = wrapper.findComponent(PlazaFilterBar)
    expect(filter.props('platforms')).toEqual(['anthropic', 'openai'])
    expect(filter.props('groups')[0].platforms).toEqual(['openai', 'anthropic'])
    await wrapper.get('input[type="search"]').setValue('Anthropic')
    expect(wrapper.findAllComponents(PlazaCatalogCard).map(card => card.props('entry').name)).toEqual(['claude-sonnet-4'])
    await wrapper.get('input[type="search"]').setValue('')
    filter.vm.$emit('update:platform', 'openai')
    await wrapper.vm.$nextTick()
    expect(wrapper.findAllComponents(PlazaCatalogCard).map(card => card.props('entry').name)).toEqual(['gpt-4.1'])
  })

  it('switches between disjoint groups, releases stale brands and restores the full brand list', async () => {
    const wrapper = mount(ModelPlazaContent, {
      props: { response: { description: '', groups: [
        group(1, 'openai', 0.15, [model('gpt-4.1')]),
        group(2, 'openai', 2, [model('qwen3-max')])
      ] }, loading: false },
      global: { stubs: { Icon: true, Select: true, Pagination: true, PlazaCatalogCard: true, PlazaProviderIcon: true } }
    })
    const filter = wrapper.findComponent(PlazaFilterBar)
    const groups = filter.findAll('.plaza-group-option')
    const brands = () => filter.findAll('.plaza-seg-item').map(button => button.text())
    expect(brands()).toEqual(['modelPlaza.filters.all', 'OpenAI', 'Qwen'])
    await groups[1].trigger('click')
    expect(brands()).toEqual(['modelPlaza.filters.all', 'OpenAI'])
    await filter.findAll('.plaza-seg-item')[1].trigger('click')
    expect(filter.props('platform')).toBe('openai')
    await groups[2].trigger('click')
    expect(filter.props()).toMatchObject({ groupId: 2, platform: 'all' })
    expect(brands()).toEqual(['modelPlaza.filters.all', 'Qwen'])
    expect(wrapper.findAllComponents(PlazaCatalogCard).map(card => card.props('entry').name)).toEqual(['qwen3-max'])
    await filter.findAll('.plaza-seg-item')[1].trigger('click')
    await groups[0].trigger('click')
    expect(filter.props('platform')).toBe('qwen')
    expect(brands()).toEqual(['modelPlaza.filters.all', 'OpenAI', 'Qwen'])
    await groups[1].trigger('click')
    expect(filter.props('platform')).toBe('all')
    expect(wrapper.findAllComponents(PlazaCatalogCard).map(card => card.props('entry').name)).toEqual(['gpt-4.1'])
  })

  it('filters and searches Chinese providers behind the same OpenAI-compatible protocol', async () => {
    const wrapper = mountContent([group(1, 'openai', 1, [model('qwen3-max'), model('deepseek-chat'), model('gpt-4.1'), model('unknown-private-model')])])
    const filter = wrapper.findComponent(PlazaFilterBar)
    expect(filter.props('platforms')).toEqual(['deepseek', 'openai', 'other', 'qwen'])
    expect(filter.props('groups')[0].modelCount).toBe(4)
    filter.vm.$emit('update:platform', 'qwen')
    await wrapper.vm.$nextTick()
    expect(wrapper.findAllComponents(PlazaCatalogCard).map(card => card.props('entry').name)).toEqual(['qwen3-max'])
    expect(wrapper.findComponent(PlazaCatalogCard).props('entry').platform).toBe('openai')
    filter.vm.$emit('update:platform', 'all')
    await wrapper.get('input[type="search"]').setValue('DeepSeek')
    expect(wrapper.findAllComponents(PlazaCatalogCard).map(card => card.props('entry').name)).toEqual(['deepseek-chat'])
    await wrapper.get('input[type="search"]').setValue('OpenAI')
    expect(wrapper.findAllComponents(PlazaCatalogCard).map(card => card.props('entry').name)).toEqual(['gpt-4.1'])
  })

  it('preserves group descriptions and access/quality badges above the models', () => {
    const exclusive = { ...group(2, 'openai', 1, [model('qwen3-max')]), description: 'Dedicated capacity', is_exclusive: true, subscription_type: 'subscription', quality_status: 'suspect' as const }
    const wrapper = mountContent([exclusive])
    const heading = wrapper.get('.plaza-section-heading')
    expect(heading.get('h2').text()).toBe('group-2')
    expect(heading.text()).toContain('Dedicated capacity')
    expect(heading.text()).toContain('modelPlaza.badges.exclusive')
    expect(heading.text()).toContain('modelPlaza.badges.subscription')
    expect(heading.text()).toContain('modelPlaza.badges.quality.suspect')
    expect(wrapper.findComponent(PlazaCatalogCard).props('grouped')).toBe(true)
  })

  it('paginates group sections and resets page/removed group and provider selections on refresh', async () => {
    const first = group(1, 'openai', 1, Array.from({ length: 25 }, (_, i) => model(`gpt-4-${i}`)))
    const second = group(2, 'openai', 2, [model('deepseek-chat')])
    const wrapper = mountContent([first, second])
    Object.defineProperty(wrapper.get('.plaza-catalog-toolbar').element, 'scrollIntoView', { value: vi.fn() })
    wrapper.findComponent(Pagination).vm.$emit('update:page', 2)
    await wrapper.vm.$nextTick()
    expect(wrapper.findAll('.plaza-catalog-section').map(section => section.attributes('data-group-id'))).toEqual(['1', '2'])
    expect(wrapper.findAllComponents(PlazaCatalogCard)).toHaveLength(2)
    const filter = wrapper.findComponent(PlazaFilterBar)
    filter.vm.$emit('update:groupId', 2)
    await wrapper.vm.$nextTick()
    expect(wrapper.findAllComponents(PlazaCatalogCard).map(card => card.props('entry').name)).toEqual(['deepseek-chat'])
    filter.vm.$emit('update:platform', 'deepseek')
    await wrapper.vm.$nextTick()
    await wrapper.setProps({ response: { description: '', groups: [first] } })
    expect(filter.props()).toMatchObject({ groupId: 'all', platform: 'all' })
    expect(wrapper.findAllComponents(PlazaCatalogCard)).toHaveLength(24)
    expect(wrapper.findComponent(Pagination).props('page')).toBe(1)
  })

  it('releases an incompatible brand after a refresh while preserving the chosen group', async () => {
    const wrapper = mountContent([
      group(1, 'openai', 1, [model('qwen3-max')]),
      group(2, 'openai', 2, [model('deepseek-chat')])
    ])
    const filter = wrapper.findComponent(PlazaFilterBar)
    filter.vm.$emit('update:groupId', 1)
    filter.vm.$emit('update:platform', 'qwen')
    await wrapper.vm.$nextTick()
    await wrapper.setProps({ response: { description: '', groups: [
      group(1, 'openai', 2, [model('deepseek-chat')]),
      group(2, 'openai', 1, [model('qwen3-max')])
    ] } })
    expect(filter.props()).toMatchObject({ groupId: 1, platform: 'all', platforms: ['deepseek'] })
    expect(wrapper.findAllComponents(PlazaCatalogCard).map(card => card.props('entry').name)).toEqual(['deepseek-chat'])
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

  it('sorts models within each group while keeping sections and toggling mobile filters', async () => {
    const wrapper = mountContent([
      group(1, 'openai', 1, [model('qwen3-max'), model('deepseek-chat')]),
      group(2, 'openai', 2, [model('qwen3-max'), model('deepseek-chat')])
    ])
    const sort = wrapper.findComponent(Select)
    sort.vm.$emit('update:modelValue', 'name-desc')
    await wrapper.vm.$nextTick()
    expect(wrapper.findAllComponents(PlazaCatalogCard).map(card => [card.props('entry').offers[0].group.id, card.props('entry').name])).toEqual([
      [1, 'qwen3-max'], [1, 'deepseek-chat'], [2, 'qwen3-max'], [2, 'deepseek-chat']
    ])
    sort.vm.$emit('update:modelValue', 'provider')
    await wrapper.vm.$nextTick()
    expect(wrapper.findAllComponents(PlazaCatalogCard).map(card => card.props('entry').provider)).toEqual(['deepseek', 'qwen', 'deepseek', 'qwen'])
    const toggle = wrapper.get('.plaza-mobile-filter-toggle')
    expect(toggle.attributes('aria-expanded')).toBe('false')
    await toggle.trigger('click')
    expect(toggle.attributes('aria-expanded')).toBe('true')
    expect(wrapper.get('#plaza-catalog-filters').classes()).toContain('is-open')
    await toggle.trigger('click')
    expect(toggle.attributes('aria-expanded')).toBe('false')
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
