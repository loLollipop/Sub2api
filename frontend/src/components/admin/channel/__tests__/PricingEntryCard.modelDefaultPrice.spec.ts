import { enableAutoUnmount, flushPromises, shallowMount } from '@vue/test-utils'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import PricingEntryCard from '../PricingEntryCard.vue'
import type { PricingFormEntry } from '../types'
import groupsAPI from '@/api/admin/groups'

vi.mock('@/api/admin/groups', () => ({
  default: { getModelDefaultPricing: vi.fn() },
}))

vi.mock('vue-i18n', async importOriginal => ({
  ...await importOriginal<typeof import('vue-i18n')>(),
  useI18n: () => ({ t: (key: string) => key }),
}))

const getModelDefaultPricing = vi.mocked(groupsAPI.getModelDefaultPricing)
enableAutoUnmount(afterEach)

function createEntry(models: string[] = []): PricingFormEntry {
  return {
    models,
    billing_mode: 'token',
    input_price: null,
    output_price: null,
    cache_write_price: null,
    cache_read_price: null,
    fast_multiplier: null,
    flex_multiplier: null,
    max_reasoning_effort_multiplier: null,
    image_input_price: null,
    image_output_price: null,
    per_request_price: null,
    intervals: [],
    time_pricing: {
      timezone: 'Asia/Shanghai',
      weekdays_only: false,
      periods: [],
    },
  }
}

function addModels(entry: PricingFormEntry, models: string[]) {
  const wrapper = shallowMount(PricingEntryCard, { props: { entry } })
  wrapper.findComponent({ name: 'ModelTagInput' }).vm.$emit('update:models', models)
  return wrapper
}

const emittedEntries = (wrapper: ReturnType<typeof shallowMount>) =>
  (wrapper.emitted('update') ?? []).map(e => e[0] as PricingFormEntry)

describe('PricingEntryCard default price auto-fill', () => {
  beforeEach(() => {
    vi.clearAllMocks()
    getModelDefaultPricing.mockImplementation(async model => model === 'claude-opus-4-7'
      ? { found: true, input_price: 5e-6, output_price: 25e-6 }
      : { found: true, input_price: 3e-6, output_price: 15e-6 })
  })

  it('auto-fills the default price when a model is added to an empty entry', async () => {
    const wrapper = addModels(createEntry(), ['claude-opus-4-8'])
    await flushPromises()

    expect(getModelDefaultPricing).toHaveBeenCalledTimes(1)
    expect(getModelDefaultPricing).toHaveBeenCalledWith('claude-opus-4-8')
    expect(emittedEntries(wrapper).at(-1)).toMatchObject({
      models: ['claude-opus-4-8'],
      input_price: 3,
      output_price: 15,
    })
  })

  it('does not overwrite prices when appending a model to an entry that already has one', async () => {
    const entry = createEntry(['claude-opus-4-7'])
    const wrapper = addModels(entry, ['claude-opus-4-7', 'claude-opus-4-8'])
    await flushPromises()

    expect(getModelDefaultPricing).not.toHaveBeenCalled()
    expect(emittedEntries(wrapper).at(-1)).toMatchObject({
      models: ['claude-opus-4-7', 'claude-opus-4-8'],
    })
    for (const emitted of emittedEntries(wrapper)) {
      expect(emitted.input_price).toBeNull()
      expect(emitted.output_price).toBeNull()
      expect(emitted.cache_write_price).toBeNull()
      expect(emitted.cache_read_price).toBeNull()
    }
  })

  it('does not auto-fill a shared price when several models are added to an empty entry at once', async () => {
    const wrapper = addModels(createEntry(), ['claude-opus-4-7', 'claude-opus-4-8'])
    await flushPromises()

    expect(getModelDefaultPricing).not.toHaveBeenCalled()
    expect(emittedEntries(wrapper).at(-1)).toMatchObject({
      models: ['claude-opus-4-7', 'claude-opus-4-8'],
      input_price: null,
      output_price: null,
    })
  })

  it('keeps manual prices untouched when a model is appended', async () => {
    const entry = { ...createEntry(['claude-opus-4-7']), input_price: '1.5' }
    const wrapper = addModels(entry, ['claude-opus-4-7', 'claude-opus-4-8'])
    await flushPromises()

    expect(getModelDefaultPricing).not.toHaveBeenCalled()
    for (const emitted of emittedEntries(wrapper)) {
      expect(emitted.input_price).toBe('1.5')
    }
  })

  it.each(['image_input_price', 'image_output_price', 'per_request_price'] as const)('preserves an existing %s when selecting the first model', async field => {
    const wrapper = addModels({ ...createEntry(), [field]: '7' }, ['claude-opus-4-8'])
    await flushPromises()
    expect(getModelDefaultPricing).not.toHaveBeenCalled()
    expect(emittedEntries(wrapper).at(-1)?.[field]).toBe('7')
  })

  it.each([null, 1.5])('preserves a manual reasoning multiplier with default response %s', async multiplier => {
    getModelDefaultPricing.mockResolvedValue({ found: true, max_reasoning_effort_multiplier: multiplier })
    const wrapper = addModels({ ...createEntry(), max_reasoning_effort_multiplier: '7' }, ['claude-opus-4-8'])
    await flushPromises()
    expect(getModelDefaultPricing).not.toHaveBeenCalled()
    expect(emittedEntries(wrapper).at(-1)?.max_reasoning_effort_multiplier).toBe('7')
  })

  it.each([null, 1.5])('auto-fills a default reasoning multiplier %s when the entry is empty', async multiplier => {
    getModelDefaultPricing.mockResolvedValue({ found: true, max_reasoning_effort_multiplier: multiplier })
    const wrapper = addModels(createEntry(), ['claude-opus-4-8'])
    await flushPromises()
    expect(getModelDefaultPricing).toHaveBeenCalledTimes(1)
    expect(emittedEntries(wrapper).at(-1)?.max_reasoning_effort_multiplier).toBe(multiplier)
  })

  it.each(['price', 'clear-price', 'remove-model', 'replace-model', 'remove-entry', 'unmount'])('discards delayed prices after %s', async action => {
    let finish!: (value: { found: boolean; input_price: number; output_price: number }) => void
    getModelDefaultPricing.mockReturnValueOnce(new Promise(resolve => { finish = resolve }))
    const wrapper = addModels(createEntry(), ['claude-opus-4-8'])
    await wrapper.setProps({ entry: emittedEntries(wrapper)[0] })
    if (action === 'price' || action === 'clear-price') {
      await wrapper.findAll('input[type="number"]')[0].setValue('99')
      await wrapper.setProps({ entry: emittedEntries(wrapper).at(-1)! })
      if (action === 'clear-price') {
        await wrapper.findAll('input[type="number"]')[0].setValue('')
        await wrapper.setProps({ entry: emittedEntries(wrapper).at(-1)! })
      }
    } else if (action === 'remove-model' || action === 'replace-model') {
      wrapper.findComponent({ name: 'ModelTagInput' }).vm.$emit('update:models', action === 'remove-model' ? [] : ['claude-opus-4-7'])
      await wrapper.setProps({ entry: emittedEntries(wrapper).at(-1)! })
    } else if (action === 'remove-entry') {
      await wrapper.get('button').trigger('click')
    } else wrapper.unmount()
    const updateCount = emittedEntries(wrapper).length
    finish({ found: true, input_price: 3e-6, output_price: 15e-6 })
    await flushPromises()
    expect(emittedEntries(wrapper)).toHaveLength(updateCount)
    expect(getModelDefaultPricing).toHaveBeenCalledTimes(1)
    wrapper.unmount()
  })

  it('uses the newly selected model price when an older empty-entry lookup finishes last', async () => {
    let finishOld!: (value: { found: boolean; input_price: number; output_price: number }) => void
    getModelDefaultPricing.mockReturnValueOnce(new Promise(resolve => { finishOld = resolve }))
    const wrapper = addModels(createEntry(), ['claude-opus-4-8'])
    await wrapper.setProps({ entry: emittedEntries(wrapper)[0] })
    wrapper.findComponent({ name: 'ModelTagInput' }).vm.$emit('update:models', [])
    await wrapper.setProps({ entry: emittedEntries(wrapper).at(-1)! })
    wrapper.findComponent({ name: 'ModelTagInput' }).vm.$emit('update:models', ['claude-opus-4-7'])
    await wrapper.setProps({ entry: emittedEntries(wrapper).at(-1)! })
    await flushPromises()
    expect(emittedEntries(wrapper).at(-1)).toMatchObject({ models: ['claude-opus-4-7'], input_price: 5, output_price: 25 })
    const count = emittedEntries(wrapper).length
    finishOld({ found: true, input_price: 3e-6, output_price: 15e-6 })
    await flushPromises()
    expect(emittedEntries(wrapper)).toHaveLength(count)
    wrapper.unmount()
  })
})
