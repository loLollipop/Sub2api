import { DOMWrapper, enableAutoUnmount, flushPromises, mount } from '@vue/test-utils'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import ProfileInfoCard from '@/components/user/profile/ProfileInfoCard.vue'
import type { User } from '@/types'
const { updateProfileMock, authState } = vi.hoisted(() => ({
  updateProfileMock: vi.fn(), authState: { user: null as User | null }
}))
vi.mock('@/api', () => ({ userAPI: { updateProfile: updateProfileMock } }))
enableAutoUnmount(afterEach)
afterEach(() => { document.body.innerHTML = '' })
beforeEach(() => { updateProfileMock.mockReset(); authState.user = null })

vi.mock('vue-router', () => ({
  useRoute: () => ({
    fullPath: '/profile'
  })
}))

vi.mock('@/stores/auth', () => ({
  useAuthStore: () => authState
}))

vi.mock('@/stores/app', () => ({
  useAppStore: () => ({
    showError: vi.fn(),
    showSuccess: vi.fn()
  })
}))

vi.mock('vue-i18n', async (importOriginal) => {
  const actual = await importOriginal<typeof import('vue-i18n')>()
  return {
    ...actual,
    useI18n: () => ({
      t: (key: string, params?: Record<string, string>) => {
        if (key === 'profile.accountBalance') return 'Account Balance'
        if (key === 'profile.concurrencyLimit') return 'Concurrency Limit'
        if (key === 'profile.memberSince') return 'Member Since'
        if (key === 'profile.administrator') return 'Administrator'
        if (key === 'profile.user') return 'User'
        if (key === 'profile.authBindings.providers.email') return 'Email'
        if (key === 'profile.authBindings.providers.linuxdo') return 'LinuxDo'
        if (key === 'profile.authBindings.providers.wechat') return 'WeChat'
        if (key === 'profile.authBindings.providers.oidc') return params?.providerName || 'OIDC'
        if (key === 'profile.authBindings.source.avatar') {
          return `Avatar synced from ${params?.providerName || 'provider'}`
        }
        if (key === 'profile.authBindings.source.username') {
          return `Username synced from ${params?.providerName || 'provider'}`
        }
        return key
      }
    })
  }
})

function createUser(overrides: Partial<User> = {}): User {
  return {
    id: 5,
    username: 'alice',
    email: 'alice@example.com',
    avatar_url: null,
    role: 'user',
    balance: 10,
    concurrency: 2,
    status: 'active',
    allowed_groups: null,
    balance_notify_enabled: true,
    balance_notify_threshold: null,
    balance_notify_extra_emails: [],
    created_at: '2026-04-20T00:00:00Z',
    updated_at: '2026-04-20T00:00:00Z',
    ...overrides
  }
}

describe('ProfileInfoCard', () => {
  it('renders basic account information inside the new overview shell', () => {
    const wrapper = mount(ProfileInfoCard, {
      props: {
        user: createUser()
      },
      global: {
        stubs: {
          Icon: true
        }
      }
    })

    expect(wrapper.text()).toContain('alice@example.com')
    expect(wrapper.text()).toContain('alice')
    expect(wrapper.text()).toContain('User')
    expect(wrapper.find('form').exists()).toBe(false)
    expect(wrapper.get('[data-testid="profile-avatar-open"]').attributes('aria-haspopup')).toBe('dialog')
    expect(wrapper.get('[data-testid="profile-username-open"]').exists()).toBe(true)
  })

  it('renders third-party source hints from profile sources', () => {
    const wrapper = mount(ProfileInfoCard, {
      props: {
        user: createUser({
          avatar_url: 'https://cdn.example.com/linuxdo.png',
          profile_sources: {
            avatar: { provider: 'linuxdo', source: 'linuxdo' },
            username: { provider: 'linuxdo', source: 'linuxdo' }
          }
        })
      },
      global: {
        stubs: {
          Icon: true
        }
      }
    })

    expect(wrapper.text()).toContain('Avatar synced from LinuxDo')
    expect(wrapper.text()).toContain('Username synced from LinuxDo')
  })

  it('uses the configured OIDC provider name in source hints', () => {
    const wrapper = mount(ProfileInfoCard, {
      props: {
        user: createUser({
          profile_sources: {
            username: { provider: 'oidc', source: 'oidc' }
          }
        }),
        oidcProviderName: 'ExampleID'
      },
      global: {
        stubs: {
          Icon: true
        }
      }
    })

    expect(wrapper.text()).toContain('Username synced from ExampleID')
  })

  it('does not display synthetic oauth-only emails as a real bound email', () => {
    const wrapper = mount(ProfileInfoCard, {
      props: {
        user: createUser({
          email: 'legacy-user@oidc-connect.invalid',
          email_bound: false,
          auth_bindings: {
            email: { bound: false }
          }
        })
      },
      global: {
        stubs: {
          Icon: true
        }
      }
    })

    expect(wrapper.text()).not.toContain('legacy-user@oidc-connect.invalid')
  })

  it('does not display synthetic oauth-only emails when only legacy identity bindings mark email as unbound', () => {
    const wrapper = mount(ProfileInfoCard, {
      props: {
        user: createUser({
          email: 'legacy-user@wechat-connect.invalid',
          identity_bindings: {
            email: { bound: false }
          }
        })
      },
      global: {
        stubs: {
          Icon: true
        }
      }
    })

    expect(wrapper.text()).not.toContain('legacy-user@wechat-connect.invalid')
  })

  it('keeps the account summary without duplicated editing panels', () => {
    const wrapper = mount(ProfileInfoCard, {
      props: {
        user: createUser()
      },
      global: {
        stubs: {
          Icon: true
        }
      }
    })

    expect(wrapper.get('[data-testid="profile-overview-hero"]').text()).toContain('alice@example.com')
    expect(wrapper.get('[data-testid="profile-overview-metric-balance"]').text()).toContain('Account Balance')
    expect(wrapper.get('[data-testid="profile-overview-metric-concurrency"]').text()).toContain('Concurrency Limit')
    expect(wrapper.get('[data-testid="profile-overview-metric-member-since"]').text()).toContain('Member Since')
    expect(wrapper.find('[data-testid="profile-info-summary-grid"]').exists()).toBe(false)
    expect(wrapper.find('[data-testid="profile-basics-panel"]').exists()).toBe(false)
    expect(wrapper.find('[data-testid="profile-side-column"]').exists()).toBe(false)
  })

  function mountDialog() {
    mount(ProfileInfoCard, {
      attachTo: document.body,
      props: { user: createUser() },
      global: { stubs: { Icon: true, transition: true } }
    })
    return new DOMWrapper(document.body)
  }

  it('opens username editing on demand and discards a cancelled draft', async () => {
    const wrapper = mountDialog()
    await wrapper.get('[data-testid="profile-username-open"]').trigger('click')
    expect(wrapper.get('#username').element).toHaveProperty('value', 'alice')
    await wrapper.get('#username').setValue('unsaved')
    await wrapper.get('form button[type="button"]').trigger('click')
    expect(wrapper.find('#username').exists()).toBe(false)
    await wrapper.get('[data-testid="profile-username-open"]').trigger('click')
    expect(wrapper.get('#username').element).toHaveProperty('value', 'alice')
    expect(updateProfileMock).not.toHaveBeenCalled()
  })

  it('keeps the username dialog open on error and closes after a successful retry', async () => {
    updateProfileMock.mockRejectedValueOnce(new Error('retry')).mockResolvedValueOnce(createUser({ username: 'new name' }))
    const wrapper = mountDialog()
    await wrapper.get('[data-testid="profile-username-open"]').trigger('click')
    await wrapper.get('#username').setValue('new name')
    expect(wrapper.get('#username').element).toHaveProperty('value', 'new name')
    await wrapper.get('form').trigger('submit')
    expect(updateProfileMock).toHaveBeenCalledWith({ username: 'new name' })
    await flushPromises()
    expect(wrapper.get('#username').element).toHaveProperty('value', 'new name')
    await wrapper.get('form').trigger('submit')
    await flushPromises()
    expect(wrapper.find('#username').exists()).toBe(false)
    expect(authState.user?.username).toBe('new name')
  })

  it('blocks Escape while saving and preserves a newer username draft', async () => {
    let resolve!: (user: User) => void
    updateProfileMock.mockReturnValue(new Promise<User>((done) => { resolve = done }))
    const wrapper = mountDialog()
    await wrapper.get('[data-testid="profile-username-open"]').trigger('click')
    await wrapper.get('#username').setValue('submitted')
    await wrapper.get('form').trigger('submit')
    expect(updateProfileMock).toHaveBeenCalledWith({ username: 'submitted' })
    document.dispatchEvent(new KeyboardEvent('keydown', { key: 'Escape', bubbles: true }))
    await flushPromises()
    expect(wrapper.get('#username').element).toHaveProperty('value', 'submitted')
    await wrapper.get('#username').setValue('newer draft')
    resolve(createUser({ username: 'submitted' }))
    await flushPromises()
    expect(wrapper.get('#username').element).toHaveProperty('value', 'newer draft')
    expect(authState.user?.username).toBe('submitted')
  })

  it('keeps the dialog open when Escape cancels Chinese input composition', async () => {
    const wrapper = mountDialog()
    await wrapper.get('[data-testid="profile-username-open"]').trigger('click')
    const event = new KeyboardEvent('keydown', { key: 'Escape', isComposing: true, bubbles: true, cancelable: true })
    wrapper.get('#username').element.dispatchEvent(event)
    await flushPromises()
    expect(wrapper.find('#username').exists()).toBe(true)
    expect(event.defaultPrevented).toBe(false)
    wrapper.get('#username').element.dispatchEvent(new KeyboardEvent('keydown', { key: 'Escape', bubbles: true }))
    await flushPromises()
    expect(wrapper.find('#username').exists()).toBe(false)
  })

  it('opens the avatar preview without showing a separate editor on the page', async () => {
    const wrapper = mountDialog()
    expect(wrapper.find('[data-testid="profile-avatar-file-input"]').exists()).toBe(false)
    await wrapper.get('[data-testid="profile-avatar-open"]').trigger('click')
    expect(wrapper.find('[data-testid="profile-avatar-file-input"]').exists()).toBe(true)
    expect(wrapper.get('[data-testid="profile-avatar-delete"]').attributes('disabled')).toBeDefined()
    await wrapper.get('[data-testid="profile-avatar-close"]').trigger('click')
    expect(wrapper.find('[data-testid="profile-avatar-file-input"]').exists()).toBe(false)
  })
})
