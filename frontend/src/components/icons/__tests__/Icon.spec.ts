import { mount } from '@vue/test-utils'
import { describe, expect, it } from 'vitest'
import Icon from '../Icon.vue'

describe('Icon', () => {
  it('keeps the existing string-path defaults and forwards accessibility attributes', () => {
    const wrapper = mount(Icon, { props: { name: 'plus' }, attrs: { 'aria-label': 'Add' } })
    expect(wrapper.attributes('viewBox')).toBe('0 0 24 24')
    expect(wrapper.attributes('stroke-width')).toBe('1.5')
    expect(wrapper.classes()).toContain('h-5')
    expect(wrapper.get('path').attributes('d')).toBe('M12 4.5v15m7.5-7.5h-15')
    expect(wrapper.attributes('aria-label')).toBe('Add')
  })

  it('renders multiple whitelisted shapes in the SVG namespace', () => {
    const wrapper = mount(Icon, { props: { name: 'calendarOutline', size: 'none', strokeWidth: 2.5 }, attrs: { class: 'h-4 w-4' } })
    expect(wrapper.findAll('rect')).toHaveLength(1)
    expect(wrapper.findAll('line')).toHaveLength(3)
    for (const node of wrapper.findAll('rect, line')) {
      expect(node.element.namespaceURI).toBe('http://www.w3.org/2000/svg')
    }
    expect(wrapper.attributes('stroke-width')).toBe('2.5')
    expect(wrapper.classes()).toEqual(['h-4', 'w-4'])
  })

  it('supports filled geometry and its original viewBox without an outline', async () => {
    const wrapper = mount(Icon, { props: { name: 'checkCircleSolid' } })
    expect(wrapper.attributes('fill')).toBe('currentColor')
    expect(wrapper.attributes('stroke')).toBe('none')
    expect(wrapper.attributes('viewBox')).toBe('0 0 20 20')
    expect(wrapper.get('path').attributes('fill-rule')).toBe('evenodd')
    await wrapper.setProps({ name: 'stopSolid' })
    expect(wrapper.find('path').exists()).toBe(false)
    expect(wrapper.get('rect').attributes('rx')).toBe('2')
  })
})
