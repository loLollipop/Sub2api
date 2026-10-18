import { enableAutoUnmount, flushPromises, mount } from '@vue/test-utils'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import Select from '@/components/common/Select.vue'
import Toggle from '@/components/common/Toggle.vue'
import ScheduledTestsPanel from '../ScheduledTestsPanel.vue'

enableAutoUnmount(afterEach)

const { listByAccount, listResults, create, update } = vi.hoisted(() => ({
  listByAccount: vi.fn(),
  listResults: vi.fn(),
  create: vi.fn(),
  update: vi.fn(),
}))

vi.mock('@/api/admin', () => ({ adminAPI: { scheduledTests: { listByAccount, listResults, create, update } } }))
vi.mock('@/stores/app', () => ({ useAppStore: () => ({ showError: vi.fn(), showSuccess: vi.fn() }) }))
vi.mock('vue-i18n', async (importOriginal) => ({
  ...await importOriginal<typeof import('vue-i18n')>(),
  useI18n: () => ({ t: (key: string) => key })
}))

describe('ScheduledTestsPanel quality results', () => {
  beforeEach(() => vi.clearAllMocks())

  it('loads when lazily mounted open and does not restart polling after a late unmount', async () => {
    vi.useFakeTimers()
    let finish!: (plans: never[]) => void
    listByAccount.mockImplementationOnce(() => new Promise<never[]>(resolve => { finish = resolve }))
    const wrapper = mount(ScheduledTestsPanel, {
      props: { show: true, accountId: 2, modelOptions: [] },
      global: { stubs: { BaseDialog: { template: '<div><slot /></div>' }, ConfirmDialog: true } },
    })
    try {
      expect(listByAccount).toHaveBeenCalledTimes(1)
      wrapper.unmount()
      finish([])
      await flushPromises()
      await vi.advanceTimersByTimeAsync(30000)
      expect(listByAccount).toHaveBeenCalledTimes(1)
    } finally { vi.useRealTimers() }
  })

  it('creates ordinary plans with quality checking off and resets after explicit opt-in', async () => {
    listByAccount.mockResolvedValue([])
    create.mockResolvedValue({ id: 1 })
    const wrapper = mount(ScheduledTestsPanel, {
      props: { show: false, accountId: 2, modelOptions: [{ value: 'gpt-test', label: 'gpt-test' }] },
      global: { stubs: { BaseDialog: { template: '<div><slot /></div>' }, ConfirmDialog: true } },
    })
    await wrapper.setProps({ show: true }); await flushPromises()
    const open = async () => {
      await wrapper.findAll('button').find(button => button.text() === 'admin.scheduledTests.addPlan')!.trigger('click')
      wrapper.getComponent(Select).vm.$emit('update:modelValue', 'gpt-test')
      await flushPromises()
    }
    const quality = () => wrapper.findAllComponents(Toggle).find(toggle => toggle.element.closest('label')?.textContent?.includes('admin.scheduledTests.qualityCheck'))!
    const save = async () => {
      await wrapper.findAll('button').find(button => button.text() === 'common.save')!.trigger('click')
      await flushPromises()
    }
    await open()
    expect(quality().props('modelValue')).toBe(false)
    expect(wrapper.find('[data-testid="scheduled-quality-config"]').exists()).toBe(false)
    await save()
    expect(create).toHaveBeenLastCalledWith(expect.objectContaining({ quality_check_enabled: false, quality_provider: 'chanshui' }))

    await open()
    await quality().trigger('click')
    expect(wrapper.find('[data-testid="scheduled-quality-config"]').exists()).toBe(true)
    expect(wrapper.text()).toContain('admin.scheduledTests.auditKeyNotice')
    await save()
    expect(create).toHaveBeenLastCalledWith(expect.objectContaining({ quality_check_enabled: true, quality_provider: 'chanshui' }))

    await open()
    expect(quality().props('modelValue')).toBe(false)
    expect(wrapper.find('[data-testid="scheduled-quality-config"]').exists()).toBe(false)
  })

  it.each(['pelican', undefined])('does not turn a legacy %s plan into quality opt-in when edited', async provider => {
    const plan = { id: 1, account_id: 2, model_id: 'gpt-test', cron_expression: '*/5 * * * *',
      enabled: true, max_results: 20, auto_recover: false, quality_check_enabled: true, quality_provider: provider }
    listByAccount.mockResolvedValue([plan])
    update.mockResolvedValue({ ...plan, quality_provider: 'chanshui', quality_check_enabled: false })
    const wrapper = mount(ScheduledTestsPanel, {
      props: { show: false, accountId: 2, modelOptions: [] },
      global: { stubs: { BaseDialog: { template: '<div><slot /></div>' }, ConfirmDialog: true } },
    })
    await wrapper.setProps({ show: true }); await flushPromises()
    await wrapper.get('button[title="admin.scheduledTests.editPlan"]').trigger('click')
    const quality = wrapper.findAllComponents(Toggle).find(toggle => toggle.element.closest('label')?.textContent?.includes('admin.scheduledTests.qualityCheck'))!
    expect(quality.props('modelValue')).toBe(false)
    expect(wrapper.find('[data-testid="scheduled-quality-config"]').exists()).toBe(false)
    await wrapper.findAll('button').find(button => button.text() === 'common.save')!.trigger('click')
    await flushPromises()
    expect(update).toHaveBeenCalledWith(1, expect.objectContaining({ quality_provider: 'chanshui', quality_check_enabled: false }))
  })

  it('does not reload unchanged reports or poll while the page is hidden', async () => {
    vi.useFakeTimers()
    const visibility = vi.spyOn(document, 'visibilityState', 'get').mockReturnValue('visible')
    const plan = { id: 1, account_id: 2, model_id: 'perf-model', enabled: true, max_results: 20, quality_provider: 'pelican', last_run_at: '2026-10-04T00:00:00Z' }
    listByAccount.mockReset().mockResolvedValue([plan])
    listResults.mockReset().mockResolvedValue([{ id: 3, plan_id: 1, status: 'success', response_text: '<html>fixture</html>' }])
    const wrapper = mount(ScheduledTestsPanel, { props: { show: false, accountId: 2, modelOptions: [] }, global: { stubs: { BaseDialog: { template: '<div><slot /></div>' }, ConfirmDialog: true } } })
    try {
      await wrapper.setProps({ show: true }); await flushPromises()
      await wrapper.findAll('div').find(el => el.classes().includes('cursor-pointer') && el.text().includes('perf-model'))!.trigger('click'); await flushPromises()
      expect(listResults).toHaveBeenCalledTimes(1)
      await vi.advanceTimersByTimeAsync(30000); await flushPromises()
      expect(listResults).toHaveBeenCalledTimes(1)
      listByAccount.mockResolvedValue([{ ...plan, last_run_at: '2026-10-04T00:05:00Z' }])
      listResults.mockResolvedValue([{ id: 4, plan_id: 1, status: 'success', response_text: '<html>new</html>' }])
      await vi.advanceTimersByTimeAsync(15000); await flushPromises()
      expect(listResults).toHaveBeenLastCalledWith(1, 20, 3)
      visibility.mockReturnValue('hidden')
      const before = listByAccount.mock.calls.length
      await vi.advanceTimersByTimeAsync(30000); await flushPromises()
      expect(listByAccount).toHaveBeenCalledTimes(before)
    } finally { wrapper.unmount(); visibility.mockRestore(); vi.useRealTimers() }
  })
  it('reloads the full result window when the configured history size changes', async () => {
    vi.useFakeTimers()
    const plan = { id: 1, account_id: 2, model_id: 'window-model', enabled: true, max_results: 3, quality_provider: 'pelican', last_run_at: '2026-10-04T00:00:00Z' }
    listByAccount.mockReset().mockResolvedValue([plan])
    listResults.mockReset().mockResolvedValue([
      { id: 3, plan_id: 1, status: 'success', response_text: '<html>three</html>' },
      { id: 2, plan_id: 1, status: 'success', response_text: '<html>two</html>' },
      { id: 1, plan_id: 1, status: 'success', response_text: '<html>one</html>' },
    ])
    const wrapper = mount(ScheduledTestsPanel, { props: { show: false, accountId: 2, modelOptions: [] }, global: { stubs: { BaseDialog: { template: '<div><slot /></div>' }, ConfirmDialog: true } } })
    try {
      await wrapper.setProps({ show: true }); await flushPromises()
      await wrapper.findAll('div').find(el => el.classes().includes('cursor-pointer') && el.text().includes('window-model'))!.trigger('click'); await flushPromises()

      listByAccount.mockResolvedValue([{ ...plan, max_results: 1 }])
      listResults.mockResolvedValue([{ id: 3, plan_id: 1, status: 'success', response_text: '<html>three</html>' }])
      await vi.advanceTimersByTimeAsync(15000); await flushPromises()
      expect(listResults).toHaveBeenLastCalledWith(1, 1, undefined)
      expect(wrapper.text()).not.toContain('#2')

      listByAccount.mockResolvedValue([plan])
      listResults.mockResolvedValue([
        { id: 3, plan_id: 1, status: 'success', response_text: '<html>three</html>' },
        { id: 2, plan_id: 1, status: 'success', response_text: '<html>two</html>' },
        { id: 1, plan_id: 1, status: 'success', response_text: '<html>one</html>' },
      ])
      await vi.advanceTimersByTimeAsync(15000); await flushPromises()
      expect(listResults).toHaveBeenLastCalledWith(1, 3, undefined)
      expect(wrapper.text()).toContain('#2')
    } finally { wrapper.unmount(); vi.useRealTimers() }
  })
  it('shows active audit progress instead of selecting old Pelican artwork after switching', async () => {
    listByAccount.mockResolvedValue([{ id: 1, account_id: 2, model_id: 'gpt-6-astra', cron_expression: '*/5 * * * *',
      enabled: true, max_results: 20, auto_recover: false, quality_check_enabled: true, quality_provider: 'chanshui',
      active_audit: { id: 'fixture-audit', status: 'running' } }])
    listResults.mockResolvedValue([{ id: 3, plan_id: 1, status: 'success', response_text: '<html>old pelican</html>', latency_ms: 2 }])
    const wrapper = mount(ScheduledTestsPanel, { props: { show: false, accountId: 2, modelOptions: [] },
      global: { stubs: { BaseDialog: { template: '<div><slot /></div>' }, ConfirmDialog: true } } })
    await wrapper.setProps({ show: true }); await flushPromises()
    await wrapper.findAll('div').find(el => el.classes().includes('cursor-pointer') && el.text().includes('gpt-6-astra'))!.trigger('click')
    await flushPromises()
    expect(wrapper.find('[data-testid="audit-progress"]').exists()).toBe(true)
    expect(wrapper.find('iframe').exists()).toBe(false)
    expect(wrapper.find('[data-testid="show-all-results"]').exists()).toBe(false)
  })
  it('loads and saves the provider and section selection when editing a plan', async () => {
    const plan = { id: 1, account_id: 2, model_id: 'gpt-test', cron_expression: '*/5 * * * *',
      enabled: true, max_results: 20, auto_recover: false, quality_check_enabled: true,
      quality_provider: 'chanshui', quality_config: { base_url: 'https://audit.example', protocol: 'auto', timeout: 120, sections: ['fingerprint', 'tools'] } }
    listByAccount.mockResolvedValue([plan])
    update.mockResolvedValue(plan)
    const wrapper = mount(ScheduledTestsPanel, {
      props: { show: false, accountId: 2, modelOptions: [] },
      global: { stubs: { BaseDialog: { template: '<div><slot /></div>' }, ConfirmDialog: true } },
    })
    await wrapper.setProps({ show: true }); await flushPromises()
    await wrapper.get('button[title="admin.scheduledTests.editPlan"]').trigger('click')
    expect((wrapper.get('[data-testid="chanshui-service-url"]').element as HTMLInputElement).value).toBe('https://audit.example')
    expect((wrapper.get('input[data-section="tools"]').element as HTMLInputElement).checked).toBe(true)
    await wrapper.findAll('button').find(button => button.text() === 'common.save')!.trigger('click')
    await flushPromises()
    expect(update).toHaveBeenCalledWith(1, expect.objectContaining({ quality_provider: 'chanshui', quality_config: expect.objectContaining(plan.quality_config) }))
    expect(JSON.stringify(update.mock.calls.at(-1))).not.toContain('api_key')
  })
  it('keeps inconclusive responses distinct from success and failure', async () => {
    listByAccount.mockResolvedValue([{
      id: 1, account_id: 2, model_id: 'quality-test-model',
      cron_expression: '*/5 * * * *', enabled: true, max_results: 20,
      auto_recover: false, quality_check_enabled: false, next_run_at: '2026-09-26T10:00:00Z'
    }])
    listResults.mockResolvedValue([{
      id: 3, plan_id: 1, status: 'unknown', latency_ms: 1000,
      response_text: '<html><body><svg></svg></body></html>',
      error_message: 'quality check inconclusive: rendered evaluation required',
      created_at: '2026-09-26T10:00:00Z'
    }])
    const wrapper = mount(ScheduledTestsPanel, {
      props: { show: false, accountId: 2, modelOptions: [] },
      global: { stubs: { BaseDialog: { template: '<div><slot /></div>' }, ConfirmDialog: true } }
    })
    await wrapper.setProps({ show: true })
    await flushPromises()
    const header = wrapper.findAll('div').find(el => el.classes().includes('cursor-pointer') && el.text().includes('quality-test-model'))
    expect(header).toBeDefined()
    await header!.trigger('click')
    await flushPromises()
    expect(listResults).toHaveBeenCalledWith(1, 20)
    expect(wrapper.text()).toContain('admin.scheduledTests.unknown')
    expect(wrapper.text()).not.toContain('admin.scheduledTests.success')
    expect(wrapper.text()).not.toContain('admin.scheduledTests.failed')
    expect(wrapper.text()).not.toContain('admin.scheduledTests.errorMessage')
    expect(wrapper.text()).toContain('rendered evaluation required')
    expect(wrapper.text()).toContain('#3')
    expect(wrapper.get('iframe').attributes('sandbox')).toBe('allow-scripts')
  })

  it('marks only degradation-check plans with the quality badge', async () => {
    listByAccount.mockResolvedValue([
      {
        id: 1, account_id: 2, model_id: 'plain-model',
        cron_expression: '*/5 * * * *', enabled: true, max_results: 20,
        auto_recover: false, quality_check_enabled: false, next_run_at: null
      },
      {
        id: 2, account_id: 2, model_id: 'quality-model',
        cron_expression: '*/5 * * * *', enabled: true, max_results: 20,
        auto_recover: false, quality_check_enabled: true, next_run_at: null
      }
    ])
    listResults.mockResolvedValue([])
    const wrapper = mount(ScheduledTestsPanel, {
      props: { show: false, accountId: 2, modelOptions: [] },
      global: { stubs: { BaseDialog: { template: '<div><slot /></div>' }, ConfirmDialog: true } }
    })
    await wrapper.setProps({ show: true })
    await flushPromises()
    const badges = wrapper.findAll('span').filter(el => el.text() === 'admin.scheduledTests.qualityCheck')
    expect(badges).toHaveLength(1)
  })
})
