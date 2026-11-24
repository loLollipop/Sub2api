import { enableAutoUnmount, flushPromises, mount } from '@vue/test-utils'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import ProfilePasskeyCard from '../ProfilePasskeyCard.vue'

const { list, register, remove, showError, showWarning, showSuccess } = vi.hoisted(() => ({
  list: vi.fn(), register: vi.fn(), remove: vi.fn(),
  showError: vi.fn(), showWarning: vi.fn(), showSuccess: vi.fn()
}))

vi.mock('@/api', () => ({ passkeyAPI: { isSupported: () => true, list, register, remove } }))
vi.mock('@/stores/app', () => ({ useAppStore: () => ({ showError, showWarning, showSuccess }) }))
vi.mock('vue-i18n', () => ({ useI18n: () => ({ t: (key: string) => key }) }))

enableAutoUnmount(afterEach)

beforeEach(() => {
  vi.resetAllMocks()
  list.mockResolvedValue([])
})

async function beginRegistration() {
  const wrapper = mount(ProfilePasskeyCard, { props: { enabled: true, embedded: true } })
  await flushPromises()
  await wrapper.get('button').trigger('click')
  await wrapper.get('#passkey-add-password').setValue('fixture-password')
  return wrapper
}

describe('Passkey operation feedback', () => {
  it('shows load failure separately from an empty list and retries', async () => {
    list.mockRejectedValueOnce({ reason: 'SERVICE_UNAVAILABLE' })
    const wrapper = mount(ProfilePasskeyCard, { props: { enabled: true } })
    await flushPromises()
    expect(wrapper.find('[data-testid="passkey-load-error"]').exists()).toBe(true)
    expect(wrapper.find('[data-testid="passkey-empty"]').exists()).toBe(false)
    await wrapper.get('[data-testid="passkey-load-error"] button').trigger('click')
    await flushPromises()
    expect(list).toHaveBeenCalledTimes(2)
    expect(wrapper.find('[data-testid="passkey-load-error"]').exists()).toBe(false)
    expect(wrapper.find('[data-testid="passkey-empty"]').exists()).toBe(true)
  })

  it('reports cancellation and keeps the form available', async () => {
    register.mockRejectedValueOnce(new DOMException('Cancelled', 'NotAllowedError'))
    const wrapper = await beginRegistration()
    await wrapper.get('form').trigger('submit')
    await flushPromises()
    expect(showWarning).toHaveBeenCalledWith('profile.passkey.cancelled')
    expect(showSuccess).not.toHaveBeenCalled()
    expect(wrapper.find('#passkey-add-password').exists()).toBe(true)
  })

  it.each([
    ['PASSKEY_CHALLENGE_EXPIRED', 'challengeExpired'],
    ['PASSKEY_CHALLENGE_MISMATCH', 'challengeMismatch'],
    ['PASSKEY_ORIGIN_MISMATCH', 'originMismatch'],
    ['PASSKEY_RP_ID_MISMATCH', 'rpIdMismatch'],
    ['PASSKEY_VERIFICATION_FAILED', 'verificationFailed']
  ])('shows the localized reason for %s', async (reason, key) => {
    register.mockRejectedValueOnce({ code: 400, reason, message: 'upstream detail' })
    const wrapper = await beginRegistration()
    await wrapper.get('form').trigger('submit')
    await flushPromises()
    expect(showError).toHaveBeenCalledWith(`profile.passkey.${key}`)
    expect(wrapper.find('#passkey-add-password').exists()).toBe(true)
  })

  it('refreshes the list after registration and clears the password', async () => {
    register.mockResolvedValueOnce({ id: 1 })
    const wrapper = await beginRegistration()
    await wrapper.get('form').trigger('submit')
    await flushPromises()
    expect(register).toHaveBeenCalledWith('', 'fixture-password')
    expect(showSuccess).toHaveBeenCalledWith('profile.passkey.added')
    expect(list).toHaveBeenCalledTimes(2)
    expect(wrapper.find('#passkey-add-password').exists()).toBe(false)
  })
})
