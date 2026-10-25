import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { flushPromises, mount, type VueWrapper } from '@vue/test-utils'
import { createPinia, setActivePinia } from 'pinia'
import { createI18n } from 'vue-i18n'
import { createMemoryHistory, createRouter } from 'vue-router'
import { useAdminSettingsStore, useAppStore, useAuthStore } from '@/stores'
import AppSidebar from '../AppSidebar.vue'
import UserTopNav from '../UserTopNav.vue'

const batchAccess = vi.hoisted(() => ({ allowed: true }))
vi.mock('@/composables/useBatchImageAccess', async () => {
  const { ref } = await import('vue')
  return { useBatchImageAccess: () => ({ canUseBatchImage: ref(batchAccess.allowed), refreshBatchImageAccess: vi.fn() }) }
})

let wrapper: VueWrapper | undefined
async function render(simple = false, enabled = true, role: 'user' | 'admin' = 'user') {
  const pinia = createPinia()
  setActivePinia(pinia)
  const app = useAppStore()
  app.cachedPublicSettings = {
    model_plaza_enabled: true,
    available_channels_enabled: enabled, channel_monitor_enabled: enabled,
    purchase_subscription_enabled: enabled, affiliate_enabled: enabled,
    custom_menu_items: [
      { id: 'extra', label: 'Custom workspace', visibility: 'user', sort_order: 1, icon_svg: '<svg><path fill="red" d="M0 0h8v8H0z" /></svg>' },
      { id: 'admin-only', label: 'Admin only', visibility: 'admin', sort_order: 0, icon_svg: '' },
      { id: '322273f5aaa4d036', label: 'Embedded recharge', visibility: 'user', sort_order: 0, icon_svg: '' },
    ],
  } as NonNullable<typeof app.cachedPublicSettings>
  const auth = useAuthStore()
  auth.user = { id: 12, role } as NonNullable<typeof auth.user>
  vi.spyOn(auth, 'isSimpleMode', 'get').mockReturnValue(simple)
  useAdminSettingsStore().fetch = vi.fn().mockResolvedValue(undefined)
  const router = createRouter({ history: createMemoryHistory(), routes: [{ path: '/:pathMatch(.*)*', component: { template: '<div />' } }] })
  await router.push(role === 'admin' ? '/admin/dashboard' : '/dashboard')
  await router.isReady()
  wrapper = mount({ components: { AppSidebar, UserTopNav }, template: '<div><AppSidebar /><UserTopNav /><button id="outside">Outside</button></div>' }, {
    attachTo: document.body,
    global: {
      plugins: [pinia, router, createI18n({ legacy: false, locale: 'en', missingWarn: false, fallbackWarn: false, messages: { en: {} } })],
      stubs: { VersionBadge: true },
    },
  })
  return { app, auth, router }
}
const primary = () => wrapper!.findAll('.top-browse-nav > a').map(item => item.attributes('href'))
const sidebarLinks = () => wrapper!.findAll('.sidebar-nav a').map(item => item.attributes('href'))

beforeEach(() => {
  document.documentElement.classList.remove('dark')
  localStorage.setItem('theme', 'light')
  batchAccess.allowed = true
  vi.spyOn(window, 'matchMedia').mockImplementation(query => ({
    matches: query === '(min-width: 1024px)', media: query, onchange: null,
    addListener: vi.fn(), removeListener: vi.fn(), addEventListener: vi.fn(), removeEventListener: vi.fn(), dispatchEvent: vi.fn(),
  }))
})

afterEach(() => {
  wrapper?.unmount()
  wrapper = undefined
  localStorage.clear()
  document.documentElement.classList.remove('dark')
  vi.restoreAllMocks()
})

describe('browse navigation and sidebar operations', () => {
  it('gates the leaderboard entry and marks it active without selecting the console', async () => {
    const { app, router } = await render()
    expect(primary()).not.toContain('/leaderboard')
    app.cachedPublicSettings!.leaderboard_enabled = true
    await router.push('/leaderboard')
    await flushPromises()
    expect(primary()).toContain('/leaderboard')
    expect(wrapper!.findAll('.top-browse-link.is-active').map(link => link.attributes('href'))).toEqual(['/leaderboard'])
    expect(wrapper!.get('a[href="/leaderboard"]').attributes('aria-current')).toBe('page')
    app.cachedPublicSettings!.leaderboard_enabled = false
    await flushPromises()
    expect(primary()).not.toContain('/leaderboard')
  })
  it('keeps navigation flat without category headings', async () => {
    await render()
    expect(primary()).toEqual(['/', '/dashboard', '/model-plaza?embedded=1', '/available-channels', '/monitor'])
    expect(wrapper!.find('.top-browse-nav button').exists()).toBe(false)
    expect(wrapper!.find('.consumer-more-menu').exists()).toBe(false)
    const sections = wrapper!.findAll('.sidebar-nav .sidebar-section')
    expect(sections).toHaveLength(1)
    expect(sections[0].findAll('a').map(link => link.attributes('href'))).toEqual(['/dashboard', '/keys', '/batch-image', '/usage', '/purchase', '/redeem', '/affiliate', '/profile', '/custom/extra'])
    expect(wrapper!.find('.sidebar-section-title').exists()).toBe(false)
    expect(wrapper!.text()).not.toContain('Admin only')
    expect(wrapper!.text()).not.toContain('Embedded recharge')
    expect(wrapper!.get('.sidebar-svg-icon path').attributes('fill')).toBe('red')
    expect(wrapper!.findAll('[data-tour="sidebar-my-keys"]')).toHaveLength(1)
  })

  it('reacts to shared feature gates for browse links and wallet tools', async () => {
    const { app } = await render(false, false)
    expect(primary()).toEqual(['/', '/dashboard', '/model-plaza?embedded=1'])
    expect(sidebarLinks()).not.toContain('/purchase')
    expect(sidebarLinks()).not.toContain('/orders')
    expect(sidebarLinks()).not.toContain('/affiliate')
    app.cachedPublicSettings!.available_channels_enabled = true
    app.cachedPublicSettings!.channel_monitor_enabled = true
    app.cachedPublicSettings!.purchase_subscription_enabled = true
    await flushPromises()
    expect(primary()).toContain('/available-channels')
    expect(primary()).toContain('/monitor')
    expect(sidebarLinks()).toContain('/purchase')
    expect(sidebarLinks()).not.toContain('/orders')
  })

  it('preserves simple and backend mode restrictions', async () => {
    const { app } = await render(true)
    expect(primary()).toEqual(['/', '/dashboard', '/model-plaza?embedded=1', '/monitor'])
    expect(sidebarLinks()).toEqual(['/dashboard', '/keys', '/profile', '/custom/extra'])
    app.cachedPublicSettings!.backend_mode_enabled = true
    await flushPromises()
    expect(primary()).toEqual(['/', '/dashboard', '/model-plaza?embedded=1'])
    expect(sidebarLinks()).toEqual([])
  })

  it('hides batch image without eligible key access', async () => {
    batchAccess.allowed = false
    await render()
    expect(sidebarLinks()).not.toContain('/batch-image')
    expect(sidebarLinks()).not.toContain('/infinite-canvas')
  })

  it('sanitizes custom SVG and external documentation links', async () => {
    const { app } = await render()
    app.docUrl = 'javascript:alert(1)'
    app.cachedPublicSettings!.custom_menu_items![0].icon_svg = '<svg><script>alert(1)</script><path onload="alert(1)" fill="red" /></svg>'
    await flushPromises()
    expect(wrapper!.find('.top-browse-nav a[target="_blank"]').exists()).toBe(false)
    expect(wrapper!.find('.sidebar-svg-icon script').exists()).toBe(false)
    expect(wrapper!.get('.sidebar-svg-icon path').attributes('onload')).toBeUndefined()
    app.docUrl = 'https://docs.example.test/guide'
    await flushPromises()
    expect(wrapper!.get('.top-browse-nav a[target="_blank"]').attributes()).toMatchObject({ href: 'https://docs.example.test/guide', rel: 'noopener noreferrer' })
  })

  it('keeps browse links available for administrators and uses their dashboard', async () => {
    const { router, app } = await render(false, true, 'admin')
    expect(primary()).toEqual(['/', '/admin/dashboard', '/model-plaza?embedded=1', '/available-channels', '/monitor'])
    expect(sidebarLinks()).toContain('/admin/accounts')
    expect(sidebarLinks()).toContain('/admin/keys')
    expect(sidebarLinks()).not.toContain('/keys')
    await router.push('/dashboard')
    app.cachedPublicSettings!.backend_mode_enabled = true
    await flushPromises()
    expect(primary()).toEqual(['/', '/dashboard', '/model-plaza?embedded=1', '/available-channels', '/monitor'])
    expect(sidebarLinks()).toContain('/keys')
    expect(sidebarLinks()).not.toContain('/admin/accounts')
  })
})
