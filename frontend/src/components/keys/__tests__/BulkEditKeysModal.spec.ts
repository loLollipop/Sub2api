import { flushPromises, mount } from '@vue/test-utils'
import { beforeEach, describe, expect, it, vi } from 'vitest'
import BulkEditKeysModal from '../BulkEditKeysModal.vue'

const { update } = vi.hoisted(() => ({ update: vi.fn() }))
vi.mock('@/api', () => ({ keysAPI: { update } }))
vi.mock('vue-i18n', () => ({ useI18n: () => ({ t: (key: string) => key }) }))

function createModal(ids = [1, 2]) {
  return mount(BulkEditKeysModal, {
    props: { selectedIds: ids, groupOptions: [{ value: 42, label: 'Group' }] },
    global: {
      stubs: {
        BaseDialog: { name: 'BaseDialog', props: ['showCloseButton'], template: '<div><slot /><slot name="footer" /></div>' },
        Select: {
          props: ['modelValue', 'options'], emits: ['update:modelValue'],
          template: '<select :value="modelValue" @change="$emit(\'update:modelValue\', Number($event.target.value))"><option value="">Choose</option><option v-for="o in options" :value="o.value">{{ o.label }}</option></select>'
        }
      }
    }
  })
}

describe('BulkEditKeysModal', () => {
  beforeEach(() => { update.mockReset(); update.mockResolvedValue({}) })

  it('sends only checked fields, preserving zero and explicit empty lists/expiry', async () => {
    const wrapper = createModal()
    await wrapper.get('[data-test="enable-quota"]').setValue(true)
    await wrapper.get('[data-test="bulk-edit-quota"]').setValue('99')
    await wrapper.get('[data-test="enable-quota"]').setValue(false)
    for (const field of ['rateLimit5h', 'expiresAt', 'ipWhitelist']) {
      await wrapper.get(`[data-test="enable-${field}"]`).setValue(true)
    }
    await wrapper.get('[data-test="bulk-edit-rateLimit5h"]').setValue('0')
    await wrapper.get('form').trigger('submit')
    await flushPromises()
    expect(update.mock.calls).toEqual([
      [1, { rate_limit_5h: 0, expires_at: '', ip_whitelist: [] }],
      [2, { rate_limit_5h: 0, expires_at: '', ip_whitelist: [] }]
    ])
    expect(wrapper.emitted('applied')).toEqual([[{ updated: 2, failed: 0 }]])
    wrapper.unmount()
  })

  it('requires a selected group and explicitly replaces all smart routes', async () => {
    const wrapper = createModal([1])
    await wrapper.get('[data-test="enable-group"]').setValue(true)
    await wrapper.get('form').trigger('submit')
    expect(update).not.toHaveBeenCalled()
    expect(wrapper.get('[role="alert"]').text()).toBe('keys.bulkEdit.invalidGroup')
    await wrapper.get('[data-test="bulk-edit-group"]').setValue('42')
    await wrapper.get('form').trigger('submit')
    await flushPromises()
    expect(update).toHaveBeenCalledWith(1, { group_ids: [42] })
    wrapper.unmount()
  })

  it.each(['', '-1'])('rejects invalid amounts before changing any key (%s)', async amount => {
    const wrapper = createModal()
    await wrapper.get('[data-test="enable-quota"]').setValue(true)
    await wrapper.get('[data-test="bulk-edit-quota"]').setValue(amount)
    await wrapper.get('form').trigger('submit')
    expect(update).not.toHaveBeenCalled()
    expect(wrapper.get('[role="alert"]').text()).toBe('keys.bulkEdit.invalidNumber')
    wrapper.unmount()
  })

  it('rejects past expiry and sends future expiry in ISO format', async () => {
    const wrapper = createModal([1])
    await wrapper.get('[data-test="enable-expiresAt"]').setValue(true)
    await wrapper.get('[data-test="bulk-edit-expires"]').setValue('2001-01-01T00:00')
    await wrapper.get('form').trigger('submit')
    expect(update).not.toHaveBeenCalled()
    await wrapper.get('[data-test="bulk-edit-expires"]').setValue('2099-01-01T00:00')
    await wrapper.get('form').trigger('submit')
    await flushPromises()
    expect(update).toHaveBeenCalledWith(1, { expires_at: new Date('2099-01-01T00:00').toISOString() })
    wrapper.unmount()
  })

  it('freezes targets and blocks duplicate submissions and closure while saving', async () => {
    let resolve!: (value: object) => void
    update.mockImplementationOnce(() => new Promise(r => { resolve = r }))
    update.mockRejectedValueOnce(new Error('second failed'))
    const wrapper = createModal([1, 2, 1])
    await wrapper.get('[data-test="enable-status"]').setValue(true)
    await wrapper.get('form').trigger('submit')
    await wrapper.get('form').trigger('submit')
    wrapper.findComponent({ name: 'BaseDialog' }).vm.$emit('close')
    expect(wrapper.emitted('close')).toBeUndefined()
    expect(wrapper.get('fieldset').attributes('disabled')).toBeDefined()
    expect(update).toHaveBeenCalledTimes(1)
    await wrapper.setProps({ selectedIds: [99] })
    resolve({})
    await flushPromises()
    expect(update.mock.calls.map(call => call[0])).toEqual([1, 2])
    expect(wrapper.emitted('applied')).toEqual([[{ updated: 1, failed: 1 }]])
    wrapper.unmount()
  })
})
