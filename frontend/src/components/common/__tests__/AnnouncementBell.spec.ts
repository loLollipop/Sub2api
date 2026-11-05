import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { enableAutoUnmount, flushPromises, mount } from '@vue/test-utils'
import { createPinia, setActivePinia } from 'pinia'
import AnnouncementBell from '../AnnouncementBell.vue'
import BaseDialog from '../BaseDialog.vue'
import AnnouncementPopup from '../AnnouncementPopup.vue'
import { useAnnouncementStore } from '@/stores/announcements'

const { markRead, showError, showSuccess } = vi.hoisted(() => ({ markRead: vi.fn(), showError: vi.fn(), showSuccess: vi.fn() }))
vi.mock('@/api', () => ({ announcementsAPI: { markRead } }))
vi.mock('@/stores/app', () => ({ useAppStore: () => ({ showError, showSuccess }) }))
vi.mock('vue-i18n', () => ({ useI18n: () => ({ t: (key: string) => key }) }))
vi.mock('@/utils/format', () => ({ formatRelativeTime: () => 'now', formatRelativeWithDateTime: () => 'now' }))
enableAutoUnmount(afterEach)
beforeEach(() => {
  setActivePinia(createPinia())
  vi.clearAllMocks()
  vi.spyOn(console, 'error').mockImplementation(() => {})
  useAnnouncementStore().announcements = [{ id: 1, title: 'Notice', content: 'Details', notify_mode: 'silent', created_at: '2026-09-30', updated_at: '2026-09-30' }]
})
afterEach(() => { vi.restoreAllMocks(); document.body.style.overflow = '' })

describe('announcement read confirmation', () => {
  it('blocks Escape behind a required announcement popup', async () => {
    const bell = mount(AnnouncementBell, { global: { stubs: { Teleport: true, Transition: true, Icon: true } } })
    await bell.get('button').trigger('click')
    expect(bell.find('.modal-overlay--drawer').exists()).toBe(true)
    expect(bell.get('.modal-header .modal-description').text()).toBe('1 announcements.unread')
    expect(bell.get('.modal-header-actions button').text()).toBe('announcements.markAllRead')
    expect(bell.get('.announcement-row-heading time').text()).toBe('now')
    expect(bell.find('.announcement-unread-chip').exists()).toBe(true)
    const base = mount(BaseDialog, { props: { show: true, title: 'Base', zIndex: 115 } })
    const store = useAnnouncementStore()
    const popup = mount(AnnouncementPopup, { global: { stubs: { Teleport: true, Transition: true, Icon: true } } })
    store.currentPopup = store.announcements[0]
    await flushPromises()
    expect(popup.get('[role="dialog"]').attributes('aria-modal')).toBe('true')
    document.dispatchEvent(new KeyboardEvent('keydown', { key: 'Escape' }))
    await flushPromises()
    expect(store.currentPopup).not.toBeNull()
    expect(base.emitted('close')).toBeUndefined()
    expect(bell.find('.group.relative').exists()).toBe(true)
    store.currentPopup = null
    await flushPromises()
    document.dispatchEvent(new KeyboardEvent('keydown', { key: 'Escape' }))
    expect(base.emitted('close')).toHaveLength(1)
  })

  it('closes one visual layer across announcement dialogs and BaseDialog', async () => {
    markRead.mockResolvedValue(undefined)
    const bell = mount(AnnouncementBell, { global: { stubs: { Teleport: true, Transition: true, Icon: true } } })
    await bell.get('button').trigger('click')
    await bell.get('.group.relative').trigger('click')
    await flushPromises()
    // Mounted later, but visually below both announcement layers.
    const base = mount(BaseDialog, { props: { show: true, title: 'Base', zIndex: 50 } })
    const escape = async () => {
      document.dispatchEvent(new KeyboardEvent('keydown', { key: 'Escape' }))
      await flushPromises()
    }
    await escape()
    expect(bell.find('.markdown-body').exists()).toBe(false)
    expect(bell.find('.group.relative').exists()).toBe(true)
    expect(base.emitted('close')).toBeUndefined()
    expect(document.body.classList.contains('modal-open')).toBe(true)

    await base.setProps({ zIndex: 105 })
    await escape()
    expect(base.emitted('close')).toHaveLength(1)
    expect(bell.find('.group.relative').exists()).toBe(true)
    base.unmount()
    expect(document.body.classList.contains('modal-open')).toBe(true)
    await escape()
    expect(bell.find('.group.relative').exists()).toBe(false)
    expect(document.body.classList.contains('modal-open')).toBe(false)
  })

  it('removes announcement layers on unmount without releasing another dialog lock', async () => {
    const base = mount(BaseDialog, { props: { show: true, title: 'Base' } })
    const bell = mount(AnnouncementBell, { global: { stubs: { Teleport: true, Transition: true, Icon: true } } })
    await bell.get('button').trigger('click')
    bell.unmount()
    expect(document.body.classList.contains('modal-open')).toBe(true)
    document.dispatchEvent(new KeyboardEvent('keydown', { key: 'Escape' }))
    expect(base.emitted('close')).toHaveLength(1)
    base.unmount()
    expect(document.body.classList.contains('modal-open')).toBe(false)
  })

  it('keeps a failed read confirmation open and only reports success after retry', async () => {
    markRead.mockRejectedValue(new Error('offline'))
    const wrapper = mount(AnnouncementBell, { global: { stubs: { Teleport: true, Transition: true, Icon: true } } })
    await wrapper.get('button').trigger('click')
    await wrapper.get('.group.relative').trigger('click')
    await flushPromises()
    expect(wrapper.get('.announcement-detail-panel .modal-header .announcement-meta').exists()).toBe(true)
    expect(wrapper.find('.announcement-detail-panel .modal-body .announcement-meta').exists()).toBe(false)
    const confirm = () => wrapper.findAll('button').find(button => button.text() === 'announcements.markRead')!
    await confirm().trigger('click')
    await flushPromises()
    expect(markRead).toHaveBeenCalledTimes(2)
    expect(showSuccess).not.toHaveBeenCalled()
    expect(showError).toHaveBeenCalled()
    expect(wrapper.find('.markdown-body').exists()).toBe(true)
    markRead.mockResolvedValue(undefined)
    await confirm().trigger('click')
    await flushPromises()
    expect(showSuccess).toHaveBeenCalledWith('announcements.markedAsRead')
    expect(wrapper.find('.markdown-body').exists()).toBe(false)
    expect(useAnnouncementStore().unreadCount).toBe(0)
  })

  it('does not notify or stop the new session loading after an old bulk read completes', async () => {
    let resolve!: () => void
    markRead.mockReturnValueOnce(new Promise<void>(done => { resolve = done }))
    const wrapper = mount(AnnouncementBell, { global: { stubs: { Teleport: true, Transition: true, Icon: true } } })
    await wrapper.get('button').trigger('click')
    await wrapper.findAll('button').find(button => button.text() === 'announcements.markAllRead')!.trigger('click')
    const store = useAnnouncementStore()
    store.reset()
    store.loading = true
    resolve()
    await flushPromises()
    expect(store.loading).toBe(true)
    expect(showSuccess).not.toHaveBeenCalled()
    expect(showError).not.toHaveBeenCalled()
  })

  it('keeps the drawer and its scroll position while reading a detail and restores row focus', async () => {
    markRead.mockResolvedValue(undefined)
    const wrapper = mount(AnnouncementBell, { attachTo: document.body, global: { stubs: { Teleport: true, Transition: true, Icon: true } } })
    await wrapper.get('button').trigger('click')
    const drawer = wrapper.get('.announcement-overlay--drawer')
    const body = drawer.get('.modal-body').element
    body.scrollTop = 180
    const row = drawer.get<HTMLButtonElement>('.announcement-row')
    row.element.focus()
    await row.trigger('click')
    await flushPromises()
    expect(wrapper.findAll('.announcement-overlay')).toHaveLength(2)
    expect(drawer.attributes('aria-hidden')).toBe('true')
    document.dispatchEvent(new KeyboardEvent('keydown', { key: 'Escape' }))
    await flushPromises()
    expect(wrapper.findAll('.announcement-overlay')).toHaveLength(1)
    expect(body.scrollTop).toBe(180)
    expect(document.activeElement).toBe(row.element)
  })
})
