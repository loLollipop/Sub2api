import { afterEach, describe, expect, it } from 'vitest'
import { CONCRETE_PLATFORM_OPTIONS, GROUP_PLATFORM_OPTIONS, GROUP_CREATION_PLATFORM_OPTIONS } from '@/constants/platforms'
import { BUILTIN_PLATFORM_CATALOG, resetPlatformCatalog, setPlatformCatalog } from '@/constants/platformCatalog'

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
  'command_code',
  'cline',
  'typesafe'
]

describe('platform option catalogs', () => {
  afterEach(resetPlatformCatalog)

  it('updates selectors when the registry changes and exposes native providers', () => {
    setPlatformCatalog({
      ...BUILTIN_PLATFORM_CATALOG,
      platforms: [...BUILTIN_PLATFORM_CATALOG.platforms, {
        id: 'new_provider', display_name: 'New Provider', gateway: 'openai', cn_provider: false
      }]
    })
    for (const options of [CONCRETE_PLATFORM_OPTIONS, GROUP_PLATFORM_OPTIONS, GROUP_CREATION_PLATFORM_OPTIONS]) {
      expect(options.some(option => option.value === 'new_provider')).toBe(true)
    }
    expect(GROUP_CREATION_PLATFORM_OPTIONS.some(option => option.value === 'kimi')).toBe(true)
  })

  it('creates each native domestic group independently', () => {
    expect(GROUP_CREATION_PLATFORM_OPTIONS.map(option => option.value)).toEqual([
      'anthropic', 'openai', 'gemini', 'antigravity', 'grok', 'kimi', 'zhipu', 'deepseek', 'minimax', 'command_code', 'cline', 'typesafe', 'composite'
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
