import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { enableAutoUnmount, flushPromises, mount } from '@vue/test-utils'
import type { AdminGroup, AdminUser } from '@/types'
import GroupRateMultipliersModal from '../GroupRateMultipliersModal.vue'

const mocks = vi.hoisted(() => ({ getGroupRateMultipliers: vi.fn(), batchSetGroupRateMultipliers: vi.fn(), listUsers: vi.fn(), showError: vi.fn(), showSuccess: vi.fn() }))
vi.mock('@/api/admin', () => ({ adminAPI: { groups: mocks, users: { list: mocks.listUsers } } }))
vi.mock('@/stores/app', () => ({ useAppStore: () => ({ showError: mocks.showError, showSuccess: mocks.showSuccess }) }))
vi.mock('vue-i18n', () => ({ useI18n: () => ({ t: (key: string) => key }) }))
enableAutoUnmount(afterEach)
beforeEach(() => vi.resetAllMocks())
afterEach(() => { vi.restoreAllMocks(); vi.useRealTimers() })

function deferred() {
  let resolve!: (value: unknown) => void
  let reject!: (reason: unknown) => void
  const promise = new Promise((res, rej) => { resolve = res; reject = rej })
  return { promise, resolve, reject }
}
const group = (id: number) => ({ id, name: `Group ${id}`, platform: 'openai', rate_multiplier: 1 } as AdminGroup)
const entries = (id: number) => [{ user_id: id, user_name: '', user_email: `user${id}@example.com`, user_status: 'active', rate_multiplier: 2, rpm_override: null }]

async function openGroup() {
  const wrapper = mount(GroupRateMultipliersModal, {
    props: { show: false, group: group(1) },
    global: { stubs: {
      BaseDialog: { props: ['show'], template: '<div v-if="show"><slot /></div>' },
      Icon: true, PlatformIcon: true, Pagination: true
    } }
  })
  await wrapper.setProps({ show: true })
  return wrapper
}

describe('group rate multiplier request ownership', () => {
  it('keeps group B when group A completes late without closing the dialog', async () => {
    const old = deferred()
    mocks.getGroupRateMultipliers.mockReturnValueOnce(old.promise).mockResolvedValueOnce(entries(2))
    const wrapper = await openGroup()
    await wrapper.setProps({ group: group(2) })
    await flushPromises()
    old.resolve(entries(1))
    await flushPromises()
    expect(wrapper.get('tbody').text()).toContain('user2@example.com')
    expect(wrapper.get('tbody').text()).not.toContain('user1@example.com')
  })

  it('does not save before group entries have loaded', async () => {
    mocks.getGroupRateMultipliers.mockReturnValueOnce(deferred().promise)
    const wrapper = await openGroup()
    await (wrapper.vm as unknown as { handleSave: () => Promise<void> }).handleSave()
    expect(mocks.batchSetGroupRateMultipliers).not.toHaveBeenCalled()
  })

  it('blocks initial-load edits and allows adding a draft after loading', async () => {
    vi.useFakeTimers()
    const request = deferred()
    mocks.getGroupRateMultipliers.mockReturnValueOnce(request.promise)
    const wrapper = await openGroup()
    const view = wrapper.vm as unknown as {
      searchQuery: string
      newRate: number
      selectedUser: AdminUser
      handleSearchUsers: () => void
      selectUser: (user: AdminUser) => void
      handleAddLocal: () => void
    }
    const user = { id: 3, email: 'user3@example.com', status: 'active' } as AdminUser
    expect((wrapper.get('input[type="text"]').element as HTMLInputElement).disabled).toBe(true)
    expect((wrapper.get('input[type="number"]').element as HTMLInputElement).disabled).toBe(true)
    view.searchQuery = 'user3'
    view.handleSearchUsers()
    await vi.advanceTimersByTimeAsync(300)
    expect(mocks.listUsers).not.toHaveBeenCalled()
    view.selectUser(user)
    expect(view.selectedUser).toBeNull()
    // Handler guard protects against programmatic calls as well as disabled UI.
    view.selectedUser = user
    view.newRate = 0.15
    view.handleAddLocal()
    expect(wrapper.findAll('button').some(button => button.text() === 'common.save')).toBe(false)
    request.resolve(entries(1))
    await flushPromises()
    expect((wrapper.get('input[type="text"]').element as HTMLInputElement).disabled).toBe(false)
    view.selectUser(user)
    view.newRate = 0.15
    view.handleAddLocal()
    await flushPromises()
    expect(wrapper.get('tbody').text()).toContain('user3@example.com')
    await wrapper.findAll('button').find(button => button.text() === 'common.save')!.trigger('click')
    expect(mocks.batchSetGroupRateMultipliers).toHaveBeenCalledWith(1, [
      { user_id: 1, rate_multiplier: 2 }, { user_id: 3, rate_multiplier: 0.15 }
    ])
  })

  it('keeps editing disabled after the initial load fails', async () => {
    vi.spyOn(console, 'error').mockImplementation(() => {})
    mocks.getGroupRateMultipliers.mockRejectedValueOnce(new Error('offline'))
    const wrapper = await openGroup()
    await flushPromises()
    expect((wrapper.get('input[type="text"]').element as HTMLInputElement).disabled).toBe(true)
    expect((wrapper.get('input[type="number"]').element as HTMLInputElement).disabled).toBe(true)
    expect(wrapper.findAll('button').find(button => button.text() === 'common.add')!.attributes('disabled')).toBeDefined()
    expect(mocks.showError).toHaveBeenCalledOnce()
  })

  it('removes its document listener and pending search on unmount', async () => {
    vi.useFakeTimers()
    const add = vi.spyOn(document, 'addEventListener')
    const remove = vi.spyOn(document, 'removeEventListener')
    mocks.getGroupRateMultipliers.mockResolvedValueOnce(entries(1))
    const wrapper = await openGroup()
    await wrapper.get('input[type="text"]').setValue('user')
    const clickListener = add.mock.calls.find(([event]) => event === 'click')?.[1]
    expect(clickListener).toBeDefined()
    expect(vi.getTimerCount()).toBeGreaterThan(0)
    wrapper.unmount()
    expect(remove).toHaveBeenCalledWith('click', clickListener)
    expect(vi.getTimerCount()).toBe(0)
    expect(mocks.listUsers).not.toHaveBeenCalled()
  })

  it('keeps the reopened group data when an older response finishes last', async () => {
    const old = deferred()
    mocks.getGroupRateMultipliers.mockReturnValueOnce(old.promise).mockResolvedValueOnce(entries(2))
    const wrapper = await openGroup()
    await wrapper.setProps({ show: false })
    await wrapper.setProps({ show: true, group: group(2) })
    await flushPromises()
    old.resolve(entries(1))
    await flushPromises()
    expect(wrapper.get('tbody').text()).toContain('user2@example.com')
    expect(wrapper.get('tbody').text()).not.toContain('user1@example.com')
    await wrapper.get('tbody input[type="number"]').setValue('3')
    await wrapper.findAll('button').find(button => button.text() === 'common.save')!.trigger('click')
    expect(mocks.batchSetGroupRateMultipliers).toHaveBeenCalledWith(2, [{ user_id: 2, rate_multiplier: 3 }])
  })

  it('does not end the new load or report an error when an old request fails', async () => {
    vi.spyOn(console, 'error').mockImplementation(() => {})
    const old = deferred()
    const current = deferred()
    mocks.getGroupRateMultipliers.mockReturnValueOnce(old.promise).mockReturnValueOnce(current.promise)
    const wrapper = await openGroup()
    await wrapper.setProps({ show: false })
    await wrapper.setProps({ show: true, group: group(2) })
    old.reject(new Error('old request failed'))
    await flushPromises()
    expect(mocks.showError).not.toHaveBeenCalled()
    expect(wrapper.find('svg.animate-spin').exists()).toBe(true)
    current.resolve(entries(2))
    await flushPromises()
    expect(wrapper.get('tbody').text()).toContain('user2@example.com')
  })

  it('loads a different group while the dialog remains open', async () => {
    mocks.getGroupRateMultipliers.mockResolvedValueOnce(entries(1)).mockResolvedValueOnce(entries(2))
    const wrapper = await openGroup()
    await flushPromises()
    await wrapper.setProps({ group: group(2) })
    await flushPromises()
    expect(mocks.getGroupRateMultipliers).toHaveBeenLastCalledWith(2)
    expect(wrapper.get('tbody').text()).toContain('user2@example.com')
  })

  it('discards the previous group draft before loading the next group', async () => {
    mocks.getGroupRateMultipliers.mockResolvedValueOnce(entries(1)).mockReturnValueOnce(deferred().promise)
    const wrapper = await openGroup()
    await flushPromises()
    await wrapper.get('tbody input[type="number"]').setValue('3')
    await wrapper.setProps({ show: false })
    await wrapper.setProps({ show: true, group: group(2) })
    expect(wrapper.findAll('button').some(button => button.text() === 'common.save')).toBe(false)
  })

  it('ignores a request failure after unmounting', async () => {
    vi.spyOn(console, 'error').mockImplementation(() => {})
    const request = deferred()
    mocks.getGroupRateMultipliers.mockReturnValue(request.promise)
    const wrapper = await openGroup()
    wrapper.unmount()
    request.reject(new Error('offline'))
    await flushPromises()
    expect(mocks.showError).not.toHaveBeenCalled()
  })

  it.each(['resolve', 'reject'] as const)('preserves a reopened group draft when an old save finishes (%s)', async (outcome) => {
    vi.spyOn(console, 'error').mockImplementation(() => {})
    const oldSave = deferred()
    mocks.getGroupRateMultipliers.mockResolvedValueOnce(entries(1)).mockResolvedValueOnce(entries(2))
    mocks.batchSetGroupRateMultipliers.mockReturnValueOnce(oldSave.promise)
    const wrapper = await openGroup()
    await flushPromises()
    await wrapper.get('tbody input[type="number"]').setValue('3')
    const view = wrapper.vm as unknown as { handleSave: () => Promise<void> }
    const saving = view.handleSave()
    expect(mocks.batchSetGroupRateMultipliers).toHaveBeenCalledWith(1, [{ user_id: 1, rate_multiplier: 3 }])
    await wrapper.setProps({ show: false })
    await wrapper.setProps({ show: true, group: group(2) })
    await flushPromises()
    await wrapper.get('tbody input[type="number"]').setValue('5')
    // Keep saves serialized until the request that actually writes group A finishes.
    await view.handleSave()
    expect(mocks.batchSetGroupRateMultipliers).toHaveBeenCalledTimes(1)
    if (outcome === 'resolve') oldSave.resolve(undefined)
    else oldSave.reject(new Error('old save failed'))
    await saving
    await flushPromises()
    expect(wrapper.emitted('close')).toBeUndefined()
    expect(wrapper.emitted('success')).toBeUndefined()
    expect(mocks.showError).not.toHaveBeenCalled()
    expect(mocks.showSuccess).not.toHaveBeenCalled()
    expect((wrapper.get('tbody input[type="number"]').element as HTMLInputElement).value).toBe('5')
    await view.handleSave()
    expect(mocks.batchSetGroupRateMultipliers).toHaveBeenLastCalledWith(2, [{ user_id: 2, rate_multiplier: 5 }])
    expect(wrapper.emitted('success')).toHaveLength(1)
  })

  it('does not close a different group after an in-place switch during saving', async () => {
    const oldSave = deferred()
    mocks.getGroupRateMultipliers.mockResolvedValueOnce(entries(1)).mockResolvedValueOnce(entries(2))
    mocks.batchSetGroupRateMultipliers.mockReturnValueOnce(oldSave.promise)
    const wrapper = await openGroup()
    await flushPromises()
    const saving = (wrapper.vm as unknown as { handleSave: () => Promise<void> }).handleSave()
    await wrapper.setProps({ group: group(2) })
    await flushPromises()
    oldSave.resolve(undefined)
    await saving
    expect(wrapper.emitted('close')).toBeUndefined()
    expect(wrapper.get('tbody').text()).toContain('user2@example.com')
  })
})
