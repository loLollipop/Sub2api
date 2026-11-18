import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { flushPromises, mount, type VueWrapper } from '@vue/test-utils'
import { reactive } from 'vue'
import App from '../App.vue'

const context = vi.hoisted(() => ({
  auth: {} as { isAuthenticated: boolean; isAdmin: boolean },
  summary: vi.fn(), popups: vi.fn(), list: vi.fn(), reset: vi.fn(), removeRouteHook: vi.fn(),
  afterEach: vi.fn(),
}))
vi.mock('@/stores', () => ({
  useAuthStore: () => context.auth,
  useAnnouncementStore: () => ({ fetchSummary: context.summary, fetchPopups: context.popups, fetchAnnouncements: context.list, reset: context.reset }),
  useSubscriptionStore: () => ({ fetchActiveSubscriptions: async () => {}, startPolling: vi.fn(), clear: vi.fn() }),
  useAdminComplianceStore: () => ({ reset: vi.fn(), fetchStatus: async () => {}, requireAcknowledgement: vi.fn() }),
  useAdminSettingsStore: () => ({ customMenuItems: [] }),
  useAppStore: () => ({ siteName: 'Test', siteLogo: '', cachedPublicSettings: null, fetchPublicSettings: async () => {} }),
}))
vi.mock('vue-router', () => ({
  RouterView: { template: '<div />' },
  useRouter: () => ({ afterEach: context.afterEach, replace: vi.fn() }),
  useRoute: () => ({ fullPath: '/', path: '/', meta: {} }),
}))
vi.mock('@/router/title', () => ({ resolveRouteDocumentTitle: () => 'Test' }))
vi.mock('@/api/setup', () => ({ getSetupStatus: async () => ({ needs_setup: false }) }))
vi.mock('@/utils/branding', () => ({ updateFavicon: vi.fn() }))
vi.mock('@/components/common/Toast.vue', () => ({ default: { template: '<div />' } }))
vi.mock('@/components/common/NavigationProgress.vue', () => ({ default: { template: '<div />' } }))
vi.mock('@/components/common/AnnouncementPopup.vue', () => ({ default: { template: '<div />' } }))
vi.mock('@/components/admin/AdminComplianceDialog.vue', () => ({ default: { template: '<div />' } }))

let wrapper: VueWrapper | undefined
beforeEach(() => {
  vi.useFakeTimers({ toFake: ['setTimeout', 'clearTimeout', 'setInterval', 'clearInterval', 'Date'] })
  vi.resetAllMocks()
  context.auth = reactive({ isAuthenticated: false, isAdmin: false })
  context.afterEach.mockReturnValue(context.removeRouteHook)
})
afterEach(() => {
  wrapper?.unmount()
  wrapper = undefined
  vi.useRealTimers()
})

describe('application announcement bootstrap', () => {
  it('restores counts and bounded popups; routes and focus refresh only counts', async () => {
    context.auth.isAuthenticated = true
    wrapper = mount(App)
    await flushPromises()
    expect(context.summary).toHaveBeenCalledTimes(1)
    expect(context.popups).toHaveBeenCalledTimes(1)
    context.afterEach.mock.calls[0][0]()
    window.dispatchEvent(new Event('focus'))
    document.dispatchEvent(new Event('visibilitychange'))
    expect(context.summary).toHaveBeenCalledTimes(4)
    expect(context.popups).toHaveBeenCalledTimes(1)
    expect(context.list).not.toHaveBeenCalled()
  })

  it('cancels the delayed login request on logout and starts a fresh timer for the next session', async () => {
    wrapper = mount(App)
    context.auth.isAuthenticated = true
    await flushPromises()
    await vi.advanceTimersByTimeAsync(2000)
    context.auth.isAuthenticated = false
    await flushPromises()
    await vi.advanceTimersByTimeAsync(2000)
    expect(context.summary).not.toHaveBeenCalled()
    expect(context.popups).not.toHaveBeenCalled()
    context.auth.isAuthenticated = true
    await flushPromises()
    await vi.advanceTimersByTimeAsync(2999)
    expect(context.summary).not.toHaveBeenCalled()
    await vi.advanceTimersByTimeAsync(1)
    expect(context.summary).toHaveBeenCalledTimes(1)
    expect(context.summary).toHaveBeenCalledWith(true)
    expect(context.popups).toHaveBeenCalledTimes(1)
    expect(context.list).not.toHaveBeenCalled()
  })

  it('removes pending timers, route hooks and focus listeners on unmount', async () => {
    wrapper = mount(App)
    context.auth.isAuthenticated = true
    await flushPromises()
    wrapper.unmount()
    wrapper = undefined
    await vi.advanceTimersByTimeAsync(3000)
    window.dispatchEvent(new Event('focus'))
    expect(context.summary).not.toHaveBeenCalled()
    expect(context.popups).not.toHaveBeenCalled()
    expect(context.removeRouteHook).toHaveBeenCalledTimes(1)
  })
})
