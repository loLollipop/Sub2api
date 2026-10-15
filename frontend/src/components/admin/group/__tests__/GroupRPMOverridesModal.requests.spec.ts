import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { enableAutoUnmount, flushPromises, mount } from '@vue/test-utils'
import type { AdminGroup } from '@/types'
import GroupRPMOverridesModal from '../GroupRPMOverridesModal.vue'

const mocks = vi.hoisted(() => ({ getGroupRPMOverrides: vi.fn(), batchSetGroupRPMOverrides: vi.fn(), clearGroupRPMOverrides: vi.fn(), listUsers: vi.fn(), showError: vi.fn() }))
vi.mock('@/api/admin', () => ({ adminAPI: { groups: mocks, users: { list: mocks.listUsers } } }))
vi.mock('@/stores/app', () => ({ useAppStore: () => ({ showError: mocks.showError, showSuccess: vi.fn() }) }))
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

const group = (id: number) => ({ id, name: `Group ${id}`, platform: 'openai', rpm_limit: 100 } as AdminGroup)
const entries = (id: number) => [{ user_id: id, user_name: '', user_email: `user${id}@example.com`, user_status: 'active', rpm_override: 100 }]

async function openGroup() {
  const wrapper = mount(GroupRPMOverridesModal, {
    props: { show: false, group: group(1) },
    global: { stubs: {
      BaseDialog: { props: ['show'], template: '<div v-if="show"><slot /></div>' },
      Icon: true, PlatformIcon: true, Pagination: true
    } }
  })
  await wrapper.setProps({ show: true })
  return wrapper
}

describe('group RPM override request ownership', () => {
  it.each(['save', 'clear'] as const)('locks all draft edits during a pending %s', async (operation) => {
    vi.useFakeTimers()
    vi.spyOn(console, 'error').mockImplementation(() => {})
    const request = deferred()
    const pending = operation === 'save' ? mocks.batchSetGroupRPMOverrides : mocks.clearGroupRPMOverrides
    pending.mockReturnValueOnce(request.promise)
    mocks.getGroupRPMOverrides.mockResolvedValue(entries(1))
    mocks.listUsers.mockResolvedValue({ items: [{ id: 3, email: 'user3@example.com', username: '', status: 'active' }] })
    const wrapper = await openGroup()
    await flushPromises()
    await wrapper.get('tbody input[type="number"]').setValue('200')
    const search = wrapper.get('input[type="text"]')
    await search.setValue('user3')
    await vi.advanceTimersByTimeAsync(300)
    await wrapper.findAll('button').find(button => button.text().includes('#3'))!.trigger('click')
    const newRpm = wrapper.findAll('input[type="number"]')[0]!
    await newRpm.setValue('300')
    const buttonWithText = (key: string) => wrapper.findAll('button').find(button => button.text() === key)!
    await buttonWithText(operation === 'save' ? 'common.save' : 'admin.groups.clearAll').trigger('click')

    for (const input of wrapper.findAll('input')) expect(input.attributes('disabled')).toBeDefined()
    for (const key of ['common.add', 'admin.groups.clearAll', 'admin.groups.revertChanges', 'common.save']) {
      expect(buttonWithText(key).attributes('disabled')).toBeDefined()
    }
    const remove = wrapper.get('tbody button')
    expect(remove.attributes('disabled')).toBeDefined()

    // Dispatch directly to exercise guards even for synthetic events on disabled controls.
    await wrapper.get('tbody input[type="number"]').setValue('999')
    await search.setValue('different user')
    await newRpm.setValue('999')
    wrapper.get('tbody input[type="number"]').element.dispatchEvent(new Event('change', { bubbles: true }))
    search.element.dispatchEvent(new Event('input', { bubbles: true }))
    newRpm.element.dispatchEvent(new Event('input', { bubbles: true }))
    for (const button of [remove, buttonWithText('admin.groups.revertChanges'), buttonWithText('common.add')]) {
      button.element.dispatchEvent(new MouseEvent('click', { bubbles: true }))
    }
    await flushPromises()
    expect(wrapper.get('tbody').text()).toContain('user1@example.com')
    expect(wrapper.get('tbody').text()).not.toContain('user3@example.com')
    expect(mocks.listUsers).toHaveBeenCalledTimes(1)
    request.reject(new Error('write failed'))
    await flushPromises()
    expect(wrapper.get('tbody input[type="number"]').element).toHaveProperty('value', '200')
    expect(search.element).toHaveProperty('value', 'user3@example.com')
    expect(newRpm.element).toHaveProperty('value', '300')
    await buttonWithText('common.save').trigger('click')
    expect(mocks.batchSetGroupRPMOverrides).toHaveBeenLastCalledWith(1, [{ user_id: 1, rpm_override: 200 }])
  })

  it('disables search result selection during a pending save', async () => {
    vi.useFakeTimers()
    const request = deferred()
    mocks.batchSetGroupRPMOverrides.mockReturnValue(request.promise)
    mocks.getGroupRPMOverrides.mockResolvedValue(entries(1))
    mocks.listUsers.mockResolvedValue({ items: [{ id: 3, email: 'user3@example.com' }] })
    const wrapper = await openGroup()
    await flushPromises()
    await wrapper.get('tbody input[type="number"]').setValue('200')
    await wrapper.get('input[type="text"]').setValue('user3')
    await vi.advanceTimersByTimeAsync(300)
    wrapper.findAll('button').find(button => button.text() === 'common.save')!.element.dispatchEvent(new MouseEvent('click'))
    await flushPromises()
    const result = wrapper.findAll('button').find(button => button.text().includes('#3'))!
    expect(result.attributes('disabled')).toBeDefined()
    result.element.dispatchEvent(new MouseEvent('click', { bubbles: true }))
    request.resolve(undefined)
    await flushPromises()
    expect(wrapper.get('input[type="text"]').element).toHaveProperty('value', 'user3')
  })

  it('blocks draft writes before the initial load succeeds', async () => {
    mocks.getGroupRPMOverrides.mockRejectedValue(new Error('offline'))
    vi.spyOn(console, 'error').mockImplementation(() => {})
    const wrapper = await openGroup()
    await flushPromises()
    const add = wrapper.findAll('button').find(button => button.text() === 'common.add')!
    expect(add.attributes('disabled')).toBeDefined()
    expect(mocks.batchSetGroupRPMOverrides).not.toHaveBeenCalled()
    expect(mocks.clearGroupRPMOverrides).not.toHaveBeenCalled()
  })

  it('does not close a different group when an older save completes', async () => {
    const save = deferred()
    mocks.getGroupRPMOverrides.mockResolvedValueOnce(entries(1)).mockResolvedValueOnce(entries(2))
    mocks.batchSetGroupRPMOverrides.mockReturnValue(save.promise)
    const wrapper = await openGroup()
    await flushPromises()
    await wrapper.get('tbody input[type="number"]').setValue('200')
    await wrapper.findAll('button').find(button => button.text() === 'common.save')!.trigger('click')
    await wrapper.setProps({ group: group(2) })
    await flushPromises()
    save.resolve(undefined)
    await flushPromises()
    expect(wrapper.emitted('close')).toBeUndefined()
    expect(wrapper.emitted('success')).toBeUndefined()
    expect(wrapper.get('tbody').text()).toContain('user2@example.com')
  })

  it('does not erase a different group when an older clear completes', async () => {
    const clear = deferred()
    mocks.getGroupRPMOverrides.mockResolvedValueOnce(entries(1)).mockResolvedValueOnce(entries(2))
    mocks.clearGroupRPMOverrides.mockReturnValue(clear.promise)
    const wrapper = await openGroup()
    await flushPromises()
    await wrapper.findAll('button').find(button => button.text() === 'admin.groups.clearAll')!.trigger('click')
    await wrapper.setProps({ group: group(2) })
    await flushPromises()
    clear.resolve(undefined)
    await flushPromises()
    expect(wrapper.get('tbody').text()).toContain('user2@example.com')
    expect(mocks.clearGroupRPMOverrides).toHaveBeenCalledWith(1)
  })

  it('keeps the reopened group data when an older response finishes last', async () => {
    const old = deferred()
    mocks.getGroupRPMOverrides.mockReturnValueOnce(old.promise).mockResolvedValueOnce(entries(2))
    const wrapper = await openGroup()
    await wrapper.setProps({ show: false })
    await wrapper.setProps({ show: true, group: group(2) })
    await flushPromises()
    old.resolve(entries(1))
    await flushPromises()
    expect(wrapper.get('tbody').text()).toContain('user2@example.com')
    expect(wrapper.get('tbody').text()).not.toContain('user1@example.com')
    await wrapper.get('tbody input[type="number"]').setValue('200')
    await wrapper.findAll('button').find(button => button.text() === 'common.save')!.trigger('click')
    expect(mocks.batchSetGroupRPMOverrides).toHaveBeenCalledWith(2, [{ user_id: 2, rpm_override: 200 }])
  })

  it('does not end the new load or report an error when an old request fails', async () => {
    vi.spyOn(console, 'error').mockImplementation(() => {})
    const old = deferred()
    const current = deferred()
    mocks.getGroupRPMOverrides.mockReturnValueOnce(old.promise).mockReturnValueOnce(current.promise)
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
    mocks.getGroupRPMOverrides.mockResolvedValueOnce(entries(1)).mockResolvedValueOnce(entries(2))
    const wrapper = await openGroup()
    await flushPromises()
    await wrapper.setProps({ group: group(2) })
    await flushPromises()
    expect(mocks.getGroupRPMOverrides).toHaveBeenLastCalledWith(2)
    expect(wrapper.get('tbody').text()).toContain('user2@example.com')
  })

  it('discards the previous group draft before loading the next group', async () => {
    mocks.getGroupRPMOverrides.mockResolvedValueOnce(entries(1)).mockReturnValueOnce(deferred().promise)
    const wrapper = await openGroup()
    await flushPromises()
    await wrapper.get('tbody input[type="number"]').setValue('200')
    await wrapper.setProps({ show: false })
    await wrapper.setProps({ show: true, group: group(2) })
    expect(wrapper.findAll('button').some(button => button.text() === 'common.save')).toBe(false)
  })

  it('ignores a request failure after unmounting', async () => {
    vi.spyOn(console, 'error').mockImplementation(() => {})
    const request = deferred()
    mocks.getGroupRPMOverrides.mockReturnValue(request.promise)
    const wrapper = await openGroup()
    wrapper.unmount()
    request.reject(new Error('offline'))
    await flushPromises()
    expect(mocks.showError).not.toHaveBeenCalled()
  })
})
