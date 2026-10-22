import { afterEach, describe, expect, it, vi } from 'vitest'
import { enableAutoUnmount, mount } from '@vue/test-utils'
import { h, nextTick } from 'vue'
import BaseDialog from '../BaseDialog.vue'
import Select from '../Select.vue'

vi.mock('vue-i18n', () => ({ useI18n: () => ({ t: (key: string) => key }) }))
enableAutoUnmount(afterEach)
afterEach(() => { document.body.innerHTML = '' })

function pressTab(shiftKey = false) {
  const event = new KeyboardEvent('keydown', { key: 'Tab', shiftKey, bubbles: true, cancelable: true })
  document.activeElement?.dispatchEvent(event)
  return event
}

function triggerButton() {
  const button = document.createElement('button')
  document.body.appendChild(button)
  button.focus()
  return button
}

function dialog(title = 'Details', slot = '<button id="first">First</button><button id="last">Last</button>') {
  return mount(BaseDialog, {
    attachTo: document.body,
    props: { show: true, title, showCloseButton: false },
    slots: { default: slot },
  })
}

describe('BaseDialog keyboard focus', () => {
  it('focuses an initially open dialog and cycles past disabled and hidden controls', async () => {
    triggerButton()
    dialog('Details', '<button disabled>Disabled</button><button style="display:none">Hidden</button><div hidden><button>Hidden parent</button></div><input type="hidden" /><button id="first">First</button><input id="last" />')
    await nextTick()
    expect(document.activeElement?.id).toBe('first')
    expect(pressTab(true).defaultPrevented).toBe(true)
    expect(document.activeElement?.id).toBe('last')
    pressTab()
    expect(document.activeElement?.id).toBe('first')
    pressTab()
    expect(document.activeElement?.id).toBe('last')
  })

  it('keeps an empty dialog focused and restores the trigger on close and unmount', async () => {
    const trigger = triggerButton()
    const wrapper = dialog('Empty', '<p>No controls</p>')
    await nextTick()
    const panel = document.body.querySelector('.modal-content')
    expect(document.activeElement).toBe(panel)
    pressTab()
    pressTab(true)
    expect(document.activeElement).toBe(panel)
    await wrapper.setProps({ show: false })
    expect(document.activeElement).toBe(trigger)
    await wrapper.setProps({ show: true })
    await nextTick()
    expect(document.activeElement).toBe(document.body.querySelector('.modal-content'))
    wrapper.unmount()
    expect(document.activeElement).toBe(trigger)
  })

  it('tabs through disclosure summaries and skips controls inside collapsed details', async () => {
    dialog('Advanced', '<button id="before">Before</button><details id="advanced"><summary id="summary">Advanced</summary><input id="advanced-input" /><details><summary id="nested-summary">Nested</summary><input id="nested-input" /></details></details><button id="after">After</button>')
    await nextTick()
    pressTab()
    expect(document.activeElement?.id).toBe('summary')
    pressTab()
    expect(document.activeElement?.id).toBe('after')
    pressTab(true)
    expect(document.activeElement?.id).toBe('summary')

    document.querySelector<HTMLDetailsElement>('#advanced')!.open = true
    pressTab()
    expect(document.activeElement?.id).toBe('advanced-input')
    pressTab()
    expect(document.activeElement?.id).toBe('nested-summary')
    pressTab()
    expect(document.activeElement?.id).toBe('after')
    pressTab(true)
    expect(document.activeElement?.id).toBe('nested-summary')
  })

  it('cycles only in the top dialog and restores its parent trigger', async () => {
    const trigger = triggerButton()
    const parent = dialog()
    await nextTick()
    const parentLast = document.getElementById('last')!
    parentLast.focus()
    const child = dialog('Child', '<button id="child">Child action</button>')
    await nextTick()
    expect(document.activeElement?.id).toBe('child')
    pressTab()
    expect(document.activeElement?.id).toBe('child')
    child.unmount()
    expect(document.activeElement).toBe(parentLast)
    await parent.setProps({ show: false })
    expect(document.activeElement).toBe(trigger)
  })

  it('does not steal focus when a lower dialog closes behind the top layer', async () => {
    triggerButton()
    const parent = dialog()
    await nextTick()
    const child = dialog('Child', '<button id="child">Child action</button>')
    await nextTick()
    await parent.setProps({ show: false })
    expect(document.activeElement?.id).toBe('child')
    child.unmount()
    // The saved trigger was removed with its parent; never focus detached nodes.
    expect(document.activeElement?.isConnected).toBe(true)
  })

  it('preserves Select portal search focus and resumes Tab order from its trigger', async () => {
    const wrapper = mount(BaseDialog, {
      attachTo: document.body,
      props: { show: true, title: 'Select', showCloseButton: false },
      slots: { default: () => [
        h(Select, { modelValue: null, searchable: true, options: [{ value: 'one', label: 'One' }] }),
        h('button', { id: 'after-select' }, 'Next'),
      ] },
    })
    await nextTick()
    await wrapper.findComponent(Select).get('button.select-trigger').trigger('click')
    await nextTick()
    const search = document.body.querySelector<HTMLInputElement>('.select-search-input')!
    expect(document.activeElement).toBe(search)
    search.value = 'one'
    search.dispatchEvent(new Event('input', { bubbles: true }))
    pressTab()
    await nextTick()
    expect(document.activeElement?.id).toBe('after-select')
    expect(wrapper.findComponent(Select).get('button.select-trigger').attributes('aria-expanded')).toBe('false')
    pressTab()
    expect(document.activeElement?.classList.contains('select-trigger')).toBe(true)
  })
})
