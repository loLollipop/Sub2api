import { beforeEach, describe, expect, it, vi } from 'vitest'
import { isPurchaseEnabled } from '../featureFlags'
const appStore = vi.hoisted(() => ({ cachedPublicSettings: null as null | { purchase_subscription_enabled?: boolean } }))
vi.mock('@/stores/app', () => ({ useAppStore: () => appStore }))
beforeEach(() => { appStore.cachedPublicSettings = null })
describe('external shop feature flag', () => {
  it.each([true, false])('uses the external shop setting %s', enabled => {
    appStore.cachedPublicSettings = { purchase_subscription_enabled: enabled }
    expect(isPurchaseEnabled()).toBe(enabled)
  })
  it('hides purchase until the external shop is explicitly enabled', () => {
    expect(isPurchaseEnabled()).toBe(false)
    appStore.cachedPublicSettings = {}
    expect(isPurchaseEnabled()).toBe(false)
  })
})
