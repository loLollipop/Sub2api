import { beforeEach, describe, expect, it, vi } from 'vitest'
import { flushPromises, shallowMount } from '@vue/test-utils'
import { reactive } from 'vue'
import PaymentView from '../PaymentView.vue'
import ExternalShopRecharge from '@/components/payment/ExternalShopRecharge.vue'
const state = vi.hoisted(() => ({
  publicSettingsLoaded: true,
  cachedPublicSettings: { purchase_subscription_enabled: true, purchase_subscription_products: { '10': 'https://shop.example.test/item/10' } },
  fetchPublicSettings: vi.fn(),
}))
let app: typeof state
vi.mock('@/stores/app', () => ({ useAppStore: () => app }))
vi.mock('@/stores/auth', () => ({ useAuthStore: () => ({ user: { username: 'demo-user', balance: 12.5 } }) }))
vi.mock('vue-i18n', async () => ({ ...await vi.importActual<typeof import('vue-i18n')>('vue-i18n'), useI18n: () => ({ t: (key: string) => key }) }))
const mountView = () => shallowMount(PaymentView, { global: { stubs: { AppLayout: { template: '<main><slot /></main>' } } } })
beforeEach(() => {
  state.fetchPublicSettings.mockReset()
  app = reactive({ ...state, publicSettingsLoaded: true,
    cachedPublicSettings: { purchase_subscription_enabled: true, purchase_subscription_products: { '10': 'https://shop.example.test/item/10' } } })
})
describe('external shop purchase page', () => {
  it('renders the configured shop with the current account and balance', async () => {
    const wrapper = mountView()
    await flushPromises()
    expect(wrapper.getComponent(ExternalShopRecharge).props()).toEqual({ products: app.cachedPublicSettings.purchase_subscription_products, username: 'demo-user', balance: 12.5 })
    expect(app.fetchPublicSettings).not.toHaveBeenCalled()
    expect(wrapper.find('iframe').exists()).toBe(false)
  })
  it('shows unavailable when the shop is disabled', async () => {
    app.cachedPublicSettings.purchase_subscription_enabled = false
    const wrapper = mountView()
    await flushPromises()
    expect(wrapper.text()).toContain('purchase.notEnabledTitle')
    expect(wrapper.findComponent(ExternalShopRecharge).exists()).toBe(false)
  })
  it('waits for settings before showing the shop', async () => {
    app.publicSettingsLoaded = false
    let finish!: () => void
    app.fetchPublicSettings.mockImplementation(() => new Promise<void>(resolve => { finish = () => { app.publicSettingsLoaded = true; resolve() } }))
    const wrapper = mountView()
    expect(wrapper.text()).toContain('common.loading')
    expect(wrapper.findComponent(ExternalShopRecharge).exists()).toBe(false)
    finish()
    await flushPromises()
    expect(wrapper.findComponent(ExternalShopRecharge).exists()).toBe(true)
  })
  it.each(['null', 'rejected'])('retries after a %s settings failure and recovers', async failure => {
    app.publicSettingsLoaded = false
    if (failure === 'null') app.fetchPublicSettings.mockResolvedValueOnce(null)
    else app.fetchPublicSettings.mockRejectedValueOnce(new Error('offline'))
    const wrapper = mountView()
    await flushPromises()
    expect(wrapper.get('[role="alert"]').text()).toContain('purchase.loadFailed')
    expect(wrapper.findComponent(ExternalShopRecharge).exists()).toBe(false)
    app.fetchPublicSettings.mockImplementationOnce(async () => { app.publicSettingsLoaded = true })
    await wrapper.get('button').trigger('click')
    await flushPromises()
    expect(app.fetchPublicSettings).toHaveBeenLastCalledWith(true)
    expect(wrapper.findComponent(ExternalShopRecharge).exists()).toBe(true)
  })
})
