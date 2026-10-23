import { mount, flushPromises, type VueWrapper } from '@vue/test-utils'
import { createPinia, setActivePinia } from 'pinia'
import { reactive, nextTick } from 'vue'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import VersionBadge from '../VersionBadge.vue'
import { useAppStore, VERSION_CACHE_TTL_MS } from '@/stores/app'
import { checkUpdates, type VersionInfo } from '@/api/admin/system'

const authState = reactive({ isAdmin: true })
vi.mock('@/stores', async () => ({
  useAppStore: (await import('@/stores/app')).useAppStore,
  useAuthStore: () => authState,
}))
vi.mock('@/api/admin/system', () => ({
  checkUpdates: vi.fn(), restartService: vi.fn(), getRollbackVersions: vi.fn(), rollback: vi.fn(),
}))
vi.mock('@/api/auth', () => ({ getPublicSettings: vi.fn() }))
vi.mock('@/i18n', () => ({ i18n: { global: { t: (key: string) => key } } }))
vi.mock('vue-i18n', () => ({ useI18n: () => ({ t: (key: string) => key }) }))
vi.mock('@/composables/useClipboard', () => ({ useClipboard: () => ({ copied: false, copyToClipboard: vi.fn() }) }))

const info: VersionInfo = {
  current_version: '2.1.16', latest_version: '2.1.16', has_update: false,
  cached: false, build_type: 'release', update_strategy: 'disabled', check_mode: 'upstream',
  upstream_updates: [{
    repository: 'kiss-kedaya/sub2api', branch: 'main', baseline_commit: 'a'.repeat(40),
    latest_commit: 'b'.repeat(40), has_update: true,
    baseline_release_tag: 'v2.2.6', release_tag: 'v2.2.7', published_at: '2026-10-20T00:00:00Z',
    release_url: 'https://github.com/kiss-kedaya/sub2api/releases/tag/v2.2.7',
    compare_url: `https://github.com/kiss-kedaya/sub2api/compare/${'a'.repeat(40)}...${'b'.repeat(40)}`,
  }, {
    repository: 'Wei-Shaw/sub2api', branch: 'main', baseline_commit: 'c'.repeat(40),
    latest_commit: '', has_update: false, compare_url: '', warning: 'rate limited',
  }],
}

let wrapper: VueWrapper | undefined
beforeEach(() => {
  setActivePinia(createPinia())
  vi.useFakeTimers()
  vi.mocked(checkUpdates).mockReset().mockResolvedValue(info)
  authState.isAdmin = true
  vi.spyOn(document, 'hidden', 'get').mockReturnValue(false)
})
afterEach(() => {
  wrapper?.unmount()
  wrapper = undefined
  vi.useRealTimers()
  vi.restoreAllMocks()
})

async function openBadge(placement?: 'sidebar' | 'header') {
  wrapper = mount(VersionBadge, { attachTo: document.body, props: { placement }, global: { stubs: { Icon: true, transition: true, teleport: true } } })
  await flushPromises()
  await wrapper.find('button').trigger('click')
  return wrapper
}

describe('VersionBadge source checks', () => {
  it('preserves keyboard focus when the pointer leaves the teleported panel', async () => {
    wrapper = mount(VersionBadge, {
      attachTo: document.body, props: { placement: 'header' },
      global: { stubs: { Icon: true, transition: true } },
    })
    await flushPromises()
    const trigger = wrapper.get<HTMLButtonElement>('.version-trigger')
    trigger.element.focus()
    await nextTick()
    await trigger.trigger('keydown', { key: 'Tab' })
    const panel = document.getElementById(trigger.attributes('aria-controls'))!
    const refresh = panel.querySelector('button')!
    panel.dispatchEvent(new MouseEvent('mouseleave'))
    await vi.advanceTimersByTimeAsync(200)
    expect(document.body.contains(panel)).toBe(true)
    expect(document.activeElement).toBe(refresh)
    const outside = document.createElement('button')
    document.body.appendChild(outside)
    try {
      outside.focus()
      await vi.advanceTimersByTimeAsync(200)
      expect(document.body.contains(panel)).toBe(false)
      expect(document.activeElement).toBe(outside)
    } finally {
      outside.remove()
    }
  })

  it('enters the teleported panel from the keyboard and returns to the trigger', async () => {
    wrapper = mount(VersionBadge, {
      attachTo: document.body, props: { placement: 'header' },
      global: { stubs: { Icon: true, transition: true } },
    })
    await flushPromises()
    const trigger = wrapper.get<HTMLButtonElement>('.version-trigger')
    trigger.element.focus()
    await nextTick()
    const panel = document.getElementById(trigger.attributes('aria-controls'))!
    expect(panel).not.toBeNull()
    expect(wrapper.element.contains(panel)).toBe(false)
    const refresh = panel.querySelector('button')!
    await trigger.trigger('keydown', { key: 'Tab' })
    expect(document.activeElement).toBe(refresh)
    await vi.advanceTimersByTimeAsync(200)
    expect(document.body.contains(panel)).toBe(true)
    refresh.dispatchEvent(new KeyboardEvent('keydown', { key: 'Tab', shiftKey: true, bubbles: true, cancelable: true }))
    expect(document.activeElement).toBe(trigger.element)
    await trigger.trigger('keydown', { key: 'Escape' })
    expect(document.body.contains(panel)).toBe(false)
  })

  it('clears a pending hover close timer on unmount', async () => {
    const view = await openBadge('header')
    await view.get('.version-badge').trigger('mouseleave')
    expect(vi.getTimerCount()).toBeGreaterThan(1)
    view.unmount()
    wrapper = undefined
    expect(vi.getTimerCount()).toBe(0)
  })

  it('opens beside the brand on hover and lets the pointer enter the information panel', async () => {
    const view = await openBadge('header')
    await view.get('.version-trigger').trigger('keydown', { key: 'Escape' })
    view.get<HTMLButtonElement>('.version-trigger').element.blur()
    await view.get('.version-badge').trigger('mouseenter')
    expect(view.find('.version-popover').exists()).toBe(true)
    expect(view.find('.version-trigger icon-stub').exists()).toBe(false)
    expect(view.find('.version-popover').text()).not.toContain('version.rollback')
    await view.get('.version-badge').trigger('mouseleave')
    await vi.advanceTimersByTimeAsync(80)
    await view.get('.version-popover').trigger('mouseenter')
    await vi.advanceTimersByTimeAsync(200)
    expect(view.find('.version-popover').exists()).toBe(true)
    await view.get('.version-popover').trigger('mouseleave')
    await vi.advanceTimersByTimeAsync(160)
    expect(view.find('.version-popover').exists()).toBe(false)
  })

  it('opens the header entry with source details and closes on Escape or an outside button', async () => {
    const view = await openBadge('header')
    const trigger = view.get<HTMLButtonElement>('.version-trigger')
    expect(trigger.attributes('aria-label')).toContain('version.versionAndUpdates')
    expect(trigger.attributes('aria-label')).toContain('v2.1.16')
    expect(trigger.attributes('aria-expanded')).toBe('true')
    expect(view.get('.version-popover').attributes('id')).toBe(trigger.attributes('aria-controls'))
    expect(view.get('[data-testid="upstream-updates"]').text()).toContain('kiss-kedaya/sub2api')
    await trigger.trigger('keydown', { key: 'Escape' })
    expect(view.find('.version-popover').exists()).toBe(false)
    expect(document.activeElement).toBe(trigger.element)
    await trigger.trigger('click')
    // A temporarily disabled refresh button can leave focus on the document.
    trigger.element.blur()
    document.dispatchEvent(new KeyboardEvent('keydown', { key: 'Escape', bubbles: true }))
    await nextTick()
    expect(view.find('.version-popover').exists()).toBe(false)
    expect(document.activeElement).toBe(trigger.element)
    await trigger.trigger('click')
    const outside = document.createElement('button')
    document.body.appendChild(outside)
    try {
      outside.click()
      await nextTick()
      expect(view.find('.version-popover').exists()).toBe(false)
      expect(trigger.attributes('aria-expanded')).toBe('false')
    } finally {
      outside.remove()
    }
  })

  it('shows independent source changes and warnings and lights the badge', async () => {
    const view = await openBadge()
    expect(view.find('button .animate-ping').exists()).toBe(true)
    expect(view.text()).toContain('version.projectRelease')
    expect(view.text()).toContain('kiss-kedaya/sub2api')
    expect(view.text()).toContain('Wei-Shaw/sub2api')
    expect(view.text()).toContain('version.upstreamUpdateAvailable')
    expect(view.text()).toContain('version.officialRelease')
    expect(view.text()).toContain('version.releaseCommit')
    expect(view.text()).not.toContain('version.branchHead')
    expect(view.find('[href="https://github.com/kiss-kedaya/sub2api/releases/tag/v2.2.7"]').exists()).toBe(true)
    expect(view.find('[href*="/tree/main"]').exists()).toBe(false)
    expect(view.text()).toContain('rate limited')
    expect(view.find('[data-testid="upstream-updates"] a[href*="/compare/"]').exists()).toBe(true)
    expect(view.find('[data-testid="upstream-updates"] button').exists()).toBe(false)
    expect(view.find('button').attributes('title')).toBe('version.updateAvailable')
  })

  it('keeps successful upstream release checks visible when the project release check fails', async () => {
    vi.mocked(checkUpdates).mockResolvedValueOnce({ ...info, warning: 'project release unavailable' })
    const view = await openBadge()
    expect(view.text()).toContain('version.upstreamUpdateAvailable')
    expect(view.find('[data-testid="version-warning"]').text()).toContain('project release unavailable')
  })

  it('keeps API failure visible and never claims an unknown check is up to date', async () => {
    vi.mocked(checkUpdates).mockRejectedValueOnce(new Error('offline'))
    const view = await openBadge()
    expect(view.find('[data-testid="version-warning"]').text()).toContain('offline')
    expect(view.find('button').attributes('title')).toBe('version.checkUnknown')
    expect(view.find('.bg-green-100').exists()).toBe(false)
  })

  it('labels default release updates as project releases', async () => {
    vi.mocked(checkUpdates).mockResolvedValueOnce({ ...info, has_update: true, latest_version: '2.1.17', check_mode: 'release', upstream_updates: undefined })
    const view = await openBadge()
    expect(view.text()).toContain('version.updateAvailable')
    expect(view.text()).not.toContain('version.upstreamUpdateAvailable')
    expect(view.find('[data-testid="upstream-updates"]').exists()).toBe(false)
  })

  it('marks previous matching source results unknown after a refresh fails', async () => {
    const matched = info.upstream_updates!.map(source => ({ ...source, has_update: false, warning: undefined }))
    vi.mocked(checkUpdates).mockResolvedValueOnce({ ...info, upstream_updates: matched })
    const view = await openBadge()
    expect(view.text()).toContain('version.checkpointMatched')
    vi.mocked(checkUpdates).mockRejectedValueOnce(new Error('offline'))
    await useAppStore().fetchVersion(true)
    await nextTick()
    expect(view.text()).not.toContain('version.checkpointMatched')
    expect(view.find('[data-testid="upstream-updates"]').text()).toContain('version.checkUnknown')
  })

  it('refreshes admin sessions after TTL and cleans up on logout and unmount', async () => {
    await openBadge()
    const store = useAppStore()
    expect(checkUpdates).toHaveBeenCalledTimes(1)
    await vi.advanceTimersByTimeAsync(VERSION_CACHE_TTL_MS)
    expect(checkUpdates).toHaveBeenCalledTimes(2)
    authState.isAdmin = false
    await nextTick()
    expect(wrapper?.find('.version-popover').exists()).toBe(false)
    store.clearVersionCache()
    await vi.advanceTimersByTimeAsync(VERSION_CACHE_TTL_MS)
    expect(checkUpdates).toHaveBeenCalledTimes(2)
    authState.isAdmin = true
    await nextTick()
    await flushPromises()
    expect(checkUpdates).toHaveBeenCalledTimes(3)
    expect(wrapper?.find('.version-popover').exists()).toBe(false)
    expect(wrapper?.get('.version-trigger').attributes('aria-expanded')).toBe('false')
    wrapper?.unmount()
    wrapper = undefined
    store.clearVersionCache()
    await vi.advanceTimersByTimeAsync(VERSION_CACHE_TTL_MS)
    expect(checkUpdates).toHaveBeenCalledTimes(3)
  })

  it('does not request administrative updates for ordinary users', async () => {
    authState.isAdmin = false
    wrapper = mount(VersionBadge, { props: { version: '2.1.16' }, global: { stubs: { Icon: true } } })
    await flushPromises()
    await vi.advanceTimersByTimeAsync(VERSION_CACHE_TTL_MS)
    expect(checkUpdates).not.toHaveBeenCalled()
    expect(wrapper.text()).toBe('v2.1.16')
  })
})
