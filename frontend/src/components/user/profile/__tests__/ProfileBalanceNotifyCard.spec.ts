import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { enableAutoUnmount, flushPromises, mount, type VueWrapper } from '@vue/test-utils'
import ProfileBalanceNotifyCard from '../ProfileBalanceNotifyCard.vue'

const { sendNotifyEmailCode, verifyNotifyEmail, getProfile, toggleNotifyEmail, updateProfile, removeNotifyEmail, showError } = vi.hoisted(() => ({
  sendNotifyEmailCode: vi.fn(),
  verifyNotifyEmail: vi.fn(),
  getProfile: vi.fn(),
  toggleNotifyEmail: vi.fn(),
  updateProfile: vi.fn(),
  removeNotifyEmail: vi.fn(),
  showError: vi.fn()
}))

vi.mock('@/api', () => ({
  userAPI: { sendNotifyEmailCode, verifyNotifyEmail, getProfile, toggleNotifyEmail, updateProfile, removeNotifyEmail }
}))
vi.mock('@/stores/auth', () => ({ useAuthStore: () => ({ user: null }) }))
vi.mock('@/stores/app', () => ({
  useAppStore: () => ({ showSuccess: vi.fn(), showError })
}))
vi.mock('vue-i18n', () => ({ useI18n: () => ({ t: (key: string) => key }) }))

enableAutoUnmount(afterEach)

const deferred = () => {
  let resolve!: () => void
  const promise = new Promise<void>((done) => { resolve = done })
  return { promise, resolve }
}

const pendingRows = (wrapper: VueWrapper) => wrapper.findAll('.bg-yellow-50')
const pendingEmails = (wrapper: VueWrapper) => pendingRows(wrapper).map(row => row.get('span').text())

const button = (wrapper: VueWrapper, text: string) =>
  wrapper.findAll('button').find(item => item.text() === text)!

describe('ProfileBalanceNotifyCard', () => {
  beforeEach(() => {
    vi.useFakeTimers()
    vi.resetAllMocks()
    sendNotifyEmailCode.mockResolvedValue({})
    getProfile.mockResolvedValue({ balance_notify_extra_emails: [] })
  })

  afterEach(() => {
    vi.clearAllTimers()
    vi.useRealTimers()
  })

  it.each(['remove', 'unmount'])('does not start a pending email timer after %s', async (action) => {
    const request = deferred()
    sendNotifyEmailCode.mockReturnValueOnce(request.promise)
    const wrapper = mount(ProfileBalanceNotifyCard, {
      props: { enabled: true, threshold: null, extraEmails: [], systemDefaultThreshold: 5, userEmail: '' }
    })
    await wrapper.get('input[type="email"]').setValue('new@example.com')
    await button(wrapper, 'common.add').trigger('click')
    await button(wrapper, 'profile.balanceNotify.sendCode').trigger('click')
    if (action === 'remove') await button(wrapper, 'profile.balanceNotify.removeEmail').trigger('click')
    else wrapper.unmount()

    request.resolve()
    await flushPromises()
    expect(vi.getTimerCount()).toBe(0)
  })

  it('does not start a saved email timer after unmount', async () => {
    const request = deferred()
    sendNotifyEmailCode.mockReturnValueOnce(request.promise)
    const wrapper = mount(ProfileBalanceNotifyCard, {
      props: {
        enabled: true, threshold: null, systemDefaultThreshold: 5, userEmail: '',
        extraEmails: [{ email: 'saved@example.com', disabled: false, verified: false }]
      }
    })
    await button(wrapper, 'profile.balanceNotify.verify').trigger('click')
    wrapper.unmount()
    request.resolve()
    await flushPromises()
    expect(vi.getTimerCount()).toBe(0)
  })

  it('serializes verification and profile refresh before another list write', async () => {
    const request = deferred()
    verifyNotifyEmail.mockReturnValueOnce(request.promise).mockResolvedValue({})
    const wrapper = mount(ProfileBalanceNotifyCard, {
      props: { enabled: true, threshold: null, extraEmails: [], systemDefaultThreshold: 5, userEmail: '' }
    })
    for (const email of ['first@example.com', 'second@example.com']) {
      await wrapper.get('input[type="email"]').setValue(email)
      await button(wrapper, 'common.add').trigger('click')
    }
    for (const row of pendingRows(wrapper)) {
      await row.findAll('button').find(item => item.text() === 'profile.balanceNotify.sendCode')!.trigger('click')
    }
    await flushPromises()
    for (const row of pendingRows(wrapper)) await row.get('input').setValue('123456')
    const verify = (index: number) => pendingRows(wrapper)[index]!.findAll('button')
      .find(item => item.text() === 'profile.balanceNotify.verify')!
    await verify(0).trigger('click')
    expect(verify(1).element.matches(':disabled')).toBe(true)
    await verify(1).trigger('click')
    expect(verifyNotifyEmail).toHaveBeenCalledTimes(1)
    request.resolve()
    await flushPromises()
    expect(pendingEmails(wrapper)).toEqual(['second@example.com'])
    await verify(0).trigger('click')
    await flushPromises()
    expect(verifyNotifyEmail.mock.calls).toEqual([
      ['first@example.com', '123456'], ['second@example.com', '123456']
    ])
    expect(pendingEmails(wrapper)).toEqual([])
    expect(getProfile).toHaveBeenCalledTimes(2)
  })

  it('sends the checked email state, locks list writes, and restores failure before retry', async () => {
    let reject!: (error: Error) => void
    toggleNotifyEmail.mockReturnValueOnce(new Promise((_, fail) => { reject = fail }))
    const entries = [
      { email: 'first@example.com', disabled: false, verified: true },
      { email: 'second@example.com', disabled: false, verified: true }
    ]
    const wrapper = mount(ProfileBalanceNotifyCard, {
      props: { enabled: true, threshold: null, extraEmails: entries, systemDefaultThreshold: 5, userEmail: '' }
    })
    const email = wrapper.get('input[aria-label="first@example.com"]')
    await email.setValue(false)
    expect(toggleNotifyEmail).toHaveBeenLastCalledWith('first@example.com', true)
    await email.setValue(true)
    expect(toggleNotifyEmail).toHaveBeenCalledTimes(1)
    expect(wrapper.get('input[aria-label="second@example.com"]').element.matches(':disabled')).toBe(true)
    expect(wrapper.get('input[aria-label="profile.balanceNotify.enabled"]').element.matches(':disabled')).toBe(true)
    await button(wrapper, 'profile.balanceNotify.removeEmail').trigger('click')
    expect(removeNotifyEmail).not.toHaveBeenCalled()
    reject(new Error('offline'))
    await flushPromises()
    expect((email.element as HTMLInputElement).checked).toBe(true)
    expect(showError).toHaveBeenCalledTimes(1)
    toggleNotifyEmail.mockResolvedValueOnce({ balance_notify_extra_emails: entries.map(entry => ({ ...entry, disabled: true })) })
    await email.setValue(false)
    await flushPromises()
    expect(toggleNotifyEmail).toHaveBeenCalledTimes(2)
    expect((email.element as HTMLInputElement).checked).toBe(false)
    toggleNotifyEmail.mockResolvedValueOnce({ balance_notify_extra_emails: entries })
    await email.setValue(true)
    await flushPromises()
    expect(toggleNotifyEmail).toHaveBeenLastCalledWith('first@example.com', false)
    expect((email.element as HTMLInputElement).checked).toBe(true)
  })

  it('locks the main switch while saving and restores its previous state on failure', async () => {
    let reject!: (error: Error) => void
    updateProfile.mockReturnValueOnce(new Promise((_, fail) => { reject = fail }))
    const wrapper = mount(ProfileBalanceNotifyCard, {
      props: { enabled: true, threshold: null, extraEmails: [], systemDefaultThreshold: 5, userEmail: '' }
    })
    const toggle = wrapper.get('input[aria-label="profile.balanceNotify.enabled"]')
    await toggle.setValue(false)
    expect(toggle.attributes('disabled')).toBeDefined()
    await toggle.setValue(true)
    expect(updateProfile).toHaveBeenCalledTimes(1)
    reject(new Error('offline'))
    await flushPromises()
    expect((toggle.element as HTMLInputElement).checked).toBe(true)
    updateProfile.mockResolvedValueOnce({ balance_notify_enabled: false })
    await toggle.setValue(false)
    await flushPromises()
    expect(updateProfile).toHaveBeenLastCalledWith({ balance_notify_enabled: false })
    expect((toggle.element as HTMLInputElement).checked).toBe(false)
  })
})
