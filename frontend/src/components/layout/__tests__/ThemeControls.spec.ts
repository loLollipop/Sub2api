import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { mount, type VueWrapper } from '@vue/test-utils'
import { createI18n } from 'vue-i18n'
import ThemeControls from '../ThemeControls.vue'

let wrapper: VueWrapper | undefined
let media: MediaQueryList
let onChange: (() => void) | undefined
function render() {
  wrapper = mount(ThemeControls, { global: { plugins: [createI18n({ legacy: false, locale: 'en', messages: { en: {} } })] } })
  return wrapper
}
beforeEach(() => {
  localStorage.clear()
  document.documentElement.classList.remove('dark')
  media = { matches: false, addEventListener: vi.fn((_event, handler) => { onChange = handler }), removeEventListener: vi.fn() } as unknown as MediaQueryList
  vi.spyOn(window, 'matchMedia').mockReturnValue(media)
})
afterEach(() => {
  wrapper?.unmount()
  wrapper = undefined
  vi.restoreAllMocks()
})
describe('header appearance buttons', () => {
  it('supports explicit light and dark choices and persists the selection', async () => {
    const view = render()
    await view.get('[aria-label="Dark"]').trigger('click')
    expect(document.documentElement.classList.contains('dark')).toBe(true)
    expect(localStorage.getItem('theme')).toBe('dark')
    expect(view.get('[aria-label="Dark"]').attributes('aria-pressed')).toBe('true')
    await view.get('[aria-label="Light"]').trigger('click')
    expect(document.documentElement.classList.contains('dark')).toBe(false)
    expect(localStorage.getItem('theme')).toBe('light')
  })
  it('follows system changes only while system mode is selected and restores it on remount', async () => {
    localStorage.setItem('theme', 'system')
    const view = render()
    expect(view.get('[aria-label="System"]').attributes('aria-pressed')).toBe('true')
    Object.defineProperty(media, 'matches', { configurable: true, value: true })
    onChange?.()
    expect(document.documentElement.classList.contains('dark')).toBe(true)
    await view.get('[aria-label="Light"]').trigger('click')
    onChange?.()
    expect(document.documentElement.classList.contains('dark')).toBe(false)
    await view.get('[aria-label="System"]').trigger('click')
    expect(document.documentElement.classList.contains('dark')).toBe(true)
    expect(localStorage.getItem('theme')).toBe('system')
    view.unmount()
    expect(media.removeEventListener).toHaveBeenCalledWith('change', expect.any(Function))
    wrapper = undefined
    const restored = render()
    expect(restored.get('[aria-label="System"]').attributes('aria-pressed')).toBe('true')
    expect(document.documentElement.classList.contains('dark')).toBe(true)
  })
})
