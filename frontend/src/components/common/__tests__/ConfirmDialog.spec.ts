import { shallowMount } from '@vue/test-utils'
import { describe, expect, it, vi } from 'vitest'
import ConfirmDialog from '../ConfirmDialog.vue'

vi.mock('vue-i18n', () => ({ useI18n: () => ({ t: (key: string) => key }) }))

describe('ConfirmDialog source button styles', () => {
  for (const danger of [false, true]) {
    it(`preserves confirmation events with danger=${danger}`, async () => {
      const wrapper = shallowMount(ConfirmDialog, {
        props: { show: true, title: 'Confirm', message: 'Continue?', danger },
        global: { stubs: { BaseDialog: { template: '<div><slot /><slot name="footer" /></div>' } } }
      })
      const [cancel, confirm] = wrapper.findAll('button')
      expect(cancel.classes()).toEqual(expect.arrayContaining(['btn', 'btn-secondary']))
      expect(confirm.classes()).toEqual(expect.arrayContaining(['btn', danger ? 'btn-danger' : 'btn-primary']))
      await cancel.trigger('click')
      await confirm.trigger('click')
      expect(wrapper.emitted('cancel')).toHaveLength(1)
      expect(wrapper.emitted('confirm')).toHaveLength(1)
      wrapper.unmount()
    })
  }
})
