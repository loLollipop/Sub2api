import { readFileSync } from 'node:fs'
import { dirname, resolve } from 'node:path'
import { fileURLToPath } from 'node:url'

import { afterEach, describe, expect, it, vi } from 'vitest'
import { flushPromises, mount, type VueWrapper } from '@vue/test-utils'
import { createPinia, setActivePinia } from 'pinia'
import { createI18n } from 'vue-i18n'
import { createMemoryHistory, createRouter } from 'vue-router'
import { useAdminSettingsStore, useAppStore, useAuthStore } from '@/stores'
import AppSidebar from '../AppSidebar.vue'
import BaseDialog from '@/components/common/BaseDialog.vue'

vi.mock('@/composables/useBatchImageAccess', () => ({
  useBatchImageAccess: () => ({ canUseBatchImage: false, refreshBatchImageAccess: vi.fn() })
}))

let wrapper: VueWrapper | undefined
let dialog: VueWrapper | undefined
async function renderSidebar(role: 'user' | 'admin' = 'user', desktop = true) {
  vi.spyOn(window, 'matchMedia').mockImplementation(query => ({
    matches: query === '(min-width: 1024px)' && desktop, media: query, onchange: null,
    addListener: vi.fn(), removeListener: vi.fn(), addEventListener: vi.fn(), removeEventListener: vi.fn(), dispatchEvent: vi.fn(),
  }))
  const pinia = createPinia()
  setActivePinia(pinia)
  const app = useAppStore()
  app.publicSettingsLoaded = true
  const auth = useAuthStore()
  auth.user = { id: 1, role } as NonNullable<typeof auth.user>
  useAdminSettingsStore().fetch = vi.fn().mockResolvedValue(undefined)
  const router = createRouter({ history: createMemoryHistory(), routes: [{ path: '/:pathMatch(.*)*', component: { template: '<div />' } }] })
  await router.push(role === 'admin' ? '/admin/dashboard' : '/keys')
  await router.isReady()
  wrapper = mount({ components: { AppSidebar }, template: '<div><button id="mobile-navigation-trigger">Menu</button><AppSidebar /></div>' }, {
    attachTo: document.body,
    global: { plugins: [pinia, router, createI18n({ legacy: false, locale: 'en', missingWarn: false, fallbackWarn: false, messages: { en: {} } })], stubs: { VersionBadge: true } },
  })
  return { app, router }
}

afterEach(() => {
  dialog?.unmount()
  dialog = undefined
  wrapper?.unmount()
  wrapper = undefined
  localStorage.clear()
  document.documentElement.classList.remove('dark')
  vi.restoreAllMocks()
})

const componentPath = resolve(dirname(fileURLToPath(import.meta.url)), '../AppSidebar.vue')
const componentSource = readFileSync(componentPath, 'utf8')
const stylePath = resolve(dirname(fileURLToPath(import.meta.url)), '../../../style.css')
const styleSource = readFileSync(stylePath, 'utf8')
const navigationSource = readFileSync(resolve(dirname(componentPath), '../../composables/useConsoleNavigation.ts'), 'utf8')

describe('AppSidebar custom SVG styles', () => {
  it('does not override uploaded SVG fill or stroke colors', () => {
    expect(componentSource).toContain('.sidebar-svg-icon {')
    expect(componentSource).toContain('color: currentColor;')
    expect(componentSource).toContain('display: block;')
    expect(componentSource).not.toContain('stroke: currentColor;')
    expect(componentSource).not.toContain('fill: none;')
  })
})

describe('AppSidebar scroll position persistence', () => {
  it('binds a template ref to the sidebar nav element', () => {
    expect(componentSource).toContain('ref="sidebarNavRef"')
    expect(componentSource).toContain('sidebar-nav')
  })

  it('declares sidebarNavRef in script setup', () => {
    expect(componentSource).toContain("const sidebarNavRef = ref<HTMLElement | null>(null)")
  })

  it('saves scroll position on beforeUnmount', () => {
    expect(componentSource).toContain('onBeforeUnmount')
    expect(componentSource).toContain('appStore.sidebarScrollTop')
    expect(componentSource).toContain('sidebarNavRef.value.scrollTop')
  })

  it('restores scroll position on mount', () => {
    expect(componentSource).toContain('onMounted')
    expect(componentSource).toContain('appStore.sidebarScrollTop')
    expect(componentSource).toContain('nextTick')
  })
})

describe('AppSidebar direct navigation and responsive collapse', () => {
  it('keeps admin children directly accessible in both expanded and collapsed states', async () => {
    const { app } = await renderSidebar('admin')
    expect(wrapper!.find('.sidebar-nav button').exists()).toBe(false)
    expect(wrapper!.find('.sidebar-nav a[href="/admin/channels"]').exists()).toBe(false)
    expect(wrapper!.get('.sidebar-nav a[href="/admin/channels/pricing"]').exists()).toBe(true)
    expect(wrapper!.get('#sidebar-group-manage').attributes('href')).toBe('/admin/groups')
    expect(wrapper!.get('#sidebar-channel-manage').attributes('href')).toBe('/admin/accounts')
    app.sidebarCollapsed = true
    await flushPromises()
    expect(app.sidebarCollapsed).toBe(true)
    const child = wrapper!.get('.sidebar-nav a[href="/admin/channels/pricing"]')
    expect(child.attributes('aria-label')).toBe('nav.channelPricing')
    expect(child.classes()).toContain('sidebar-link-collapsed')
  })

  it('reflects the user desktop collapse state and saves its scroll position', async () => {
    const { app } = await renderSidebar()
    expect(wrapper!.get('.sidebar').attributes('inert')).toBeUndefined()
    app.sidebarCollapsed = true
    await flushPromises()
    expect(app.sidebarCollapsed).toBe(true)
    expect(wrapper!.get('.sidebar').classes()).toContain('w-[72px]')
    expect(wrapper!.get('a[data-tour="sidebar-my-keys"]').attributes('aria-label')).toBe('nav.apiKeys')
    const nav = wrapper!.get('.sidebar-nav').element
    nav.scrollTop = 160
    wrapper!.unmount()
    wrapper = undefined
    expect(app.sidebarScrollTop).toBe(160)
  })

  it('disables the closed mobile drawer, contains focus, and closes with Escape', async () => {
    const { app } = await renderSidebar('user', false)
    app.sidebarCollapsed = true
    await flushPromises()
    expect(wrapper!.get('.sidebar').attributes('inert')).toBeDefined()
    expect(wrapper!.get('.sidebar').attributes('aria-hidden')).toBe('true')
    expect(wrapper!.get('.sidebar').classes()).toContain('w-64')
    expect(wrapper!.find('.sidebar-footer').exists()).toBe(false)
    app.setMobileOpen(true)
    await flushPromises()
    expect(wrapper!.get('.sidebar').attributes('inert')).toBeUndefined()
    const first = wrapper!.get('.sidebar a')
    const last = wrapper!.get('.sidebar-nav a[href="/monitor"]')
    expect(wrapper!.findAll('.sidebar a').at(-1)?.element).toBe(last.element)
    expect(document.activeElement).toBe(first.element)
    await first.trigger('keydown', { key: 'Tab', shiftKey: true })
    expect(document.activeElement).toBe(last.element)
    await last.trigger('keydown', { key: 'Tab' })
    expect(document.activeElement).toBe(first.element)
    document.dispatchEvent(new KeyboardEvent('keydown', { key: 'Escape', bubbles: true }))
    await flushPromises()
    expect(app.mobileOpen).toBe(false)
    expect(document.activeElement).toBe(wrapper!.get('#mobile-navigation-trigger').element)
  })

  it('returns to the public home from the mobile site icon, closes the drawer and retains browse destinations', async () => {
    const { app, router } = await renderSidebar('user', false)
    expect(wrapper!.find('.sidebar-nav a[href="/monitor"]').exists()).toBe(true)
    app.setMobileOpen(true)
    await flushPromises()
    expect(wrapper!.get('.sidebar-logo').attributes('href')).toBe('/')
    await wrapper!.get('.sidebar-logo').trigger('click')
    await flushPromises()
    expect(router.currentRoute.value.path).toBe('/')
    expect(app.mobileOpen).toBe(false)
  })

  it('yields keyboard focus and Escape to a dialog above the mobile drawer', async () => {
    const { app } = await renderSidebar('user', false)
    app.setMobileOpen(true)
    await flushPromises()
    dialog = mount(BaseDialog, {
      attachTo: document.body,
      props: { show: true, title: 'Announcement', showCloseButton: false },
      slots: { default: '<button id="dialog-first">First</button><button id="dialog-last">Last</button>' },
    })
    await flushPromises()
    const press = (key: string, shiftKey = false) => document.activeElement?.dispatchEvent(
      new KeyboardEvent('keydown', { key, shiftKey, bubbles: true, cancelable: true }))
    expect(document.activeElement?.id).toBe('dialog-first')
    press('Tab')
    expect(document.activeElement?.id).toBe('dialog-last')
    press('Tab')
    expect(document.activeElement?.id).toBe('dialog-first')
    press('Tab', true)
    expect(document.activeElement?.id).toBe('dialog-last')
    press('Escape')
    expect(dialog.emitted('close')).toHaveLength(1)
    expect(app.mobileOpen).toBe(true)
    await dialog.setProps({ show: false })
    expect(wrapper!.get('.sidebar').element.contains(document.activeElement)).toBe(true)
    press('Escape')
    await flushPromises()
    expect(app.mobileOpen).toBe(false)
  })
})

describe('AppSidebar header styles', () => {
  it('does not clip the version badge dropdown', () => {
    const sidebarHeaderBlockMatch = styleSource.match(/\.sidebar-header\s*\{[\s\S]*?\n {2}\}/)
    const sidebarBrandBlockMatch = componentSource.match(/\.sidebar-brand\s*\{[\s\S]*?\n\}/)

    expect(sidebarHeaderBlockMatch).not.toBeNull()
    expect(sidebarBrandBlockMatch).not.toBeNull()
    expect(sidebarHeaderBlockMatch?.[0]).not.toContain('@apply overflow-hidden;')
    expect(sidebarBrandBlockMatch?.[0]).not.toContain('overflow: hidden;')
  })
})

describe('AppSidebar user navigation', () => {
  it('hides the legacy subscriptions entry while keeping the route available', () => {
    expect(navigationSource).not.toContain("{ path: '/subscriptions'")
  })

  it('omits the removed canvas and ticket destinations', () => {
    expect(navigationSource).not.toContain("{ path: '/infinite-canvas'")
    expect(navigationSource).not.toContain("t('nav.infiniteCanvas')")
    expect(navigationSource).not.toContain("path: '/tickets'")
    expect(navigationSource).not.toContain("path: '/admin/tickets'")
  })
})
