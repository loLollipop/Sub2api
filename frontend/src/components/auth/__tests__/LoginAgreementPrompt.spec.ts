import { afterEach, describe, expect, it, vi } from 'vitest'
import { enableAutoUnmount, mount } from '@vue/test-utils'
import { nextTick } from 'vue'
import LoginAgreementPrompt from '../LoginAgreementPrompt.vue'

vi.mock('vue-i18n', () => ({ useI18n: () => ({ t: (key: string) => key }) }))
enableAutoUnmount(afterEach)
afterEach(() => { document.body.innerHTML = '' })

describe('login agreement dialog', () => {
  it('keeps focus in the terms, requires an explicit choice, and restores focus and scrolling', async () => {
    const trigger = document.createElement('button')
    document.body.appendChild(trigger)
    trigger.focus()
    const wrapper = mount(LoginAgreementPrompt, {
      attachTo: document.body,
      props: { accepted: false, visible: true, mode: 'modal', documents: [{ id: 'privacy', title: 'Privacy policy' }] },
      global: { stubs: { RouterLink: { template: '<a href="/legal/privacy"><slot /></a>' } } },
    })
    await nextTick()
    const panel = document.querySelector('[role="dialog"]')!
    expect(panel.contains(document.activeElement)).toBe(true)
    expect(document.body.classList.contains('modal-open')).toBe(true)
    const buttons = [...panel.querySelectorAll('button')]
    buttons.at(-1)!.focus()
    document.activeElement!.dispatchEvent(new KeyboardEvent('keydown', { key: 'Tab', bubbles: true, cancelable: true }))
    expect(document.activeElement?.tagName).toBe('A')
    document.dispatchEvent(new KeyboardEvent('keydown', { key: 'Escape', bubbles: true, cancelable: true }))
    expect(wrapper.emitted('reject')).toBeUndefined()
    expect(panel.isConnected).toBe(true)
    buttons[0].click()
    expect(wrapper.emitted('reject')).toHaveLength(1)
    buttons[1].click()
    expect(wrapper.emitted('accept')).toHaveLength(1)
    await wrapper.setProps({ visible: false })
    expect(document.activeElement).toBe(trigger)
    expect(document.body.classList.contains('modal-open')).toBe(false)
  })
})
