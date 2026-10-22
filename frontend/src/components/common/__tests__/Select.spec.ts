import { mount } from '@vue/test-utils'
import { afterEach, describe, expect, it, vi } from 'vitest'
import { nextTick } from 'vue'

import Select from '../Select.vue'

vi.mock('vue-i18n', async () => {
  const actual = await vi.importActual<typeof import('vue-i18n')>('vue-i18n')
  return {
    ...actual,
    useI18n: () => ({ t: (key: string) => key }),
  }
})

const originalInnerWidth = window.innerWidth
let unmountWrapper: (() => void) | undefined

const setViewportWidth = (width: number) => {
  Object.defineProperty(window, 'innerWidth', {
    configurable: true,
    value: width,
  })
}

const mockTriggerRect = (left: number, width: number) => {
  vi.spyOn(HTMLElement.prototype, 'getBoundingClientRect').mockReturnValue({
    x: left,
    y: 20,
    top: 20,
    right: left + width,
    bottom: 60,
    left,
    width,
    height: 40,
    toJSON: () => ({}),
  })
}

const openSelect = async () => {
  const wrapper = mount(Select, {
    props: {
      modelValue: null,
      options: [
        {
          value: 'example',
          label: 'very-long-unbroken-option-value-that-must-not-overflow',
        },
      ],
    },
  })
  unmountWrapper = () => wrapper.unmount()

  await wrapper.get('button').trigger('click')
  await nextTick()

  return document.body.querySelector<HTMLElement>('.select-dropdown-portal')
}

afterEach(() => {
  unmountWrapper?.()
  unmountWrapper = undefined
  document.body.innerHTML = ''
  setViewportWidth(originalInnerWidth)
  vi.useRealTimers()
  vi.restoreAllMocks()
})

describe('Select dropdown viewport constraints', () => {
  it('preserves the existing 200px minimum width when space is available', async () => {
    setViewportWidth(1024)
    mockTriggerRect(20, 80)

    const dropdown = await openSelect()

    expect(dropdown).not.toBeNull()
    expect(dropdown?.style.left).toBe('20px')
    expect(dropdown?.style.minWidth).toBe('200px')
    expect(dropdown?.style.maxWidth).toBe('996px')
  })

  it('shrinks the minimum width to fit near the right viewport edge', async () => {
    setViewportWidth(320)
    mockTriggerRect(220, 80)

    const dropdown = await openSelect()

    expect(dropdown).not.toBeNull()
    expect(dropdown?.style.left).toBe('220px')
    expect(dropdown?.style.minWidth).toBe('92px')
    expect(dropdown?.style.maxWidth).toBe('92px')
  })

  it('clamps a trigger left of the viewport to the safe padding', async () => {
    setViewportWidth(320)
    mockTriggerRect(-20, 80)

    const dropdown = await openSelect()

    expect(dropdown).not.toBeNull()
    expect(dropdown?.style.left).toBe('8px')
    expect(dropdown?.style.minWidth).toBe('200px')
    expect(dropdown?.style.maxWidth).toBe('304px')
  })

  it('clamps an offscreen-right trigger position to the viewport boundary', async () => {
    setViewportWidth(320)
    mockTriggerRect(400, 80)

    const dropdown = await openSelect()

    expect(dropdown).not.toBeNull()
    expect(dropdown?.style.left).toBe('312px')
    expect(dropdown?.style.minWidth).toBe('0px')
    expect(dropdown?.style.maxWidth).toBe('0px')
  })
})

describe('Select keyboard interaction without a search field', () => {
  const mountKeyboardSelect = () => {
    const wrapper = mount(Select, {
      attachTo: document.body,
      props: {
        modelValue: 10,
        searchable: false,
        options: [
          { value: 10, label: '10' },
          { value: 20, label: '20', disabled: true },
          { value: 30, label: '30' },
        ],
      },
    })
    unmountWrapper = () => wrapper.unmount()
    return wrapper
  }

  it('opens with arrows and closes with Escape while focus remains on the trigger', async () => {
    const wrapper = mountKeyboardSelect()
    const trigger = wrapper.get('button')
    trigger.element.focus()
    await trigger.trigger('keydown', { key: 'ArrowDown' })
    expect(trigger.attributes('aria-expanded')).toBe('true')
    expect(document.activeElement).toBe(trigger.element)
    await trigger.trigger('keydown', { key: 'Escape' })
    expect(trigger.attributes('aria-expanded')).toBe('false')
    expect(document.activeElement).toBe(trigger.element)
    expect(wrapper.emitted('update:modelValue')).toBeUndefined()
  })

  it('skips disabled choices, confirms with Enter and dismisses with Tab', async () => {
    const wrapper = mountKeyboardSelect()
    const trigger = wrapper.get('button')
    await trigger.trigger('keydown', { key: 'ArrowDown' })
    await trigger.trigger('keydown', { key: 'ArrowDown' })
    await trigger.trigger('keydown', { key: 'Enter' })
    expect(wrapper.emitted('update:modelValue')).toEqual([[30]])
    expect(trigger.attributes('aria-expanded')).toBe('false')
    await trigger.trigger('keydown', { key: 'ArrowUp' })
    expect(trigger.attributes('aria-expanded')).toBe('true')
    await trigger.trigger('keydown', { key: 'Tab' })
    expect(trigger.attributes('aria-expanded')).toBe('false')
    expect(wrapper.emitted('update:modelValue')).toEqual([[30]])
  })
})

describe('Select Escape propagation', () => {
  it('consumes Escape only while open so an enclosing dialog stays open', async () => {
    const wrapper = mount(Select, {
      attachTo: document.body,
      props: { modelValue: 10, searchable: false, options: [{ value: 10, label: '10' }] },
    })
    unmountWrapper = () => wrapper.unmount()
    const trigger = wrapper.get('button')
    const onDocumentEscape = vi.fn()
    document.addEventListener('keydown', onDocumentEscape)
    try {
      await trigger.trigger('keydown', { key: 'ArrowDown' })
      onDocumentEscape.mockClear()
      await trigger.trigger('keydown', { key: 'Escape' })
      expect(onDocumentEscape).not.toHaveBeenCalled()
      expect(trigger.attributes('aria-expanded')).toBe('false')
      await trigger.trigger('keydown', { key: 'Escape' })
      expect(onDocumentEscape).toHaveBeenCalledOnce()
    } finally {
      document.removeEventListener('keydown', onDocumentEscape)
    }
  })
})

describe('Select remote search', () => {
  const mountRemoteSelect = (props: Record<string, unknown> = {}) => {
    const wrapper = mount(Select, {
      props: {
        modelValue: null,
        remote: true,
        options: [
          { value: 'alpha', label: 'Alpha account' },
          { value: 'beta', label: 'Beta account' },
        ],
        ...props,
      },
    })
    unmountWrapper = () => wrapper.unmount()
    return wrapper
  }

  const openDropdown = async () => {
    const dropdown = document.body.querySelector<HTMLElement>('.select-dropdown-portal')
    expect(dropdown).not.toBeNull()
    return dropdown as HTMLElement
  }

  const typeSearchQuery = async (query: string) => {
    const dropdown = await openDropdown()
    const input = dropdown.querySelector<HTMLInputElement>('.select-search-input')
    expect(input).not.toBeNull()
    input!.value = query
    input!.dispatchEvent(new Event('input'))
    await nextTick()
  }

  it('emits debounced search events and skips local filtering in remote mode', async () => {
    vi.useFakeTimers()
    const wrapper = mountRemoteSelect()
    await wrapper.get('button').trigger('click')
    await nextTick()

    await typeSearchQuery('zzz')

    // 防抖窗口内不触发。
    expect(wrapper.emitted('search')).toBeUndefined()
    await vi.advanceTimersByTimeAsync(300)

    expect(wrapper.emitted('search')).toEqual([['zzz']])
    // 远程模式不做本地过滤：无命中的 query 下选项仍完整展示（由父组件更新 options）。
    const dropdown = await openDropdown()
    const labels = [...dropdown.querySelectorAll('.select-option-label')].map((el) => el.textContent)
    expect(labels).toContain('Alpha account')
    expect(labels).toContain('Beta account')
  })

  it('does not emit search when the dropdown closes and the query resets', async () => {
    vi.useFakeTimers()
    const wrapper = mountRemoteSelect()
    await wrapper.get('button').trigger('click')
    await nextTick()

    await typeSearchQuery('hidden')

    // 关闭下拉：排队中的防抖定时器应被取消，也不应因 query 重置而尾随 emit。
    await wrapper.get('button').trigger('click')
    await nextTick()
    await vi.advanceTimersByTimeAsync(300)

    expect(wrapper.emitted('search')).toBeUndefined()
  })

  it('shows the loading text instead of empty text while loading with no options', async () => {
    const wrapper = mountRemoteSelect({ options: [], loading: true })
    await wrapper.get('button').trigger('click')
    await nextTick()

    const dropdown = await openDropdown()
    expect(dropdown.querySelector('.select-empty')?.textContent).toContain('common.loading')
  })

  it('keeps local filtering and emits nothing when remote is not set', async () => {
    vi.useFakeTimers()
    const wrapper = mount(Select, {
      props: {
        modelValue: null,
        searchable: true,
        options: [
          { value: 'alpha', label: 'Alpha account' },
          { value: 'beta', label: 'Beta account' },
        ],
      },
    })
    unmountWrapper = () => wrapper.unmount()
    await wrapper.get('button').trigger('click')
    await nextTick()

    await typeSearchQuery('alpha')
    await vi.advanceTimersByTimeAsync(300)

    expect(wrapper.emitted('search')).toBeUndefined()
    const dropdown = await openDropdown()
    const labels = [...dropdown.querySelectorAll('.select-option-label')].map((el) => el.textContent)
    expect(labels).toEqual(['Alpha account'])
  })
})

describe('Select keyboard focus when search results change', () => {
  const options = [
    { value: 1, label: 'First' },
    { value: 2, label: 'Second' },
    { value: 3, label: 'Third' },
    { value: 4, label: 'Fourth' },
    { value: 5, label: 'Fifth' },
    { value: 6, label: 'Sixth' },
  ]

  const mountSearchSelect = async (remote = false, initialOptions = options) => {
    const wrapper = mount(Select, {
      props: { modelValue: 6, options: initialOptions, searchable: true, remote },
      global: { stubs: { Teleport: true, Transition: true } },
    })
    unmountWrapper = () => wrapper.unmount()
    await wrapper.get('.select-trigger').trigger('click')
    return wrapper
  }

  it('selects the first remaining result after local filtering removes the sixth focused option', async () => {
    const wrapper = await mountSearchSelect()
    expect(wrapper.get('.select-option-focused').text()).toBe('Sixth')
    await wrapper.get('input').setValue('First')
    expect(wrapper.get('.select-option-focused').text()).toBe('First')
    await wrapper.get('input').trigger('keydown', { key: 'Enter' })
    expect(wrapper.emitted('update:modelValue')).toEqual([[1]])
  })

  it('retains the active value when local filtering moves it to a different index', async () => {
    const wrapper = await mountSearchSelect()
    await wrapper.get('input').setValue('Sixth')
    expect(wrapper.get('.select-option-focused').text()).toBe('Sixth')
    await wrapper.get('input').trigger('keydown', { key: 'Enter' })
    expect(wrapper.emitted('update:modelValue')).toEqual([[6]])
  })

  it('retains the keyboard-active value when remote results are replaced and reordered', async () => {
    const wrapper = await mountSearchSelect(true)
    await wrapper.get('input').trigger('keydown', { key: 'ArrowUp' })
    expect(wrapper.get('.select-option-focused').text()).toBe('Fifth')
    await wrapper.setProps({ options: [
      { value: 5, label: 'Fifth updated' },
      { value: 6, label: 'Sixth updated' },
    ] })
    expect(wrapper.get('.select-option-focused').text()).toBe('Fifth updated')
    await wrapper.get('input').trigger('keydown', { key: 'Enter' })
    expect(wrapper.emitted('update:modelValue')).toEqual([[5]])
  })

  it('selects the first enabled remote result after loading from an empty list', async () => {
    const wrapper = await mountSearchSelect(true, [])
    expect(wrapper.find('.select-option-focused').exists()).toBe(false)
    await wrapper.get('input').trigger('keydown', { key: 'Enter' })
    expect(wrapper.emitted('update:modelValue')).toBeUndefined()
    await wrapper.setProps({ options: [
      { value: 7, label: 'Unavailable', disabled: true },
      { value: 8, label: 'Available' },
    ] })
    expect(wrapper.get('.select-option-focused').text()).toBe('Available')
    await wrapper.get('input').trigger('keydown', { key: 'Enter' })
    expect(wrapper.emitted('update:modelValue')).toEqual([[8]])
  })

  it('clears focus for empty or all-disabled results and restores it when a result becomes enabled', async () => {
    const wrapper = await mountSearchSelect(true)
    await wrapper.setProps({ options: [] })
    expect(wrapper.find('.select-option-focused').exists()).toBe(false)
    await wrapper.get('input').trigger('keydown', { key: 'Enter' })
    await wrapper.setProps({ options: [
      { value: 6, label: 'Sixth', disabled: true },
      { value: 7, label: 'Seventh', disabled: true },
    ] })
    expect(wrapper.find('.select-option-focused').exists()).toBe(false)
    await wrapper.get('input').trigger('keydown', { key: 'Enter' })
    expect(wrapper.emitted('update:modelValue')).toBeUndefined()
    await wrapper.setProps({ options: [
      { value: 6, label: 'Sixth', disabled: true },
      { value: 7, label: 'Seventh', disabled: false },
    ] })
    expect(wrapper.get('.select-option-focused').text()).toBe('Seventh')
    await wrapper.get('input').trigger('keydown', { key: 'Enter' })
    expect(wrapper.emitted('update:modelValue')).toEqual([[7]])
  })

  it('falls back to the first enabled result when the retained active value becomes disabled', async () => {
    const wrapper = await mountSearchSelect(true)
    await wrapper.setProps({ options: [
      { value: 7, label: 'Unavailable', disabled: true },
      { value: 8, label: 'Available' },
      { value: 6, label: 'Sixth', disabled: true },
    ] })
    expect(wrapper.get('.select-option-focused').text()).toBe('Available')
    await wrapper.get('input').trigger('keydown', { key: 'Enter' })
    expect(wrapper.emitted('update:modelValue')).toEqual([[8]])
  })
})
