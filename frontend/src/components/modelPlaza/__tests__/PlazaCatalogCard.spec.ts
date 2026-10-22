import { afterEach, describe, expect, it, vi } from 'vitest'
import { mount, type VueWrapper } from '@vue/test-utils'
import { nextTick } from 'vue'
import type { ModelPlazaGroup, PlazaModel } from '@/api/modelPlaza'
import type { UserPricingInterval } from '@/api/channels'
import Select from '@/components/common/Select.vue'
import BaseDialog from '@/components/common/BaseDialog.vue'
import PlazaModelPricingTable from '../PlazaModelPricingTable.vue'
import PlazaCatalogCard from '../PlazaCatalogCard.vue'
import { buildModelCatalog } from '../modelPlazaCatalog'

const { copyName } = vi.hoisted(() => ({ copyName: vi.fn() }))
vi.mock('vue-i18n', async () => ({
  ...await vi.importActual<typeof import('vue-i18n')>('vue-i18n'),
  useI18n: () => ({ t: (key: string, params?: Record<string, unknown>) => params ? `${key} ${Object.values(params).join(' ')}` : key })
}))
vi.mock('@/stores/app', () => ({ useAppStore: () => ({ cachedPublicSettings: { server_utc_offset: '+08:00' } }) }))
vi.mock('@/composables/useClipboard', async () => {
  const { ref } = await import('vue')
  return { useClipboard: () => {
    const copied = ref(false)
    return { copied, copyToClipboard: async (name: string) => { copyName(name); copied.value = true; return true } }
  } }
})

function model(overrides: Partial<PlazaModel> = {}): PlazaModel {
  return {
    name: 'shared-model', platform: 'openai',
    pricing: {
      billing_mode: 'token', input_price: 3e-6, output_price: 15e-6,
      cache_write_price: null, cache_read_price: 0.3e-6,
      image_input_price: null, image_output_price: null, per_request_price: null, intervals: []
    },
    official_pricing: { input_price: 3e-6, output_price: 15e-6, cache_write_price: null, cache_read_price: null },
    ...overrides
  }
}
function group(overrides: Partial<ModelPlazaGroup> = {}): ModelPlazaGroup {
  return {
    id: 1, name: 'Standard', description: 'Standard group description', platform: 'openai',
    subscription_type: 'standard', rate_multiplier: 1, peak_rate_enabled: false,
    peak_start: '', peak_end: '', peak_rate_multiplier: 1, is_exclusive: false,
    image_rate_independent: false, image_rate_multiplier: 1, long_context_pricing_enabled: true,
    models: [], ...overrides
  }
}
function interval(overrides: Partial<UserPricingInterval> = {}): UserPricingInterval {
  return {
    min_tokens: 0, max_tokens: 100000, input_price: 6e-6, output_price: 30e-6,
    cache_read_price: 0.6e-6, cache_write_price: null, per_request_price: null, ...overrides
  }
}
function entry(offers = [{ group: group(), model: model() }]) {
  return { key: 'openai:shared-model', name: 'shared-model', platform: 'openai', provider: 'openai', offers }
}
const wrappers: VueWrapper[] = []
function mountCard(catalog = entry(), view: 'grid' | 'list' = 'grid') {
  const wrapper = mount(PlazaCatalogCard, { props: { entry: catalog, view }, attachTo: document.body })
  wrappers.push(wrapper)
  return wrapper
}
afterEach(() => {
  wrappers.splice(0).forEach(wrapper => wrapper.unmount())
  document.body.innerHTML = ''
  vi.clearAllMocks()
})

describe('PlazaCatalogCard', () => {
  it('keeps same-name group prices and open details bound to refreshed original models', async () => {
    const firstModel = model({ name: 'deepseek-chat' })
    const secondModel = model({ name: 'deepseek-chat', pricing: { ...firstModel.pricing!, input_price: 7e-6 } })
    const firstGroup = group({ models: [firstModel], rate_multiplier: 2 })
    const secondGroup = group({ id: 2, name: 'Dedicated', models: [secondModel], rate_multiplier: 9, user_rate_multiplier: 0 })
    const entries = buildModelCatalog([firstGroup, secondGroup])
    const standard = mountCard(entries[0])
    const dedicated = mountCard(entries[1])
    expect(standard.get('[data-price="input"]').text()).toContain('$6.00')
    expect(dedicated.get('[data-price="input"]').text()).toContain('$0.00')
    expect(standard.get('.catalog-name p').text()).toContain('DeepSeek')
    expect(standard.get('.catalog-name p').text()).not.toContain('OpenAI')
    await standard.get('.catalog-details').trigger('click')
    const refreshedModel = model({ name: 'deepseek-chat', pricing: { ...firstModel.pricing!, input_price: 8e-6 } })
    const refreshedGroup = { ...firstGroup, models: [refreshedModel], rate_multiplier: 4 }
    await standard.setProps({ entry: buildModelCatalog([refreshedGroup])[0] })
    expect(standard.get('[data-price="input"]').text()).toContain('$32.00')
    expect(standard.getComponent(PlazaModelPricingTable).props('models')).toEqual([refreshedModel])
    expect(standard.getComponent(PlazaModelPricingTable).props('rateMultiplier')).toBe(4)
    expect(dedicated.get('[data-price="input"]').text()).toContain('$0.00')
    await standard.setProps({ entry: entries[1] })
    expect(standard.get('[data-price="input"]').text()).toContain('$0.00')
    expect(standard.getComponent(PlazaModelPricingTable).props('models')).toEqual([secondModel])
    expect(standard.getComponent(PlazaModelPricingTable).props('userRateMultiplier')).toBe(0)
  })

  it('switches prices and open details together, preserving a zero custom rate and group disclosures', async () => {
    const first = { group: group(), model: model() }
    const freeGroup = group({
      id: 2, name: 'Free exclusive', description: 'Exclusive group description', rate_multiplier: 9, user_rate_multiplier: 0,
      subscription_type: 'subscription', is_exclusive: true, quality_status: 'suspect',
      peak_rate_enabled: true, peak_start: '14:00', peak_end: '18:00', peak_rate_multiplier: 2,
      long_context_pricing_enabled: false
    })
    const secondModel = model()
    secondModel.official_pricing!.intervals = [interval(), interval({ min_tokens: 100000, max_tokens: null })]
    const wrapper = mountCard(entry([first, { group: freeGroup, model: secondModel }]))
    expect(wrapper.get('[data-price="input"]').text()).toContain('$3.00')
    expect(wrapper.findComponent(PlazaModelPricingTable).exists()).toBe(false)
    await wrapper.get('.catalog-details').trigger('click')
    expect(document.querySelector('[role="dialog"]')?.textContent).toContain('Standard group description')

    wrapper.findAllComponents(Select)[1].vm.$emit('update:modelValue', 2)
    await nextTick()
    expect(wrapper.get('[data-price="input"]').text()).toContain('$0.00')
    expect(wrapper.get('[data-price="output"]').text()).toContain('$0.00')
    expect(wrapper.get('[data-price="cache-read"]').text()).toContain('$0.00')
    expect(wrapper.get('.catalog-rate s').text()).toBe('9x')
    const dialog = document.querySelector('[role="dialog"]')!
    expect(dialog.textContent).toContain('Exclusive group description')
    expect(dialog.textContent).not.toContain('Standard group description')
    expect(dialog.textContent).toContain('modelPlaza.badges.exclusive')
    expect(dialog.textContent).toContain('modelPlaza.badges.subscription')
    expect(dialog.textContent).toContain('modelPlaza.badges.quality.suspect')
    expect(dialog.textContent).toContain('UTC+08:00')
    expect(dialog.textContent).toContain('modelPlaza.detail.longContextDisabledNote')
    const table = wrapper.getComponent(PlazaModelPricingTable)
    expect(table.props('models')).toEqual([secondModel])
    expect(table.props('userRateMultiplier')).toBe(0)
    expect(table.props('rateMultiplier')).toBe(9)
    expect(table.props('peakRateMultiplier')).toBe(2)
    expect(table.props('peakWindow')).toContain('UTC+08:00')

    wrapper.findAllComponents(Select)[1].vm.$emit('update:modelValue', 1)
    await nextTick()
    expect(wrapper.get('[data-price="input"]').text()).toContain('$3.00')
    expect(dialog.textContent).toContain('Standard group description')
    expect(dialog.textContent).not.toContain('longContextDisabledNote')
    wrapper.getComponent(BaseDialog).vm.$emit('close')
    await nextTick()
    expect(wrapper.getComponent(BaseDialog).props('show')).toBe(false)
  })

  it('falls back to a current offer after filtering and forgets an invalid selection', async () => {
    const first = { group: group(), model: model() }
    const second = { group: group({ id: 2, rate_multiplier: 4 }), model: model() }
    const wrapper = mountCard(entry([first, second]))
    wrapper.getComponent(Select).vm.$emit('update:modelValue', 2)
    await nextTick()
    expect(wrapper.get('[data-price="input"]').text()).toContain('$12.00')
    await wrapper.get('.catalog-details').trigger('click')
    await wrapper.setProps({ entry: entry([first]) })
    expect(wrapper.get('[data-price="input"]').text()).toContain('$3.00')
    expect(wrapper.getComponent(PlazaModelPricingTable).props('rateMultiplier')).toBe(1)
    expect(wrapper.findComponent(Select).exists()).toBe(false)
    await wrapper.setProps({ entry: entry([first, second]) })
    expect(wrapper.getComponent(Select).props('modelValue')).toBe(1)
    expect(wrapper.get('[data-price="input"]').text()).toContain('$3.00')
  })

  it('uses resolved first-tier token prices and labels standard time pricing', () => {
    const tierModel = model({ time_pricing: { timezone: 'Asia/Shanghai', periods: [{ start_time: '12:00', end_time: '18:00', multiplier: 0.5 }] } })
    tierModel.pricing!.intervals = [interval({ input_price: null, input_multiplier: 2, tier_label: 'First tier' })]
    const wrapper = mountCard(entry([{ group: group({ rate_multiplier: 0.5 }), model: tierModel }]))
    expect(wrapper.get('[data-price="input"]').text()).toContain('$3.00')
    expect(wrapper.get('[data-price="output"]').text()).toContain('$15.00')
    expect(wrapper.get('[data-price="cache-read"]').text()).toContain('$0.30')
    expect(wrapper.get('.catalog-price-notes').text()).toContain('modelPlaza.list.tierPreview First tier')
    expect(wrapper.get('.catalog-price-notes').text()).toContain('modelPlaza.list.standardPrice')
    expect(wrapper.get('.catalog-price-notes').text()).toContain('modelPlaza.list.timePricing')
  })

  it.each([0, 0.5])('uses the independent image rate %s with request tiers and image units', async (imageRate) => {
    const image = model()
    image.pricing = { ...image.pricing!, billing_mode: 'image', per_request_price: 99, intervals: [interval({ tier_label: 'HD', per_request_price: 0.04 })] }
    const imageGroup = group({ rate_multiplier: 9, user_rate_multiplier: 0, image_rate_independent: true, image_rate_multiplier: imageRate })
    const wrapper = mountCard(entry([{ group: imageGroup, model: image }]))
    expect(wrapper.get('[data-price="request"]').text()).toContain(imageRate === 0 ? '$0.00' : '$0.02')
    expect(wrapper.get('[data-price="request"]').text()).toContain('modelPlaza.table.perUnitImage')
    expect(wrapper.text()).not.toContain('modelPlaza.table.unitPerMillion')
    expect(wrapper.get('.catalog-price-notes').text()).toContain('modelPlaza.list.tierPreview HD')
    expect(wrapper.get('.catalog-price-notes').text()).toContain(`modelPlaza.list.imageRate ${imageRate}`)
    expect(wrapper.find('.catalog-rate s').exists()).toBe(false)
    await wrapper.get('.catalog-details').trigger('click')
    expect(wrapper.getComponent(PlazaModelPricingTable).props('imageRateIndependent')).toBe(true)
    expect(wrapper.getComponent(PlazaModelPricingTable).props('imageRateMultiplier')).toBe(imageRate)
  })

  it('shows per-request billing without applying image rates or million-token units', () => {
    const request = model()
    request.pricing = { ...request.pricing!, billing_mode: 'per_request', per_request_price: 0.04 }
    const wrapper = mountCard(entry([{ group: group({ rate_multiplier: 0.5, image_rate_independent: true, image_rate_multiplier: 9 }), model: request }]))
    expect(wrapper.get('[data-price="request"]').text()).toContain('$0.02')
    expect(wrapper.get('[data-price="request"]').text()).toContain('modelPlaza.table.perUnitRequest')
    expect(wrapper.text()).not.toContain('modelPlaza.table.unitPerMillion')
    expect(wrapper.text()).not.toContain('modelPlaza.list.imageRate')
  })

  it.each([undefined, 0, 2])('withholds unverified video prices and rates with user rate %s', async (userRate) => {
    const video = model({ name: 'video-model', official_pricing: null })
    video.pricing = { ...video.pricing!, billing_mode: 'video', per_request_price: 0.37 }
    const wrapper = mountCard(entry([{
      group: group({ rate_multiplier: 9, user_rate_multiplier: userRate }), model: video
    }]))
    expect(wrapper.text()).toContain('modelPlaza.list.videoBilling')
    expect(wrapper.text()).toContain('modelPlaza.list.videoPricingUnavailable')
    expect(wrapper.text()).not.toContain('modelPlaza.table.perRequest')
    expect(wrapper.find('[data-price="request"]').exists()).toBe(false)
    expect(wrapper.find('.catalog-rate').exists()).toBe(false)
    await wrapper.get('.catalog-details').trigger('click')
    expect(document.querySelector('[role="dialog"]')?.textContent).toContain('modelPlaza.list.videoPricingUnavailable')
    expect(wrapper.findComponent(PlazaModelPricingTable).exists()).toBe(false)
  })

  it('keeps same-name models distinguishable by provider and offers copy feedback', async () => {
    const openai = mountCard()
    const anthropic = mountCard({ ...entry([{ group: group({ platform: 'anthropic' }), model: model({ platform: 'anthropic' }) }]), platform: 'anthropic', provider: 'anthropic', key: 'anthropic:shared-model' }, 'list')
    expect(openai.get('h3').text()).toBe(anthropic.get('h3').text())
    expect(openai.get('.catalog-name p').text()).toContain('OpenAI')
    expect(anthropic.get('.catalog-name p').text()).toContain('Anthropic')
    expect(openai.attributes('data-model-key')).not.toBe(anthropic.attributes('data-model-key'))
    expect(anthropic.classes()).toContain('plaza-catalog-card--list')
    await openai.get('.catalog-copy').trigger('click')
    expect(copyName).toHaveBeenCalledWith('shared-model')
    expect(openai.get('.catalog-copy').attributes('aria-label')).toBe('modelPlaza.list.copied')
  })

  it('provides a keyboard-operable group selector and handles unavailable pricing', async () => {
    const wrapper = mountCard(entry([
      { group: group(), model: model() },
      { group: group({ id: 2, name: 'Other', rate_multiplier: 2 }), model: model({ pricing: null }) }
    ]))
    const trigger = wrapper.getComponent(Select).get('button')
    await trigger.trigger('keydown', { key: 'ArrowDown' })
    const dropdown = document.querySelector('[role="listbox"]')!
    expect(dropdown).not.toBeNull()
    dropdown.dispatchEvent(new KeyboardEvent('keydown', { key: 'ArrowDown', bubbles: true }))
    dropdown.dispatchEvent(new KeyboardEvent('keydown', { key: 'Enter', bubbles: true }))
    await nextTick()
    expect(wrapper.text()).toContain('modelPlaza.list.noPricing')
    expect(wrapper.find('[data-price="input"]').exists()).toBe(false)
    await wrapper.setProps({ entry: entry([]) })
    expect(wrapper.get('.catalog-details').attributes('disabled')).toBeDefined()
    expect(wrapper.text()).toContain('modelPlaza.list.noPricing')
  })
})
