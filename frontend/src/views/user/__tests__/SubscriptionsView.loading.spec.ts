import { afterEach, describe, expect, it, vi } from 'vitest'
import { enableAutoUnmount, flushPromises, mount } from '@vue/test-utils'
import SubscriptionsView from '../SubscriptionsView.vue'

const { getMySubscriptions } = vi.hoisted(() => ({ getMySubscriptions: vi.fn() }))
vi.mock('@/api/subscriptions', () => ({ default: { getMySubscriptions } }))
vi.mock('vue-i18n', async importOriginal => ({ ...await importOriginal<typeof import('vue-i18n')>(), useI18n: () => ({ t: (key: string) => key }) }))
vi.mock('vue-router', () => ({ useRouter: () => ({ push: vi.fn() }) }))
vi.mock('@/stores/app', () => ({ useAppStore: () => ({ cachedPublicSettings: {} }) }))
enableAutoUnmount(afterEach)

describe('subscription loading states', () => {
  it('distinguishes a load failure from empty history and retries in place', async () => {
    getMySubscriptions.mockRejectedValueOnce(new Error('offline')).mockResolvedValueOnce([])
    const wrapper = mount(SubscriptionsView, { global: { stubs: { AppLayout: { template: '<div><slot /></div>' }, Icon: true } } })
    await flushPromises()
    expect(wrapper.get('[role="alert"]').text()).toContain('userSubscriptions.failedToLoad')
    expect(wrapper.text()).not.toContain('userSubscriptions.noActiveSubscriptions')
    await wrapper.get('[role="alert"] button').trigger('click')
    await flushPromises()
    expect(getMySubscriptions).toHaveBeenCalledTimes(2)
    expect(wrapper.find('[role="alert"]').exists()).toBe(false)
    expect(wrapper.text()).toContain('userSubscriptions.noActiveSubscriptions')
  })
})
