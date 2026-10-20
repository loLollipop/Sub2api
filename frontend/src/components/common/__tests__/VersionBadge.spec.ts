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

async function openBadge() {
  wrapper = mount(VersionBadge, { global: { stubs: { Icon: true } } })
  await flushPromises()
  await wrapper.find('button').trigger('click')
  return wrapper
}

describe('VersionBadge source checks', () => {
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
    store.clearVersionCache()
    await vi.advanceTimersByTimeAsync(VERSION_CACHE_TTL_MS)
    expect(checkUpdates).toHaveBeenCalledTimes(2)
    authState.isAdmin = true
    await nextTick()
    await flushPromises()
    expect(checkUpdates).toHaveBeenCalledTimes(3)
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
