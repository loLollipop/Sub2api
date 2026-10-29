import { mount } from '@vue/test-utils'
import { defineComponent, h, nextTick } from 'vue'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'

const stores = vi.hoisted(() => ({
  app: { siteName: 'Test Site', cachedPublicSettings: { custom_menu_items: [] } },
  auth: { isAdmin: false },
  adminSettings: { customMenuItems: [] }
}))

vi.mock('@/router', () => ({
  default: {
    currentRoute: {
      value: { name: 'Dashboard', params: {}, meta: { titleKey: 'common.settings' } }
    }
  }
}))
vi.mock('@/stores/app', () => ({ useAppStore: () => stores.app }))
vi.mock('@/stores/auth', () => ({ useAuthStore: () => stores.auth }))
vi.mock('@/stores/adminSettings', () => ({ useAdminSettingsStore: () => stores.adminSettings }))

describe('locale initialization and switching', () => {
  beforeEach(() => {
    vi.resetModules()
    localStorage.clear()
    document.documentElement.removeAttribute('lang')
    document.title = ''
    vi.spyOn(navigator, 'language', 'get').mockReturnValue('en-US')
  })

  afterEach(() => {
    vi.restoreAllMocks()
  })

  it('prefers a saved locale and lazily loads its messages on initialization', async () => {
    localStorage.setItem('sub2api_locale', 'zh')
    const { i18n, initI18n, getLocale } = await import('../index')

    expect(i18n.mode).toBe('composition')
    expect(i18n.global.availableLocales).toEqual([])
    await initI18n()

    expect(getLocale()).toBe('zh')
    expect(i18n.global.availableLocales).toEqual(['zh'])
    expect(i18n.global.t('common.save')).toBe('保存')
    expect(document.documentElement.lang).toBe('zh')
  })

  it('uses the browser language when the saved locale is unsupported', async () => {
    localStorage.setItem('sub2api_locale', 'fr')
    vi.spyOn(navigator, 'language', 'get').mockReturnValue('zh-CN')
    const { initI18n, getLocale } = await import('../index')

    await initI18n()

    expect(getLocale()).toBe('zh')
    expect(document.documentElement.lang).toBe('zh')
  })

  it('defaults to English for other browser languages', async () => {
    vi.spyOn(navigator, 'language', 'get').mockReturnValue('fr-FR')
    const { i18n, initI18n, getLocale } = await import('../index')

    await initI18n()

    expect(getLocale()).toBe('en')
    expect(i18n.global.t('common.save')).toBe('Save')
    expect(document.documentElement.lang).toBe('en')
  })

  it('updates Composition API consumers, interpolation, storage, and the page title', async () => {
    const { i18n, initI18n, setLocale } = await import('../index')
    const { useI18n } = await import('vue-i18n')
    await initI18n()
    const wrapper = mount(defineComponent({
      setup() {
        const { t } = useI18n()
        return () => h('span', t('common.autoRefresh.countdown', { seconds: 5 }))
      }
    }), { global: { plugins: [i18n] } })

    try {
      expect(wrapper.text()).toBe('Auto refresh: 5s')

      await setLocale('zh')
      await nextTick()
      expect(wrapper.text()).toBe('自动刷新: 5s')
      expect(localStorage.getItem('sub2api_locale')).toBe('zh')
      expect(document.documentElement.lang).toBe('zh')
      expect(document.title).toBe('设置 - Test Site')

      await setLocale('en')
      await nextTick()
      expect(wrapper.text()).toBe('Auto refresh: 5s')
      expect(localStorage.getItem('sub2api_locale')).toBe('en')
      expect(document.documentElement.lang).toBe('en')
      expect(document.title).toBe('Settings - Test Site')
      expect(i18n.global.availableLocales).toEqual(['en', 'zh'])
    } finally {
      wrapper.unmount()
    }
  })

  it('ignores unsupported locale switches without changing state', async () => {
    const { i18n, initI18n, setLocale, getLocale } = await import('../index')
    await initI18n()
    document.title = 'Existing title'

    await setLocale('fr')

    expect(getLocale()).toBe('en')
    expect(i18n.global.availableLocales).toEqual(['en'])
    expect(localStorage.getItem('sub2api_locale')).toBeNull()
    expect(document.documentElement.lang).toBe('en')
    expect(document.title).toBe('Existing title')
  })
})
