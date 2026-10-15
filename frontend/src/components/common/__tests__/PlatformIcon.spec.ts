import { mount } from '@vue/test-utils'
import { describe, expect, it } from 'vitest'

import PlatformIcon from '../PlatformIcon.vue'

describe('PlatformIcon native providers', () => {
  it.each(['kimi', 'zhipu', 'deepseek', 'minimax', 'command_code', 'cline'] as const)('renders %s', (platform) => {
    const wrapper = mount(PlatformIcon, { props: { platform } })
    expect(wrapper.find('svg').exists()).toBe(true)
    expect(wrapper.find('path').exists()).toBe(true)
    wrapper.unmount()
  })
  it.each([
    ['xs', 'w-3', 'h-3'],
    ['sm', 'w-3.5', 'h-3.5'],
    ['md', 'w-4', 'h-4'],
    ['lg', 'w-5', 'h-5'],
  ] as const)('preserves the %s size and caller tint', (size, width, height) => {
    const wrapper = mount(PlatformIcon, {
      props: { platform: 'kimi', size },
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
    await wrapper.setProps({ platform: 'openai' })
    expect(wrapper.get('path').attributes('d')).toBe(openAIPath)
    await wrapper.setProps({ platform: 'anthropic' })
    expect(wrapper.get('path').attributes('d')).toBe(anthropicPath)
    await wrapper.setProps({ platform: undefined })
    expect(wrapper.get('path').attributes('d')).toBe(mount(PlatformIcon).get('path').attributes('d'))
  })
})
