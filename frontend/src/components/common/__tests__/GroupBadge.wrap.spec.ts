import { describe, expect, it, vi } from 'vitest'
import { mount } from '@vue/test-utils'
import GroupBadge from '../GroupBadge.vue'
import AccountGroupsCell from '@/components/account/AccountGroupsCell.vue'
import type { Group } from '@/types'

vi.mock('vue-i18n', () => ({ useI18n: () => ({ t: (key: string) => key }) }))
vi.mock('@/stores/app', () => ({ useAppStore: () => ({ publicSettings: { timezone: 'UTC' } }) }))
const global = { stubs: { PlatformIcon: true, Icon: true, MixedProviderRates: true } }

describe('complete account group names', () => {
  it('defaults to compact names and permits full wrapping when requested', () => {
    const compact = mount(GroupBadge, { props: { name: 'A very long group name', showRate: false }, global })
    expect(compact.get('span.truncate').text()).toBe('A very long group name')
    const full = mount(GroupBadge, { props: { name: 'A very long group name', showRate: false, truncate: false }, global })
    expect(full.find('span.truncate').exists()).toBe(false)
    expect(full.text()).toBe('A very long group name')
  })


  it('does not clip the account group rows and preserves the overflow counter', () => {
    const groups = [1, 2, 3].map(id => ({ id, name: `Long group ${id}`, platform: 'kimi' } as Group))
    const wrapper = mount(AccountGroupsCell, { props: { groups, maxDisplay: 2 }, global })
    expect(wrapper.getComponent(GroupBadge).props('truncate')).toBe(false)
    expect(wrapper.get('button').text()).toBe('+2')
    expect(wrapper.html()).not.toContain('max-h-14')
    wrapper.unmount()
  })
})
