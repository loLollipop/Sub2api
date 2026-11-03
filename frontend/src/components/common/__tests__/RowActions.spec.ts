import { mount, flushPromises } from '@vue/test-utils'
import { afterEach, describe, expect, it, vi } from 'vitest'
import { defineComponent } from 'vue'
import RowActions from '../RowActions.vue'

vi.mock('vue-i18n', () => ({ useI18n: () => ({ t: (key: string) => key }) }))
const wrappers: ReturnType<typeof mount>[] = []
afterEach(() => { wrappers.splice(0).forEach(wrapper => wrapper.unmount()); document.body.innerHTML = '' })

function render() {
  const select = vi.fn()
  const wrapper = mount(RowActions, {
    attachTo: document.body,
    slots: { default: defineComponent({ setup: () => ({ select }), template: '<button @click="select">Copy</button><button disabled>Unavailable</button><button>Delete</button>' }) }
  })
  wrappers.push(wrapper)
  return { wrapper, select }
}

describe('RowActions', () => {
  it('renders outside the table, skips disabled items and restores focus on Escape', async () => {
    const { wrapper } = render()
    await wrapper.get('button').trigger('keydown', { key: 'ArrowDown' })
    await flushPromises()
    const menu = document.querySelector<HTMLElement>('[role="menu"]')!
    const buttons = menu.querySelectorAll('button')
    expect(menu.parentElement).toBe(document.body)
    expect(document.activeElement).toBe(buttons[0])
    menu.dispatchEvent(new KeyboardEvent('keydown', { key: 'ArrowDown', bubbles: true }))
    expect(document.activeElement).toBe(buttons[2])
    menu.dispatchEvent(new KeyboardEvent('keydown', { key: 'Home', bubbles: true }))
    expect(document.activeElement).toBe(buttons[0])
    menu.dispatchEvent(new KeyboardEvent('keydown', { key: 'Escape', bubbles: true }))
    await flushPromises()
    expect(document.querySelector('[role="menu"]')).toBeNull()
    expect(document.activeElement).toBe(wrapper.get('button').element)
  })

  it('executes an action once and dismisses the menu', async () => {
    const { wrapper, select } = render()
    await wrapper.get('button').trigger('click')
    await flushPromises()
    document.querySelector<HTMLButtonElement>('[role="menuitem"]')!.click()
    await flushPromises()
    expect(select).toHaveBeenCalledTimes(1)
    expect(wrapper.get('button').attributes('aria-expanded')).toBe('false')
    expect(document.activeElement).toBe(wrapper.get('button').element)
  })

  it('dismisses on outside interaction and viewport changes', async () => {
    const { wrapper } = render()
    await wrapper.get('button').trigger('click')
    await flushPromises()
    document.body.dispatchEvent(new Event('pointerdown', { bubbles: true }))
    await flushPromises()
    expect(document.querySelector('[role="menu"]')).toBeNull()
    await wrapper.get('button').trigger('click')
    await flushPromises()
    window.dispatchEvent(new Event('resize'))
    await flushPromises()
    expect(document.querySelector('[role="menu"]')).toBeNull()
  })
})
