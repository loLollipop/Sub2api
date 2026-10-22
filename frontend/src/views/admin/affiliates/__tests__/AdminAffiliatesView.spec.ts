import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { flushPromises, mount, type VueWrapper } from '@vue/test-utils'
import { createMemoryHistory, createRouter } from 'vue-router'
import AdminAffiliatesView from '../AdminAffiliatesView.vue'
import AdminAffiliateRecordsTable from '../AdminAffiliateRecordsTable.vue'

const { listInvites, listRebates, listTransfers, getUserOverview, showError } = vi.hoisted(() => ({
  listInvites: vi.fn(),
  listRebates: vi.fn(),
  listTransfers: vi.fn(),
  getUserOverview: vi.fn(),
  showError: vi.fn(),
}))

vi.mock('@/api/admin/affiliates', () => ({
  affiliatesAPI: {
    listInviteRecords: listInvites,
    listRebateRecords: listRebates,
    listTransferRecords: listTransfers,
    getUserOverview,
  },
}))
vi.mock('@/stores/app', () => ({ useAppStore: () => ({ showError }) }))
vi.mock('vue-i18n', async () => ({
  ...await vi.importActual<typeof import('vue-i18n')>('vue-i18n'),
  useI18n: () => ({ t: (key: string) => key }),
}))
vi.mock('@/components/layout/AppLayout.vue', () => ({
  default: { template: '<div data-testid="app-layout"><slot /></div>' },
}))

const DataTableStub = {
  props: ['columns', 'data', 'loading', 'sortStorageKey'],
  emits: ['sort'],
  template: `
    <div data-testid="records" :data-loading="loading" :data-sort-key="sortStorageKey">
      <div v-for="(row, index) in data" :key="index">
        <template v-for="column in columns" :key="column.key">
          <slot :name="'cell-' + column.key" :row="row" />
        </template>
      </div>
    </div>
  `,
}
const PaginationStub = {
  props: ['page', 'pageSize', 'total'],
  emits: ['update:page', 'update:pageSize'],
  template: '<div data-testid="pagination" :data-page="page" :data-size="pageSize" :data-total="total" />',
}
const BaseDialogStub = {
  props: ['show'],
  emits: ['close'],
  template: '<div v-if="show" role="dialog"><button @click="$emit(\'close\')">Close</button><slot /></div>',
}
const stubs = {
  AppLayout: { template: '<div data-testid="app-layout"><slot /></div>' },
  TablePageLayout: { template: '<div><slot name="filters" /><slot name="table" /><slot name="pagination" /></div>' },
  DataTable: DataTableStub,
  Pagination: PaginationStub,
  BaseDialog: BaseDialogStub,
  Icon: true,
  OrderStatusBadge: true,
}

const invite = {
  inviter_id: 1, inviter_email: 'inviter@example.com', inviter_username: 'Inviter',
  invitee_id: 2, invitee_email: 'invitee@example.com', invitee_username: 'Invitee',
  aff_code: 'invite-code', total_rebate: 12, created_at: '',
}
const rebate = {
  ...invite, order_id: 100, out_trade_no: 'order-100', order_amount: 10,
  pay_amount: 70, rebate_amount: 1, payment_type: 'card', order_status: 'completed',
}
const transfer = {
  ledger_id: 100, user_id: 3, user_email: 'transfer@example.com', username: 'Transfer',
  amount: 5, snapshot_available: false, created_at: '',
}
const overview = {
  user_id: 1, email: 'overview@example.com', username: 'User', aff_code: 'code',
  rebate_rate_percent: 10, invited_count: 2, rebated_invitee_count: 1,
  available_quota: 3, history_quota: 4,
}
const page = (items: unknown[]) => ({ items, total: items.length, page: 1, page_size: 20, pages: 1 })

function deferred<T>() {
  let resolve!: (value: T) => void
  let reject!: (reason: unknown) => void
  const promise = new Promise<T>((resolvePromise, rejectPromise) => {
    resolve = resolvePromise
    reject = rejectPromise
  })
  return { promise, resolve, reject }
}

let wrapper: VueWrapper | undefined

async function mountHub(path = '/admin/affiliates') {
  const router = createRouter({
    history: createMemoryHistory(),
    routes: [{ path: '/admin/affiliates', component: AdminAffiliatesView }],
  })
  await router.push(path)
  await router.isReady()
  wrapper = mount(AdminAffiliatesView, { global: { plugins: [router], stubs } })
  await flushPromises()
  return { wrapper, router }
}

beforeEach(() => {
  vi.resetAllMocks()
  localStorage.clear()
  listInvites.mockResolvedValue(page([invite]))
  listRebates.mockResolvedValue(page([rebate]))
  listTransfers.mockResolvedValue(page([transfer]))
  getUserOverview.mockResolvedValue(overview)
})

afterEach(() => {
  wrapper?.unmount()
  wrapper = undefined
  vi.useRealTimers()
})

describe('admin affiliates hub', () => {
  it.each([
    ['', 'invites'], ['?tab=invites', 'invites'], ['?tab=rebates', 'rebates'],
    ['?tab=transfers', 'transfers'], ['?tab=invalid', 'invites'],
    ['?tab=rebates&tab=transfers', 'invites'],
  ])('loads only the active table on a fresh visit to %s', async (query, expectedTab) => {
    const { wrapper } = await mountHub(`/admin/affiliates${query}`)
    expect(wrapper.findAll('[data-testid="app-layout"]')).toHaveLength(1)
    expect(wrapper.findAllComponents(AdminAffiliateRecordsTable)).toHaveLength(1)
    expect(wrapper.getComponent(AdminAffiliateRecordsTable).props('type')).toBe(expectedTab)
    expect(listInvites).toHaveBeenCalledTimes(expectedTab === 'invites' ? 1 : 0)
    expect(listRebates).toHaveBeenCalledTimes(expectedTab === 'rebates' ? 1 : 0)
    expect(listTransfers).toHaveBeenCalledTimes(expectedTab === 'transfers' ? 1 : 0)
    const selected = wrapper.get('[aria-selected="true"]')
    expect(selected.attributes('aria-controls')).toBe(wrapper.get('[role="tabpanel"]').attributes('id'))
  })

  it('syncs tab clicks, query/hash, browser back/forward and resets each keyed table', async () => {
    const { wrapper, router } = await mountHub('/admin/affiliates?search=kept#records')
    const initialTable = wrapper.getComponent(AdminAffiliateRecordsTable).vm
    await wrapper.get('input[type="text"]').setValue('old-search')
    await wrapper.get('#admin-affiliates-tab-rebates').trigger('click')
    await flushPromises()
    expect(router.currentRoute.value.query).toEqual({ search: 'kept', tab: 'rebates' })
    expect(router.currentRoute.value.hash).toBe('#records')
    expect(wrapper.getComponent(AdminAffiliateRecordsTable).vm === initialTable).toBe(false)
    expect((wrapper.get('input[type="text"]').element as HTMLInputElement).value).toBe('')
    expect(wrapper.text()).toContain('order-100')

    router.back()
    await flushPromises()
    expect(wrapper.getComponent(AdminAffiliateRecordsTable).props('type')).toBe('invites')
    expect(listInvites).toHaveBeenCalledTimes(2)
    router.forward()
    await flushPromises()
    expect(wrapper.getComponent(AdminAffiliateRecordsTable).props('type')).toBe('rebates')
    await wrapper.get('#admin-affiliates-tab-transfers').trigger('click')
    await flushPromises()
    expect(wrapper.text()).toContain('transfer@example.com')
    expect(listTransfers).toHaveBeenCalledOnce()

    await wrapper.get('#admin-affiliates-tab-invites').trigger('click')
    await flushPromises()
    expect(router.currentRoute.value.query).toEqual({ search: 'kept' })
    await wrapper.get('#admin-affiliates-tab-invites').trigger('click')
    await flushPromises()
    expect(listInvites).toHaveBeenCalledTimes(3)
  })

  it('preserves search/date filters, sorting, pagination and user overview', async () => {
    localStorage.setItem('admin-affiliate-invites-table-sort', JSON.stringify({ key: 'inviter', order: 'asc' }))
    const { wrapper } = await mountHub()
    expect(listInvites).toHaveBeenLastCalledWith(expect.objectContaining({ sort_by: 'inviter', sort_order: 'asc' }))
    vi.useFakeTimers()
    await wrapper.get('input[type="text"]').setValue('  invitee  ')
    await vi.advanceTimersByTimeAsync(300)
    expect(listInvites).toHaveBeenLastCalledWith(expect.objectContaining({ search: 'invitee', page: 1 }))
    const dates = wrapper.findAll('input[type="date"]')
    await dates[0].setValue('2026-10-01')
    await dates[1].setValue('2026-10-22')
    expect(listInvites).toHaveBeenLastCalledWith(expect.objectContaining({ start_at: '2026-10-01', end_at: '2026-10-22' }))
    wrapper.getComponent(PaginationStub).vm.$emit('update:page', 3)
    expect(listInvites).toHaveBeenLastCalledWith(expect.objectContaining({ page: 3 }))
    wrapper.getComponent(PaginationStub).vm.$emit('update:pageSize', 50)
    expect(listInvites).toHaveBeenLastCalledWith(expect.objectContaining({ page: 1, page_size: 50 }))
    wrapper.getComponent(DataTableStub).vm.$emit('sort', 'total_rebate', 'desc')
    expect(listInvites).toHaveBeenLastCalledWith(expect.objectContaining({ page: 1, sort_by: 'total_rebate', sort_order: 'desc' }))
    await flushPromises()
    await wrapper.get('[data-testid="records"] button').trigger('click')
    await flushPromises()
    expect(getUserOverview).toHaveBeenCalledWith(1)
    expect(wrapper.get('[role="dialog"]').text()).toContain('overview@example.com')
  })

  it('cancels a pending search and ignores the old tab request after switching', async () => {
    const pending = deferred<ReturnType<typeof page>>()
    listInvites.mockReturnValueOnce(pending.promise)
    vi.useFakeTimers()
    const { wrapper } = await mountHub()
    await wrapper.get('input[type="text"]').setValue('pending')
    await wrapper.get('#admin-affiliates-tab-rebates').trigger('click')
    await flushPromises()
    await vi.advanceTimersByTimeAsync(300)
    expect(listInvites).toHaveBeenCalledOnce()
    pending.resolve(page([invite]))
    await flushPromises()
    expect(wrapper.text()).toContain('order-100')
    expect(wrapper.find('[data-testid="records"]').attributes('data-sort-key')).toBe('admin-affiliate-rebates-table-sort')
    expect(showError).not.toHaveBeenCalled()
  })

  it('ignores stale responses and stale failures within the current table', async () => {
    const stale = deferred<ReturnType<typeof page>>()
    listInvites.mockReturnValueOnce(stale.promise)
    const { wrapper } = await mountHub()
    wrapper.getComponent(DataTableStub).vm.$emit('sort', 'inviter', 'asc')
    await flushPromises()
    expect(wrapper.text()).toContain('invite-code')
    stale.resolve(page([{ ...invite, aff_code: 'stale-code' }]))
    await flushPromises()
    expect(wrapper.text()).not.toContain('stale-code')

    const failed = deferred<ReturnType<typeof page>>()
    listInvites.mockReturnValueOnce(failed.promise)
    await wrapper.get('button[title="common.refresh"]').trigger('click')
    wrapper.getComponent(PaginationStub).vm.$emit('update:page', 2)
    await flushPromises()
    failed.reject(new Error('stale failure'))
    await flushPromises()
    expect(showError).not.toHaveBeenCalled()
    expect(wrapper.get('[data-testid="records"]').attributes('data-loading')).toBe('false')
  })

  it('keeps an overview response for the latest user and ignores one after dialog close', async () => {
    const first = deferred<typeof overview>()
    const second = deferred<typeof overview>()
    getUserOverview.mockReturnValueOnce(first.promise).mockReturnValueOnce(second.promise)
    const { wrapper } = await mountHub()
    const users = wrapper.findAll('[data-testid="records"] button')
    await users[0].trigger('click')
    await users[1].trigger('click')
    second.resolve({ ...overview, email: 'latest@example.com' })
    await flushPromises()
    first.resolve(overview)
    await flushPromises()
    expect(wrapper.get('[role="dialog"]').text()).toContain('latest@example.com')

    const closed = deferred<typeof overview>()
    getUserOverview.mockReturnValueOnce(closed.promise)
    await users[0].trigger('click')
    await wrapper.get('[role="dialog"] button').trigger('click')
    closed.reject(new Error('closed failure'))
    await flushPromises()
    expect(wrapper.find('[role="dialog"]').exists()).toBe(false)
    expect(showError).not.toHaveBeenCalled()
  })

  it('suppresses late list and overview errors after the tab unmounts', async () => {
    const records = deferred<ReturnType<typeof page>>()
    const user = deferred<typeof overview>()
    const { wrapper } = await mountHub()
    listInvites.mockReturnValueOnce(records.promise)
    getUserOverview.mockReturnValueOnce(user.promise)
    await wrapper.get('[data-testid="records"] button').trigger('click')
    await wrapper.get('button[title="common.refresh"]').trigger('click')
    await wrapper.get('#admin-affiliates-tab-transfers').trigger('click')
    await flushPromises()
    records.reject(new Error('unmounted records'))
    user.reject(new Error('unmounted overview'))
    await flushPromises()
    expect(showError).not.toHaveBeenCalled()
    expect(wrapper.find('[role="dialog"]').exists()).toBe(false)
  })
})
