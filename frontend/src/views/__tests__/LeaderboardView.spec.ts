import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { flushPromises, mount, type VueWrapper } from '@vue/test-utils'
import { reactive } from 'vue'
import { createI18n } from 'vue-i18n'
import LeaderboardView from '../LeaderboardView.vue'
import type { LeaderboardResponse } from '@/api/leaderboard'

const mocks = vi.hoisted(() => ({ get: vi.fn(), replace: vi.fn(), query: { period: 'week' } }))
vi.mock('@/api/leaderboard', () => ({ getLeaderboard: mocks.get }))
vi.mock('@/stores/auth', () => ({ useAuthStore: () => ({ isAuthenticated: false }) }))
const route = reactive({ query: mocks.query, fullPath: '/leaderboard?period=week' })
vi.mock('vue-router', () => ({ useRoute: () => route, useRouter: () => ({ replace: mocks.replace }) }))
vi.mock('@/components/layout/AppLayout.vue', () => ({ default: { template: '<main><slot /></main>' } }))

let wrapper: VueWrapper | undefined
const payload = (period = 'week') => ({ period, timezone: 'UTC', start: '2026-10-19T00:00:00Z', end: '2026-10-20T00:00:00Z', updated_at: '2026-10-20T00:00:00Z', total_tokens: 50, total_requests: 1, models: [{ model: 'gpt-5', total_tokens: 50, previous_tokens: 0, requests: 1, previous_requests: 0 }], trend: [], granularity: 'day' }) as LeaderboardResponse
function render() {
  wrapper = mount(LeaderboardView, { global: { plugins: [createI18n({ legacy: false, locale: 'en', missingWarn: false, fallbackWarn: false, messages: { en: {} } })], stubs: { LeaderboardChart: true, PlazaProviderIcon: true } } })
}
beforeEach(() => { mocks.get.mockReset(); mocks.replace.mockReset(); route.query.period = 'week' })
afterEach(() => { wrapper?.unmount(); wrapper = undefined })

describe('leaderboard request lifecycle', () => {
  it('aborts an old period request and never lets a late result replace the new period', async () => {
    let finishOld!: (value: LeaderboardResponse) => void
    mocks.get.mockReturnValueOnce(new Promise<LeaderboardResponse>(resolve => { finishOld = resolve })).mockResolvedValueOnce(payload('day'))
    render()
    const oldSignal = mocks.get.mock.calls[0][1] as AbortSignal
    route.query.period = 'day'
    await flushPromises()
    expect(oldSignal.aborted).toBe(true)
    expect(wrapper!.findComponent({ name: 'LeaderboardContent' }).props('data').period).toBe('day')
    finishOld(payload())
    await flushPromises()
    expect(wrapper!.findComponent({ name: 'LeaderboardContent' }).props('data').period).toBe('day')
  })

  it('shows a disabled state for a server-side feature switch without retaining data', async () => {
    mocks.get.mockRejectedValueOnce({ status: 404 })
    render()
    await flushPromises()
    expect(wrapper!.text()).toContain('leaderboard.disabled')
    expect(wrapper!.find('.ranking-model-list').exists()).toBe(false)
  })

  it('redirects an unauthenticated visitor while preserving the intended destination', async () => {
    mocks.get.mockRejectedValueOnce({ status: 401 })
    render()
    await flushPromises()
    expect(mocks.replace).toHaveBeenCalledWith({ path: '/login', query: { redirect: '/leaderboard?period=week' } })
  })

  it('aborts the outstanding request on unmount', () => {
    mocks.get.mockReturnValue(new Promise(() => {}))
    render()
    const signal = mocks.get.mock.calls[0][1] as AbortSignal
    wrapper!.unmount(); wrapper = undefined
    expect(signal.aborted).toBe(true)
  })
})
