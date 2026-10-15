import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { enableAutoUnmount, flushPromises, shallowMount } from '@vue/test-utils'
import ChannelStatusV1View from '../ChannelStatusV1View.vue'
import { DEFAULT_INTERVAL_SECONDS } from '@/constants/channelMonitor'

const mocks = vi.hoisted(() => ({ list: vi.fn(), status: vi.fn(), showError: vi.fn() }))
vi.mock('@/api/channelMonitor', () => ({ list: mocks.list, status: mocks.status }))
vi.mock('@/stores/app', () => ({ useAppStore: () => ({ cachedPublicSettings: { channel_monitor_enabled: true }, showError: mocks.showError }) }))
vi.mock('vue-i18n', async () => ({
  ...await vi.importActual<typeof import('vue-i18n')>('vue-i18n'),
  useI18n: () => ({ t: (key: string) => key })
}))
enableAutoUnmount(afterEach)
afterEach(() => { vi.useRealTimers(); localStorage.clear() })

function detail(availability: number) {
  return { id: 1, models: [{ model: 'model', availability_15d: availability, availability_30d: availability }] }
}
function deferred<T>() {
  let resolve!: (value: T) => void
  let reject!: (error: Error) => void
  const promise = new Promise<T>((yes, no) => { resolve = yes; reject = no })
  return { promise, resolve, reject }
}
beforeEach(() => {
  vi.useFakeTimers()
  vi.resetAllMocks()
  localStorage.clear()
  localStorage.setItem('channel-status-auto-refresh', JSON.stringify({ enabled: true, interval_seconds: 30 }))
  mocks.list.mockImplementation(async () => ({ items: [{ id: 1, name: 'Monitor', primary_model: 'model', primary_status: 'operational', availability_7d: 99 }] }))
  mocks.status.mockResolvedValue(detail(90))
})

function mountView() {
  return shallowMount(ChannelStatusV1View, {
    global: { stubs: {
      AppLayout: { template: '<div><slot /></div>' },
      MonitorHero: { props: ['loading'], emits: ['update:window', 'refresh'], template: `<div :data-loading="loading">
        <button class="fifteen" @click="$emit('update:window', '15d')">15d</button>
        <button class="thirty" @click="$emit('update:window', '30d')">30d</button>
        <button class="refresh" @click="$emit('refresh')">Refresh</button>
      </div>` },
      MonitorCardGrid: false,
      MonitorCard: { props: ['availabilityValue'], template: '<output>{{ availabilityValue }}</output>' }
    } }
  })
}

describe('channel monitor automatic detail refresh', () => {
  it.each(['resolve', 'reject'])('ignores an older forced detail refresh that %ss last', async outcome => {
    const wrapper = mountView()
    await flushPromises()
    await wrapper.get('.fifteen').trigger('click')
    await flushPromises()
    const old = deferred<ReturnType<typeof detail>>()
    mocks.status.mockReturnValueOnce(old.promise).mockResolvedValueOnce(detail(98))
    await wrapper.get('.refresh').trigger('click')
    await flushPromises()
    await wrapper.get('.refresh').trigger('click')
    await flushPromises()
    expect(wrapper.get('output').text()).toBe('98')
    if (outcome === 'resolve') old.resolve(detail(75))
    else old.reject(new Error('stale failure'))
    await flushPromises()
    expect(wrapper.get('output').text()).toBe('98')
    expect(mocks.showError).not.toHaveBeenCalled()
  })

  it('keeps loading owned by the latest list request and skips stale detail follow-ups', async () => {
    const wrapper = mountView()
    await flushPromises()
    await wrapper.get('.fifteen').trigger('click')
    await flushPromises()
    const old = deferred<{ items: [] }>()
    const current = deferred<{ items: [] }>()
    mocks.list.mockReturnValueOnce(old.promise).mockReturnValueOnce(current.promise)
    await wrapper.get('.refresh').trigger('click')
    await wrapper.get('.refresh').trigger('click')
    old.resolve({ items: [] })
    await flushPromises()
    expect(wrapper.get('[data-loading]').attributes('data-loading')).toBe('true')
    expect(mocks.status).toHaveBeenCalledTimes(1)
    current.resolve({ items: [] })
    await flushPromises()
    expect(wrapper.get('[data-loading]').attributes('data-loading')).toBe('false')
  })

  it.each(['resolve', 'reject'])('ignores detail requests that %s after unmount', async outcome => {
    const wrapper = mountView()
    await flushPromises()
    const pending = deferred<ReturnType<typeof detail>>()
    mocks.status.mockReturnValueOnce(pending.promise)
    await wrapper.get('.fifteen').trigger('click')
    const cache = wrapper.findComponent({ name: 'MonitorCardGrid' }).props('detailCache')
    wrapper.unmount()
    if (outcome === 'resolve') pending.resolve(detail(98))
    else pending.reject(new Error('late failure'))
    await flushPromises()
    expect(cache[1]).toBeUndefined()
    expect(mocks.showError).not.toHaveBeenCalled()
  })

  it.each(['fifteen', 'thirty'])('refreshes the cached %s-day availability on timer ticks', async (tab) => {
    const wrapper = mountView()
    await flushPromises()
    await wrapper.get('.' + tab).trigger('click')
    await flushPromises()
    expect(wrapper.get('output').text()).toBe('90')
    mocks.status.mockResolvedValue(detail(98))
    await vi.advanceTimersByTimeAsync((DEFAULT_INTERVAL_SECONDS + 1) * 1000)
    await flushPromises()
    expect(wrapper.get('output').text()).toBe('98')
    expect(mocks.status).toHaveBeenCalledTimes(2)
    expect(wrapper.get('[data-loading]').attributes('data-loading')).toBe('false')
  })

  it('keeps the seven-day view on the list endpoint without extra detail requests', async () => {
    const wrapper = mountView()
    await flushPromises()
    await vi.advanceTimersByTimeAsync((DEFAULT_INTERVAL_SECONDS + 1) * 1000)
    await flushPromises()
    expect(mocks.list).toHaveBeenCalledTimes(2)
    expect(mocks.status).not.toHaveBeenCalled()
    expect(wrapper.get('output').text()).toBe('99')
  })

  it('still refreshes cached availability on manual reload', async () => {
    const wrapper = mountView()
    await flushPromises()
    await wrapper.get('.fifteen').trigger('click')
    await flushPromises()
    mocks.status.mockResolvedValue(detail(98))
    await wrapper.get('.refresh').trigger('click')
    await flushPromises()
    expect(wrapper.get('output').text()).toBe('98')
  })
})
