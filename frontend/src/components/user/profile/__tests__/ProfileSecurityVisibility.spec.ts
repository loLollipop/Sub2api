import { enableAutoUnmount, flushPromises, mount } from '@vue/test-utils'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import ProfileTotpCard from '../ProfileTotpCard.vue'
import ProfilePasskeyCard from '../ProfilePasskeyCard.vue'

const { getStatus, listPasskeys } = vi.hoisted(() => ({
  getStatus: vi.fn(),
  listPasskeys: vi.fn(),
}))

vi.mock('@/api', () => ({
  totpAPI: { getStatus },
  passkeyAPI: { isSupported: () => true, list: listPasskeys },
}))
vi.mock('@/stores/app', () => ({
  useAppStore: () => ({ showError: vi.fn(), showSuccess: vi.fn() }),
}))
vi.mock('vue-i18n', () => ({ useI18n: () => ({ t: (key: string) => key }) }))

enableAutoUnmount(afterEach)

beforeEach(() => {
  vi.clearAllMocks()
  listPasskeys.mockResolvedValue([])
})

describe('optional profile security controls', () => {
  it.each([true, false])('hides globally disabled TOTP in embedded=%s mode', async embedded => {
    getStatus.mockResolvedValue({ feature_enabled: false, enabled: false, enabled_at: null })
    const wrapper = mount(ProfileTotpCard, { props: { embedded } })
    await flushPromises()
    expect(wrapper.find('.profile-totp-setting, .card').exists()).toBe(false)
    expect(wrapper.text()).toBe('')
  })

  it.each([true, false])('keeps the TOTP %s management action available when the feature is enabled', async enabled => {
    getStatus.mockResolvedValue({ feature_enabled: true, enabled, enabled_at: null })
    const wrapper = mount(ProfileTotpCard, {
      props: { embedded: true },
      global: { stubs: { TotpSetupModal: true, TotpDisableDialog: true } },
    })
    await flushPromises()
    expect(wrapper.get('button').text()).toBe(`profile.totp.${enabled ? 'disable' : 'enable'}`)
    await wrapper.get('button').trigger('click')
    expect(wrapper.findComponent({ name: enabled ? 'TotpDisableDialog' : 'TotpSetupModal' }).exists()).toBe(true)
  })

  it('hides disabled Passkey and only loads credentials after activation', async () => {
    const wrapper = mount(ProfilePasskeyCard, { props: { enabled: false, embedded: true } })
    await flushPromises()
    expect(wrapper.text()).toBe('')
    expect(listPasskeys).not.toHaveBeenCalled()
    await wrapper.setProps({ enabled: true })
    await flushPromises()
    expect(listPasskeys).toHaveBeenCalledOnce()
    expect(wrapper.get('button').text()).toBe('profile.passkey.add')
    await wrapper.get('button').trigger('click')
    expect(wrapper.find('#passkey-add-password').exists()).toBe(true)
    await wrapper.setProps({ enabled: false })
    expect(wrapper.find('.profile-passkey-setting').exists()).toBe(false)
  })
})
