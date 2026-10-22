import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { flushPromises, mount, type VueWrapper } from '@vue/test-utils'
import { createPinia, setActivePinia } from 'pinia'
import { createI18n } from 'vue-i18n'
import { createMemoryHistory, createRouter } from 'vue-router'
import { useAppStore, useAuthStore } from '@/stores'
import { useAdminSettingsStore } from '@/stores/adminSettings'
import AppLayout from '../AppLayout.vue'

vi.mock('@/composables/useOnboardingTour', () => ({
  useOnboardingTour: () => ({ replayTour: vi.fn() })
}))

vi.mock('@/composables/useBatchImageAccess', () => ({
  useBatchImageAccess: () => ({ canUseBatchImage: false, refreshBatchImageAccess: vi.fn() })
}))

const userPaths = ['/dashboard', '/keys', '/usage', '/monitor', '/available-channels', '/purchase',
  '/subscriptions', '/profile', '/redeem', '/affiliate',
  '/model-plaza', '/batch-image', '/custom/demo']
const adminPaths = ['/admin/dashboard', '/admin/accounts', '/admin/groups', '/admin/users', '/admin/keys', '/admin/profile', '/admin/usage']
const leftoverPaths = ['/login', '/keys/other', '/usage-extra']
let wrapper: VueWrapper | undefined

async function renderShell(path: string, role: 'user' | 'admin' = 'user') {
  const pinia = createPinia()
  setActivePinia(pinia)
  const app = useAppStore()
  app.siteName = 'SIGNAL'
  app.publicSettingsLoaded = true
  app.docUrl = '/docs'
  const auth = useAuthStore()
  auth.user = {
    id: 1,
    role,
    username: 'Local Test',
    email: 'test@example.test',
    balance: 20,
    frozen_balance: 3
  } as NonNullable<typeof auth.user>
  useAdminSettingsStore().fetch = vi.fn().mockResolvedValue(undefined)

  const router = createRouter({
    history: createMemoryHistory(),
    routes: [...new Set([...userPaths, ...adminPaths, ...leftoverPaths, '/:pathMatch(.*)*'])].map((routePath) => ({
      path: routePath,
      component: { template: '<div />' },
      meta: { title: routePath === '/keys' ? 'API Keys' : routePath }
    }))
  })
  await router.push(path)
  await router.isReady()

  wrapper = mount(AppLayout, {
    slots: { default: '<h1>Content title</h1><button id="page-action">Page action</button>' },
    global: {
      plugins: [pinia, router, createI18n({ legacy: false, locale: 'en', missingWarn: false, fallbackWarn: false, messages: { en: {} } })],
      stubs: { AnnouncementBell: true, SubscriptionProgressMini: true, LocaleSwitcher: true, VersionBadge: true }
    }
  })
  return { router, app }
}

beforeEach(() => {
  localStorage.clear()
  document.documentElement.classList.remove('dark')
  vi.spyOn(window, 'matchMedia').mockImplementation(query => ({
    matches: query === '(min-width: 1024px)', media: query, onchange: null,
    addListener: vi.fn(), removeListener: vi.fn(), addEventListener: vi.fn(), removeEventListener: vi.fn(), dispatchEvent: vi.fn(),
  }))
})

afterEach(() => {
  wrapper?.unmount()
  wrapper = undefined
  vi.restoreAllMocks()
})

describe('SIGNAL shell route isolation', () => {
  it.each([...userPaths, '/keys?search=test#active', '/keys/'])('opts in %s and leaves one content heading', async (path) => {
    await renderShell(path)
    expect(wrapper!.classes()).toContain('console-signal')
    expect(wrapper!.find('.bg-mesh-gradient').exists()).toBe(false)
    expect(wrapper!.find('header h1').exists()).toBe(false)
    expect(wrapper!.findAll('h1')).toHaveLength(1)
    expect(wrapper!.get('.top-brand').attributes('href')).toBe('/')
    expect(wrapper!.get('#page-action').text()).toBe('Page action')
  })

  it.each(leftoverPaths)('still applies the shared theme on leftover AppLayout routes like %s', async (path) => {
    await renderShell(path)
    expect(wrapper!.classes()).toContain('console-signal')
    expect(wrapper!.find('.bg-mesh-gradient').exists()).toBe(false)
    expect(wrapper!.find('.top-header').exists()).toBe(true)
  })

  it.each(adminPaths)('applies the admin workspace on %s with the shared top header', async (path) => {
    await renderShell(path, 'admin')
    expect(wrapper!.classes()).toContain('signal-admin')
    expect(wrapper!.find('header h1').exists()).toBe(false)
    expect(wrapper!.find('.sidebar a[href="/admin/dashboard"]').exists()).toBe(true)
    expect(wrapper!.get('.top-brand').attributes('href')).toBe('/')
  })

  it('switches between admin and personal workspace without leaving theme state behind', async () => {
    const { router } = await renderShell('/keys', 'admin')
    expect(wrapper!.classes()).toContain('console-signal')
    expect(wrapper!.classes()).toContain('consumer-layout')
    expect(wrapper!.find('.sidebar a[href="/admin/dashboard"]').exists()).toBe(false)
    expect(wrapper!.find('.sidebar a[href="/keys"]').exists()).toBe(true)
    await router.push('/admin/dashboard')
    expect(wrapper!.classes()).toContain('console-signal')
    expect(wrapper!.classes()).toContain('signal-admin')
    expect(document.body.classList.contains('console-signal')).toBe(false)
    await router.push('/usage')
    expect(wrapper!.classes()).toContain('console-signal')
    expect(wrapper!.classes()).not.toContain('signal-admin')
  })

  it.each(['/model-plaza?embedded=1', '/batch-image', '/custom/demo'])('returns administrators to the public home from %s while retaining the console entry', async (path) => {
    const { router } = await renderShell(path, 'admin')
    const home = wrapper!.get('.top-brand')
    expect(home.attributes('href')).toBe('/')
    expect(wrapper!.get('.top-browse-nav a[href="/dashboard"]').exists()).toBe(true)
    await home.trigger('click')
    await vi.waitFor(() => expect(router.currentRoute.value.path).toBe('/'))
  })

  it('keeps a user sidebar and compact account utilities beside the browse links', async () => {
    const { app } = await renderShell('/keys')
    expect(wrapper!.classes()).toContain('top-layout')
    expect(wrapper!.classes()).toContain('consumer-layout')
    expect(wrapper!.classes()).not.toContain('browse-layout')
    expect(wrapper!.find('.console-atmosphere').exists()).toBe(false)
    expect(wrapper!.find('.top-browse-nav').exists()).toBe(true)
    expect(wrapper!.get('.shell-workspace > .sidebar').exists()).toBe(true)
    expect(wrapper!.get('.shell-workspace > .signal-frame').exists()).toBe(true)
    expect(wrapper!.find('.sidebar-footer').exists()).toBe(false)
    expect(wrapper!.get('.top-account-area').element.children[0]).toBe(wrapper!.get('.console-search-trigger').element)
    expect(wrapper!.get('.top-account-area subscription-progress-mini-stub').exists()).toBe(true)
    expect(wrapper!.get('.top-account-area .top-theme-controls').exists()).toBe(true)
    expect(wrapper!.get('.top-account-area announcement-bell-stub').exists()).toBe(true)
    expect(wrapper!.get('.top-account-area locale-switcher-stub').attributes('compact')).toBeDefined()
    expect(wrapper!.get('.header-user-button').classes()).toContain('avatar-only')
    expect(wrapper!.find('.header-identity').exists()).toBe(false)
    expect(wrapper!.find('.header-balance').exists()).toBe(false)
    expect(wrapper!.get('.account-utilities-panel').attributes('style')).toContain('display: none')
    await wrapper!.get('.header-user-button').trigger('click')
    expect(wrapper!.get('.header-user-button').attributes('aria-expanded')).toBe('true')
    expect(wrapper!.get('.account-utilities-panel').attributes('style') || '').not.toContain('display: none')
    expect(wrapper!.get('.account-balance').findAll('dd').map((amount) => amount.text())).toEqual(['$20.00', '$3.00', '$23.00'])
    expect(wrapper!.find('.account-utilities-panel locale-switcher-stub').exists()).toBe(false)
    expect(wrapper!.find('.account-utilities-panel a[href="/tickets"]').exists()).toBe(false)
    expect(wrapper!.find('.sidebar a[href="/tickets"]').exists()).toBe(false)
    await wrapper!.get('#mobile-navigation-trigger').trigger('click')
    expect(app.sidebarCollapsed).toBe(true)
    expect(app.mobileOpen).toBe(false)
    await wrapper!.get('.top-theme-controls button[aria-label="Dark"]').trigger('click')
    expect(document.documentElement.classList.contains('dark')).toBe(true)
    await wrapper!.get('.top-theme-controls button[aria-label="Light"]').trigger('click')
    expect(document.documentElement.classList.contains('dark')).toBe(false)
    expect(wrapper!.find('.sidebar a[href="/keys"]').exists()).toBe(true)
    expect(wrapper!.find('.sidebar a[href="/usage"]').exists()).toBe(true)
  })

  it.each(['/model-plaza', '/available-channels', '/monitor'])('marks %s as a full-width browse layout', async path => {
    const { router } = await renderShell(path)
    expect(wrapper!.classes()).toContain('top-layout')
    expect(wrapper!.classes()).toContain('browse-layout')
    expect(wrapper!.find('.top-header-spacer').exists()).toBe(true)
    await router.push('/keys')
    await flushPromises()
    expect(wrapper!.classes()).not.toContain('browse-layout')
  })

  it('preserves the admin sidebar collapse and personal destinations', async () => {
    const { app } = await renderShell('/admin/dashboard', 'admin')
    await wrapper!.get('#mobile-navigation-trigger').trigger('click')
    expect(app.sidebarCollapsed).toBe(true)
    expect(wrapper!.classes()).toContain('signal-nav-collapsed')
    expect(wrapper!.get('.sidebar').classes()).toContain('w-[72px]')
    expect(wrapper!.find('.top-browse-nav').exists()).toBe(true)
    expect(wrapper!.find('.sidebar a[href="/admin/channels/pricing"]').exists()).toBe(false)
    expect(wrapper!.find('.sidebar a[href="/admin/keys"]').exists()).toBe(true)
  })

  it('makes the header and content inert only while the mobile drawer is open', async () => {
    vi.mocked(window.matchMedia).mockImplementation(query => ({
      matches: false, media: query, onchange: null,
      addListener: vi.fn(), removeListener: vi.fn(), addEventListener: vi.fn(), removeEventListener: vi.fn(), dispatchEvent: vi.fn(),
    }))
    const { app } = await renderShell('/keys')
    expect(wrapper!.get('.top-header-spacer').attributes('inert')).toBeUndefined()
    await wrapper!.get('#mobile-navigation-trigger').trigger('click')
    expect(wrapper!.get('.top-header-spacer').attributes('inert')).toBeDefined()
    expect(wrapper!.get('.signal-frame').attributes('inert')).toBeDefined()
    expect(wrapper!.get('.sidebar').attributes('inert')).toBeUndefined()
    app.setMobileOpen(false)
    await wrapper!.vm.$nextTick()
    expect(wrapper!.get('.top-header-spacer').attributes('inert')).toBeUndefined()
    expect(wrapper!.get('.signal-frame').attributes('inert')).toBeUndefined()
  })
})
