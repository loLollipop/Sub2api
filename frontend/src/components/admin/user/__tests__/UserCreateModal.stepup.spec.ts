import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { enableAutoUnmount, flushPromises, mount } from '@vue/test-utils'
import UserCreateModal from '../UserCreateModal.vue'

const api = vi.hoisted(() => ({ create: vi.fn(), stepUp: vi.fn(), showError: vi.fn(), showSuccess: vi.fn() }))
vi.mock('@/api/admin', () => ({ adminAPI: { users: { create: api.create } } }))
vi.mock('@/api', () => ({ totpAPI: { stepUp: api.stepUp } }))
vi.mock('@/stores/app', () => ({ useAppStore: () => ({ showError: api.showError, showSuccess: api.showSuccess }) }))
vi.mock('@/stores', () => ({ useAppStore: () => ({ showError: api.showError, showSuccess: api.showSuccess }) }))
vi.mock('vue-i18n', () => ({ useI18n: () => ({ t: (key: string) => key }) }))
enableAutoUnmount(afterEach)
beforeEach(() => {
  vi.resetAllMocks()
  api.create.mockRejectedValueOnce({ code: 'STEP_UP_REQUIRED' }).mockResolvedValue({ id: 8 })
  api.stepUp.mockResolvedValue({})
})

async function open() {
  const wrapper = mount(UserCreateModal, { props: { show: true }, attachTo: document.body,
    global: { stubs: { teleport: true } } })
  await wrapper.get('input[type="email"]').setValue('admin@example.test')
  await wrapper.get('input[type="text"]').setValue('test-password')
  await wrapper.get('select').setValue('admin')
  await wrapper.get('form').trigger('submit')
  await flushPromises()
  return wrapper
}

describe('creating an administrator with two-factor verification', () => {
  it('verifies above the existing form and retries the original payload once', async () => {
    const wrapper = await open()
    const dialogs = wrapper.findAll('[role="dialog"]')
    expect(dialogs).toHaveLength(2)
    const child = dialogs.find(dialog => dialog.text().includes('stepUp.title'))!
    expect(child.attributes('aria-modal')).toBe('true')
    await wrapper.get('input[autocomplete="one-time-code"]').setValue('012345')
    await flushPromises()
    expect(api.stepUp).toHaveBeenCalledWith('012345')
    expect(api.create).toHaveBeenCalledTimes(2)
    expect(api.create.mock.calls[1][0]).toEqual(api.create.mock.calls[0][0])
    expect(wrapper.emitted('success')).toHaveLength(1)
    expect(api.showError).not.toHaveBeenCalled()
  })

  it('cancels only verification and preserves the administrator form', async () => {
    const wrapper = await open()
    document.dispatchEvent(new KeyboardEvent('keydown', { key: 'Escape', cancelable: true }))
    await flushPromises()
    expect(api.create).toHaveBeenCalledOnce()
    expect(api.stepUp).not.toHaveBeenCalled()
    expect(wrapper.emitted('close')).toBeUndefined()
    expect((wrapper.get('input[type="email"]').element as HTMLInputElement).value).toBe('admin@example.test')
    expect((wrapper.get('select').element as HTMLSelectElement).value).toBe('admin')
    expect(wrapper.find('input[autocomplete="one-time-code"]').exists()).toBe(false)
    expect(api.showError).not.toHaveBeenCalled()
  })
})
