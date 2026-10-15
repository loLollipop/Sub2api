import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { enableAutoUnmount, flushPromises, mount } from '@vue/test-utils'
import { createPinia, setActivePinia } from 'pinia'
import AnnouncementBell from '../AnnouncementBell.vue'
import BaseDialog from '../BaseDialog.vue'
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
    const base = mount(BaseDialog, { props: { show: true, title: 'Base', zIndex: 115 } })
    const store = useAnnouncementStore()
    store.currentPopup = store.announcements[0]
    document.dispatchEvent(new KeyboardEvent('keydown', { key: 'Escape' }))
    await flushPromises()
    expect(store.currentPopup).not.toBeNull()
    expect(base.emitted('close')).toBeUndefined()
    expect(bell.find('.group.relative').exists()).toBe(true)
    store.currentPopup = null
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
})
