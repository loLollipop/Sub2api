import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { flushPromises, mount, type VueWrapper } from '@vue/test-utils'
import { createPinia, setActivePinia } from 'pinia'
import { createI18n } from 'vue-i18n'
import { defineComponent, h, nextTick } from 'vue'
import { createMemoryHistory, createRouter, RouterView } from 'vue-router'
import type { Config } from 'driver.js'
import { useAuthStore } from '@/stores/auth'
import { useOnboardingStore } from '@/stores/onboarding'
import AppLayout from '../AppLayout.vue'

const { createDriver } = vi.hoisted(() => ({ createDriver: vi.fn() }))
vi.mock('driver.js', () => ({ driver: createDriver }))

const browsePaths = ['/model-plaza', '/available-channels', '/monitor', '/leaderboard']
let wrapper: VueWrapper | undefined

async function renderLayout(path: string, role: 'admin' | 'user' = 'admin', reuseLayout = false, simpleMode = false) {
  const pinia = createPinia()
  setActivePinia(pinia)
  const auth = useAuthStore()
  auth.user = { id: 1, role } as NonNullable<typeof auth.user>
  if (simpleMode) vi.spyOn(auth, 'isSimpleMode', 'get').mockReturnValue(true)
  const page = defineComponent({ setup: () => () => h(AppLayout) })
  const router = createRouter({
    history: createMemoryHistory(),
    routes: [...browsePaths, '/admin/dashboard', '/dashboard'].map(routePath => ({
      path: routePath,
      // Distinct route components reproduce view-owned layouts being replaced.
      component: reuseLayout ? page : defineComponent({ setup: () => () => h(AppLayout) })
    }))
  })
  const startedPaths: string[] = []
  createDriver.mockImplementation((config: Config) => {
    let active = false
    return {
      drive: vi.fn(() => {
        active = true
        startedPaths.push(router.currentRoute.value.path)
      }),
      destroy: vi.fn(() => {
        active = false
        const onDestroyed = config.onDestroyed as (() => void) | undefined
        onDestroyed?.()
      }),
      isActive: () => active
    }
  })
  await router.push(path)
  await router.isReady()
  wrapper = mount(RouterView, {
    global: {
      plugins: [pinia, router, createI18n({
        legacy: false, locale: 'en', missingWarn: false, fallbackWarn: false, messages: { en: {} }
      })],
      stubs: {
        AppHeader: true,
        AppSidebar: { template: '<aside><a id="sidebar-group-manage" /><a data-tour="sidebar-my-keys" /></aside>' }
      }
    }
  })
  return { router, onboarding: useOnboardingStore(), auth, startedPaths }
}

beforeEach(() => {
  vi.useFakeTimers()
  createDriver.mockReset()
  localStorage.clear()
})

afterEach(() => {
  useOnboardingStore().getDriverInstance()?.destroy()
  wrapper?.unmount()
  wrapper = undefined
  vi.restoreAllMocks()
  vi.useRealTimers()
})

describe('AppLayout onboarding', () => {
  it.each(browsePaths)('does not automatically start the admin tour on %s', async path => {
    await renderLayout(path)
    await vi.advanceTimersByTimeAsync(1500)
    expect(createDriver).not.toHaveBeenCalled()
    expect(wrapper!.find('.browse-layout').exists()).toBe(true)
  })

  it('still automatically starts unseen admin tours in the management console', async () => {
    const { startedPaths } = await renderLayout('/admin/dashboard')
    await vi.advanceTimersByTimeAsync(1000)
    expect(startedPaths).toEqual(['/admin/dashboard'])
    expect(createDriver.mock.calls[0][0].steps[1].element).toBe('#sidebar-group-manage')
  })

  it('does not automatically replay a previously seen tour', async () => {
    localStorage.setItem('admin_guide_1_admin_v4_interactive', 'true')
    await renderLayout('/admin/dashboard')
    await vi.advanceTimersByTimeAsync(1000)
    expect(createDriver).not.toHaveBeenCalled()
  })

  it('keeps automatic tours disabled in simple mode', async () => {
    await renderLayout('/admin/dashboard', 'admin', false, true)
    await vi.advanceTimersByTimeAsync(1000)
    expect(createDriver).not.toHaveBeenCalled()
  })

  it('keeps automatic tours disabled for regular users', async () => {
    await renderLayout('/dashboard', 'user')
    await vi.advanceTimersByTimeAsync(1000)
    expect(createDriver).not.toHaveBeenCalled()
  })

  it('lets administrators preview the user workspace without starting the management tour', async () => {
    const { onboarding, startedPaths } = await renderLayout('/dashboard')
    await vi.advanceTimersByTimeAsync(1500)
    expect(createDriver).not.toHaveBeenCalled()
    onboarding.replay()
    await flushPromises()
    expect(startedPaths).toEqual(['/admin/dashboard'])
  })

  it.each(['admin', 'user'] as const)('replays %s tours once after replacing the browse layout', async role => {
    const { router, onboarding, startedPaths } = await renderLayout('/model-plaza', role)
    const push = vi.spyOn(router, 'push')
    const destination = role === 'admin' ? '/admin/dashboard' : '/dashboard'
    localStorage.setItem(`${role}_guide_1_${role}_v4_interactive`, 'true')

    onboarding.replay()
    onboarding.replay()
    expect(createDriver).not.toHaveBeenCalled()
    await flushPromises()
    await nextTick()

    expect(push).toHaveBeenCalledTimes(1)
    expect(push).toHaveBeenCalledWith(destination)
    expect(router.currentRoute.value.path).toBe(destination)
    expect(wrapper!.find('.browse-layout').exists()).toBe(false)
    expect(startedPaths).toEqual([destination])
    expect(localStorage.getItem(`${role}_guide_1_${role}_v4_interactive`)).toBeNull()
    await vi.advanceTimersByTimeAsync(1500)
    expect(startedPaths).toEqual([destination])
  })

  it('cancels a fresh dashboard auto-start timer when handing off manual replay', async () => {
    const { onboarding, startedPaths } = await renderLayout('/monitor')
    onboarding.replay()
    await flushPromises()
    await vi.advanceTimersByTimeAsync(1500)
    expect(startedPaths).toEqual(['/admin/dashboard'])
  })

  it('also replays once when navigation keeps the current layout mounted', async () => {
    const { onboarding, startedPaths } = await renderLayout('/available-channels', 'admin', true)
    onboarding.replay()
    await flushPromises()
    await vi.advanceTimersByTimeAsync(1500)
    expect(startedPaths).toEqual(['/admin/dashboard'])
  })

  it('does not start on aborted navigation and allows another replay attempt', async () => {
    const { router, onboarding, startedPaths } = await renderLayout('/monitor')
    const removeGuard = router.beforeEach(() => false)
    onboarding.replay()
    await flushPromises()
    expect(router.currentRoute.value.path).toBe('/monitor')
    expect(createDriver).not.toHaveBeenCalled()
    removeGuard()
    onboarding.replay()
    await flushPromises()
    expect(startedPaths).toEqual(['/admin/dashboard'])
  })

  it('checks the current route again before an auto-start timer fires', async () => {
    const { router } = await renderLayout('/admin/dashboard', 'admin', true)
    await router.push('/model-plaza')
    await vi.advanceTimersByTimeAsync(1500)
    expect(createDriver).not.toHaveBeenCalled()
  })

  it('does not start from a stale replay callback after its layout is unmounted', async () => {
    await renderLayout('/admin/dashboard')
    const replay = (wrapper!.findComponent(AppLayout).vm as unknown as { replayTour: () => void }).replayTour
    wrapper!.unmount()
    wrapper = undefined
    replay()
    await vi.advanceTimersByTimeAsync(1500)
    expect(createDriver).not.toHaveBeenCalled()
  })

  it('drops a queued tour start when its layout unmounts before the DOM is ready', async () => {
    const { onboarding } = await renderLayout('/admin/dashboard')
    onboarding.replay()
    wrapper!.unmount()
    wrapper = undefined
    await flushPromises()
    await vi.advanceTimersByTimeAsync(1500)
    expect(createDriver).not.toHaveBeenCalled()
  })

  it('starts only once for repeated manual replay requests in the console', async () => {
    const { onboarding, startedPaths } = await renderLayout('/admin/dashboard')
    onboarding.replay()
    onboarding.replay()
    await flushPromises()
    await vi.advanceTimersByTimeAsync(1500)
    expect(startedPaths).toEqual(['/admin/dashboard'])
  })
})
