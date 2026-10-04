import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { mount, type VueWrapper } from '@vue/test-utils'
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
  '/orders', '/subscriptions', '/profile', '/redeem', '/affiliate',
  '/payment/qrcode', '/payment/result', '/payment/stripe', '/payment/airwallex',
  '/model-plaza', '/batch-image', '/infinite-canvas', '/custom/demo']
const adminPaths = ['/admin/dashboard', '/admin/accounts', '/admin/groups', '/admin/users', '/admin/keys', '/admin/usage']
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
    expect(wrapper!.classes()).toContain('console-local')
    expect(wrapper!.find('.bg-mesh-gradient').exists()).toBe(false)
    expect(wrapper!.find('header h1').exists()).toBe(false)
    expect(wrapper!.findAll('h1')).toHaveLength(1)
    expect(wrapper!.find('.signal-breadcrumb').exists()).toBe(false)
    expect(wrapper!.get('header .local-brand').attributes('href')).toBe('/dashboard')
    expect(wrapper!.get('header .local-brand-name').text()).toBe('SIGNAL')
    expect(wrapper!.find('.local-navigation').exists()).toBe(true)
    expect(wrapper!.get('aside.local-mobile-sidebar').attributes('aria-hidden')).toBe('true')
    expect(wrapper!.get('.signal-frame').classes()).toContain('local-frame')
    expect(wrapper!.get('.signal-frame').classes().some(name => name.startsWith('lg:ml-'))).toBe(false)
    expect(wrapper!.get('#page-action').text()).toBe('Page action')
  })

  it.each(leftoverPaths)('still applies the shared theme on leftover AppLayout routes like %s', async (path) => {
    await renderShell(path)
    expect(wrapper!.classes()).toContain('console-signal')
    expect(wrapper!.classes()).toContain('console-local')
    expect(wrapper!.find('.bg-mesh-gradient').exists()).toBe(false)
    expect(wrapper!.find('.signal-header').exists()).toBe(true)
    expect(wrapper!.get('header .local-brand').attributes('href')).toBe('/dashboard')
  })

  it.each(adminPaths)('applies the local admin workspace on %s', async (path) => {
    await renderShell(path, 'admin')
    expect(wrapper!.classes()).toContain('signal-admin')
    expect(wrapper!.classes()).toContain('console-local')
    expect(wrapper!.find('header h1').exists()).toBe(false)
    expect(wrapper!.findAll('h1')).toHaveLength(1)
    expect(wrapper!.get('header .local-brand').attributes('href')).toBe('/admin/dashboard')
    expect(wrapper!.find('header .local-role-badge').exists()).toBe(true)
    expect(wrapper!.find('.local-navigation a[href="/admin/dashboard"]').exists()).toBe(true)
  })

  it('switches between admin and personal workspace without leaving theme state behind', async () => {
    const { router } = await renderShell('/keys', 'admin')
    expect(wrapper!.classes()).toContain('console-signal')
    expect(wrapper!.classes()).not.toContain('signal-admin')
    expect(wrapper!.find('.local-navigation a[href="/admin/dashboard"]').exists()).toBe(true)
    await router.push('/admin/dashboard')
    expect(wrapper!.classes()).toContain('console-signal')
    expect(wrapper!.classes()).toContain('signal-admin')
    expect(document.body.classList.contains('console-signal')).toBe(false)
    await router.push('/usage')
    expect(wrapper!.classes()).toContain('console-signal')
    expect(wrapper!.classes()).not.toContain('signal-admin')
  })

  it.each(['/model-plaza?embedded=1', '/infinite-canvas', '/batch-image', '/custom/demo'])('returns administrators to their dashboard from %s', async (path) => {
    const { router } = await renderShell(path, 'admin')
    const home = wrapper!.get('header .local-brand')
    expect(home.attributes('href')).toBe('/admin/dashboard')
    await home.trigger('click')
    await vi.waitFor(() => expect(router.currentRoute.value.path).toBe('/admin/dashboard'))
  })

  it('keeps horizontal navigation, mobile drawer, theme and balance semantics', async () => {
    const { app } = await renderShell('/keys')
    expect(wrapper!.get('.header-balance-value').text()).toBe('$20.00')
    expect(wrapper!.get('.header-balance-frozen').text()).toContain('$3.00')
    expect(wrapper!.get('.header-balance-detail').text()).toContain('$23.00')
    expect(wrapper!.get('.header-balance').attributes('tabindex')).toBe('0')

    expect(wrapper!.findAll('.sidebar-footer button')).toHaveLength(1)
    expect(app.sidebarCollapsed).toBe(false)
    expect(wrapper!.get('.signal-frame').classes().some(name => name.startsWith('lg:ml-'))).toBe(false)
    expect(wrapper!.find('.local-navigation a[href="/keys"]').exists()).toBe(true)
    expect(wrapper!.find('.local-navigation a[href="/usage"]').exists()).toBe(true)

    await wrapper!.get('.header-location > button').trigger('click')
    expect(app.mobileOpen).toBe(true)
    expect(wrapper!.get('aside.local-mobile-sidebar').attributes('aria-hidden')).toBeUndefined()

    const wasDark = document.documentElement.classList.contains('dark')
    await wrapper!.get('header .local-theme-toggle').trigger('click')
    const isDark = document.documentElement.classList.contains('dark')
    expect(isDark).toBe(!wasDark)
    expect(localStorage.getItem('theme')).toBe(isDark ? 'dark' : 'light')
    await vi.waitFor(() => expect(wrapper!.get('[data-test="theme-toggle"]').text())
      .toContain(isDark ? 'nav.lightMode' : 'nav.darkMode'))
    expect(wrapper!.find('.sidebar a[href="/keys"]').exists()).toBe(true)
    expect(wrapper!.find('.sidebar a[href="/usage"]').exists()).toBe(true)
  })
})
