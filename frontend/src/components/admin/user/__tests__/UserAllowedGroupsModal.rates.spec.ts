import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { enableAutoUnmount, flushPromises, mount } from '@vue/test-utils'
import type { AdminUser, Group, GroupPlatform } from '@/types'
import UserAllowedGroupsModal from '../UserAllowedGroupsModal.vue'

const mocks = vi.hoisted(() => ({ list: vi.fn(), update: vi.fn(), showError: vi.fn(), showSuccess: vi.fn() }))
vi.mock('@/api/admin', () => ({ adminAPI: { groups: { list: mocks.list }, users: { update: mocks.update } } }))
vi.mock('@/stores/app', () => ({ useAppStore: () => ({ showError: mocks.showError, showSuccess: mocks.showSuccess }) }))
vi.mock('vue-i18n', () => ({ useI18n: () => ({ t: (key: string) => key }) }))
enableAutoUnmount(afterEach)
beforeEach(() => vi.resetAllMocks())

async function open(platform: GroupPlatform, customRate?: number) {
  const group = { id: 10, name: 'Group', platform, rate_multiplier: platform === 'mixed' ? 9 : 1, status: 'active', subscription_type: 'standard', is_exclusive: false } as Group
  mocks.list.mockResolvedValue({ items: [group], total: 1 })
  mocks.update.mockResolvedValue({})
  const user = { id: 42, email: 'user@example.com', group_rates: customRate === undefined ? {} : { 10: customRate } } as AdminUser
  const wrapper = mount(UserAllowedGroupsModal, {
    props: { show: false, user },
    global: { stubs: {
      BaseDialog: { props: ['show'], template: '<div v-if="show"><slot /><slot name="footer" /></div>' },
      Icon: true, PlatformIcon: true
    } }
  })
  await wrapper.setProps({ show: true })
  await flushPromises()
  return wrapper
}

describe('user-specific Mixed rate editing', () => {
  it('does not advertise a hidden Mixed total as its default or placeholder', async () => {
    const wrapper = await open('mixed')
    expect(wrapper.text()).toContain('common.mixedProviderRates')
    expect(wrapper.text()).not.toContain('9x')
    expect(wrapper.get('input[type="number"]').attributes('placeholder')).toBe('')
  })
  it.each([['mixed', '0', 0], ['mixed', '0.5', 0.5], ['openai', '0.5', 0.5]] as const)(
    'saves %s override %s', async (platform, input, expected) => {
      const wrapper = await open(platform)
      const field = wrapper.get('input[type="number"]')
      expect(field.attributes('min')).toBe(platform === 'mixed' ? '0' : '0.001')
      await field.setValue(input)
      await wrapper.findAll('button').find(b => b.text() === 'common.save')!.trigger('click')
      await flushPromises()
      expect(mocks.update).toHaveBeenCalledWith(42, expect.objectContaining({ group_rates: { 10: expected } }))
    }
  )

  it.each([['openai', 0], ['mixed', -1], ['mixed', 1000000], ['mixed', Number.NaN], ['mixed', Infinity], ['mixed', -Infinity]] as const)(
    'rejects invalid %s override %s before sending', async (platform, value) => {
      const wrapper = await open(platform, value)
      await wrapper.findAll('button').find(b => b.text() === 'common.save')!.trigger('click')
      expect(mocks.update).not.toHaveBeenCalled()
      expect(mocks.showError).toHaveBeenCalled()
    }
  )

  it('clears a saved Mixed zero with null', async () => {
    const wrapper = await open('mixed', 0)
    await wrapper.get('input[type="number"]').setValue('')
    await wrapper.findAll('button').find(b => b.text() === 'common.save')!.trigger('click')
    await flushPromises()
    expect(mocks.update).toHaveBeenCalledWith(42, expect.objectContaining({ group_rates: { 10: null } }))
  })
})
