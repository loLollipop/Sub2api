import { beforeAll, beforeEach, describe, expect, it, vi } from 'vitest'

type NavigationGuard = (
  to: Record<string, any>,
  from: Record<string, any>,
  next: ReturnType<typeof vi.fn>
) => Promise<void>

const routerHarness = vi.hoisted(() => ({
  guard: null as NavigationGuard | null,
  paths: [] as string[],
}))

const authStore = vi.hoisted(() => ({
  checkAuth: vi.fn(),
  isAuthenticated: true,
  isAdmin: false,
  isSimpleMode: false,
  hasPendingAuthSession: false,
}))

const appStore = vi.hoisted(() => ({
  siteName: 'Sub2API',
  backendModeEnabled: false,
  publicSettingsLoaded: false,
  cachedPublicSettings: null as null | {
    purchase_subscription_enabled?: boolean
    risk_control_enabled?: boolean
    custom_menu_items?: []
    leaderboard_enabled?: boolean
    leaderboard_require_auth?: boolean
  },
  fetchPublicSettings: vi.fn(),
}))

vi.mock('vue-router', () => ({
  createWebHistory: vi.fn(() => ({})),
  createRouter: vi.fn((config: { routes: { path: string }[] }) => {
    routerHarness.paths = config.routes.map(route => route.path)
    return ({
    beforeEach: vi.fn((guard: NavigationGuard) => {
      routerHarness.guard = guard
    }),
    afterEach: vi.fn(),
    onError: vi.fn(),
  })}),
}))

vi.mock('@/stores/auth', () => ({
  useAuthStore: () => authStore,
}))

vi.mock('@/stores/app', () => ({
  useAppStore: () => appStore,
}))

vi.mock('@/stores/adminSettings', () => ({
  useAdminSettingsStore: () => ({ customMenuItems: [] }),
}))

vi.mock('@/stores/adminCompliance', () => ({
  useAdminComplianceStore: () => ({
    initialized: true,
    fetchStatus: vi.fn(),
    requireAcknowledgement: vi.fn(),
  }),
}))

vi.mock('@/composables/useNavigationLoading', () => ({
  useNavigationLoadingState: () => ({
    startNavigation: vi.fn(),
    endNavigation: vi.fn(),
    isLoading: { value: false },
  }),
}))

vi.mock('@/composables/useRoutePrefetch', () => ({
  useRoutePrefetch: () => ({
    triggerPrefetch: vi.fn(),
    cancelPendingPrefetch: vi.fn(),
    resetPrefetchState: vi.fn(),
  }),
}))

function createDeferred<T>() {
  let resolve!: (value: T | PromiseLike<T>) => void
  const promise = new Promise<T>((resolvePromise) => {
    resolve = resolvePromise
  })
  return { promise, resolve }
}

function runGuard(meta: Record<string, unknown>, path: string) {
  if (!routerHarness.guard) {
    throw new Error('router guard was not registered')
  }

  const next = vi.fn()
  const navigation = routerHarness.guard(
    {
      path,
      fullPath: path,
      name: 'FeatureRoute',
      params: {},
      meta: { requiresAuth: true, ...meta },
    },
    {},
    next
  )
  return { navigation, next }
}

describe('feature route guard', () => {
  beforeAll(async () => {
    await import('@/router')
  })

  beforeEach(() => {
    authStore.isAuthenticated = true
    authStore.isAdmin = false
    authStore.isSimpleMode = false
    appStore.backendModeEnabled = false
    appStore.publicSettingsLoaded = false
    appStore.cachedPublicSettings = null
    appStore.fetchPublicSettings.mockReset()
  })

  it('registers the public leaderboard and hides the route when explicitly disabled', async () => {
    expect(routerHarness.paths).toContain('/leaderboard')
    appStore.publicSettingsLoaded = true
    appStore.cachedPublicSettings = { leaderboard_enabled: false }
    const { navigation, next } = runGuard({ requiresAuth: false }, '/leaderboard')
    await navigation
    expect(next).toHaveBeenCalledWith('/dashboard')
  })

  it('preserves the leaderboard destination when sign-in is required', async () => {
    authStore.isAuthenticated = false
    appStore.publicSettingsLoaded = true
    appStore.cachedPublicSettings = { leaderboard_enabled: true, leaderboard_require_auth: true }
    const { navigation, next } = runGuard({ requiresAuth: false }, '/leaderboard')
    await navigation
    expect(next).toHaveBeenCalledWith({ path: '/login', query: { redirect: '/leaderboard' } })
  })

  it('allows anonymous leaderboard access only when public and outside backend-only mode', async () => {
    authStore.isAuthenticated = false
    appStore.publicSettingsLoaded = true
    appStore.cachedPublicSettings = { leaderboard_enabled: true, leaderboard_require_auth: false }
    const publicVisit = runGuard({ requiresAuth: false }, '/leaderboard')
    await publicVisit.navigation
    expect(publicVisit.next).toHaveBeenCalledWith()
    appStore.backendModeEnabled = true
    const backendVisit = runGuard({ requiresAuth: false }, '/leaderboard')
    await backendVisit.navigation
    expect(backendVisit.next).toHaveBeenCalledWith('/login')
  })

  it('removes native checkout routes while retaining shop, redemption, subscriptions and WeChat login', () => {
    expect(routerHarness.paths).toEqual(expect.arrayContaining(['/purchase', '/redeem', '/subscriptions', '/auth/wechat/callback']))
    expect(routerHarness.paths.some(path => path.startsWith('/payment/') || path.startsWith('/admin/orders') || path === '/orders' || path === '/auth/wechat/payment/callback')).toBe(false)
  })

  it('waits for the first public-settings request before deciding payment access', async () => {
    const deferred = createDeferred<{ purchase_subscription_enabled: boolean }>()
    appStore.fetchPublicSettings.mockImplementation(async () => {
      const settings = await deferred.promise
      appStore.cachedPublicSettings = settings
      appStore.publicSettingsLoaded = true
      return settings
    })

    const { navigation, next } = runGuard({ requiresPurchase: true }, '/purchase')

    await vi.waitFor(() => expect(appStore.fetchPublicSettings).toHaveBeenCalledTimes(1))
    expect(next).not.toHaveBeenCalled()

    deferred.resolve({ purchase_subscription_enabled: true })
    await navigation
    expect(next).toHaveBeenCalledOnce()
    expect(next).toHaveBeenCalledWith()
  })

  it.each([
    ['payment', { requiresPurchase: true }, '/purchase'],
    ['risk control', { requiresRiskControl: true }, '/admin/risk-control'],
  ])('does not treat a failed %s settings load as explicitly disabled', async (_name, meta, path) => {
    authStore.isAdmin = meta.requiresRiskControl === true
    appStore.fetchPublicSettings.mockResolvedValue(null)

    const { navigation, next } = runGuard(meta, path)
    await navigation

    expect(appStore.publicSettingsLoaded).toBe(false)
    expect(next).toHaveBeenCalledOnce()
    expect(next).toHaveBeenCalledWith()
  })

  it.each([
    ['purchase', { requiresPurchase: true }, { purchase_subscription_enabled: false }, '/dashboard'],
    [
      'risk control',
      { requiresRiskControl: true },
      { risk_control_enabled: false },
      '/admin/settings',
    ],
  ])('redirects when loaded settings explicitly disable %s', async (_name, meta, settings, target) => {
    authStore.isAdmin = meta.requiresRiskControl === true
    appStore.cachedPublicSettings = settings
    appStore.publicSettingsLoaded = true

    const { navigation, next } = runGuard(meta, '/feature')
    await navigation

    expect(appStore.fetchPublicSettings).not.toHaveBeenCalled()
    expect(next).toHaveBeenCalledOnce()
    expect(next).toHaveBeenCalledWith(target)
  })

  it('allows purchases when the external shop is enabled', async () => {
    appStore.publicSettingsLoaded = true
    appStore.cachedPublicSettings = { purchase_subscription_enabled: true }
    const purchase = runGuard({ requiresPurchase: true }, '/purchase')
    await purchase.navigation
    expect(purchase.next).toHaveBeenCalledWith()
  })
})
