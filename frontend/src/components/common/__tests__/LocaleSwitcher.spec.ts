import { flushPromises, mount } from '@vue/test-utils'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'

import LocaleSwitcher from '../LocaleSwitcher.vue'

const { setLocale } = vi.hoisted(() => ({ setLocale: vi.fn() }))

vi.mock('@/i18n', () => ({
  setLocale,
  availableLocales: [
    { code: 'en', name: 'English', flag: '🇺🇸' },
    { code: 'zh', name: '中文', flag: '🇨🇳' }
  ]
}))

vi.mock('vue-i18n', () => ({ useI18n: () => ({ locale: { value: 'zh' } }) }))

const reload = vi.fn()

const wrappers: ReturnType<typeof mount>[] = []
const mountSwitcher = (compact = false) => {
  const wrapper = mount(LocaleSwitcher, { props: { compact }, attachTo: document.body, global: { stubs: { Icon: true, transition: false } } })
  wrappers.push(wrapper)
  return wrapper
}

const choose = async (wrapper: ReturnType<typeof mountSwitcher>, name: string) => {
  await wrapper.get('button').trigger('click')
  const option = wrapper.findAll('button').find((button) => button.text().includes(name))
  await option!.trigger('click')
  await flushPromises()
}

describe('LocaleSwitcher', () => {
  beforeEach(() => {
    setLocale.mockReset().mockResolvedValue(undefined)
    reload.mockReset()
    vi.stubGlobal('location', { ...window.location, reload })
  })

  afterEach(() => {
    wrappers.splice(0).forEach(wrapper => wrapper.unmount())
    vi.unstubAllGlobals()
  })

  // 分组名称等内容由后端按请求语言返回，已加载的数据要靠整页刷新换成新语言
  it('reloads the page after switching to another language', async () => {
    const wrapper = mountSwitcher()

    await choose(wrapper, 'English')

    expect(setLocale).toHaveBeenCalledWith('en')
    expect(reload).toHaveBeenCalledTimes(1)
    expect(setLocale.mock.invocationCallOrder[0]).toBeLessThan(reload.mock.invocationCallOrder[0])
  })

  it('does nothing when the current language is chosen again', async () => {
    const wrapper = mountSwitcher()

    await choose(wrapper, '中文')

    expect(setLocale).not.toHaveBeenCalled()
    expect(reload).not.toHaveBeenCalled()
  })

  it('opens the compact header control with the keyboard and restores focus on Escape', async () => {
    const wrapper = mountSwitcher(true)
    const trigger = wrapper.get<HTMLButtonElement>('button')
    expect(trigger.attributes('aria-label')).toBe('切换语言')
    expect(trigger.find('svg').exists()).toBe(true)
    expect(trigger.text()).toBe('')
    expect(trigger.attributes('aria-expanded')).toBe('false')
    await trigger.trigger('keydown', { key: 'ArrowDown' })
    await flushPromises()
    const options = wrapper.findAll('.locale-option')
    expect(trigger.attributes('aria-controls')).toBe(wrapper.get('.locale-options').attributes('id'))
    expect(document.activeElement).toBe(options[0].element)
    expect(options[1].attributes('aria-pressed')).toBe('true')
    await options[0].trigger('keydown', { key: 'End' })
    expect(document.activeElement).toBe(options[1].element)
    await options[1].trigger('keydown', { key: 'ArrowDown' })
    expect(document.activeElement).toBe(options[0].element)
    await options[0].trigger('keydown', { key: 'Escape' })
    expect(trigger.attributes('aria-expanded')).toBe('false')
    expect(document.activeElement).toBe(trigger.element)
  })

  it('dismisses the header options when clicking outside or tabbing away', async () => {
    const wrapper = mountSwitcher(true)
    const trigger = wrapper.get('button')
    await trigger.trigger('click')
    document.body.click()
    await flushPromises()
    expect(trigger.attributes('aria-expanded')).toBe('false')
    await trigger.trigger('click')
    await wrapper.trigger('focusout', { relatedTarget: document.body })
    expect(trigger.attributes('aria-expanded')).toBe('false')
  })
})
