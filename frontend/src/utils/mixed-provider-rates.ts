import type { Group } from '@/types'
export type ModelScope = 'all' | 'openai' | 'anthropic' | 'cn' | 'other'

export const mixedModelVendors = ['deepseek', 'kimi', 'zhipu', 'qwen', 'minimax', 'mimo', 'stepfun', 'doubao', 'yi', 'baidu', 'spark', 'hunyuan', 'openai', 'anthropic', 'gemini', 'grok', 'other'] as const
export type MixedModelVendor = typeof mixedModelVendors[number]
export type MixedProviderRates = Partial<Record<MixedModelVendor, number>>
export interface VendorRate { vendor: MixedModelVendor; rate: number }

const labels: Record<MixedModelVendor, string> = {
  deepseek: 'DeepSeek', kimi: 'Kimi', zhipu: 'GLM', qwen: 'Qwen', minimax: 'MiniMax', mimo: 'MiMo', stepfun: 'Step',
  doubao: 'Doubao', yi: 'Yi', baidu: 'ERNIE', spark: 'Spark', hunyuan: 'Hunyuan',
  openai: 'OpenAI', anthropic: 'Claude', gemini: 'Gemini', grok: 'Grok', other: '',
}
export const modelVendorLabel = (vendor: MixedModelVendor, other: string) => labels[vendor] || other
export const modelVendorScope = (vendor: MixedModelVendor): ModelScope => {
  if (vendor === 'openai' || vendor === 'anthropic') return vendor
  return vendor === 'gemini' || vendor === 'grok' || vendor === 'other' ? 'other' : 'cn'
}

// Each vendor is independent. Missing fields start at the standard 1x rate.
export function initializeMixedProviderRates(rates?: MixedProviderRates | null): Record<MixedModelVendor, number> {
  return Object.fromEntries(mixedModelVendors.map(vendor => [vendor, rates?.[vendor] ?? 1])) as Record<MixedModelVendor, number>
}

export function validMixedProviderRates(rates: MixedProviderRates): boolean {
  return mixedModelVendors.every(vendor => typeof rates[vendor] === 'number' && Number.isFinite(rates[vendor]) && rates[vendor]! >= 0 && rates[vendor]! <= 999999.9999)
}

// undefined preserves ordinary group rendering. [] means Mixed with no confirmed catalogue.
export function visibleMixedProviderRates(group: Pick<Group, 'platform' | 'rate_multiplier' | 'mixed_provider_rate_multipliers' | 'supported_model_vendors'> | undefined, scope: ModelScope = 'all', userRate?: number | null): VendorRate[] | undefined {
  if (group?.platform !== 'mixed') return undefined
  return mixedModelVendors.filter(vendor => group.supported_model_vendors?.includes(vendor) && (scope === 'all' || modelVendorScope(vendor) === scope))
    .map(vendor => ({ vendor, rate: userRate ?? group.mixed_provider_rate_multipliers?.[vendor] ?? 1 }))
}
