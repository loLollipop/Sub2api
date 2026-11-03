import { defineComponent, ref } from 'vue'
import { flushPromises, mount } from '@vue/test-utils'
import { afterEach, describe, expect, it, vi } from 'vitest'
import AccountActionMenu from '../AccountActionMenu.vue'
import type { Account } from '@/types'

vi.mock('vue-i18n', () => ({ useI18n: () => ({ t: (key: string) => key }) }))
const wrappers: ReturnType<typeof mount>[] = []
afterEach(() => { wrappers.splice(0).forEach(wrapper => wrapper.unmount()); document.body.innerHTML = '' })

function render() {
  const account = { id: 7, platform: 'openai', type: 'apikey', status: 'active' } as Account
  const remove = vi.fn()
  const wrapper = mount(defineComponent({
    components: { AccountActionMenu },
    setup: () => ({ show: ref(false), account, remove, rect: new DOMRect(20, 20, 32, 32) }),
    template: '<button @click="show = true">More</button><AccountActionMenu :show="show" :account="account" :anchor-rect="rect" @close="show = false" @delete="remove" />'
  }), { attachTo: document.body })
  wrappers.push(wrapper)
  return { wrapper, remove }
}

describe('AccountActionMenu keyboard access', () => {
  it('focuses its first item, navigates to delete, then returns to the trigger on Escape', async () => {
    const { wrapper } = render()
    wrapper.get('button').element.focus()
    await wrapper.get('button').trigger('click')
    await flushPromises()
    const menu = document.querySelector<HTMLElement>('[role="menu"]')!
    const items = menu.querySelectorAll<HTMLButtonElement>('[role="menuitem"]')
    expect(document.activeElement).toBe(items[0])
    menu.dispatchEvent(new KeyboardEvent('keydown', { key: 'End', bubbles: true }))
    expect(document.activeElement?.textContent).toContain('common.delete')
    menu.dispatchEvent(new KeyboardEvent('keydown', { key: 'Escape', bubbles: true }))
    await flushPromises()
    expect(document.querySelector('[role="menu"]')).toBeNull()
    expect(document.activeElement).toBe(wrapper.get('button').element)
  })

  it('emits the selected account for deletion and closes the menu', async () => {
    const { wrapper, remove } = render()
    await wrapper.get('button').trigger('click')
    await flushPromises()
    const items = document.querySelectorAll<HTMLButtonElement>('[role="menuitem"]')
    items[items.length - 1].click()
    await flushPromises()
    expect(remove).toHaveBeenCalledTimes(1)
    expect(remove).toHaveBeenCalledWith(expect.objectContaining({ id: 7 }))
    expect(document.querySelector('[role="menu"]')).toBeNull()
  })
})
