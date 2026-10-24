import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { enableAutoUnmount, flushPromises, mount } from '@vue/test-utils'
import { nextTick } from 'vue'
import TotpStepUpDialog from '../TotpStepUpDialog.vue'
import BaseDialog from '@/components/common/BaseDialog.vue'
import { useStepUp } from '@/composables/useStepUp'

const mocks = vi.hoisted(() => ({ stepUp: vi.fn(), showError: vi.fn() }))
vi.mock('@/api', () => ({ totpAPI: { stepUp: mocks.stepUp } }))
vi.mock('@/stores', () => ({ useAppStore: () => ({ showError: mocks.showError }) }))
vi.mock('vue-i18n', () => ({ useI18n: () => ({ t: (key: string) => key }) }))
enableAutoUnmount(afterEach)
afterEach(() => { document.body.innerHTML = '' })
beforeEach(() => { vi.resetAllMocks(); mocks.stepUp.mockResolvedValue({}) })

function field() { return document.body.querySelector<HTMLInputElement>('input[autocomplete="one-time-code"]')! }
async function enter(code: string) {
  field().value = code
  field().dispatchEvent(new Event('input', { bubbles: true }))
  await nextTick()
}
function escape() { document.dispatchEvent(new KeyboardEvent('keydown', { key: 'Escape', bubbles: true, cancelable: true })) }

describe('nested administrator verification', () => {
  it('opens above a custom parent, owns keyboard focus and restores the unchanged parent on cancel', async () => {
    const parent = mount(BaseDialog, { attachTo: document.body,
      props: { show: true, title: 'Parent', zIndex: 200, showCloseButton: false },
      slots: { default: '<input id="parent-value" value="Retain me" /><button id="parent-submit">Submit</button>' } })
    await nextTick()
    const trigger = document.getElementById('parent-submit')!
    trigger.focus()
    const controller = useStepUp()
    mount(TotpStepUpDialog, { props: { controller } })
    const result = controller.prompt()
    await nextTick(); await nextTick()
    const dialogs = Array.from(document.body.querySelectorAll<HTMLElement>('[role="dialog"]'))
    const child = dialogs.find(dialog => dialog.textContent?.includes('stepUp.title'))!
    const outer = dialogs.find(dialog => dialog.textContent?.includes('Parent'))!
    expect(Number(child.style.zIndex)).toBeGreaterThan(200)
    expect(child.getAttribute('aria-modal')).toBe('true')
    expect(outer.getAttribute('aria-hidden')).toBe('true')
    expect(outer.hasAttribute('aria-modal')).toBe(false)
    expect(document.activeElement).toBe(field())
    const tab = new KeyboardEvent('keydown', { key: 'Tab', bubbles: true, cancelable: true })
    field().dispatchEvent(tab)
    expect(child.contains(document.activeElement)).toBe(true)
    escape()
    expect(await result).toBe(false)
    await nextTick()
    expect(parent.emitted('close')).toBeUndefined()
    expect((document.getElementById('parent-value') as HTMLInputElement).value).toBe('Retain me')
    expect(document.activeElement).toBe(trigger)
    expect(outer.getAttribute('aria-modal')).toBe('true')
    expect(document.body.classList.contains('modal-open')).toBe(true)
  })

  it('sends only one verification request and ignores Escape while pending', async () => {
    let finish!: () => void
    mocks.stepUp.mockImplementationOnce(() => new Promise<void>(resolve => { finish = resolve }))
    const controller = useStepUp()
    mount(TotpStepUpDialog, { props: { controller } })
    const result = controller.prompt()
    await nextTick()
    await enter('012345')
    await enter('654321')
    escape()
    expect(controller.visible.value).toBe(true)
    expect(mocks.stepUp).toHaveBeenCalledOnce()
    expect(mocks.stepUp).toHaveBeenCalledWith('012345')
    finish()
    await flushPromises()
    expect(await result).toBe(true)
    expect(controller.visible.value).toBe(false)
  })

  it('clears rejected codes, focuses the input and supports a retry', async () => {
    mocks.stepUp.mockRejectedValueOnce({ message: 'Expired code' })
    const controller = useStepUp()
    mount(TotpStepUpDialog, { props: { controller } })
    const result = controller.prompt()
    await nextTick()
    await enter('123456')
    await flushPromises()
    expect(mocks.showError).toHaveBeenCalledWith('Expired code')
    expect(field().value).toBe('')
    expect(document.activeElement).toBe(field())
    await enter('012345')
    await flushPromises()
    expect(await result).toBe(true)
    expect(mocks.stepUp).toHaveBeenLastCalledWith('012345')
  })

  it('ignores an old verification response after the dialog closes and reopens', async () => {
    let finish!: () => void
    mocks.stepUp.mockImplementationOnce(() => new Promise<void>(resolve => { finish = resolve }))
    const controller = useStepUp()
    mount(TotpStepUpDialog, { props: { controller } })
    const oldResult = controller.prompt()
    await nextTick()
    await enter('123456')
    controller.onCancel()
    expect(await oldResult).toBe(false)
    const newResult = controller.prompt()
    await nextTick()
    finish()
    await flushPromises()
    expect(controller.visible.value).toBe(true)
    expect(field().value).toBe('')
    await enter('654321')
    await flushPromises()
    expect(await newResult).toBe(true)
  })
})
