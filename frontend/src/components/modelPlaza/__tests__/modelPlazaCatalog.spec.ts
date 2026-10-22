import { describe, expect, it } from 'vitest'
import type { ModelPlazaGroup, PlazaModel } from '@/api/modelPlaza'
import { buildModelCatalog } from '../modelPlazaCatalog'

function group(id: number, rate: number, inputPrice: number): ModelPlazaGroup {
  const model: PlazaModel = {
    name: 'deepseek-chat', platform: 'openai', official_pricing: null,
    pricing: {
      billing_mode: 'token', input_price: inputPrice, output_price: null,
      cache_read_price: null, cache_write_price: null, per_request_price: null,
      image_input_price: null, image_output_price: null, intervals: []
    }
  }
  return {
    id, name: `Group ${id}`, description: '', platform: 'openai',
    rate_multiplier: rate, subscription_type: 'standard', is_exclusive: false,
    peak_rate_enabled: false, peak_start: '', peak_end: '', peak_rate_multiplier: 1,
    image_rate_independent: false, image_rate_multiplier: 1,
    long_context_pricing_enabled: true, models: [model]
  }
}

describe('buildModelCatalog', () => {
  it('keeps prices, model identity and a zero custom rate isolated across identical protocol/name pairs', () => {
    const standard = group(1, 2, 3e-6)
    const exclusive = { ...group(2, 9, 7e-6), user_rate_multiplier: 0, is_exclusive: true }
    const entries = buildModelCatalog([standard, exclusive])
    expect(entries).toHaveLength(2)
    expect(entries[0].key).not.toBe(entries[1].key)
    expect(entries.map(entry => entry.offers.length)).toEqual([1, 1])
    expect(entries[0].offers[0].group).toBe(standard)
    expect(entries[1].offers[0].group).toBe(exclusive)
    expect(entries[0].offers[0].model).toBe(standard.models[0])
    expect(entries[1].offers[0].model).toBe(exclusive.models[0])
    expect(entries.map(entry => entry.offers[0].model.pricing?.input_price)).toEqual([3e-6, 7e-6])
    expect(entries[1].offers[0].group.user_rate_multiplier).toBe(0)
    expect(entries.map(entry => entry.provider)).toEqual(['deepseek', 'deepseek'])
    expect(entries.map(entry => entry.platform)).toEqual(['openai', 'openai'])
  })

  it('rebuilds refreshed pricing from current group/model objects without retaining old offers', () => {
    const original = group(1, 1, 3e-6)
    const refreshed = group(1, 4, 8e-6)
    const before = buildModelCatalog([original])[0]
    const after = buildModelCatalog([refreshed])[0]
    expect(after.key).toBe(before.key)
    expect(after.offers).toHaveLength(1)
    expect(after.offers[0].model).toBe(refreshed.models[0])
    expect(after.offers[0].group.rate_multiplier).toBe(4)
    expect(after.offers[0].model.pricing?.input_price).toBe(8e-6)
    expect(before.offers[0].model.pricing?.input_price).toBe(3e-6)
  })
})
