import { defineComponent, h, ref } from 'vue'
import { enableAutoUnmount, flushPromises, mount } from '@vue/test-utils'
import { afterEach, describe, expect, it, vi } from 'vitest'
import { useAffiliateCustomUsersPanel } from '../AffiliateCustomUsersPanel.vue'

enableAutoUnmount(afterEach)
const mocks = vi.hoisted(() => ({ listUsers: vi.fn(), showError: vi.fn(), showSuccess: vi.fn() }))
vi.mock('@/api/admin/affiliates', () => ({ affiliatesAPI: mocks }))
vi.mock('@/stores', () => ({ useAppStore: () => mocks }))
vi.mock('vue-i18n', () => ({ useI18n: () => ({ t: (key: string) => key }) }))

describe('affiliate custom users pagination', () => {
  it('uses the server page and capped size on the next request while preserving selection cleanup', async () => {
    mocks.listUsers.mockResolvedValue({ items: [{ user_id: 1 }], total: 101, page: 1, page_size: 5 })
    const enabled = ref(false)
    let controller!: ReturnType<typeof useAffiliateCustomUsersPanel>
    mount(defineComponent({ setup() {
      controller = useAffiliateCustomUsersPanel(() => enabled.value)
      return () => h('div')
    } }))
    controller.affiliateState.selected = [1, 2]
    enabled.value = true
    await flushPromises()
    expect(controller.affiliateState.pageSize).toBe(5)
    expect(controller.affiliateState.selected).toEqual([1])
    mocks.listUsers.mockResolvedValue({ items: [], total: 101, page: 2, page_size: 5 })
    controller.changeAffiliatePage(2)
    await flushPromises()
    expect(mocks.listUsers).toHaveBeenLastCalledWith({ page: 2, page_size: 5, search: '' })
    expect(controller.affiliateState.page).toBe(2)
    expect(controller.affiliateState.selected).toEqual([])
  })
})
