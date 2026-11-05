import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { createPinia, setActivePinia } from 'pinia'
import { useAnnouncementStore } from '../announcements'
import type { UserAnnouncement } from '@/types'

const { list, markRead } = vi.hoisted(() => ({ list: vi.fn(), markRead: vi.fn() }))
vi.mock('@/api', () => ({ announcementsAPI: { list, markRead } }))

const notice = (id: number): UserAnnouncement => ({
  id, title: `Notice ${id}`, content: 'Content', notify_mode: 'popup',
  created_at: '2026-11-05T00:00:00Z', updated_at: '2026-11-05T00:00:00Z',
})

beforeEach(() => {
  setActivePinia(createPinia())
  vi.resetAllMocks()
  vi.spyOn(console, 'error').mockImplementation(() => {})
})
afterEach(() => vi.restoreAllMocks())

describe('required announcement confirmation', () => {
  it('keeps a failed confirmation open and advances the queue only after a successful retry', async () => {
    const store = useAnnouncementStore()
    list.mockResolvedValue([notice(1), notice(2)])
    await store.fetchAnnouncements()
    markRead.mockRejectedValueOnce(new Error('offline')).mockResolvedValueOnce(undefined)
    expect(await store.dismissPopup()).toBe(false)
    expect(store.currentPopup?.id).toBe(1)
    expect(store.unreadCount).toBe(2)
    expect(await store.dismissPopup()).toBe(true)
    expect(store.currentPopup?.id).toBe(2)
    expect(store.unreadCount).toBe(1)
  })

  it('deduplicates confirmation clicks while the request is pending', async () => {
    const store = useAnnouncementStore()
    store.currentPopup = notice(1)
    let resolve!: () => void
    markRead.mockReturnValue(new Promise<void>(done => { resolve = done }))
    const pending = store.dismissPopup()
    await store.dismissPopup()
    expect(markRead).toHaveBeenCalledTimes(1)
    expect(store.dismissingPopup).toBe(true)
    expect(store.currentPopup?.id).toBe(1)
    resolve()
    await pending
    expect(store.currentPopup).toBeNull()
    expect(store.dismissingPopup).toBe(false)
  })

  it('does not dismiss or mark the new session notice when an old confirmation completes', async () => {
    const store = useAnnouncementStore()
    store.currentPopup = notice(1)
    let resolve!: () => void
    markRead.mockReturnValueOnce(new Promise<void>(done => { resolve = done }))
    const pending = store.dismissPopup()
    store.reset()
    store.announcements = [notice(1)]
    store.currentPopup = store.announcements[0]
    resolve()
    await pending
    expect(store.currentPopup?.id).toBe(1)
    expect(store.announcements[0].read_at).toBeUndefined()
    expect(store.dismissingPopup).toBe(false)
  })

  it.each(['resolve', 'reject'] as const)('does not end new-session loading or report an old bulk confirmation (%s)', async outcome => {
    const store = useAnnouncementStore()
    store.announcements = [notice(1)]
    let resolve!: () => void
    let reject!: (reason: Error) => void
    markRead.mockReturnValueOnce(new Promise<void>((done, fail) => { resolve = done; reject = fail }))
    const pending = store.markAllAsRead()
    store.reset()
    store.announcements = [notice(1)]
    store.loading = true
    if (outcome === 'resolve') resolve()
    else reject(new Error('old session failed'))
    expect(await pending).toBe(false)
    expect(store.announcements[0].read_at).toBeUndefined()
    expect(store.loading).toBe(true)
    expect(console.error).not.toHaveBeenCalled()
  })
})
