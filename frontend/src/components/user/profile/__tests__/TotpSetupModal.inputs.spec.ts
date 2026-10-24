import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { enableAutoUnmount, flushPromises, mount } from '@vue/test-utils'
import TotpSetupModal from '../TotpSetupModal.vue'

const api = vi.hoisted(() => ({ getVerificationMethod: vi.fn(), initiateSetup: vi.fn(), enable: vi.fn() }))
vi.mock('@/api', () => ({ totpAPI: api }))
vi.mock('@/stores/app', () => ({ useAppStore: () => ({ showError: vi.fn(), showSuccess: vi.fn() }) }))
vi.mock('vue-i18n', () => ({ useI18n: () => ({ t: (key: string) => key }) }))
vi.mock('qrcode', () => ({ default: { toDataURL: vi.fn() } }))
enableAutoUnmount(afterEach)

beforeEach(() => {
  vi.resetAllMocks()
  api.getVerificationMethod.mockResolvedValue({ method: 'password' })
  api.initiateSetup.mockResolvedValue({ secret: 'EXAMPLE', qr_code_url: '', setup_token: 'setup-token' })
  api.enable.mockResolvedValue({ success: true })
})

async function openCodeStep() {
  const wrapper = mount(TotpSetupModal, { attachTo: document.body })
  await flushPromises()
  await wrapper.get('input[type="password"]').setValue('password')
  await wrapper.get('.btn-primary').trigger('click')
  await flushPromises()
  await wrapper.get('.btn-primary').trigger('click')
  await wrapper.get('input[autocomplete="one-time-code"]').setValue('123456')
  return wrapper
}

describe('TOTP setup input synchronization', () => {
  it('prevents duplicate enable requests while verification is pending', async () => {
    let finish!: (value: { success: boolean }) => void
    api.enable.mockImplementationOnce(() => new Promise(resolve => { finish = resolve }))
    const wrapper = await openCodeStep()
    await wrapper.get('form').trigger('submit')
    await wrapper.get('form').trigger('submit')
    expect(api.enable).toHaveBeenCalledOnce()
    finish({ success: true })
    await flushPromises()
    expect(wrapper.emitted('success')).toHaveLength(1)
  })

  it('clears displayed digits after verification fails and focuses the first cell', async () => {
    api.enable.mockRejectedValueOnce({ message: 'Invalid code' })
    const wrapper = await openCodeStep()
    await wrapper.get('form').trigger('submit')
    await flushPromises()
    const input = wrapper.get('input[autocomplete="one-time-code"]')
    expect((input.element as HTMLInputElement).value).toBe('')
    expect(document.activeElement).toBe(input.element)
    expect(wrapper.get('[type="submit"]').attributes('disabled')).toBeDefined()
    await input.trigger('paste', { clipboardData: { getData: () => '654321' } })
    await wrapper.get('form').trigger('submit')
    await flushPromises()
    expect(api.enable).toHaveBeenLastCalledWith({ totp_code: '654321', setup_token: 'setup-token' })
    expect(wrapper.emitted('success')).toHaveLength(1)
  })

  it('displays retained digits when returning from the QR-code step', async () => {
    const wrapper = await openCodeStep()
    await wrapper.findAll('button').find(button => button.text() === 'common.back')!.trigger('click')
    await wrapper.get('.btn-primary').trigger('click')
    expect((wrapper.get('input[autocomplete="one-time-code"]').element as HTMLInputElement).value).toBe('123456')
    await wrapper.get('form').trigger('submit')
    await flushPromises()
    expect(api.enable).toHaveBeenCalledWith({ totp_code: '123456', setup_token: 'setup-token' })
  })

  it('submits normally entered digits', async () => {
    const wrapper = await openCodeStep()
    await wrapper.get('form').trigger('submit')
    await flushPromises()
    expect(api.enable).toHaveBeenCalledWith({ totp_code: '123456', setup_token: 'setup-token' })
    expect(wrapper.emitted('success')).toHaveLength(1)
  })
})
