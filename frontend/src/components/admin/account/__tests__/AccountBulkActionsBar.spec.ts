import { afterEach, describe, expect, it, vi } from 'vitest'
import { flushPromises, mount } from '@vue/test-utils'

import AccountBulkActionsBar from '../AccountBulkActionsBar.vue'

const wrappers: ReturnType<typeof mount>[] = []
afterEach(() => {
  wrappers.splice(0).forEach(wrapper => wrapper.unmount())
  document.body.innerHTML = ''
})

function render(selectedIds: number[] = []) {
  const wrapper = mount(AccountBulkActionsBar, {
    attachTo: document.body,
    props: { selectedIds, totalResults: 45, selectingAll: false, allResultsSelected: false }
  })
  wrappers.push(wrapper)
  return wrapper
}

async function selectMenuAction(wrapper: ReturnType<typeof mount>, label: string) {
  await wrapper.get('.row-actions-trigger').trigger('click')
  await flushPromises()
  const button = Array.from(document.querySelectorAll<HTMLButtonElement>('[role="menuitem"]'))
    .find(item => item.textContent?.includes(label))
  expect(button).toBeDefined()
  button!.click()
  await flushPromises()
  expect(document.querySelector('[role="menu"]')).toBeNull()
}

vi.mock('vue-i18n', () => ({
  useI18n: () => ({
    t: (key: string) => key
  })
}))

describe('AccountBulkActionsBar', () => {
  it('reveals bulk tools after selection and still allows selecting all results', async () => {
    const wrapper = render()
    expect(wrapper.find('.account-bulk-bar').exists()).toBe(false)
    await wrapper.setProps({ selectedIds: [1] })

    const button = wrapper.findAll('button').find(item =>
      item.text().includes('admin.accounts.bulkActions.selectAllResults')
    )

    expect(button).toBeDefined()
    await button!.trigger('click')
    expect(wrapper.emitted('select-all-results')).toHaveLength(1)
    await wrapper.setProps({ selectedIds: [], selectingAll: true })
    expect(wrapper.find('.account-bulk-bar').exists()).toBe(true)
    expect(wrapper.findAll('button').find(item => item.text().includes('admin.accounts.bulkActions.selectingAll'))?.attributes('disabled')).toBeDefined()
  })

  it('preserves the upstream billing probe action from v0.1.166', async () => {
    const wrapper = render([1])
    await selectMenuAction(wrapper, 'admin.accounts.bulkActions.probeUpstreamBilling')
    expect(wrapper.emitted('probe-upstream-billing')).toHaveLength(1)
  })

  it('emits test-models from the batch detection button', async () => {
    const wrapper = render([1, 2])
    await selectMenuAction(wrapper, 'admin.accounts.bulkActions.testModels')
    expect(wrapper.emitted('test-models')).toHaveLength(1)
  })
})
