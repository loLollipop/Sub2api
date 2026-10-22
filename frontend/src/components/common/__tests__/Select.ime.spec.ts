import { afterEach, describe, expect, it, vi } from 'vitest'
import { enableAutoUnmount, mount } from '@vue/test-utils'
import { h, nextTick } from 'vue'
import Select from '../Select.vue'
import BaseDialog from '../BaseDialog.vue'

vi.mock('vue-i18n', () => ({ useI18n: () => ({ t: (key: string) => key }) }))
enableAutoUnmount(afterEach)
afterEach(() => { document.body.innerHTML = '' })

describe('Select IME keyboard input', () => {
  it('keeps the enclosing dialog open when Escape cancels composition', async () => {
    const wrapper = mount(BaseDialog, {
      props: { show: true, title: 'Edit item' },
      slots: { default: () => h(Select, {
        modelValue: null,
        searchable: true,
        options: [{ value: 'first', label: 'First' }],
      }) },
    })
    document.querySelector<HTMLButtonElement>('.select-trigger')!.click()
    await nextTick()
    await nextTick()
    const input = document.querySelector<HTMLInputElement>('.select-search-input')!
    const composing = new KeyboardEvent('keydown', { key: 'Escape', isComposing: true, bubbles: true, cancelable: true })
    input.dispatchEvent(composing)
    await nextTick()
    expect(composing.defaultPrevented).toBe(false)
    expect(wrapper.emitted('close')).toBeUndefined()
    expect(document.querySelector('.select-dropdown-portal')).not.toBeNull()
    input.dispatchEvent(new KeyboardEvent('keydown', { key: 'Escape', bubbles: true, cancelable: true }))
    await nextTick()
    expect(document.querySelector('.select-trigger')!.getAttribute('aria-expanded')).toBe('false')
    expect(wrapper.emitted('close')).toBeUndefined()
    document.querySelector<HTMLButtonElement>('.select-trigger')!.dispatchEvent(new KeyboardEvent('keydown', { key: 'Escape', bubbles: true, cancelable: true }))
    expect(wrapper.emitted('close')).toHaveLength(1)
  })

  it.each(['Enter', 'Escape', 'ArrowDown', 'ArrowUp'])('leaves composing %s to the input method', async (key) => {
    const wrapper = mount(Select, {
      attachTo: document.body,
      props: { modelValue: null, searchable: true, options: [
        { value: 'first', label: 'First' }, { value: 'second', label: 'Second' }
      ] }
    })
    await wrapper.get('button').trigger('click')
    await nextTick()
    const input = document.querySelector<HTMLInputElement>('.select-search-input')!
    expect(document.activeElement).toBe(input)
    const composing = new KeyboardEvent('keydown', { key, isComposing: true, bubbles: true, cancelable: true })
    input.dispatchEvent(composing)
    await nextTick()
    expect(composing.defaultPrevented).toBe(false)
    expect(wrapper.emitted('update:modelValue')).toBeUndefined()
    expect(wrapper.get('button').attributes('aria-expanded')).toBe('true')
    input.dispatchEvent(new KeyboardEvent('keydown', { key: 'Enter', bubbles: true, cancelable: true }))
    await nextTick()
    expect(wrapper.emitted('update:modelValue')).toEqual([['first']])
  })
})
