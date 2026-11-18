import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { createPinia, setActivePinia } from 'pinia'
import { flushPromises } from '@vue/test-utils'
import { useAnnouncementStore } from '../announcements'
import type { UserAnnouncement } from '@/types'

const api = vi.hoisted(() => ({ list: vi.fn(), summary: vi.fn(), popups: vi.fn(), getById: vi.fn(), markRead: vi.fn() }))
vi.mock('@/api', () => ({ announcementsAPI: api }))
const notice = (id: number, revision = 'v1'): UserAnnouncement => ({
  id, title: `Notice ${id}`, content: `Body ${revision}`, notify_mode: 'popup',
  created_at: '2026-11-05T00:00:00Z', updated_at: revision,
})
function deferred<T>() {
  let resolve!: (value: T) => void
  let reject!: (reason: Error) => void
  const promise = new Promise<T>((done, fail) => { resolve = done; reject = fail })
  return { promise, resolve, reject }
}
beforeEach(() => {
  setActivePinia(createPinia())
  vi.resetAllMocks()
  vi.spyOn(console, 'error').mockImplementation(() => {})
  api.popups.mockResolvedValue([])
  api.markRead.mockResolvedValue(undefined)
})
afterEach(() => vi.restoreAllMocks())

describe('lazy announcement contracts', () => {
  it('fetches counts without downloading the timeline or popup bodies', async () => {
    api.summary.mockResolvedValue({ total: 24, unread: 23 })
    const store = useAnnouncementStore()
    await store.fetchSummary()
    await store.fetchSummary()
    expect(store.unreadCount).toBe(23)
    expect(api.summary).toHaveBeenCalledTimes(1)
    expect(api.list).not.toHaveBeenCalled()
    expect(api.popups).not.toHaveBeenCalled()
    expect(api.getById).not.toHaveBeenCalled()
  })

  it('keeps a previous count on summary failure and permits immediate retry', async () => {
    const store = useAnnouncementStore()
    api.summary.mockResolvedValueOnce({ total: 24, unread: 23 }).mockRejectedValueOnce(new Error('offline')).mockResolvedValueOnce({ total: 24, unread: 22 })
    await store.fetchSummary()
    await store.fetchSummary(true)
    expect(store.unreadCount).toBe(23)
    expect(store.summaryError).toBe(true)
    await store.fetchSummary()
    expect(store.unreadCount).toBe(22)
    expect(store.summaryError).toBe(false)
  })

  it('acknowledges only loaded timeline rows without clearing unloaded unread counts', async () => {
    const store = useAnnouncementStore()
    api.summary.mockResolvedValue({ total: 24, unread: 24 })
    await store.fetchSummary()
    store.announcements = [notice(1), notice(2)]
    await store.markAllAsRead()
    expect(api.markRead.mock.calls).toEqual([[1], [2]])
    expect(store.unreadCount).toBe(22)
    await store.markAllAsRead()
    expect(store.unreadCount).toBe(22)
  })

  it('aborts an outdated summary after acknowledgement and immediately permits a new summary', async () => {
    const store = useAnnouncementStore()
    const old = deferred<{ total: number; unread: number }>()
    api.summary.mockResolvedValueOnce({ total: 2, unread: 2 }).mockReturnValueOnce(old.promise).mockResolvedValueOnce({ total: 2, unread: 1 })
    await store.fetchSummary()
    const pending = store.fetchSummary(true)
    await store.markAsRead(1)
    expect(store.summaryLoading).toBe(false)
    expect(api.summary.mock.calls[1][0].signal.aborted).toBe(true)
    await store.fetchSummary()
    old.resolve({ total: 2, unread: 2 })
    await pending
    expect(store.unreadCount).toBe(1)
    expect(api.summary).toHaveBeenCalledTimes(3)
  })

  it.each(['summary', 'popups'] as const)('isolates %s force refreshes and session resets', async endpoint => {
    const store = useAnnouncementStore()
    const old = deferred<unknown>()
    const current = deferred<unknown>()
    api[endpoint].mockReturnValueOnce(old.promise).mockReturnValueOnce(current.promise)
    const fetch = endpoint === 'summary' ? store.fetchSummary : store.fetchPopups
    const pending = fetch()
    store.reset()
    const next = fetch()
    old.resolve(endpoint === 'summary' ? { total: 1, unread: 99 } : [notice(99)])
    await pending
    expect(store.currentPopup).toBeNull()
    expect(store.unreadCount).toBe(0)
    current.resolve(endpoint === 'summary' ? { total: 1, unread: 1 } : [notice(1)])
    await next
    if (endpoint === 'summary') expect(store.unreadCount).toBe(1)
    else expect(store.currentPopup?.id).toBe(1)
    expect(api[endpoint].mock.calls[0][0].signal.aborted).toBe(true)
  })

  it.each(['summary', 'popups'] as const)('keeps the latest forced %s response', async endpoint => {
    const store = useAnnouncementStore()
    const old = deferred<unknown>()
    api[endpoint].mockReturnValueOnce(old.promise).mockResolvedValueOnce(endpoint === 'summary' ? { total: 2, unread: 2 } : [notice(2)])
    const fetch = endpoint === 'summary' ? store.fetchSummary : store.fetchPopups
    const pending = fetch()
    await fetch(true)
    old.resolve(endpoint === 'summary' ? { total: 1, unread: 1 } : [notice(1)])
    await pending
    if (endpoint === 'summary') expect(store.unreadCount).toBe(2)
    else expect(store.currentPopup?.id).toBe(2)
  })

  it('reuses popup bootstrap bodies and requests another bounded batch after confirmations', async () => {
    const store = useAnnouncementStore()
    api.popups.mockResolvedValueOnce([1, 2, 3, 4, 5].map(id => notice(id))).mockResolvedValueOnce([notice(1), notice(6)])
    await store.fetchPopups()
    expect((await store.fetchDetail(1))?.content).toBe('Body v1')
    expect(api.getById).not.toHaveBeenCalled()
    for (let id = 1; id <= 5; id++) {
      expect(store.currentPopup?.id).toBe(id)
      await store.dismissPopup()
    }
    await flushPromises()
    expect(api.popups).toHaveBeenCalledTimes(2)
    expect(store.currentPopup?.id).toBe(6)
  })

  it('deduplicates detail requests and caches even an intentionally empty body', async () => {
    const store = useAnnouncementStore()
    const pending = deferred<UserAnnouncement>()
    api.getById.mockReturnValueOnce(pending.promise)
    const first = store.fetchDetail(1)
    const second = store.fetchDetail(1)
    pending.resolve({ ...notice(1), content: '' })
    await Promise.all([first, second])
    await store.fetchDetail(1)
    expect(api.getById).toHaveBeenCalledTimes(1)
    expect(store.isDetailLoading(1)).toBe(false)
  })

  it('keeps the latest forced detail when an aborted response arrives late', async () => {
    const store = useAnnouncementStore()
    const old = deferred<UserAnnouncement>()
    api.getById.mockReturnValueOnce(old.promise).mockResolvedValueOnce(notice(1, 'v2'))
    const pending = store.fetchDetail(1)
    await store.fetchDetail(1, true)
    old.resolve(notice(1))
    expect(await pending).toBeNull()
    expect(store.detailFor(1)?.content).toBe('Body v2')
  })

  it('does not restore old-session details or end a new-session detail load', async () => {
    const store = useAnnouncementStore()
    const old = deferred<UserAnnouncement>()
    const current = deferred<UserAnnouncement>()
    api.getById.mockReturnValueOnce(old.promise).mockReturnValueOnce(current.promise)
    const pending = store.fetchDetail(1)
    store.reset()
    const next = store.fetchDetail(1)
    old.resolve(notice(1))
    await pending
    expect(store.detailFor(1)).toBeNull()
    expect(store.isDetailLoading(1)).toBe(true)
    current.resolve(notice(1, 'v2'))
    await next
    expect(store.detailFor(1)?.updated_at).toBe('v2')
  })

  it('invalidates a pending body when a refreshed timeline reports a new revision', async () => {
    const store = useAnnouncementStore()
    store.announcements = [notice(1)]
    const old = deferred<UserAnnouncement>()
    api.getById.mockReturnValueOnce(old.promise).mockResolvedValueOnce(notice(1, 'v2'))
    const pending = store.fetchDetail(1)
    api.list.mockResolvedValue([notice(1, 'v2')])
    await store.ensureList(true)
    old.resolve(notice(1))
    await pending
    expect(store.detailFor(1)).toBeNull()
    await store.fetchDetail(1)
    expect(store.detailFor(1)?.content).toBe('Body v2')
  })

  it('does not restore unread state from late list and detail snapshots', async () => {
    const store = useAnnouncementStore()
    store.announcements = [notice(1)]
    const detail = deferred<UserAnnouncement>()
    const list = deferred<UserAnnouncement[]>()
    api.getById.mockReturnValue(detail.promise)
    api.list.mockReturnValue(list.promise)
    const body = store.fetchDetail(1)
    const timeline = store.ensureList()
    await store.markAsRead(1)
    detail.resolve(notice(1))
    list.resolve([notice(1)])
    await Promise.all([body, timeline])
    expect(store.detailFor(1)?.read_at).toBeTruthy()
    expect(store.announcements[0].read_at).toBeTruthy()
    expect(store.unreadCount).toBe(0)
  })

  it('shows failed details and permits retry', async () => {
    const store = useAnnouncementStore()
    api.getById.mockRejectedValueOnce(new Error('offline')).mockResolvedValueOnce(notice(1))
    expect(await store.fetchDetail(1)).toBeNull()
    expect(store.detailErrorFor(1)).toBe(true)
    expect(store.isDetailLoading(1)).toBe(false)
    await store.fetchDetail(1, true)
    expect(store.detailErrorFor(1)).toBe(false)
    expect(store.detailFor(1)?.content).toBe('Body v1')
  })
})
