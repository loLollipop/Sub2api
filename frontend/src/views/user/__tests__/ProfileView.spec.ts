import { enableAutoUnmount, flushPromises, mount } from '@vue/test-utils'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { reactive } from 'vue'
import ProfileView from '@/views/user/ProfileView.vue'
enableAutoUnmount(afterEach)

const {
  fetchPublicSettingsMock,
  getDashboardStatsMock,
  refreshUserMock,
  authState
} = vi.hoisted(() => ({
  fetchPublicSettingsMock: vi.fn(),
  getDashboardStatsMock: vi.fn(),
  refreshUserMock: vi.fn(),
  authState: {
    user: null as Record<string, unknown> | null,
    refreshUser: vi.fn()
  }
}))
const reactiveAuthState = reactive(authState)

vi.mock('@/api/usage', () => ({ usageAPI: { getDashboardStats: getDashboardStatsMock } }))

vi.mock('@/stores/auth', () => ({
  useAuthStore: () => reactiveAuthState
}))

vi.mock('@/stores/app', () => ({
  useAppStore: () => ({
    fetchPublicSettings: fetchPublicSettingsMock
  })
}))

vi.mock('@/utils/format', () => ({
  formatDate: () => 'April 2026'
}))

vi.mock('vue-i18n', async (importOriginal) => {
  const actual = await importOriginal<typeof import('vue-i18n')>()
  return {
    ...actual,
    useI18n: () => ({
      t: (key: string) => key
    })
  }
})

describe('ProfileView', () => {
  beforeEach(() => {
    refreshUserMock.mockReset()
    fetchPublicSettingsMock.mockReset()
    getDashboardStatsMock.mockReset().mockResolvedValue({ total_actual_cost: 24.75, total_requests: 1234 })
    refreshUserMock.mockResolvedValue(undefined)
    authState.refreshUser = refreshUserMock
    reactiveAuthState.user = {
      id: 1,
      username: 'alice',
      email: 'alice@example.com',
      role: 'user',
      balance: 10,
      concurrency: 2,
      status: 'active',
      allowed_groups: null,
      balance_notify_enabled: true,
      balance_notify_threshold: null,
      balance_notify_extra_emails: [],
      created_at: '2026-04-20T00:00:00Z',
      updated_at: '2026-04-20T00:00:00Z'
    }
    fetchPublicSettingsMock.mockResolvedValue({
      contact_info: '',
      balance_low_notify_enabled: false,
      balance_low_notify_threshold: 0,
      linuxdo_oauth_enabled: true,
      dingtalk_oauth_enabled: true,
      totp_enabled: true,
      passkey_enabled: true,
      wechat_oauth_enabled: true,
      wechat_oauth_open_enabled: true,
      wechat_oauth_mp_enabled: false,
      oidc_oauth_enabled: true,
      oidc_oauth_provider_name: 'OIDC'
    })
  })

  function mountProfile() {
    const wrapper = mount(ProfileView, {
      global: {
        stubs: {
          AppLayout: { template: '<div><slot /></div>' },
          StatCard: { template: '<div class="stat-card" />' },
          ProfileInfoCard: { props: ['stats', 'statsState'], template: '<div data-testid="profile-info-card" />' },
          ProfileIdentityBindingsSection: { props: ['linuxdoEnabled', 'dingtalkEnabled', 'wechatEnabled', 'wechatOpenEnabled', 'wechatMpEnabled', 'oidcEnabled', 'oidcProviderName'], template: '<div data-testid="profile-bindings" />' },
          ProfileBalanceNotifyCard: { props: { embedded: Boolean }, template: '<div data-testid="profile-balance-notify-card" />' },
          ProfilePasswordForm: { template: '<div data-testid="profile-password-form" />' },
          ProfileTotpCard: { template: '<div data-testid="profile-totp-card" />' },
          ProfilePasskeyCard: { props: ['enabled'], template: '<div data-testid="profile-passkey-card" />' },
          teleport: true,
          Icon: true
        }
      }
    })

    return wrapper
  }

  it('keeps security and login capabilities while showing the password form on demand', async () => {
    const wrapper = mountProfile()
    await flushPromises()

    expect(wrapper.findAll('.stat-card')).toHaveLength(0)
    expect(wrapper.get('[data-testid="profile-shell"]').exists()).toBe(true)
    expect(wrapper.get('[data-testid="profile-shell"]').html()).toContain('profile-info-card')
    expect(wrapper.find('[data-testid="profile-password-form"]').exists()).toBe(false)
    expect(wrapper.get('[data-testid="profile-shell"]').html()).toContain('profile-totp-card')
    expect(wrapper.findComponent('[data-testid="profile-bindings"]').props()).toMatchObject({
      linuxdoEnabled: true, dingtalkEnabled: true, oidcEnabled: true, wechatEnabled: true,
      wechatOpenEnabled: true, wechatMpEnabled: false, oidcProviderName: 'OIDC'
    })
    expect(wrapper.findComponent('[data-testid="profile-passkey-card"]').props('enabled')).toBe(true)
    await wrapper.get('[data-testid="profile-password-open"]').trigger('click')
    expect(wrapper.find('[data-testid="profile-password-form"]').exists()).toBe(true)
    wrapper.findComponent('[data-testid="profile-password-form"]').vm.$emit('saved')
    await flushPromises()
    expect(wrapper.find('[data-testid="profile-password-form"]').exists()).toBe(false)
  })

  it.each([false, undefined])('hides security features when the setting is %s', async enabled => {
    fetchPublicSettingsMock.mockResolvedValue({ balance_low_notify_enabled: true, balance_low_notify_threshold: 8,
      totp_enabled: enabled, passkey_enabled: enabled })
    const wrapper = mountProfile()
    await flushPromises()
    expect(wrapper.find('[data-testid="profile-balance-notify-card"]').exists()).toBe(true)
    expect(wrapper.get('[data-testid="profile-auth-bindings-panel"] [data-testid="profile-balance-notify-card"]').exists()).toBe(true)
    expect(wrapper.findComponent('[data-testid="profile-balance-notify-card"]').props('embedded')).toBe(true)
    expect(wrapper.find('.profile-security-panel [data-testid="profile-balance-notify-card"]').exists()).toBe(false)
    expect(wrapper.findComponent('[data-testid="profile-bindings"]').props()).toMatchObject({
      linuxdoEnabled: false, dingtalkEnabled: false, oidcEnabled: false, wechatEnabled: false
    })
    expect(wrapper.find('[data-testid="profile-totp-card"]').exists()).toBe(false)
    expect(wrapper.find('[data-testid="profile-passkey-card"]').exists()).toBe(false)
    expect(wrapper.find('[data-testid="profile-password-open"]').exists()).toBe(true)
  })

  it('does not mount optional security controls before settings load or when loading fails', async () => {
    let rejectSettings!: (error: Error) => void
    fetchPublicSettingsMock.mockReturnValue(new Promise((_, reject) => { rejectSettings = reject }))
    const errorLog = vi.spyOn(console, 'error').mockImplementation(() => {})
    const wrapper = mountProfile()
    expect(wrapper.find('[data-testid="profile-totp-card"]').exists()).toBe(false)
    expect(wrapper.find('[data-testid="profile-passkey-card"]').exists()).toBe(false)
    rejectSettings(new Error('Settings unavailable'))
    await flushPromises()
    expect(wrapper.find('[data-testid="profile-totp-card"]').exists()).toBe(false)
    expect(wrapper.find('[data-testid="profile-passkey-card"]').exists()).toBe(false)
    expect(wrapper.find('[data-testid="profile-password-open"]').exists()).toBe(true)
    errorLog.mockRestore()
  })

  it('loads real usage totals independently of settings and starts with unavailable values', async () => {
    let resolveStats!: (stats: { total_actual_cost: number; total_requests: number }) => void
    getDashboardStatsMock.mockReturnValueOnce(new Promise(resolve => { resolveStats = resolve }))
    const wrapper = mountProfile()
    const info = wrapper.findComponent('[data-testid="profile-info-card"]')
    expect(info.props()).toMatchObject({ stats: null, statsState: 'loading' })
    await flushPromises()
    expect(wrapper.find('[data-testid="profile-password-open"]').exists()).toBe(true)
    expect(wrapper.find('[data-testid="profile-balance-notify-card"]').exists()).toBe(false)
    resolveStats({ total_actual_cost: 24.75, total_requests: 1234 })
    await flushPromises()
    expect(getDashboardStatsMock).toHaveBeenCalledTimes(1)
    expect(info.props()).toMatchObject({ stats: { total_actual_cost: 24.75, total_requests: 1234 }, statsState: 'ready' })
  })

  it('keeps account settings usable when usage statistics fail', async () => {
    getDashboardStatsMock.mockRejectedValueOnce(new Error('Usage unavailable'))
    fetchPublicSettingsMock.mockResolvedValueOnce({ balance_low_notify_enabled: true })
    const wrapper = mountProfile()
    await flushPromises()
    expect(wrapper.findComponent('[data-testid="profile-info-card"]').props()).toMatchObject({ stats: null, statsState: 'error' })
    expect(wrapper.get('[data-testid="profile-auth-bindings-panel"] [data-testid="profile-balance-notify-card"]').exists()).toBe(true)
    await wrapper.get('[data-testid="profile-password-open"]').trigger('click')
    expect(wrapper.find('[data-testid="profile-password-form"]').exists()).toBe(true)
  })

  it('clears old account totals and ignores responses from the previous account', async () => {
    let resolveOld!: (stats: { total_actual_cost: number; total_requests: number }) => void
    let resolveNew!: (stats: { total_actual_cost: number; total_requests: number }) => void
    getDashboardStatsMock
      .mockReturnValueOnce(new Promise(resolve => { resolveOld = resolve }))
      .mockReturnValueOnce(new Promise(resolve => { resolveNew = resolve }))
    const wrapper = mountProfile()
    reactiveAuthState.user = { ...reactiveAuthState.user, id: 2 }
    await flushPromises()
    resolveNew({ total_actual_cost: 3, total_requests: 8 })
    await flushPromises()
    resolveOld({ total_actual_cost: 100, total_requests: 900 })
    await flushPromises()
    const info = wrapper.findComponent('[data-testid="profile-info-card"]')
    expect(info.props()).toMatchObject({ stats: { total_actual_cost: 3, total_requests: 8 }, statsState: 'ready' })
    reactiveAuthState.user = null
    await flushPromises()
    expect(info.props()).toMatchObject({ stats: null, statsState: 'loading' })
    expect(getDashboardStatsMock).toHaveBeenCalledTimes(2)
  })

  it('discards a pending usage response after unmount', async () => {
    let resolveStats!: (stats: { total_actual_cost: number; total_requests: number }) => void
    getDashboardStatsMock.mockReturnValueOnce(new Promise(resolve => { resolveStats = resolve }))
    const wrapper = mountProfile()
    const state = (wrapper.vm as unknown as { $: { setupState: { profileStats: unknown; statsState: string } } }).$.setupState
    wrapper.unmount()
    resolveStats({ total_actual_cost: 99, total_requests: 99 })
    await flushPromises()
    expect(state.profileStats).toBeNull()
    expect(state.statsState).toBe('loading')
  })
})
