import { mount } from '@vue/test-utils'
import { describe, expect, it } from 'vitest'

import PlatformIcon from '../PlatformIcon.vue'

describe('PlatformIcon Mixed logo', () => {
  it('renders its own inline, theme-aware ribbon mark instead of the globe fallback', () => {
    const mixed = mount(PlatformIcon, { props: { platform: 'mixed' } })
    const fallback = mount(PlatformIcon)
    const svg = mixed.get('svg')
    expect(svg.element.namespaceURI).toBe('http://www.w3.org/2000/svg')
    expect(svg.attributes('viewBox')).toBe('0 0 24 24')
    expect(svg.attributes('fill')).toBe('currentColor')
    expect(svg.attributes('aria-hidden')).toBe('true')
    expect(svg.attributes('focusable')).toBe('false')
    expect(mixed.findAll('path')).toHaveLength(3)
    expect(mixed.get('path').attributes('d')).not.toBe(fallback.get('path').attributes('d'))
    expect(mixed.find('image, defs, use, animate').exists()).toBe(false)
  })

  it.each([
    ['xs', 'w-3', 'h-3'],
    ['sm', 'w-3.5', 'h-3.5'],
    ['md', 'w-4', 'h-4'],
    ['lg', 'w-5', 'h-5'],
  ] as const)('preserves the %s size and caller tint', (size, width, height) => {
    const wrapper = mount(PlatformIcon, {
      props: { platform: 'mixed', size },
      attrs: { class: 'text-violet-600 dark:text-violet-300' },
    })
    expect(wrapper.get('svg').classes()).toEqual(expect.arrayContaining([
      width, height, 'flex-shrink-0', 'text-violet-600', 'dark:text-violet-300',
    ]))
  })

  it('leaves established platform logos and the unknown-platform fallback intact', async () => {
    const wrapper = mount(PlatformIcon, { props: { platform: 'openai' } })
    const openAIPath = wrapper.get('path').attributes('d')
    await wrapper.setProps({ platform: 'anthropic' })
    const anthropicPath = wrapper.get('path').attributes('d')
    expect(wrapper.get('svg').attributes('viewBox')).toBe('0 0 16 16')
    await wrapper.setProps({ platform: 'mixed' })
    expect(wrapper.findAll('path')).toHaveLength(3)
    await wrapper.setProps({ platform: 'openai' })
    expect(wrapper.get('path').attributes('d')).toBe(openAIPath)
    await wrapper.setProps({ platform: 'anthropic' })
    expect(wrapper.get('path').attributes('d')).toBe(anthropicPath)
    await wrapper.setProps({ platform: undefined })
    expect(wrapper.get('path').attributes('d')).toBe(mount(PlatformIcon).get('path').attributes('d'))
  })
})
