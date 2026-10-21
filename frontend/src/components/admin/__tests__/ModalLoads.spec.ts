import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { enableAutoUnmount, flushPromises, shallowMount } from '@vue/test-utils'
import ErrorPassthroughRulesModal from '../ErrorPassthroughRulesModal.vue'
import TLSFingerprintProfilesModal from '../TLSFingerprintProfilesModal.vue'
import AccountStatsModal from '../../account/AccountStatsModal.vue'
import TempUnschedStatusModal from '../../account/TempUnschedStatusModal.vue'
import type { Account } from '@/types'

const mocks = vi.hoisted(() => ({
  rules: vi.fn(), profiles: vi.fn(), stats: vi.fn(), status: vi.fn(), showError: vi.fn(),
}))
vi.mock('@/api/admin', () => ({ adminAPI: {
  errorPassthrough: { list: mocks.rules },
  tlsFingerprintProfiles: { list: mocks.profiles },
  accounts: { getStats: mocks.stats, getTempUnschedulableStatus: mocks.status },
} }))
vi.mock('@/stores/app', () => ({ useAppStore: () => ({ showError: mocks.showError }) }))
vi.mock('vue-i18n', async (importOriginal) => ({
  ...await importOriginal<typeof import('vue-i18n')>(),
  useI18n: () => ({ t: (key: string) => key }),
}))

enableAutoUnmount(afterEach)
beforeEach(() => vi.resetAllMocks())
afterEach(() => vi.restoreAllMocks())

function deferred() {
  let resolve!: (value: unknown) => void
  let reject!: (reason: unknown) => void
  const promise = new Promise((res, rej) => { resolve = res; reject = rej })
  return { promise, resolve, reject }
}

const cases = [
  { name: 'rules', component: ErrorPassthroughRulesModal, load: mocks.rules, state: 'rules', empty: [], data: [{ id: 2 }] },
  { name: 'TLS profiles', component: TLSFingerprintProfilesModal, load: mocks.profiles, state: 'profiles', empty: [], data: [{ id: 2 }] },
  { name: 'account stats', component: AccountStatsModal, load: mocks.stats, state: 'stats', empty: null, data: { account_id: 2 } },
  { name: 'temporary unschedulable status', component: TempUnschedStatusModal, load: mocks.status, state: 'status', empty: null, data: { active: false } },
]

describe.each(cases)('$name load lifecycle', ({ component, load, state, empty, data }) => {
  const mountModal = () => shallowMount(component, {
    props: { show: true, account: { id: 1 } as Account },
    // Keep rendering independent of API fixture shape; inspect the modal's data and busy state.
    global: { stubs: { BaseDialog: { template: '<div />' } } },
  })

  it.each(['resolve', 'reject'] as const)('ignores an obsolete request that later %ss after reopening', async outcome => {
    const old = deferred()
    const current = deferred()
    load.mockReturnValueOnce(old.promise).mockReturnValueOnce(current.promise)
    const wrapper = mountModal()
    const setup = Reflect.get(wrapper.vm.$, 'setupState') as Record<string, unknown>
    expect(load).toHaveBeenCalledTimes(1)
    await wrapper.setProps({ show: false })
    expect(setup[state]).toEqual(empty)
    expect(setup.loading).toBe(false)
    await wrapper.setProps({ show: true })
    if (outcome === 'resolve') old.resolve(data)
    else old.reject(new Error('obsolete'))
    await flushPromises()
    expect(setup[state]).toEqual(empty)
    expect(setup.loading).toBe(true)
    expect(mocks.showError).not.toHaveBeenCalled()
    current.resolve(data)
    await flushPromises()
    expect(setup[state]).toEqual(data)
    expect(setup.loading).toBe(false)
  })

  it.each(['resolve', 'reject'] as const)('ignores a request that %ss after unmount', async outcome => {
    const pending = deferred()
    load.mockReturnValue(pending.promise)
    const wrapper = mountModal()
    const setup = Reflect.get(wrapper.vm.$, 'setupState') as Record<string, unknown>
    wrapper.unmount()
    if (outcome === 'resolve') pending.resolve(data)
    else pending.reject(new Error('obsolete'))
    await flushPromises()
    expect(setup[state]).toEqual(empty)
    expect(setup.loading).toBe(true)
    expect(mocks.showError).not.toHaveBeenCalled()
  })

  if (state === 'stats' || state === 'status') {
    it('reloads on account changes and retains the newer response', async () => {
      const old = deferred()
      load.mockReturnValueOnce(old.promise).mockResolvedValueOnce(data)
      const wrapper = mountModal()
      const setup = Reflect.get(wrapper.vm.$, 'setupState') as Record<string, unknown>
      await wrapper.setProps({ account: { id: 2 } as Account })
      await flushPromises()
      expect(load.mock.calls.map(args => args[0])).toEqual([1, 2])
      old.resolve({ account_id: 1 })
      await flushPromises()
      expect(setup[state]).toEqual(data)
      expect(setup.loading).toBe(false)
    })
  }
})
