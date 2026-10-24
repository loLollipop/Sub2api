import { enableAutoUnmount, mount } from '@vue/test-utils'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import TotpLoginModal from '@/components/auth/TotpLoginModal.vue'

const { showErrorMock } = vi.hoisted(() => ({
  showErrorMock: vi.fn(),
}))

vi.mock('vue-i18n', () => ({
  useI18n: () => ({
    t: (key: string) => key,
  }),
}))

vi.mock('@/stores', () => ({
  useAppStore: () => ({
    showError: (...args: any[]) => showErrorMock(...args),
  }),
}))

enableAutoUnmount(afterEach)
const mountLogin = () => mount(TotpLoginModal, { props: { tempToken: 'temp-token' },
  global: { stubs: { teleport: true } }, attachTo: document.body })

describe('TotpLoginModal', () => {
  beforeEach(() => {
    showErrorMock.mockReset()
  })

  it('sends verification errors to toast and does not render inline red text', async () => {
    const wrapper = mount(TotpLoginModal, {
      props: {
        tempToken: 'temp-token',
        userEmailMasked: 'u***@example.com',
      },
    })

    ;(wrapper.vm as unknown as { setError: (message: string) => void }).setError('Invalid code')
    await wrapper.vm.$nextTick()

    expect(showErrorMock).toHaveBeenCalledWith('Invalid code')
    expect(wrapper.text()).not.toContain('Invalid code')
    expect(wrapper.find('.bg-red-50').exists()).toBe(false)
  })

  it('submits once and blocks cancellation until verification finishes', async () => {
    const wrapper = mountLogin()
    const field = wrapper.get('input[autocomplete="one-time-code"]')
    await field.setValue('012345')
    await field.trigger('input')
    expect(wrapper.emitted('verify')).toEqual([['012345']])
    expect(wrapper.get('input[autocomplete="one-time-code"]').attributes('disabled')).toBeDefined()
    document.dispatchEvent(new KeyboardEvent('keydown', { key: 'Escape' }))
    expect(wrapper.emitted('cancel')).toBeUndefined()
  })

  it('clears a rejected code and allows the same code to be retried', async () => {
    const wrapper = mountLogin()
    const field = wrapper.get('input[autocomplete="one-time-code"]')
    await field.setValue('012345')
    ;(wrapper.vm as unknown as { setError: (message: string) => void }).setError('Try again')
    await wrapper.vm.$nextTick()
    expect((wrapper.get('input[autocomplete="one-time-code"]').element as HTMLInputElement).value).toBe('')
    expect(document.activeElement).toBe(wrapper.get('input[autocomplete="one-time-code"]').element)
    await wrapper.get('input[autocomplete="one-time-code"]').setValue('012345')
    expect(wrapper.emitted('verify')).toEqual([['012345'], ['012345']])
  })
})
