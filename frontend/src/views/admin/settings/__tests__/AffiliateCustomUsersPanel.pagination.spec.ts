import { defineComponent, h, ref } from 'vue'
import { enableAutoUnmount, flushPromises, mount } from '@vue/test-utils'
import { afterEach, describe, expect, it, vi } from 'vitest'
import AffiliateCustomUsersPanel, { useAffiliateCustomUsersPanel } from '../AffiliateCustomUsersPanel.vue'

enableAutoUnmount(afterEach)
const mocks = vi.hoisted(() => ({ listUsers: vi.fn(), showError: vi.fn(), showSuccess: vi.fn() }))
vi.mock('@/api/admin/affiliates', () => ({ affiliatesAPI: mocks }))
vi.mock('@/stores', () => ({ useAppStore: () => mocks }))
vi.mock('vue-i18n', () => ({ useI18n: () => ({ t: (key: string) => key }) }))

describe('affiliate custom users pagination', () => {
  it('keeps add and batch modal actions in a shared footer and closes without writing', async () => {
    let controller!: ReturnType<typeof useAffiliateCustomUsersPanel>
    mount(defineComponent({ setup() {
      controller = useAffiliateCustomUsersPanel(() => false)
      return () => h(AffiliateCustomUsersPanel, { controller, enabled: false })
    } }), { attachTo: document.body })
    controller.openAffiliateModal(null)
    await flushPromises()
    const dialog = document.querySelector('[role="dialog"]')!
    expect(dialog.querySelector('.modal-body input')).not.toBeNull()
    expect(dialog.querySelector('.modal-footer button')).not.toBeNull()
    expect(document.body.classList.contains('modal-open')).toBe(true)
    document.dispatchEvent(new KeyboardEvent('keydown', { key: 'Escape', bubbles: true }))
    await flushPromises()
    expect(controller.affiliateModal.open).toBe(false)
    expect(document.body.classList.contains('modal-open')).toBe(false)
    controller.openAffiliateModal(null)
    await flushPromises()
    ;(document.querySelector('[role="dialog"]') as HTMLElement).click()
    await flushPromises()
    expect(controller.affiliateModal.open).toBe(false)
    controller.affiliateState.selected = [1, 2]
    controller.openAffiliateBatchModal()
    await flushPromises()
    expect(document.querySelector('[role="dialog"] .modal-footer button')).not.toBeNull()
    document.dispatchEvent(new KeyboardEvent('keydown', { key: 'Escape', bubbles: true }))
    await flushPromises()
    expect(controller.affiliateBatchModal.open).toBe(false)
    expect(controller.affiliateState.selected).toEqual([1, 2])
    controller.openAffiliateBatchModal()
    await flushPromises()
    ;(document.querySelector('[role="dialog"]') as HTMLElement).click()
    await flushPromises()
    expect(controller.affiliateBatchModal.open).toBe(false)
  })
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
