import { mount, type VueWrapper } from '@vue/test-utils'
import { reactive, ref } from 'vue'
import { createMemoryHistory, createRouter } from 'vue-router'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import type { PublicSettings } from '@/types'
import AppSidebar from '../AppSidebar.vue'

vi.mock('vue-i18n', async importOriginal => ({
  ...await importOriginal<typeof import('vue-i18n')>(),
  useI18n: () => ({ t: (key: string) => key }),
}))
vi.mock('@/stores', () => ({
  useAppStore: () => app,
  useAuthStore: () => auth,
  useAdminSettingsStore: () => admin,
  useOnboardingStore: () => onboarding,
}))
vi.mock('@/stores/app', () => ({ useAppStore: () => app }))
vi.mock('@/composables/useBatchImageAccess', () => ({
  useBatchImageAccess: () => ({ canUseBatchImage: ref(false), refreshBatchImageAccess: vi.fn() }),
}))

const app = reactive({
  sidebarCollapsed: false, mobileOpen: false, sidebarScrollTop: 0,
  backendModeEnabled: false, siteName: 'Local console', siteLogo: '', siteVersion: '2.0.48',
  publicSettingsLoaded: true, cachedPublicSettings: {} as Partial<PublicSettings>,
  setMobileOpen: (value: boolean) => { app.mobileOpen = value },
  toggleSidebar: vi.fn(),
})
const auth = reactive({ isAdmin: true, isSimpleMode: false })
const admin = reactive({
  opsMonitoringEnabled: true, paymentEnabled: true,
  customMenuItems: [] as NonNullable<PublicSettings['custom_menu_items']>, fetch: vi.fn(),
})
const onboarding = { isCurrentStep: () => false, nextStep: vi.fn() }
let wrapper: VueWrapper | undefined

async function render(horizontal = true) {
  const router = createRouter({ history: createMemoryHistory(), routes: [
    { path: '/:pathMatch(.*)*', component: { template: '<div />' } },
  ] })
  await router.push(auth.isAdmin ? '/admin/accounts' : '/dashboard')
  await router.isReady()
  wrapper = mount(AppSidebar, {
    props: { horizontal }, attachTo: document.body,
    global: { plugins: [router], stubs: { VersionBadge: true, LocaleSwitcher: true } },
  })
  return { wrapper, router }
}

beforeEach(() => {
  vi.clearAllMocks()
  Object.assign(app, { sidebarCollapsed: false, mobileOpen: false, backendModeEnabled: false, sidebarScrollTop: 0 })
  app.cachedPublicSettings = {
    channel_monitor_enabled: true, available_channels_enabled: true,
    payment_enabled: true, risk_control_enabled: true, affiliate_enabled: true,
    plugin_management_enabled: true, custom_menu_items: [],
  }
  auth.isAdmin = true
  auth.isSimpleMode = false
  admin.customMenuItems = []
  admin.paymentEnabled = true
  admin.opsMonitoringEnabled = true
  localStorage.setItem('theme', 'light')
  document.documentElement.classList.remove('dark')
})
afterEach(() => { wrapper?.unmount(); wrapper = undefined })

describe('frontend-local navigation integration', () => {
  it('keeps the existing sidebar presentation as the default-compatible option', async () => {
    app.sidebarCollapsed = true
    const { wrapper } = await render(false)
    expect(wrapper.find('.local-navigation').exists()).toBe(false)
    expect(wrapper.find('aside').classes()).toContain('w-[72px]')
    expect(wrapper.find('aside').classes()).not.toContain('local-mobile-sidebar')
  })

  it('pins administrator core pages and retains all additional child routes in More', async () => {
    const { wrapper } = await render()
    const top = wrapper.find('.local-navigation-links')
    for (const path of ['/admin/dashboard', '/admin/accounts', '/admin/users', '/admin/groups', '/admin/usage']) {
      expect(top.find(`a[href="${path}"]`).exists()).toBe(true)
    }
    await wrapper.get('[aria-controls="local-nav-more"]').trigger('click')
    const more = wrapper.get('#local-nav-more')
    for (const path of ['/admin/settings', '/admin/plugins', '/admin/tickets', '/admin/prompt-audit',
      '/admin/orders/dashboard', '/admin/orders/plans', '/admin/channels/pricing', '/admin/affiliates/transfers']) {
      expect(more.find(`a[href="${path}"]`).exists(), path).toBe(true)
    }
    expect(more.find('a[href="/admin/security-audit"]').exists()).toBe(false)
    expect(top.get('a[href="/admin/accounts"]').attributes('aria-current')).toBe('page')
  })

  it('retains the administrator personal API keys and payment routes', async () => {
    const { wrapper } = await render()
    await wrapper.get('[aria-controls="local-nav-account"]').trigger('click')
    for (const path of ['/keys', '/infinite-canvas', '/purchase', '/orders', '/tickets', '/profile']) {
      expect(wrapper.find(`#local-nav-account a[href="${path}"]`).exists(), path).toBe(true)
    }
  })

  it('retains all user features rather than replacing them with reference-fork routes', async () => {
    auth.isAdmin = false
    const { wrapper } = await render()
    expect(wrapper.find('[aria-controls="local-nav-account"]').exists()).toBe(false)
    await wrapper.get('[aria-controls="local-nav-more"]').trigger('click')
    for (const path of ['/infinite-canvas', '/purchase', '/orders', '/tickets', '/affiliate', '/available-channels', '/profile']) {
      expect(wrapper.find(`#local-nav-more a[href="${path}"]`).exists(), path).toBe(true)
    }
    expect(wrapper.find('.local-navigation a[href^="/admin/"]').exists()).toBe(false)
  })

  it('keeps simple-mode and backend-only visibility rules', async () => {
    auth.isSimpleMode = true
    const { wrapper } = await render()
    expect(wrapper.find('.local-navigation a[href="/admin/users"]').exists()).toBe(false)
    expect(wrapper.find('[aria-controls="local-nav-account"]').exists()).toBe(false)
    await wrapper.get('[aria-controls="local-nav-more"]').trigger('click')
    expect(wrapper.find('#local-nav-more a[href="/keys"]').exists()).toBe(true)
    expect(wrapper.find('#local-nav-more a[href="/admin/subscriptions"]').exists()).toBe(false)
    auth.isAdmin = false
    app.backendModeEnabled = true
    await wrapper.vm.$nextTick()
    expect(wrapper.find('.local-navigation a').exists()).toBe(false)
  })

  it('does not reintroduce disabled features in the desktop overflow menu', async () => {
    app.cachedPublicSettings.plugin_management_enabled = false
    app.cachedPublicSettings.risk_control_enabled = false
    const { wrapper } = await render()
    await wrapper.get('[aria-controls="local-nav-more"]').trigger('click')
    expect(wrapper.find('#local-nav-more a[href="/admin/plugins"]').exists()).toBe(false)
    expect(wrapper.find('#local-nav-more a[href="/admin/prompt-audit"]').exists()).toBe(false)
  })

  it('retains custom navigation entries', async () => {
    admin.customMenuItems = [{ id: 'help', label: 'Custom help', icon_svg: '', url: 'https://example.com', visibility: 'admin', sort_order: 0 }]
    const { wrapper } = await render()
    await wrapper.get('[aria-controls="local-nav-more"]').trigger('click')
    expect(wrapper.get('#local-nav-more a[href="/custom/help"]').text()).toBe('Custom help')
  })

  it('closes desktop menus on Escape, outside pointer and route changes', async () => {
    const { wrapper, router } = await render()
    const trigger = wrapper.get('[aria-controls="local-nav-more"]')
    await trigger.trigger('click')
    document.dispatchEvent(new KeyboardEvent('keydown', { key: 'Escape', bubbles: true }))
    await wrapper.vm.$nextTick()
    expect(trigger.attributes('aria-expanded')).toBe('false')
    expect(document.activeElement).toBe(trigger.element)
    await trigger.trigger('click')
    document.body.dispatchEvent(new Event('pointerdown', { bubbles: true }))
    await wrapper.vm.$nextTick()
    expect(wrapper.find('#local-nav-more').exists()).toBe(false)
    await trigger.trigger('click')
    await router.push('/admin/users')
    await wrapper.vm.$nextTick()
    expect(wrapper.find('#local-nav-more').exists()).toBe(false)
  })

  it('keeps the mobile drawer expanded despite a saved collapsed desktop sidebar', async () => {
    app.sidebarCollapsed = true
    app.mobileOpen = true
    const { wrapper } = await render()
    expect(wrapper.get('aside').classes()).toContain('w-64')
    expect(wrapper.find('.sidebar-label-collapsed').exists()).toBe(false)
    document.dispatchEvent(new KeyboardEvent('keydown', { key: 'Escape', bubbles: true }))
    await wrapper.vm.$nextTick()
    expect(app.mobileOpen).toBe(false)
    expect(wrapper.get('aside').attributes('aria-hidden')).toBe('true')
  })

  it('tracks theme changes made by the desktop header before opening the mobile drawer', async () => {
    const { wrapper } = await render()
    document.documentElement.classList.add('dark')
    await new Promise(resolve => setTimeout(resolve, 0))
    await wrapper.vm.$nextTick()
    const toggle = wrapper.get('[data-test="theme-toggle"]')
    expect(toggle.text()).toBe('nav.lightMode')
    await toggle.trigger('click')
    expect(document.documentElement.classList.contains('dark')).toBe(false)
    expect(localStorage.getItem('theme')).toBe('light')
  })

  it('resets the mobile drawer when the viewport crosses into desktop layout', async () => {
    app.mobileOpen = true
    const { wrapper } = await render()
    const originalWidth = window.innerWidth
    try {
      Object.defineProperty(window, 'innerWidth', { configurable: true, value: 1440 })
      window.dispatchEvent(new Event('resize'))
      await wrapper.vm.$nextTick()
      expect(app.mobileOpen).toBe(false)
      Object.defineProperty(window, 'innerWidth', { configurable: true, value: 375 })
      window.dispatchEvent(new Event('resize'))
      await wrapper.vm.$nextTick()
      expect(wrapper.get('aside').attributes('aria-hidden')).toBe('true')
    } finally {
      Object.defineProperty(window, 'innerWidth', { configurable: true, value: originalWidth })
    }
  })
})
