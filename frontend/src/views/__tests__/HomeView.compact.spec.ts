import { beforeEach, describe, expect, it, vi } from 'vitest'
import { mount, RouterLinkStub } from '@vue/test-utils'

import HomeView from '../HomeView.vue'

const { appStore, authStore } = vi.hoisted(() => ({
  appStore: {
    cachedPublicSettings: {} as Record<string, unknown>,
    siteName: 'Fallback site',
    siteLogo: '',
    docUrl: '',
    apiBaseUrl: '',
    publicSettingsLoaded: true,
    fetchPublicSettings: vi.fn(),
    showSuccess: vi.fn(),
    showError: vi.fn(),
  },
  authStore: {
    isAuthenticated: false,
    isAdmin: false,
    user: null as { email?: string } | null,
    checkAuth: vi.fn(),
  },
}))

const { copyToClipboard } = vi.hoisted(() => ({ copyToClipboard: vi.fn().mockResolvedValue(true) }))
vi.mock('@/composables/useClipboard', async () => {
  const { ref } = await import('vue')
  return { useClipboard: () => ({ copied: ref(false), copyToClipboard }) }
})

vi.mock('@/stores', () => ({
  useAppStore: () => appStore,
  useAuthStore: () => authStore,
}))

vi.mock('@/stores/app', () => ({
  useAppStore: () => appStore,
}))

vi.mock('vue-i18n', async (importOriginal) => {
  const actual = await importOriginal<typeof import('vue-i18n')>()
  return {
    ...actual,
    useI18n: () => ({ t: (key: string) => key }),
  }
})

function mountHome(settings: Record<string, unknown> = {}) {
  appStore.cachedPublicSettings = {
    site_name: 'Test site',
    site_subtitle: 'Test subtitle',
    ...settings,
  }

  return mount(HomeView, {
    global: {
      stubs: {
        RouterLink: RouterLinkStub,
        LocaleSwitcher: { template: '<div data-testid="locale-switcher" />' },
        Icon: { template: '<span data-testid="icon" />' },
      },
    },
  })
}

function compactDestination(wrapper: ReturnType<typeof mountHome>) {
  return wrapper.get('[data-testid="compact-home"]').findComponent(RouterLinkStub).props('to')
}

describe('HomeView compact mode', () => {
  beforeEach(() => {
    authStore.isAuthenticated = false
    authStore.isAdmin = false
    authStore.user = null
    authStore.checkAuth.mockClear()
    appStore.fetchPublicSettings.mockClear()
    copyToClipboard.mockClear()
    localStorage.clear()
    document.documentElement.classList.remove('dark')
    vi.spyOn(window, 'matchMedia').mockReturnValue({ matches: false } as MediaQueryList)
  })

  it('renders custom HTML ahead of compact mode', () => {
    const wrapper = mountHome({
      compact_home_enabled: true,
      home_content: '<section id="custom-home">Custom home</section>',
    })

    expect(wrapper.get('#custom-home').text()).toBe('Custom home')
    expect(wrapper.find('[data-testid="compact-home"]').exists()).toBe(false)
  })

  it('renders custom URL content ahead of compact mode', () => {
    const wrapper = mountHome({
      compact_home_enabled: true,
      home_content: ' https://example.com/home ',
    })

    expect(wrapper.get('iframe').attributes('src')).toBe('https://example.com/home')
    expect(wrapper.find('[data-testid="compact-home"]').exists()).toBe(false)
  })

  it('treats whitespace-only custom content as empty and selects compact mode', () => {
    const wrapper = mountHome({ compact_home_enabled: true, home_content: ' \n\t ' })

    expect(wrapper.get('[data-testid="compact-home"]').text()).toContain('Test site')
  })

  it.each([undefined, false])('selects the default home when compact mode is %s', (enabled) => {
    const settings = enabled === undefined ? {} : { compact_home_enabled: enabled }
    const wrapper = mountHome(settings)

    expect(wrapper.find('[data-testid="compact-home"]').exists()).toBe(false)
    expect(wrapper.find('[data-testid="default-home"]').exists()).toBe(true)
  })

  it('links unauthenticated visitors to login', () => {
    expect(compactDestination(mountHome({ compact_home_enabled: true }))).toBe('/login')
  })

  it('links authenticated users to their dashboard', () => {
    authStore.isAuthenticated = true

    expect(compactDestination(mountHome({ compact_home_enabled: true }))).toBe('/dashboard')
  })

  it('links administrators to the admin dashboard', () => {
    authStore.isAuthenticated = true
    authStore.isAdmin = true

    const wrapper = mountHome({ compact_home_enabled: true })
    expect(compactDestination(wrapper)).toBe('/admin/dashboard')
    expect(authStore.checkAuth).toHaveBeenCalledOnce()
    expect(appStore.fetchPublicSettings).not.toHaveBeenCalled()
  })

  it.each([false, true])('removes model plaza and integration links with compact mode=%s', (compact) => {
    authStore.isAuthenticated = true
    const wrapper = mountHome({ compact_home_enabled: compact, model_plaza_enabled: true })
    expect(wrapper.findAllComponents(RouterLinkStub).some((link) => link.props('to') === '/model-plaza')).toBe(false)
    expect(wrapper.find('a[href="#integration"]').exists()).toBe(false)
  })

  it.each([undefined, null, '', ' \n\t '])('does not render a blank or missing subtitle (%s) in either mode', (subtitle) => {
    const defaultHome = mountHome({ site_subtitle: subtitle })
    expect(defaultHome.find('.landing-subtitle').exists()).toBe(false)
    expect(defaultHome.find('.landing-headline').exists()).toBe(false)
    expect(defaultHome.text()).not.toContain('AI API Gateway Platform')
    const compact = mountHome({ compact_home_enabled: true, site_subtitle: subtitle })
    expect(compact.find('main > div > p').exists()).toBe(false)
  })

  it('lets the default feature icons inherit the marks theme colors', () => {
    const wrapper = mountHome()
    const marks = wrapper.findAll('.home-feature-mark')

    expect(marks).toHaveLength(3)
    for (const mark of marks) {
      expect(mark.get('[data-testid="icon"]').classes()).not.toContain('text-white')
    }
  })

  it('replaces the static provider grid and animated terminal with a compact API window', () => {
    const wrapper = mountHome()

    expect(wrapper.find('.home-provider-list').exists()).toBe(false)
    expect(wrapper.find('.terminal-container').exists()).toBe(false)
    expect(wrapper.text()).not.toContain('home.providers.title')
    expect(wrapper.get('[data-testid="home-api-preview"]').attributes('aria-label')).toBe('home.landing.previewTitle')
    expect(wrapper.get('.api-preview__window-title').text()).toBe('OpenAI · Responses API')
    expect(wrapper.find('.api-preview__example').exists()).toBe(false)
    expect(wrapper.findAll('.api-preview__window-dots > span')).toHaveLength(3)
    expect(wrapper.findAll('.landing-feature')).toHaveLength(3)
    expect(wrapper.find('.landing-sticker').exists()).toBe(false)
    expect(wrapper.find('.api-preview__guide').exists()).toBe(false)
    expect(wrapper.get('.landing-hero h1').text()).toBe('Test site.')
    expect(wrapper.get('[data-testid="home-code-cursor"]').attributes('aria-hidden')).toBe('true')
  })

  it.each([
    [false, false, '/login'],
    [true, false, '/dashboard'],
    [true, true, '/admin/dashboard'],
  ])('keeps the default CTA destination for authenticated=%s admin=%s', (authenticated, admin, path) => {
    authStore.isAuthenticated = authenticated as boolean
    authStore.isAdmin = admin as boolean
    authStore.user = { email: '  lol@example.test' }
    const wrapper = mountHome()

    expect(wrapper.getComponent('[data-testid="home-primary-cta"]').props('to')).toBe(path)
    const accountEntry = wrapper.getComponent('[data-testid="home-account-entry"]')
    expect(accountEntry.props('to')).toBe(path)
    expect(accountEntry.text()).toContain(authenticated ? 'home.dashboard' : 'home.login')
    if (authenticated) expect(accountEntry.get('.landing-account-mark').text()).toBe('L')
    else expect(accountEntry.find('.landing-account-mark').exists()).toBe(false)
  })

  it('switches the API example format without making an API request', async () => {
    const wrapper = mountHome()
    expect(wrapper.get('.api-preview__code').text()).toContain('/v1/responses')

    await wrapper.get('[data-testid="home-protocol-chat"]').trigger('click')

    expect(wrapper.get('.api-preview__code').text()).toContain('/v1/chat/completions')
    expect(wrapper.get('.api-preview__code').text()).toContain('"messages"')
    expect(wrapper.get('[data-testid="home-protocol-chat"]').attributes('aria-pressed')).toBe('true')
    expect(wrapper.get('[data-testid="home-protocol-responses"]').attributes('aria-pressed')).toBe('false')
    expect(appStore.fetchPublicSettings).not.toHaveBeenCalled()
  })

  it.each(['https://api.example.test', 'https://api.example.test/v1/'])('uses configured API base %s in displayed and copied examples', async (base) => {
    const wrapper = mountHome({ api_base_url: base })
    expect(wrapper.get('.api-preview__code').text()).toContain("curl 'https://api.example.test/v1/responses'")

    await wrapper.get('[data-testid="home-protocol-chat"]').trigger('click')
    await wrapper.get('[data-testid="home-copy-example"]').trigger('click')

    expect(copyToClipboard).toHaveBeenCalledWith(wrapper.get('.api-preview__code').text())
    expect(copyToClipboard.mock.calls[0][0]).toContain('https://api.example.test/v1/chat/completions')
    expect(copyToClipboard.mock.calls[0][0]).not.toContain('/v1/v1/')
  })

  it('falls back to the current origin when the API address is invalid', () => {
    const wrapper = mountHome({ api_base_url: 'javascript:alert(1)' })
    expect(wrapper.get('.api-preview__code').text()).toContain(`${window.location.origin}/v1/responses`)
  })

  it.each([
    ['https://gateway.test/api?tenant=blue', 'https://gateway.test/api/v1/responses?tenant=blue'],
    ['https://gateway.test/api/v1/?tenant=blue#fragment', 'https://gateway.test/api/v1/responses?tenant=blue'],
    ['https://gateway.test/api#fragment', 'https://gateway.test/api/v1/responses'],
  ])('appends the endpoint to the pathname for configured API base %s', async (base, expected) => {
    const wrapper = mountHome({ api_base_url: base })
    expect(wrapper.get('.api-preview__code').text()).toContain(`curl '${expected}'`)
    await wrapper.get('[data-testid="home-copy-example"]').trigger('click')
    expect(copyToClipboard).toHaveBeenCalledWith(wrapper.get('.api-preview__code').text())
  })

  it('keeps highlighting text-only and copied cURL safe for a quoted URL', async () => {
    const wrapper = mountHome({ api_base_url: "https://gateway.test/team's" })
    const code = wrapper.get('.api-preview__code')
    expect(code.get('.syntax-url').text()).toBe("'https://gateway.test/team%27s/v1/responses'")
    expect(code.get('.syntax-command').text()).toBe('curl')
    expect(code.findAll('script, img, a')).toHaveLength(0)
    expect(code.text()).toContain('"model": "YOUR_MODEL"')
    await wrapper.get('[data-testid="home-copy-example"]').trigger('click')
    expect(copyToClipboard).toHaveBeenCalledWith(code.text())
    await wrapper.get('[data-testid="home-protocol-chat"]').trigger('click')
    expect(wrapper.get('.api-preview__window-title').text()).toBe('OpenAI · Chat Completions')
    const command = wrapper.get('.api-preview__code').text()
    const body = command.slice(command.indexOf("-d '") + 4, -1)
    expect(JSON.parse(body)).toEqual({ model: 'YOUR_MODEL', messages: [{ role: 'user', content: 'Hello!' }] })
  })

  it('persists theme changes and exposes an accessible theme button', async () => {
    document.documentElement.classList.remove('dark')
    const wrapper = mountHome()

    await wrapper.get('button[aria-label="home.switchToDark"]').trigger('click')

    expect(wrapper.get('[data-testid="default-home"]').classes()).toContain('home-landing--dark')
    expect(localStorage.getItem('theme')).toBe('dark')
    document.documentElement.classList.remove('dark')
  })

  it('uses the configured branding and only safe documentation URLs', () => {
    const branded = mountHome({ site_name: 'My gateway', doc_url: 'https://docs.example.com', site_subtitle: 'My subtitle' })
    expect(branded.get('.landing-brand').text()).toBe('My gateway')
    expect(branded.get('.landing-subtitle').text()).toBe('My subtitle')
    expect(branded.findAll('a[href="https://docs.example.com/"]')).toHaveLength(2)
    const unsafe = mountHome({ doc_url: 'javascript:alert(1)' })
    expect(unsafe.find('a[href^="javascript:"]').exists()).toBe(false)
  })

  it('keeps an authenticated account entry without an email initial', () => {
    authStore.isAuthenticated = true
    const wrapper = mountHome()
    expect(wrapper.getComponent('[data-testid="home-account-entry"]').props('to')).toBe('/dashboard')
    expect(wrapper.find('.landing-account-mark').exists()).toBe(false)
  })

  it('exposes legal policy links in compact and default footers', () => {
    const compact = mountHome({ compact_home_enabled: true })
    expect(
      compact.findAllComponents(RouterLinkStub).some((link) => link.props('to') === '/legal/privacy'),
    ).toBe(true)
    expect(compact.text()).toContain('home.footer.riskStrip')

    const home = mountHome()
    expect(home.text()).toContain('home.footer.riskStrip')
    expect(
      home.findAllComponents(RouterLinkStub).some((link) => link.props('to') === '/legal/disclaimer'),
    ).toBe(true)
  })
})
