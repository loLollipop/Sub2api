import { describe, expect, it } from 'vitest'
import { initializeMixedProviderRates, validMixedProviderRates, visibleMixedProviderRates, mixedModelVendors, type ModelScope } from '../mixed-provider-rates'
import type { Group } from '@/types'

const group: Group = {
  id: 1, platform: 'mixed', rate_multiplier: 9,
  supported_model_vendors: [...mixedModelVendors],
  mixed_provider_rate_multipliers: { deepseek: 0.2, kimi: 0, openai: 3 },
} as Group

describe('Mixed vendor absolute rates', () => {
  it('initializes all vendors independently at 1x and preserves explicit zero', () => {
    const rates = initializeMixedProviderRates(group.mixed_provider_rate_multipliers)
    expect(Object.keys(rates)).toEqual([...mixedModelVendors])
    expect(rates.deepseek).toBe(0.2)
    expect(rates.kimi).toBe(0)
    expect(rates.zhipu).toBe(1)
    expect(validMixedProviderRates(rates)).toBe(true)
    for (const value of [NaN, Infinity, -1, 1000000]) expect(validMixedProviderRates({ ...rates, kimi: value })).toBe(false)
    expect(validMixedProviderRates({})).toBe(false)
  })
  it.each<[ModelScope, string[]]>([
    ['cn', ['deepseek', 'kimi', 'zhipu', 'qwen', 'minimax', 'mimo', 'stepfun', 'doubao', 'yi', 'baidu', 'spark', 'hunyuan']],
    ['openai', ['openai']], ['anthropic', ['anthropic']], ['other', ['gemini', 'grok', 'other']],
  ])('filters the %s category and uses only the actual catalogue', (scope, vendors) => {
    expect(visibleMixedProviderRates(group, scope)?.map(entry => entry.vendor)).toEqual(vendors)
    expect(visibleMixedProviderRates({ ...group, supported_model_vendors: undefined }, scope)).toEqual([])
    expect(visibleMixedProviderRates({ ...group, supported_model_vendors: ['deepseek'] }, scope)?.length).toBe(scope === 'cn' ? 1 : 0)
  })
  it('personal overrides include zero and a value equal to the old scalar', () => {
    for (const override of [0, 9]) expect(visibleMixedProviderRates(group, 'all', override)?.every(entry => entry.rate === override)).toBe(true)
    expect(visibleMixedProviderRates({ ...group, platform: 'openai' }, 'openai')).toBeUndefined()
  })
})
