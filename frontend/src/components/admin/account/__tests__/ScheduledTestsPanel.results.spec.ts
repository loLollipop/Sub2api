import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { enableAutoUnmount, flushPromises, shallowMount } from '@vue/test-utils'

vi.mock('vue-i18n', async (importOriginal) => ({
  ...await importOriginal<typeof import('vue-i18n')>(),
  useI18n: () => ({ t: (key: string) => key }),
}))
enableAutoUnmount(afterEach)

function deferred<T>() {
  let resolve!: (value: T) => void
  let reject!: (reason: unknown) => void
  const promise = new Promise<T>((res, rej) => { resolve = res; reject = rej })
  return { promise, resolve, reject }
}

import ScheduledTestsPanel from '../ScheduledTestsPanel.vue'
const { listByAccount, listResults, showError } = vi.hoisted(() => ({
  listByAccount: vi.fn(), listResults: vi.fn(), showError: vi.fn(),
}))
vi.mock('@/api/admin', () => ({ adminAPI: { scheduledTests: { listByAccount, listResults } } }))
vi.mock('@/stores/app', () => ({ useAppStore: () => ({ showError }) }))

async function openPanel() {
  const wrapper = shallowMount(ScheduledTestsPanel, {
    props: { show: false, accountId: 1, modelOptions: [] },
    global: { stubs: { BaseDialog: { template: '<div><slot /></div>' } } },
  })
  await wrapper.setProps({ show: true })
  await flushPromises()
  return wrapper
}
const result = (id: number) => ({ id, latency_ms: id, status: 'success', response_text: `result-${id}`, started_at: '2026-10-03T00:00:00Z' })

describe('scheduled test result selection', () => {
  beforeEach(() => {
    vi.resetAllMocks()
    listByAccount.mockResolvedValue([1, 2].map(id => ({ id, model_id: `model-${id}`, enabled: true })))
  })

  it.each(['resolve', 'reject'] as const)('does not let an old account load that %ss start polling or clear current loading', async outcome => {
    vi.useFakeTimers()
    const old = deferred<unknown[]>()
    const current = deferred<unknown[]>()
    listByAccount.mockReturnValueOnce(old.promise).mockReturnValueOnce(current.promise)
    const wrapper = shallowMount(ScheduledTestsPanel, {
      props: { show: true, accountId: 1, modelOptions: [] },
      global: { stubs: { BaseDialog: { template: '<div><slot /></div>' } } },
    })
    try {
      await wrapper.setProps({ accountId: 2 })
      expect(listByAccount.mock.calls).toEqual([[1], [2]])
      if (outcome === 'resolve') old.resolve([{ id: 1, model_id: 'obsolete' }])
      else old.reject(new Error('obsolete'))
      await flushPromises()
      expect(wrapper.text()).toContain('common.loading')
      expect(wrapper.text()).not.toContain('obsolete')
      expect(showError).not.toHaveBeenCalled()
      await vi.advanceTimersByTimeAsync(30000)
      expect(listByAccount).toHaveBeenCalledTimes(2)
      current.resolve([{ id: 2, model_id: 'current', enabled: true }])
      await flushPromises()
      expect(wrapper.text()).toContain('current')
      expect(wrapper.text()).not.toContain('common.loading')
      expect(vi.getTimerCount()).toBe(1)
    } finally {
      wrapper.unmount()
      vi.useRealTimers()
    }
  })

  it('ignores a background plan refresh from an earlier opening of the same account', async () => {
    vi.useFakeTimers()
    const oldRefresh = deferred<unknown[]>()
    const currentRefresh = deferred<unknown[]>()
    listByAccount.mockResolvedValueOnce([{ id: 1, model_id: 'first', enabled: true }])
      .mockReturnValueOnce(oldRefresh.promise)
      .mockResolvedValueOnce([{ id: 2, model_id: 'reopened', enabled: true }])
      .mockReturnValueOnce(currentRefresh.promise)
    const wrapper = await openPanel()
    try {
      await vi.advanceTimersByTimeAsync(15000)
      await wrapper.setProps({ show: false })
      await wrapper.setProps({ show: true })
      await flushPromises()
      await vi.advanceTimersByTimeAsync(15000)
      expect(listByAccount).toHaveBeenCalledTimes(4)
      oldRefresh.resolve([{ id: 1, model_id: 'obsolete' }])
      await flushPromises()
      expect(wrapper.text()).toContain('reopened')
      expect(wrapper.text()).not.toContain('obsolete')
      await vi.advanceTimersByTimeAsync(15000)
      expect(listByAccount).toHaveBeenCalledTimes(4)
      currentRefresh.resolve([{ id: 2, model_id: 'current refresh', enabled: true }])
      await flushPromises()
      expect(wrapper.text()).toContain('current refresh')
    } finally {
      wrapper.unmount()
      vi.useRealTimers()
    }
  })

  it.each(['resolve', 'reject'] as const)('ignores an old plan request that later %ss', async outcome => {
    const old = deferred<ReturnType<typeof result>[]>()
    listResults.mockReturnValueOnce(old.promise).mockResolvedValueOnce([result(22)])
    const wrapper = await openPanel()
    const headers = wrapper.findAll('.cursor-pointer')
    await headers[0].trigger('click')
    await headers[1].trigger('click')
    await flushPromises()
    const current = wrapper.html()
    if (outcome === 'resolve') old.resolve([result(11)])
    else old.reject(new Error('obsolete error'))
    await flushPromises()
    expect(wrapper.html()).toBe(current)
    expect(showError).not.toHaveBeenCalled()
  })

  it('keeps loading the current plan when the previous request completes', async () => {
    const old = deferred<ReturnType<typeof result>[]>()
    const current = deferred<ReturnType<typeof result>[]>()
    listResults.mockReturnValueOnce(old.promise).mockReturnValueOnce(current.promise)
    const wrapper = await openPanel()
    await wrapper.findAll('.cursor-pointer')[0].trigger('click')
    await wrapper.findAll('.cursor-pointer')[1].trigger('click')
    old.resolve([result(11)])
    await flushPromises()
    expect(wrapper.text()).toContain('common.loading')
    current.resolve([result(22)])
    await flushPromises()
    expect(wrapper.text()).not.toContain('common.loading')
  })

  it('invalidates a request when the same plan is collapsed and reopened', async () => {
    const old = deferred<ReturnType<typeof result>[]>()
    listResults.mockReturnValueOnce(old.promise).mockResolvedValueOnce([result(22)])
    const wrapper = await openPanel()
    const header = wrapper.findAll('.cursor-pointer')[0]
    await header.trigger('click')
    await header.trigger('click')
    await header.trigger('click')
    await flushPromises()
    const current = wrapper.html()
    old.resolve([result(11)])
    await flushPromises()
    expect(wrapper.html()).toBe(current)
  })
})
