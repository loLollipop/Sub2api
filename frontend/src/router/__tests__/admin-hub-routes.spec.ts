import { beforeAll, beforeEach, describe, expect, it, vi } from 'vitest'
import type { NavigationGuard, RouteRecordRaw, Router } from 'vue-router'

const harness = vi.hoisted(() => ({
  routes: [] as RouteRecordRaw[],
  guard: null as NavigationGuard | null,
}))
const authStore = vi.hoisted(() => ({
  checkAuth: vi.fn(), isAuthenticated: true, isAdmin: true, isSimpleMode: false,
}))
vi.mock('vue-router', async () => {
  const actual = await vi.importActual<typeof import('vue-router')>('vue-router')
  return {
    ...actual,
    createWebHistory: actual.createMemoryHistory,
    createRouter: (options: Parameters<typeof actual.createRouter>[0]) => {
      harness.routes = options.routes
      const router = actual.createRouter(options)
      const registerGuard = router.beforeEach.bind(router)
      router.beforeEach = guard => {
        harness.guard = guard
        return registerGuard(guard)
      }
      return router
    },
  }
})
vi.mock('@/stores/auth', () => ({ useAuthStore: () => authStore }))
vi.mock('@/stores/app', () => ({
  useAppStore: () => ({ siteName: 'Sub2API', backendModeEnabled: false, cachedPublicSettings: null }),
}))
vi.mock('@/stores/adminSettings', () => ({ useAdminSettingsStore: () => ({ customMenuItems: [] }) }))
vi.mock('@/stores/adminCompliance', () => ({ useAdminComplianceStore: () => ({ initialized: true }) }))
vi.mock('@/composables/useNavigationLoading', () => ({
  useNavigationLoadingState: () => ({ startNavigation: vi.fn(), endNavigation: vi.fn() }),
}))
vi.mock('@/composables/useRoutePrefetch', () => ({
  useRoutePrefetch: () => ({ triggerPrefetch: vi.fn() }),
}))

let router: Router

beforeAll(async () => {
  router = (await import('@/router')).default
})
beforeEach(() => {
  authStore.isAuthenticated = true
  authStore.isAdmin = true
})

describe('admin hub routes', () => {
  it.each(['invites', 'rebates', 'transfers'])('redirects the legacy %s route with query/hash preserved', async tab => {
    const actual = await vi.importActual<typeof import('vue-router')>('vue-router')
    // Use the registered route records with inert views so redirects execute without loading page dependencies.
    const routes = harness.routes.map(route => route.redirect ? route : { ...route, component: { template: '<div />' } })
    const redirectRouter = actual.createRouter({ history: actual.createMemoryHistory(), routes })
    await redirectRouter.push(`/admin/affiliates/${tab}?search=alice&tag=a&tag=b&tab=wrong#records`)
    expect(redirectRouter.currentRoute.value.path).toBe('/admin/affiliates')
    expect(redirectRouter.currentRoute.value.query).toEqual({ search: 'alice', tag: ['a', 'b'], tab })
    expect(redirectRouter.currentRoute.value.hash).toBe('#records')
    expect(redirectRouter.currentRoute.value.meta).toMatchObject({ requiresAuth: true, requiresAdmin: true })
    const name = `AdminAffiliate${tab[0].toUpperCase()}${tab.slice(1)}`
    expect(redirectRouter.hasRoute(name)).toBe(true)
  })

  it.each([
    ['/admin/keys', '/keys', 'AdminKeys'],
    ['/admin/profile', '/profile', 'AdminProfile'],
  ])('adds protected %s using the same view and copy as %s', (adminPath, userPath, name) => {
    const admin = router.resolve(adminPath)
    const user = router.resolve(userPath)
    expect(admin.name).toBe(name)
    expect(admin.meta).toMatchObject({ requiresAuth: true, requiresAdmin: true })
    expect(admin.meta.titleKey).toBe(user.meta.titleKey)
    expect(admin.meta.descriptionKey).toBe(user.meta.descriptionKey)
    expect(user.meta).toMatchObject({ requiresAuth: true, requiresAdmin: false })
    expect(harness.routes.find(route => route.path === adminPath)?.component?.toString())
      .toBe(harness.routes.find(route => route.path === userPath)?.component?.toString())
  })

  it.each(['/admin/affiliates', '/admin/keys', '/admin/profile'])('keeps authentication and administrator guards on %s', async path => {
    const to = router.resolve(path)
    const from = router.resolve('/home')
    const runGuard = async () => {
      const next = vi.fn()
      if (!harness.guard) throw new Error('missing navigation guard')
      await harness.guard.call(undefined, to, from, next)
      return next
    }
    authStore.isAuthenticated = false
    expect(await runGuard()).toHaveBeenCalledWith({ path: '/login', query: { redirect: path } })
    authStore.isAuthenticated = true
    authStore.isAdmin = false
    expect(await runGuard()).toHaveBeenCalledWith('/dashboard')
    authStore.isAdmin = true
    expect(await runGuard()).toHaveBeenCalledWith()
  })
})
