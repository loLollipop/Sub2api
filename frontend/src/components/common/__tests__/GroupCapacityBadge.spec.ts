import { describe, expect, it, vi } from 'vitest'
import { mount } from '@vue/test-utils'
import GroupCapacityBadge from '../GroupCapacityBadge.vue'

vi.mock('vue-i18n', () => ({ useI18n: () => ({ t: (key: string) => key }) }))

const props = { concurrencyUsed: 1, concurrencyMax: 2, sessionsUsed: 3, sessionsMax: 4, rpmUsed: 5, rpmMax: 6 }

describe('GroupCapacityBadge runtime availability', () => {
  it('keeps the previous runtime display when runtimeKnown is omitted', () => {
    const wrapper = mount(GroupCapacityBadge, { props })
    expect(wrapper.text()).toContain('1')
    expect(wrapper.text()).not.toContain('—')
    expect(wrapper.attributes('title')).toBe('admin.groups.capacityRuntimeHint')
  })

  it('shows unknown runtime while preserving configured capacities', () => {
    const wrapper = mount(GroupCapacityBadge, { props: { ...props, runtimeKnown: false } })
    expect(wrapper.findAll('.font-mono').map(node => node.text())).toEqual(['—', '2', '—', '4', '—', '6'])
    expect(wrapper.attributes('title')).toBe('admin.groups.capacityUnknownHint')
    expect(wrapper.html()).not.toContain('bg-yellow-100')
  })
})
