import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { flushPromises, mount, type VueWrapper } from '@vue/test-utils'
import { createPinia, setActivePinia } from 'pinia'
import { createI18n } from 'vue-i18n'
import { createMemoryHistory, createRouter } from 'vue-router'
import { useAppStore, useAuthStore } from '@/stores'
import AppSidebar from '../AppSidebar.vue'
import UserTopNav from '../UserTopNav.vue'

const batchAccess = vi.hoisted(() => ({ allowed: true }))
vi.mock('@/composables/useBatchImageAccess', async () => {
  const { ref } = await import('vue')
  return { useBatchImageAccess: () => ({ canUseBatchImage: ref(batchAccess.allowed), refreshBatchImageAccess: vi.fn() }) }
})

let wrapper: VueWrapper | undefined
async function render(simple = false, enabled = true) {
  const pinia = createPinia()
  setActivePinia(pinia)
  const app = useAppStore()
  app.cachedPublicSettings = {
    available_channels_enabled: enabled, channel_monitor_enabled: enabled,
    payment_enabled: enabled, affiliate_enabled: enabled,
    custom_menu_items: [
      { id: 'extra', label: 'Custom workspace', visibility: 'user', sort_order: 1, icon_svg: '<svg><path fill="red" d="M0 0h8v8H0z" /></svg>' },
      { id: 'admin-only', label: 'Admin only', visibility: 'admin', sort_order: 0, icon_svg: '' },
      { id: '322273f5aaa4d036', label: 'Embedded recharge', visibility: 'user', sort_order: 0, icon_svg: '' },
    ],
  } as NonNullable<typeof app.cachedPublicSettings>
  const auth = useAuthStore()
  auth.user = { id: 12, role: 'user' } as NonNullable<typeof auth.user>
  vi.spyOn(auth, 'isSimpleMode', 'get').mockReturnValue(simple)
  const router = createRouter({ history: createMemoryHistory(), routes: [{ path: '/:pathMatch(.*)*', component: { template: '<div />' } }] })
  await router.push('/dashboard')
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
const primary = () => wrapper!.findAll('.consumer-nav > a').map(item => item.attributes('href'))
const secondary = () => wrapper!.findAll('.consumer-more-menu a').map(item => item.attributes('href'))
const drawer = () => wrapper!.findAll('.sidebar-nav a').map(item => item.attributes('href'))
const openMenu = () => wrapper!.get('.consumer-more-trigger').trigger('click')

beforeEach(() => {
  document.documentElement.classList.remove('dark')
  localStorage.setItem('theme', 'light')
  batchAccess.allowed = true
})

afterEach(() => {
  wrapper?.unmount()
  wrapper = undefined
  localStorage.clear()
  document.documentElement.classList.remove('dark')
  vi.restoreAllMocks()
})

describe('user top navigation', () => {
  it('preserves every drawer destination and places the main five in the capsule', async () => {
    await render()
    await openMenu()
    expect(primary()).toEqual(['/dashboard', '/keys', '/usage', '/available-channels', '/monitor'])
    expect(secondary()).toEqual(['/infinite-canvas', '/batch-image', '/purchase', '/orders', '/redeem', '/affiliate', '/tickets', '/profile', '/custom/extra'])
    expect([...primary(), ...secondary()].sort()).toEqual(drawer().sort())
    expect(wrapper!.text()).not.toContain('Admin only')
    expect(wrapper!.text()).not.toContain('Embedded recharge')
    expect(wrapper!.get('.consumer-custom-icon path').attributes('fill')).toBe('red')
    expect(wrapper!.find('.consumer-custom-icon script').exists()).toBe(false)
  })

  it('uses the same feature gates and reacts when settings change', async () => {
    const { app } = await render(false, false)
    await openMenu()
    expect(primary()).toEqual(['/dashboard', '/keys', '/usage'])
    expect(secondary()).not.toContain('/purchase')
    expect(secondary()).not.toContain('/orders')
    expect(secondary()).not.toContain('/affiliate')
    app.cachedPublicSettings!.available_channels_enabled = true
    app.cachedPublicSettings!.channel_monitor_enabled = true
    app.cachedPublicSettings!.purchase_subscription_enabled = true
    await flushPromises()
    expect(primary()).toContain('/available-channels')
    expect(primary()).toContain('/monitor')
    expect(secondary()).toContain('/purchase')
    expect(secondary()).not.toContain('/orders')
    expect([...primary(), ...secondary()].sort()).toEqual(drawer().sort())
  })

  it('keeps simple mode and backend mode restrictions in both navigation surfaces', async () => {
    const { app } = await render(true)
    await openMenu()
    expect(primary()).toEqual(['/dashboard', '/keys', '/monitor'])
    expect(secondary()).toEqual(['/infinite-canvas', '/tickets', '/profile', '/custom/extra'])
    expect([...primary(), ...secondary()].sort()).toEqual(drawer().sort())
    app.cachedPublicSettings!.backend_mode_enabled = true
    await flushPromises()
    expect(primary()).toEqual([])
    expect(secondary()).toEqual([])
    expect(drawer()).toEqual([])
  })

  it('does not expose batch image tools without eligible key access', async () => {
    batchAccess.allowed = false
    await render()
    await openMenu()
    expect(secondary()).not.toContain('/batch-image')
    expect(drawer()).not.toContain('/batch-image')
    expect(secondary()).toContain('/infinite-canvas')
  })

  it('supports arrow keys, Home, End and Escape with focus returned to the trigger', async () => {
    await render()
    await wrapper!.get('.consumer-more-trigger').trigger('keydown', { key: 'ArrowDown' })
    expect(document.activeElement).toBe(wrapper!.get('.consumer-more-menu a').element)
    await wrapper!.get('.consumer-more-menu').trigger('keydown', { key: 'ArrowDown' })
    expect(document.activeElement).toBe(wrapper!.findAll('.consumer-more-menu a')[1].element)
    await wrapper!.get('.consumer-more-menu').trigger('keydown', { key: 'End' })
    expect(document.activeElement).toBe(wrapper!.get('.consumer-theme-toggle').element)
    await wrapper!.get('.consumer-more-menu').trigger('keydown', { key: 'Home' })
    expect(document.activeElement).toBe(wrapper!.get('.consumer-more-menu a').element)
    document.dispatchEvent(new KeyboardEvent('keydown', { key: 'Escape', bubbles: true }))
    await flushPromises()
    expect(wrapper!.find('.consumer-more-menu').exists()).toBe(false)
    expect(document.activeElement).toBe(wrapper!.get('.consumer-more-trigger').element)
    await wrapper!.get('.consumer-more-trigger').trigger('keydown', { key: 'ArrowUp' })
    expect(document.activeElement).toBe(wrapper!.get('.consumer-theme-toggle').element)
  })

  it('closes on outside pointer, focus leaving, link selection and route changes', async () => {
    const { router } = await render()
    await openMenu()
    wrapper!.get('#outside').element.dispatchEvent(new Event('pointerdown', { bubbles: true }))
    await flushPromises()
    expect(wrapper!.find('.consumer-more-menu').exists()).toBe(false)
    await openMenu()
    await wrapper!.get('.consumer-navigation').trigger('focusout', { relatedTarget: wrapper!.get('#outside').element })
    expect(wrapper!.find('.consumer-more-menu').exists()).toBe(false)
    await openMenu()
    await wrapper!.get('.consumer-more-menu a[href="/infinite-canvas"]').trigger('click')
    await flushPromises()
    expect(router.currentRoute.value.path).toBe('/infinite-canvas')
    expect(wrapper!.find('.consumer-more-menu').exists()).toBe(false)
    expect(wrapper!.get('.consumer-more-trigger').classes()).toContain('consumer-nav-link-active')
    await openMenu()
    await router.push('/keys?tab=all')
    await flushPromises()
    expect(wrapper!.find('.consumer-more-menu').exists()).toBe(false)
  })

  it('offers a desktop theme toggle and keeps the drawer toggle in sync', async () => {
    await render()
    await openMenu()
    await wrapper!.get('.consumer-theme-toggle').trigger('click')
    await flushPromises()
    expect(document.documentElement.classList.contains('dark')).toBe(true)
    expect(localStorage.getItem('theme')).toBe('dark')
    expect(wrapper!.get('[data-test="theme-toggle"]').text()).toContain('nav.lightMode')
    await wrapper!.get('[data-test="theme-toggle"]').trigger('click')
    await flushPromises()
    expect(wrapper!.get('.consumer-theme-toggle').text()).toContain('nav.darkMode')
  })
})
