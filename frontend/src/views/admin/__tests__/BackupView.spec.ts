import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { enableAutoUnmount, flushPromises, mount } from '@vue/test-utils'

import BackupView from '../BackupView.vue'

const {
  getS3Config,
  getImageStorageConfig,
  getSchedule,
  listBackups,
  getDownloadURL,
  getBackup,
  restoreBackup,
  deleteBackup,
  showError,
  showSuccess,
  showWarning,
} = vi.hoisted(() => ({
  getS3Config: vi.fn(),
  getImageStorageConfig: vi.fn(),
  getSchedule: vi.fn(),
  listBackups: vi.fn(),
  getDownloadURL: vi.fn(),
  getBackup: vi.fn(),
  restoreBackup: vi.fn(),
  deleteBackup: vi.fn(),
  showError: vi.fn(),
  showSuccess: vi.fn(),
  showWarning: vi.fn(),
}))

vi.mock('@/api', () => ({
  adminAPI: {
    backup: {
      getS3Config,
      updateS3Config: vi.fn(),
      testS3Connection: vi.fn(),
      getImageStorageConfig,
      updateImageStorageConfig: vi.fn(),
      testImageStorageConnection: vi.fn(),
      getSchedule,
      updateSchedule: vi.fn(),
      createBackup: vi.fn(),
      listBackups,
      getBackup,
      deleteBackup,
      getDownloadURL,
      restoreBackup,
    },
  },
}))

vi.mock('@/stores', () => ({
  useAppStore: () => ({
    showError,
    showSuccess,
    showWarning,
  }),
}))

vi.mock('@/composables/useStepUp', () => ({
  useStepUp: () => ({ run: (fn: () => unknown) => fn() }),
  isStepUpBlocked: () => false,
  isStepUpCancelled: () => false,
  stepUpBlockReason: () => '',
}))

vi.mock('vue-i18n', () => ({
  useI18n: () => ({
    t: (key: string, params?: Record<string, unknown>) =>
      params?.index === undefined ? key : `${key}:${params.index}`,
  }),
}))

const baseRecord = (id: string, parts?: unknown[]) => ({
  id,
  status: 'completed',
  backup_type: 'postgres',
  file_name: `${id}.sql.gz`,
  s3_key: `backups/${id}.sql.gz`,
  parts,
  size_bytes: 10,
  triggered_by: 'manual',
  started_at: '2026-08-09T00:00:00Z',
})

enableAutoUnmount(afterEach)

function mountBackupView() {
  return mount(BackupView, {
    global: {
      stubs: {
        TotpStepUpDialog: true,
      },
    },
  })
}

describe('admin BackupView 分卷备份', () => {
  beforeEach(() => {
    vi.clearAllMocks()
    getS3Config.mockResolvedValue({})
    getImageStorageConfig.mockResolvedValue({ config: {}, secret_configured: false })
    getSchedule.mockResolvedValue({ enabled: false, cron_expr: '', retain_days: 14, retain_count: 10 })
    getDownloadURL.mockReset()
    getBackup.mockReset()
    vi.spyOn(HTMLAnchorElement.prototype, 'click').mockImplementation(() => {})
  })

  afterEach(() => {
    vi.restoreAllMocks()
    vi.useRealTimers()
    document.body.innerHTML = ''
  })

  it.each([
    ['backup', 'resolve'], ['backup', 'reject'],
    ['restore', 'resolve'], ['restore', 'reject'],
  ])('ignores an in-flight %s poll that %ss after unmount', async (operation, outcome) => {
    vi.useFakeTimers()
    let resolve!: (value: object) => void
    let reject!: (reason: Error) => void
    getBackup.mockImplementationOnce(() => new Promise((yes, no) => { resolve = yes; reject = no }))
    listBackups.mockResolvedValue({ items: [{ ...baseRecord('pending'),
      status: operation === 'backup' ? 'running' : 'completed',
      restore_status: operation === 'restore' ? 'running' : undefined,
    }] })
    const wrapper = mountBackupView()
    await flushPromises()
    await vi.advanceTimersByTimeAsync(2000)
    expect(getBackup).toHaveBeenCalledTimes(1)
    wrapper.unmount()
    if (outcome === 'resolve') resolve({ ...baseRecord('pending'), restore_status: 'completed' })
    else reject(new Error('late failure'))
    await flushPromises()
    await vi.advanceTimersByTimeAsync(10000)
    expect(getBackup).toHaveBeenCalledTimes(1)
    expect(listBackups).toHaveBeenCalledTimes(1)
    expect(showSuccess).not.toHaveBeenCalled()
    expect(showError).not.toHaveBeenCalled()
    expect(showWarning).not.toHaveBeenCalled()
    expect(vi.getTimerCount()).toBe(0)
  })

  it.each(['resolve', 'reject'])('ignores a pending list refresh that %ss after unmount', async outcome => {
    let resolve!: (value: object) => void
    let reject!: (reason: Error) => void
    listBackups.mockResolvedValueOnce({ items: [] }).mockImplementationOnce(() =>
      new Promise((yes, no) => { resolve = yes; reject = no }))
    const wrapper = mountBackupView()
    await flushPromises()
    const refresh = wrapper.findAll('button').find(button => button.text() === 'common.refresh')!
    await refresh.trigger('click')
    wrapper.unmount()
    if (outcome === 'resolve') resolve({ items: [] })
    else reject(new Error('late list failure'))
    await flushPromises()
    expect(listBackups).toHaveBeenCalledTimes(2)
    expect(showError).not.toHaveBeenCalled()
  })

  it.each(['backup', 'restore'])('does not resume %s polling after navigation during initial loading', async (operation) => {
    vi.useFakeTimers()
    let finish!: (value: object) => void
    listBackups.mockImplementationOnce(() => new Promise(resolve => { finish = resolve }))
    const wrapper = mountBackupView()
    try {
      await flushPromises()
      wrapper.unmount()
      finish({ items: [{ ...baseRecord('pending'),
        status: operation === 'backup' ? 'running' : 'completed',
        restore_status: operation === 'restore' ? 'running' : undefined,
      }] })
      await flushPromises()
      expect(vi.getTimerCount()).toBe(0)
    } finally {
      wrapper.unmount()
      vi.clearAllTimers()
      vi.useRealTimers()
    }
  })

  it('显示分卷数并在下载时列出每个分卷链接', async () => {
    listBackups.mockResolvedValue({
      items: [baseRecord('split', [{ index: 1 }, { index: 2 }, { index: 3 }])],
    })
    getDownloadURL.mockResolvedValue({
      parts: [
        { index: 1, size_bytes: 5, url: 'https://example.test/part-1' },
        { index: 2, size_bytes: 6, url: 'https://example.test/part-2' },
        { index: 3, size_bytes: 7, url: 'https://example.test/part-3' },
      ],
    })

    const wrapper = mountBackupView()
    await flushPromises()

    expect(wrapper.text()).toContain('3')
    const downloadButton = wrapper.findAll('button').find(button =>
      button.text().includes('admin.backup.actions.download'),
    )
    expect(downloadButton).toBeDefined()
    await downloadButton!.trigger('click')
    await flushPromises()

    expect(document.body.textContent).toContain('admin.backup.actions.partLabel:1')
    expect(document.body.textContent).toContain('admin.backup.actions.partLabel:3')
    expect(document.body.querySelector('a[href="https://example.test/part-2"]')).not.toBeNull()
  })

  it('旧单文件记录仍使用单个下载地址', async () => {
    listBackups.mockResolvedValue({ items: [baseRecord('legacy')] })
    getDownloadURL.mockResolvedValue({ url: 'https://example.test/legacy.sql.gz' })

    const wrapper = mountBackupView()
    await flushPromises()
    const downloadButton = wrapper.findAll('button').find(button =>
      button.text().includes('admin.backup.actions.download'),
    )
    await downloadButton!.trigger('click')
    await flushPromises()

    expect(getDownloadURL).toHaveBeenCalledWith('legacy')
    expect(document.body.textContent).not.toContain('admin.backup.actions.downloadParts')
  })

  it('运行中的备份不显示删除入口', async () => {
    listBackups.mockResolvedValue({
      items: [{ ...baseRecord('running'), status: 'running', progress: 'uploading' }],
    })

    const wrapper = mountBackupView()
    await flushPromises()

    expect(wrapper.find('tbody tr td:nth-child(5)').text()).toBe('-')
    expect(wrapper.findAll('button').some(button => button.text() === 'common.delete')).toBe(false)
  })
  it('persists failed restore status and its error after loading the list', async () => {
    listBackups.mockResolvedValue({ items: [{ ...baseRecord('failed-restore'), restore_status: 'failed', restore_error: 'database unavailable' }] })
    const wrapper = mountBackupView()
    await flushPromises()
    const status = wrapper.get('tbody tr td:nth-child(2)')
    expect(status.text()).toContain('admin.backup.status.completed')
    expect(status.text()).toContain('admin.backup.actions.restore: admin.backup.status.failed')
    expect(status.text()).toContain('database unavailable')
    expect(wrapper.findAll('button').find(button => button.text() === 'admin.backup.actions.restore')!.attributes('disabled')).toBeUndefined()
  })

  it('resumes restore polling on refresh and blocks deletion and conflicting restores', async () => {
    vi.useFakeTimers()
    listBackups.mockResolvedValueOnce({ items: [baseRecord('active'), baseRecord('other')] })
      .mockResolvedValue({ items: [{ ...baseRecord('active'), restore_status: 'running' }, baseRecord('other')] })
    getBackup.mockResolvedValue({ ...baseRecord('active'), restore_status: 'running' })
    const wrapper = mountBackupView()
    await flushPromises()
    await wrapper.findAll('button').find(button => button.text() === 'common.refresh')!.trigger('click')
    await flushPromises()
    const rows = wrapper.findAll('tbody tr')
    expect(rows[0]!.get('[role="status"]').text()).toContain('admin.backup.status.running')
    const deleteButton = rows[0]!.findAll('button').find(button => button.text() === 'common.delete')!
    expect(deleteButton.attributes('disabled')).toBeDefined()
    await deleteButton.trigger('click')
    const restoreButton = rows[1]!.findAll('button').find(button => button.text() === 'admin.backup.actions.restore')!
    expect(restoreButton.attributes('disabled')).toBeDefined()
    await restoreButton.trigger('click')
    expect(deleteBackup).not.toHaveBeenCalled()
    expect(restoreBackup).not.toHaveBeenCalled()
    await vi.advanceTimersByTimeAsync(2000)
    expect(getBackup).toHaveBeenCalledWith('active')
    getBackup.mockResolvedValue({ ...baseRecord('active'), restore_status: 'failed', restore_error: 'restore failed' })
    listBackups.mockResolvedValue({ items: [{ ...baseRecord('active'), restore_status: 'failed', restore_error: 'restore failed' }, baseRecord('other')] })
    await vi.advanceTimersByTimeAsync(2000)
    await flushPromises()
    expect(rows[0]!.text()).toContain('restore failed')
    expect(showError).toHaveBeenCalledWith('restore failed')
    expect(vi.getTimerCount()).toBe(0)
    expect(restoreButton.attributes('disabled')).toBeUndefined()
  })

  it.each(['backup', 'restore'])('resumes %s polling when the visibility refresh fails', async operation => {
    vi.useFakeTimers()
    const record = { ...baseRecord('active'), status: operation === 'backup' ? 'running' : 'completed', restore_status: operation === 'restore' ? 'running' : undefined }
    listBackups.mockResolvedValueOnce({ items: [record] }).mockRejectedValueOnce(new Error('temporary list failure'))
    getBackup.mockResolvedValue(record)
    mountBackupView()
    await flushPromises()
    const hidden = vi.spyOn(document, 'hidden', 'get').mockReturnValue(true)
    document.dispatchEvent(new Event('visibilitychange'))
    expect(vi.getTimerCount()).toBe(0)
    hidden.mockReturnValue(false)
    document.dispatchEvent(new Event('visibilitychange'))
    await flushPromises()
    await vi.advanceTimersByTimeAsync(2000)
    expect(getBackup).toHaveBeenCalledWith('active')
    expect(vi.getTimerCount()).toBe(1)
  })

  it('ignores an obsolete list response after a newer visibility refresh', async () => {
    vi.useFakeTimers()
    let finishOld!: (value: object) => void
    listBackups.mockImplementationOnce(() => new Promise(resolve => { finishOld = resolve }))
      .mockResolvedValueOnce({ items: [{ ...baseRecord('current'), restore_status: 'running' }] })
    const wrapper = mountBackupView()
    await flushPromises()
    vi.spyOn(document, 'hidden', 'get').mockReturnValue(false)
    document.dispatchEvent(new Event('visibilitychange'))
    await flushPromises()
    finishOld({ items: [baseRecord('obsolete')] })
    await flushPromises()
    expect(wrapper.get('tbody').text()).toContain('current.sql.gz')
    expect(wrapper.get('tbody').text()).not.toContain('obsolete.sql.gz')
    expect(wrapper.get('[role="status"]').text()).toContain('admin.backup.status.running')
  })

  it('clears the restore lock when visibility refresh finds the operation has finished', async () => {
    vi.useFakeTimers()
    listBackups.mockResolvedValueOnce({ items: [{ ...baseRecord('active'), restore_status: 'running' }] })
      .mockResolvedValue({ items: [{ ...baseRecord('active'), restore_status: 'completed', restored_at: '2026-10-01T12:00:00Z' }] })
    const wrapper = mountBackupView()
    await flushPromises()
    expect(vi.getTimerCount()).toBe(1)
    const hidden = vi.spyOn(document, 'hidden', 'get').mockReturnValue(true)
    document.dispatchEvent(new Event('visibilitychange'))
    expect(vi.getTimerCount()).toBe(0)
    hidden.mockReturnValue(false)
    document.dispatchEvent(new Event('visibilitychange'))
    await flushPromises()
    const restore = wrapper.findAll('button').find(button => button.text() === 'admin.backup.actions.restore')!
    expect(restore.attributes('disabled')).toBeUndefined()
    expect(wrapper.get('[role="status"]').text()).toContain('admin.backup.status.completed')
    expect(vi.getTimerCount()).toBe(0)
  })

})
