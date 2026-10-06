import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { flushPromises, mount, type VueWrapper } from '@vue/test-utils'
import { createMemoryHistory, createRouter } from 'vue-router'
import AccountMenu from '../AccountMenu.vue'

const { authStore, appStore, onboardingStore } = vi.hoisted(() => ({
  authStore: {
    user: { username: 'Lollipop', email: 'lol@example.test', role: 'admin', avatar_url: '', balance: 12.5, frozen_balance: 2 },
    isAdmin: true,
    isSimpleMode: false,
    logout: vi.fn().mockResolvedValue(undefined),
  },
  appStore: { contactInfo: 'support@example.test' },
  onboardingStore: { replay: vi.fn() },
}))
vi.mock('@/stores', () => ({
  useAuthStore: () => authStore,
  useAppStore: () => appStore,
  useOnboardingStore: () => onboardingStore,
}))
vi.mock('vue-i18n', () => ({ useI18n: () => ({ t: (key: string) => key }) }))

const wrappers: VueWrapper[] = []
async function mountMenu(props: { showBalance?: boolean; showOnboarding?: boolean } = {}) {
  const router = createRouter({
    history: createMemoryHistory(),
    routes: ['/', '/profile', '/keys', '/login'].map((path) => ({ path, component: { template: '<div />' } })),
  })
  await router.push('/')
  await router.isReady()
  const wrapper = mount(AccountMenu, { props, attachTo: document.body, global: { plugins: [router], stubs: { transition: true } } })
  wrappers.push(wrapper)
  return { wrapper, router }
}

beforeEach(() => {
  authStore.user = { username: 'Lollipop', email: 'lol@example.test', role: 'admin', avatar_url: '', balance: 12.5, frozen_balance: 2 }
  authStore.isAdmin = true
  authStore.isSimpleMode = false
  authStore.logout.mockReset().mockResolvedValue(undefined)
  onboardingStore.replay.mockReset()
})
afterEach(() => {
  wrappers.splice(0).forEach((wrapper) => wrapper.unmount())
  document.body.replaceChildren()
  vi.restoreAllMocks()
})

describe('shared account menu', () => {
  it('shows console identity, avatar fallback and role with an accessible disclosure', async () => {
    const { wrapper } = await mountMenu()
    const button = wrapper.get('button.header-user-button')
    expect(wrapper.get('.header-avatar').text()).toBe('LO')
    expect(wrapper.get('.header-identity').text()).toContain('Lollipop')
    expect(wrapper.get('.header-identity').text()).toContain('admin.users.roles.admin')
    expect(button.attributes('aria-expanded')).toBe('false')
    await button.trigger('click')
    expect(button.attributes('aria-controls')).toBe(wrapper.get('.dropdown').attributes('id'))
    expect(wrapper.get('.dropdown').text()).toContain('lol@example.test')
    expect(wrapper.get('.dropdown').text()).toContain('$12.50')
    expect(wrapper.get('.dropdown').text()).toContain('$2.00')
    expect(wrapper.find('a[href="https://github.com/loLollipop/Sub2api"]').exists()).toBe(true)
  })

  it('uses the profile avatar and falls back to the email name', async () => {
    authStore.user.username = ''
    authStore.user.avatar_url = '/avatar.svg'
    const { wrapper } = await mountMenu()
    expect(wrapper.get('.header-avatar img').attributes('alt')).toBe('lol')
    expect(wrapper.get('.header-avatar img').attributes('src')).toBe('/avatar.svg')
  })

  it('dismisses on Escape, outside click and keyboard focus leaving the menu', async () => {
    const { wrapper } = await mountMenu()
    const button = wrapper.get<HTMLButtonElement>('.header-user-button')
    await button.trigger('click')
    await wrapper.get('.dropdown').trigger('keydown', { key: 'Escape' })
    expect(button.attributes('aria-expanded')).toBe('false')
    expect(document.activeElement).toBe(button.element)
    await button.trigger('click')
    document.body.click()
    await flushPromises()
    expect(button.attributes('aria-expanded')).toBe('false')
    await button.trigger('click')
    await wrapper.trigger('focusout', { relatedTarget: document.body })
    expect(button.attributes('aria-expanded')).toBe('false')
  })

  it('keeps console guide replay and closes before profile navigation', async () => {
    const { wrapper, router } = await mountMenu()
    await wrapper.get('.header-user-button').trigger('click')
    await wrapper.findAll('.dropdown button').find((button) => button.text().includes('onboarding.restartTour'))!.trigger('click')
    expect(onboardingStore.replay).toHaveBeenCalledOnce()
    expect(wrapper.find('.dropdown').exists()).toBe(false)
    await wrapper.get('.header-user-button').trigger('click')
    await wrapper.get('a[href="/profile"]').trigger('click')
    await flushPromises()
    expect(router.currentRoute.value.path).toBe('/profile')
    expect(wrapper.find('.dropdown').exists()).toBe(false)
  })

  it('omits mobile balance and console-only guide on Home', async () => {
    const { wrapper } = await mountMenu({ showBalance: false, showOnboarding: false })
    await wrapper.get('.header-user-button').trigger('click')
    expect(wrapper.text()).not.toContain('onboarding.restartTour')
    expect(wrapper.text()).not.toContain('$12.50')
    expect(wrapper.text()).toContain('nav.logout')
  })

  it.each([false, true])('does not show admin actions to a normal user in simple mode=%s', async (simple) => {
    authStore.isAdmin = false
    authStore.user.role = 'user'
    authStore.isSimpleMode = simple
    const { wrapper } = await mountMenu()
    await wrapper.get('.header-user-button').trigger('click')
    expect(wrapper.text()).not.toContain('onboarding.restartTour')
    expect(wrapper.find('a[target="_blank"]').exists()).toBe(false)
  })

  it.each([false, true])('logs out and redirects even if the request fails=%s', async (fails) => {
    if (fails) authStore.logout.mockRejectedValueOnce(new Error('offline'))
    vi.spyOn(console, 'error').mockImplementation(() => {})
    const { wrapper, router } = await mountMenu()
    await wrapper.get('.header-user-button').trigger('click')
    await wrapper.get('button.header-logout').trigger('click')
    await flushPromises()
    expect(authStore.logout).toHaveBeenCalledOnce()
    expect(router.currentRoute.value.path).toBe('/login')
  })
})
