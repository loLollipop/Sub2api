import { describe, expect, it } from 'vitest'
import { CONCRETE_PLATFORM_OPTIONS, GROUP_PLATFORM_OPTIONS, GROUP_CREATION_PLATFORM_OPTIONS } from '@/constants/platforms'

const concretePlatforms = [
  'anthropic',
  'openai',
  'gemini',
  'antigravity',
  'grok',
  'kimi',
  'zhipu',
  'deepseek',
  'minimax',
  'mixed'
]

describe('platform option catalogs', () => {
  it('consolidates new domestic groups while retaining legacy editing and filters', () => {
    expect(GROUP_CREATION_PLATFORM_OPTIONS.map(option => option.value)).toEqual([
      'anthropic', 'openai', 'gemini', 'antigravity', 'grok', 'mixed', 'composite'
    ])
    for (const platform of ['kimi', 'zhipu', 'deepseek', 'minimax']) {
      expect(GROUP_PLATFORM_OPTIONS.map(option => option.value)).toContain(platform)
    }
  })

  it('exposes every concrete account platform', () => {
    expect(CONCRETE_PLATFORM_OPTIONS.map((option) => option.value)).toEqual(concretePlatforms)
  })

  it('adds composite for group-backed filters', () => {
    expect(GROUP_PLATFORM_OPTIONS.map((option) => option.value)).toEqual([
      ...concretePlatforms,
      'composite'
    ])
  })
})
