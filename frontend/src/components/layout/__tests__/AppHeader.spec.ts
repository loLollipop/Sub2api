import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { mount, type VueWrapper } from '@vue/test-utils'
import { createPinia, setActivePinia } from 'pinia'
import { createI18n } from 'vue-i18n'
import { createMemoryHistory, createRouter } from 'vue-router'
import { useAppStore, useAuthStore } from '@/stores'
import AppHeader from '../AppHeader.vue'

let wrapper: VueWrapper | undefined
let frames: FrameRequestCallback[]

async function renderHeader(browse = true, role: 'user' | 'admin' = 'user') {
  const pinia = createPinia()
  setActivePinia(pinia)
  useAuthStore().token = 'test-session'
  useAuthStore().user = { id: 1, role } as NonNullable<ReturnType<typeof useAuthStore>['user']>
  const router = createRouter({ history: createMemoryHistory(), routes: [{ path: '/:pathMatch(.*)*', component: { template: '<div />' } }] })
  await router.push('/model-plaza')
  await router.isReady()
  wrapper = mount(AppHeader, {
    props: { browse },
    global: {
      plugins: [pinia, router, createI18n({ legacy: false, locale: 'en', missingWarn: false, fallbackWarn: false, messages: { en: {} } })],
      stubs: { UserTopNav: true, AccountMenu: true, Icon: true, ThemeControls: true, AnnouncementBell: true, LocaleSwitcher: true, ConsoleNavigationSearch: true, SubscriptionProgressMini: true, VersionBadge: true },
    },
  })
}

async function scrollTo(top: number) {
  vi.spyOn(window, 'scrollY', 'get').mockReturnValue(top)
  window.dispatchEvent(new Event('scroll'))
  frames.shift()?.(0)
  await wrapper!.vm.$nextTick()
}

beforeEach(() => {
  frames = []
  vi.spyOn(window, 'matchMedia').mockImplementation(query => ({
    matches: query === '(min-width: 1024px)' || query === '(min-width: 640px)', media: query, onchange: null,
    addListener: vi.fn(), removeListener: vi.fn(), addEventListener: vi.fn(), removeEventListener: vi.fn(), dispatchEvent: vi.fn(),
  }))
  vi.spyOn(window, 'scrollY', 'get').mockReturnValue(0)
  vi.spyOn(window, 'requestAnimationFrame').mockImplementation(callback => {
    frames.push(callback)
    return frames.length
  })
  vi.spyOn(window, 'cancelAnimationFrame').mockImplementation(() => {})
})

afterEach(() => {
  wrapper?.unmount()
  wrapper = undefined
  vi.restoreAllMocks()
})

describe('browse header authentication', () => {
  it('shows private utilities only with an authenticated session', async () => {
    await renderHeader()
    expect(wrapper!.find('console-navigation-search-stub').exists()).toBe(true)
    expect(wrapper!.find('subscription-progress-mini-stub').exists()).toBe(true)
    expect(wrapper!.find('announcement-bell-stub').exists()).toBe(true)
    expect(wrapper!.find('.top-browse-link').exists()).toBe(false)

    useAuthStore().token = null
    await wrapper!.vm.$nextTick()
    expect(wrapper!.find('console-navigation-search-stub').exists()).toBe(false)
    expect(wrapper!.find('subscription-progress-mini-stub').exists()).toBe(false)
    expect(wrapper!.find('announcement-bell-stub').exists()).toBe(false)
    expect(wrapper!.get('.top-browse-link').attributes('href')).toContain('/login?redirect=')
  })
})

describe('browse header scroll morph', () => {
  it('uses separate compact entry and exit thresholds with a permanent spacer', async () => {
    await renderHeader()
    expect(wrapper!.find('.top-header-spacer').exists()).toBe(true)
    await scrollTo(87)
    expect(wrapper!.get('header').classes()).not.toContain('is-compact')
    await scrollTo(88)
    expect(wrapper!.get('header').classes()).toContain('is-compact')
    await scrollTo(50)
    expect(wrapper!.get('header').classes()).toContain('is-compact')
    await scrollTo(39)
    expect(wrapper!.get('header').classes()).not.toContain('is-compact')
    expect(wrapper!.find('.top-header-spacer').exists()).toBe(true)
  })

  it('keeps workspaces expanded and resets when leaving browse mode', async () => {
    await renderHeader(false)
    await scrollTo(180)
    expect(wrapper!.get('header').classes()).not.toContain('is-compact')
    await wrapper!.setProps({ browse: true })
    expect(wrapper!.get('header').classes()).toContain('is-compact')
    await wrapper!.setProps({ browse: false })
    expect(wrapper!.get('header').classes()).not.toContain('is-compact')
  })

  it('protects workspace navigation while scrolled and clears it at the top or in browse mode', async () => {
    await renderHeader(false)
    expect(wrapper!.get('header').classes()).not.toContain('is-scrolled')
    await scrollTo(180)
    expect(wrapper!.get('header').classes()).toContain('is-scrolled')
    expect(wrapper!.get('header').classes()).not.toContain('is-compact')
    await wrapper!.setProps({ browse: true })
    expect(wrapper!.get('header').classes()).not.toContain('is-scrolled')
    expect(wrapper!.get('header').classes()).toContain('is-compact')
    await wrapper!.setProps({ browse: false })
    expect(wrapper!.get('header').classes()).toContain('is-scrolled')
    await scrollTo(0)
    expect(wrapper!.get('header').classes()).not.toContain('is-scrolled')
  })

  it('coalesces scroll events and cleans up the pending frame and listener', async () => {
    await renderHeader()
    window.dispatchEvent(new Event('scroll'))
    window.dispatchEvent(new Event('scroll'))
    expect(window.requestAnimationFrame).toHaveBeenCalledTimes(1)
    const remove = vi.spyOn(window, 'removeEventListener')
    wrapper!.unmount()
    wrapper = undefined
    expect(window.cancelAnimationFrame).toHaveBeenCalledWith(1)
    expect(remove).toHaveBeenCalledWith('scroll', expect.any(Function))
  })
})

describe('header sidebar toggle', () => {
  it('collapses and expands desktop navigation without opening the mobile drawer', async () => {
    await renderHeader(false)
    const app = useAppStore()
    app.sidebarCollapsed = false
    await wrapper!.vm.$nextTick()
    const trigger = wrapper!.get('#mobile-navigation-trigger')
    expect(trigger.attributes('aria-expanded')).toBe('true')
    await trigger.trigger('click')
    expect(app.sidebarCollapsed).toBe(true)
    expect(app.mobileOpen).toBe(false)
    expect(trigger.attributes('aria-expanded')).toBe('false')
    await trigger.trigger('click')
    expect(app.sidebarCollapsed).toBe(false)
    expect(trigger.attributes('aria-expanded')).toBe('true')
  })

  it('opens the mobile drawer while retaining the desktop collapse preference', async () => {
    vi.mocked(window.matchMedia).mockImplementation(query => ({
      matches: false, media: query, onchange: null,
      addListener: vi.fn(), removeListener: vi.fn(), addEventListener: vi.fn(), removeEventListener: vi.fn(), dispatchEvent: vi.fn(),
    }))
    await renderHeader()
    const app = useAppStore()
    app.sidebarCollapsed = true
    const trigger = wrapper!.get('#mobile-navigation-trigger')
    expect(trigger.attributes('aria-expanded')).toBe('false')
    await trigger.trigger('click')
    expect(app.mobileOpen).toBe(true)
    expect(app.sidebarCollapsed).toBe(true)
    expect(trigger.attributes('aria-expanded')).toBe('true')
    await trigger.trigger('click')
    expect(app.mobileOpen).toBe(false)
    expect(trigger.attributes('aria-expanded')).toBe('false')
  })
})

describe('administrator version entry', () => {
  it('keeps version checks in the top header independently of sidebar visibility', async () => {
    await renderHeader(false, 'admin')
    useAppStore().siteVersion = '2.1.32'
    await wrapper!.vm.$nextTick()
    const entry = wrapper!.get('.top-brand-area version-badge-stub')
    expect(entry.attributes('placement')).toBe('header')
    expect(entry.attributes('version')).toBe('2.1.32')
    useAppStore().sidebarCollapsed = true
    await wrapper!.vm.$nextTick()
    expect(entry.exists()).toBe(true)
    await wrapper!.setProps({ browse: true })
    expect(wrapper!.find('.top-brand-area version-badge-stub').exists()).toBe(true)
    useAuthStore().user = { id: 2, role: 'user' } as NonNullable<ReturnType<typeof useAuthStore>['user']>
    await wrapper!.vm.$nextTick()
    expect(wrapper!.find('version-badge-stub').exists()).toBe(false)
  })
})
