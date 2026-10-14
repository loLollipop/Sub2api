import { describe, expect, it, vi } from 'vitest'
import { mount } from '@vue/test-utils'
import ModelPlazaContent from '../ModelPlazaContent.vue'
import PlazaFilterBar from '../PlazaFilterBar.vue'
import PlazaGroupSection from '../PlazaGroupSection.vue'
import type { ModelPlazaGroup, PlazaModel } from '@/api/modelPlaza'

vi.mock('vue-i18n', async () => ({
  ...await vi.importActual<typeof import('vue-i18n')>('vue-i18n'),
  useI18n: () => ({ t: (key: string) => key })
}))
vi.mock('@/stores/auth', () => ({ useAuthStore: () => ({ isAuthenticated: true }) }))

function model(name: string, rate: number): PlazaModel {
  return { name, platform: 'openai', rate_multiplier: rate, pricing: null, official_pricing: null }
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
    global: { stubs: { Icon: true, PlazaFilterBar: true, PlazaGroupSection: true } }
  })
}

describe('ModelPlazaContent Mixed rate filtering', () => {
  it('filters Mixed models by their rates and sorts by the visible minimum', async () => {
    const wrapper = mountContent([
      group(1, 'mixed', 9, [model('cheap', 0.5), model('expensive-visible', 2)]),
      group(2, 'openai', 1, [model('ordinary-visible', 1)])
    ])
    const filter = wrapper.findComponent(PlazaFilterBar)
    expect(filter.props('rates')).toEqual([0.5, 1, 2])
    expect(filter.props('groups')[0].rates).toEqual([0.5, 2])
    expect(wrapper.findAllComponents(PlazaGroupSection).map((section) => section.props('group').id)).toEqual([1, 2])
    filter.vm.$emit('update:rate', 2)
    await wrapper.vm.$nextTick()
    const sections = wrapper.findAllComponents(PlazaGroupSection)
    expect(sections).toHaveLength(1)
    expect(sections[0].props('group').models.map((m: PlazaModel) => m.name)).toEqual(['expensive-visible'])
    filter.vm.$emit('update:rate', 'all')
    filter.vm.$emit('update:search', 'visible')
    await wrapper.vm.$nextTick()
    expect(wrapper.findAllComponents(PlazaGroupSection).map((section) => section.props('group').id)).toEqual([2, 1])
    expect(wrapper.findAllComponents(PlazaGroupSection)[1].props('group').models).toHaveLength(1)
  })

  it('includes explicit zero overrides and independent image rates without the hidden legacy rate', () => {
    const mixed = group(1, 'mixed', 9, [model('token', 0.5), {
      ...model('image', 7), pricing: {
        billing_mode: 'image', input_price: null, output_price: null, cache_write_price: null,
        cache_read_price: null, image_input_price: null, image_output_price: null,
        per_request_price: 0.2, intervals: []
      }
    }])
    mixed.user_rate_multiplier = 0
    mixed.image_rate_independent = true
    mixed.image_rate_multiplier = 0.4
    expect(mountContent([mixed]).findComponent(PlazaFilterBar).props('rates')).toEqual([0, 0.4])
  })
})
