import { afterEach, describe, expect, it } from 'vitest'
import { enableAutoUnmount, mount } from '@vue/test-utils'
import { defineComponent, nextTick, ref } from 'vue'
import TotpCodeInput from '../TotpCodeInput.vue'

enableAutoUnmount(afterEach)

function input(initial = '', disabled = false) {
  const code = ref(initial)
  const wrapper = mount(TotpCodeInput, {
    attachTo: document.body,
    props: { modelValue: code.value, label: 'Authenticator code', disabled,
      'onUpdate:modelValue': value => { code.value = value; void wrapper.setProps({ modelValue: value }) } },
  })
  return { wrapper, field: wrapper.get('input'), code }
}

describe('authenticator code input', () => {
  it('uses one accessible autofill field and preserves a leading zero', async () => {
    const { wrapper, field, code } = input()
    expect(wrapper.findAll('input')).toHaveLength(1)
    expect(field.attributes('autocomplete')).toBe('one-time-code')
    expect(field.attributes('aria-label')).toBe('Authenticator code')
    await field.setValue('012345')
    expect(code.value).toBe('012345')
    expect(wrapper.emitted('complete')).toEqual([['012345']])
    expect(wrapper.findAll('span').map(cell => cell.text()).join('')).toBe('012345')
    await field.trigger('input')
    expect(wrapper.emitted('complete')).toHaveLength(1)
  })

  it('normalizes fullwidth digits and pasted separators', async () => {
    const { wrapper, field, code } = input()
    await field.trigger('paste', { clipboardData: { getData: () => '０１ ２-３４５' } })
    expect(code.value).toBe('012345')
    expect(wrapper.emitted('complete')).toEqual([['012345']])
  })

  it('rejects an overlong paste rather than submitting a truncated code', async () => {
    const { wrapper, field, code } = input('123')
    await field.trigger('paste', { clipboardData: { getData: () => '0123456' } })
    expect(code.value).toBe('')
    expect((field.element as HTMLInputElement).value).toBe('')
    expect(wrapper.emitted('complete')).toBeUndefined()
  })

  it('does not submit retained or externally supplied codes and resets the visible cells', async () => {
    const { wrapper, field } = input('654321')
    await nextTick()
    expect(wrapper.emitted('complete')).toBeUndefined()
    await wrapper.setProps({ modelValue: '012345' })
    expect((field.element as HTMLInputElement).value).toBe('012345')
    expect(wrapper.emitted('complete')).toBeUndefined()
    await wrapper.setProps({ modelValue: '' })
    expect((field.element as HTMLInputElement).value).toBe('')
    expect(wrapper.findAll('span').every(cell => !/[0-9]/.test(cell.text()))).toBe(true)
  })

  it('ignores input and paste while verification is pending', async () => {
    const { wrapper, field } = input('', true)
    await field.trigger('paste', { clipboardData: { getData: () => '123456' } })
    await field.setValue('654321')
    expect(wrapper.emitted('update:modelValue')).toBeUndefined()
    expect(wrapper.emitted('complete')).toBeUndefined()
  })

  it('supports caret editing and backspace in the single native field', async () => {
    const Harness = defineComponent({ components: { TotpCodeInput }, setup: () => ({ code: ref('012345') }),
      template: '<TotpCodeInput v-model="code" label="Code" />' })
    const wrapper = mount(Harness, { attachTo: document.body })
    const field = wrapper.get('input').element as HTMLInputElement
    field.focus()
    field.setSelectionRange(2, 3)
    field.setRangeText('9', 2, 3, 'end')
    field.dispatchEvent(new Event('input', { bubbles: true }))
    await nextTick()
    expect(field.value).toBe('019345')
    expect(wrapper.vm.code).toBe('019345')
    field.setRangeText('', 2, 3, 'end')
    field.dispatchEvent(new Event('input', { bubbles: true }))
    await nextTick()
    expect(wrapper.vm.code).toBe('01345')
  })
})
