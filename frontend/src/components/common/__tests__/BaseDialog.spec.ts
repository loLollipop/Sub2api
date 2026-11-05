import { afterEach, describe, expect, it, vi } from 'vitest'
import { mount } from '@vue/test-utils'
import { nextTick } from 'vue'
import BaseDialog from '../BaseDialog.vue'

vi.mock('vue-i18n', () => ({
  useI18n: () => ({ t: (key: string) => key })
}))

describe('BaseDialog', () => {
  afterEach(() => {
    document.body.innerHTML = ''
    document.body.classList.remove('modal-open')
  })

  it('places description and actions in the header and describes the dialog', () => {
    const wrapper = mount(BaseDialog, {
      attachTo: document.body,
      props: { show: true, title: 'Details' },
      slots: { description: 'Published today', actions: '<button>Mark all read</button>', default: 'Body' },
      global: { stubs: { Teleport: true, Transition: true, Icon: true } },
    })
    const description = wrapper.get('.modal-header .modal-description')
    expect(description.text()).toBe('Published today')
    expect(wrapper.get('[role="dialog"]').attributes('aria-describedby')).toBe(description.attributes('id'))
    expect(wrapper.get('.modal-header-actions button').text()).toBe('Mark all read')
    expect(wrapper.get('.modal-body').text()).toBe('Body')
    wrapper.unmount()
  })

  it('resets body scroll position when reopened', async () => {
    const wrapper = mount(BaseDialog, {
      attachTo: document.body,
      props: { show: false, title: 'Details' },
      slots: { default: '<div style="height: 2000px">content</div>' },
      global: { stubs: { Icon: true } }
    })

    await wrapper.setProps({ show: true })
    await nextTick()
    const body = document.body.querySelector<HTMLElement>('.modal-body')
    expect(body).not.toBeNull()
    body!.scrollTop = 480

    await wrapper.setProps({ show: false })
    await wrapper.setProps({ show: true })
    await nextTick()

    expect(document.body.querySelector<HTMLElement>('.modal-body')?.scrollTop).toBe(0)
    wrapper.unmount()
  })
})
