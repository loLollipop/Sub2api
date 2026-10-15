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

function model(name: string): PlazaModel {
  return { name, platform: 'openai', pricing: null, official_pricing: null }
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

describe('ModelPlazaContent native group rates', () => {
  it('filters and sorts native groups by their effective rates', async () => {
    const wrapper = mountContent([group(1, 'kimi', 2, [model('moonshot')]), group(2, 'deepseek', 0.5, [model('deepseek-chat')])])
    const filter = wrapper.findComponent(PlazaFilterBar)
    expect(filter.props('rates')).toEqual([0.5, 2])
    expect(wrapper.findAllComponents(PlazaGroupSection).map(section => section.props('group').id)).toEqual([2, 1])
    filter.vm.$emit('update:rate', 2)
    await wrapper.vm.$nextTick()
    expect(wrapper.findAllComponents(PlazaGroupSection).map(section => section.props('group').id)).toEqual([1])
  })
  it('preserves a zero user override', () => {
    const native = group(1, 'deepseek', 2, [model('deepseek-chat')])
    native.user_rate_multiplier = 0
    expect(mountContent([native]).findComponent(PlazaFilterBar).props('rates')).toEqual([0])
  })
})
