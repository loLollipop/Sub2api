import { beforeEach, describe, expect, it, vi } from 'vitest'
import { FeatureFlags, isFeatureFlagEnabled, isPurchaseEnabled, makeSidebarFlag } from '../featureFlags'

const appStore = vi.hoisted(() => ({ cachedPublicSettings: null as null | {
  payment_enabled?: boolean
  purchase_subscription_enabled?: boolean
} }))
vi.mock('@/stores/app', () => ({ useAppStore: () => appStore }))
beforeEach(() => { appStore.cachedPublicSettings = null })

describe('purchase feature flags', () => {
  it.each([
    [true, false, true], [false, true, true], [true, true, true], [false, false, false],
  ])('combines internal %s and external %s to %s for purchases', (payment, external, expected) => {
    appStore.cachedPublicSettings = { payment_enabled: payment, purchase_subscription_enabled: external }
    expect(isPurchaseEnabled()).toBe(expected)
    expect(makeSidebarFlag(FeatureFlags.payment)()).toBe(payment)
  })

  it('preserves the internal payment opt-out fallback when settings are unknown', () => {
    expect(isPurchaseEnabled()).toBe(true)
    expect(isFeatureFlagEnabled(FeatureFlags.payment)).toBe(true)
  })
})
