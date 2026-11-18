import { defineStore } from 'pinia'
import { ref, computed } from 'vue'
import { announcementsAPI } from '@/api'
import type { UserAnnouncement } from '@/types'
import type { AnnouncementListItem } from '@/api/announcements'

const THROTTLE_MS = 60 * 1000

export const useAnnouncementStore = defineStore('announcements', () => {
  // State
  const announcements = ref<AnnouncementListItem[]>([])
  const listLoading = ref(false)
  const markingAll = ref(false)
  const loading = computed(() => listLoading.value || markingAll.value)
  const fetchError = ref(false)
  const dismissingPopup = ref(false)
  const lastFetchTime = ref(0)
  const popupQueue = ref<UserAnnouncement[]>([])
  const currentPopup = ref<UserAnnouncement | null>(null)

  // Session-scoped dedup set — not reactive, used as plain lookup only
  let shownPopupIds = new Set<number>()
  let fetchGeneration = 0
  let sessionGeneration = 0
  let listAbort: AbortController | null = null
  let summaryAbort: AbortController | null = null
  let popupAbort: AbortController | null = null
  let summaryFetched = 0
  let popupFetched = 0
  let summaryRequest: Promise<void> | null = null
  let popupRequest: Promise<void> | null = null
  const detailRequests = new Map<number, { controller: AbortController; promise: Promise<UserAnnouncement | null>; revision?: string }>()
  const summary = ref<{ total: number; unread: number } | null>(null)
  const summaryError = ref(false)
  const summaryLoading = ref(false)
  const popupError = ref(false)
  const detailCache = ref<Record<number, UserAnnouncement>>({})
  const detailLoading = ref<Record<number, boolean>>({})
  const detailError = ref<Record<number, boolean>>({})
  const readAcks = new Map<number, string>()

  // Getters
  const unreadCount = computed(() =>
    summary.value?.unread ?? announcements.value.filter((a) => !a.read_at).length
  )

  // Actions
  async function fetchAnnouncements(force = false) {
    const now = Date.now()
    if (!force && lastFetchTime.value > 0 && now - lastFetchTime.value < THROTTLE_MS) {
      return
    }

    // Set immediately to prevent concurrent duplicate requests
    lastFetchTime.value = now
    const generation = ++fetchGeneration
    listAbort?.abort()
    const controller = new AbortController()
    listAbort = controller

    try {
      listLoading.value = true
      fetchError.value = false
      const all = await announcementsAPI.list(false, { signal: controller.signal })
      if (generation !== fetchGeneration) return
      announcements.value = all.slice(0, 20).map(item => ({ ...item, read_at: readAcks.get(item.id) ?? item.read_at }))
      for (const item of all) {
        const cached = detailCache.value[item.id]
        if (cached && cached.updated_at !== item.updated_at) delete detailCache.value[item.id]
        const pending = detailRequests.get(item.id)
        if (pending && pending.revision !== item.updated_at) {
          pending.controller.abort()
          detailRequests.delete(item.id)
          detailLoading.value = { ...detailLoading.value, [item.id]: false }
        }
      }
    } catch (err: any) {
      if (generation !== fetchGeneration) return
      // Revert throttle timestamp on failure so retry is allowed
      lastFetchTime.value = 0
      fetchError.value = true
      console.error('Failed to fetch announcements:', err)
    } finally {
      if (generation === fetchGeneration) listLoading.value = false
    }
  }

  function enqueueNewPopups(items: UserAnnouncement[]) {
    const newPopups = items.filter(
      (a) => a.notify_mode === 'popup' && !a.read_at && !shownPopupIds.has(a.id)
    )
    if (newPopups.length === 0) return

    for (const p of newPopups) {
      if (!popupQueue.value.some((q) => q.id === p.id)) {
        popupQueue.value.push(p)
      }
    }

    if (!currentPopup.value) {
      showNextPopup()
    }
  }

  function showNextPopup() {
    if (popupQueue.value.length === 0) {
      currentPopup.value = null
      return
    }
    currentPopup.value = popupQueue.value.shift()!
    shownPopupIds.add(currentPopup.value.id)
  }

  async function dismissPopup() {
    if (!currentPopup.value || dismissingPopup.value) return
    const id = currentPopup.value.id
    const session = sessionGeneration
    dismissingPopup.value = true
    try {
      const marked = await markAsRead(id)
      if (session !== sessionGeneration || currentPopup.value?.id !== id) return
      if (!marked) return false
      showNextPopup()
      if (!currentPopup.value) void fetchPopups(true)
      return true
    } finally {
      if (session === sessionGeneration) dismissingPopup.value = false
    }
  }

  async function markAsRead(id: number) {
    const session = sessionGeneration
    try {
      await announcementsAPI.markRead(id)
      if (session !== sessionGeneration) return
      applyReadLocally(id)
      return true
    } catch (err: any) {
      if (session !== sessionGeneration) return
      console.error('Failed to mark announcement as read:', err)
      return false
    }
  }

  async function markAllAsRead() {
    const session = sessionGeneration
    const unread = announcements.value.filter((a) => !a.read_at)
    if (unread.length === 0) return true

    try {
      markingAll.value = true
      const results = await Promise.allSettled(unread.map(async (a) => {
        await announcementsAPI.markRead(a.id)
        if (session === sessionGeneration) applyReadLocally(a.id)
      }))
      if (session !== sessionGeneration) return false
      const failure = results.find((result) => result.status === 'rejected')
      if (failure) throw failure.reason
      return true
    } catch (err: any) {
      if (session !== sessionGeneration) return false
      console.error('Failed to mark all as read:', err)
      throw err
    } finally {
      if (session === sessionGeneration) markingAll.value = false
    }
  }

  function applyReadLocally(id: number) {
    const row = announcements.value.find(item => item.id === id)
    const cached = detailCache.value[id]
    const alreadyRead = readAcks.has(id) || !!row?.read_at || !!cached?.read_at ||
      (currentPopup.value?.id === id && !!currentPopup.value.read_at)
    const at = new Date().toISOString()
    readAcks.set(id, at)
    if (row) row.read_at = at
    if (cached) cached.read_at = at
    if (currentPopup.value?.id === id) currentPopup.value.read_at = at
    for (const popup of popupQueue.value) if (popup.id === id) popup.read_at = at
    popupQueue.value = popupQueue.value.filter(item => item.id !== id)
    if (summary.value && !alreadyRead) summary.value.unread = Math.max(0, summary.value.unread - 1)
    // A count snapshot started before this acknowledgement must not restore it.
    summaryAbort?.abort()
    summaryAbort = null
    summaryRequest = null
    summaryLoading.value = false
    summaryFetched = 0
  }

  async function fetchSummary(force = false): Promise<void> {
    if (!force && summaryRequest) return summaryRequest
    if (!force && summaryFetched && Date.now() - summaryFetched < THROTTLE_MS) return
    summaryAbort?.abort()
    const controller = new AbortController(), session = sessionGeneration
    summaryAbort = controller
    summaryLoading.value = true
    const request = (async () => {
      try {
        const data = await announcementsAPI.summary({ signal: controller.signal })
        if (session !== sessionGeneration || summaryAbort !== controller) return
        summary.value = data
        summaryFetched = Date.now()
        summaryError.value = false
      } catch (error) {
        if (session !== sessionGeneration || summaryAbort !== controller) return
        summaryFetched = 0
        summaryError.value = true
        console.error('Failed to fetch announcement summary:', error)
      } finally {
        if (session === sessionGeneration && summaryAbort === controller) summaryLoading.value = false
      }
    })()
    summaryRequest = request
    try { await request } finally { if (summaryRequest === request) summaryRequest = null }
  }

  async function fetchPopups(force = false): Promise<void> {
    if (!force && popupRequest) return popupRequest
    if (!force && popupFetched && Date.now() - popupFetched < 5 * THROTTLE_MS) return
    popupAbort?.abort()
    const controller = new AbortController(), session = sessionGeneration
    popupAbort = controller
    const request = (async () => {
      try {
        const items = await announcementsAPI.popups({ signal: controller.signal })
        if (session !== sessionGeneration || popupAbort !== controller) return
        popupFetched = Date.now()
        popupError.value = false
        const unread = items.filter(item => !readAcks.has(item.id) && !item.read_at)
        for (const item of unread) {
          if (typeof item.content === 'string') {
            detailRequests.get(item.id)?.controller.abort()
            detailRequests.delete(item.id)
            detailLoading.value = { ...detailLoading.value, [item.id]: false }
            detailError.value = { ...detailError.value, [item.id]: false }
            detailCache.value[item.id] = item
          }
        }
        enqueueNewPopups(unread)
      } catch (error) {
        if (session !== sessionGeneration || popupAbort !== controller) return
        popupFetched = 0
        popupError.value = true
        console.error('Failed to fetch announcement popups:', error)
      }
    })()
    popupRequest = request
    try { await request } finally { if (popupRequest === request) popupRequest = null }
  }

  async function fetchDetail(id: number, force = false): Promise<UserAnnouncement | null> {
    if (!force && typeof detailCache.value[id]?.content === 'string') return detailCache.value[id]
    const old = detailRequests.get(id)
    if (!force && old) return old.promise
    old?.controller.abort()
    const controller = new AbortController(), session = sessionGeneration
    detailLoading.value = { ...detailLoading.value, [id]: true }
    detailError.value = { ...detailError.value, [id]: false }
    const request = (async () => {
      try {
        const item = await announcementsAPI.getById(id, { signal: controller.signal })
        if (session !== sessionGeneration || controller.signal.aborted) return null
        const row = { ...item, read_at: readAcks.get(id) ?? item.read_at }
        detailCache.value = { ...detailCache.value, [id]: row }
        return row
      } catch (error) {
        if (session !== sessionGeneration || controller.signal.aborted) return null
        detailError.value = { ...detailError.value, [id]: true }
        console.error('Failed to fetch announcement detail:', error)
        return null
      } finally {
        if (session === sessionGeneration && detailRequests.get(id)?.controller === controller) {
          detailLoading.value = { ...detailLoading.value, [id]: false }
          detailRequests.delete(id)
        }
      }
    })()
    detailRequests.set(id, { controller, promise: request, revision: announcements.value.find(item => item.id === id)?.updated_at })
    return request
  }

  const detailFor = (id: number | null | undefined) => id ? detailCache.value[id] ?? null : null
  const isDetailLoading = (id: number) => !!detailLoading.value[id]
  const detailErrorFor = (id: number | null | undefined) => !!id && !!detailError.value[id]

  function reset() {
    fetchGeneration++
    sessionGeneration++
    listAbort?.abort()
    summaryAbort?.abort()
    popupAbort?.abort()
    for (const request of detailRequests.values()) request.controller.abort()
    detailRequests.clear()
    summaryRequest = popupRequest = null
    summaryAbort = popupAbort = listAbort = null
    summaryFetched = popupFetched = 0
    summary.value = null
    summaryLoading.value = summaryError.value = popupError.value = markingAll.value = false
    detailCache.value = {}
    detailLoading.value = {}
    detailError.value = {}
    readAcks.clear()
    announcements.value = []
    lastFetchTime.value = 0
    shownPopupIds = new Set()
    popupQueue.value = []
    currentPopup.value = null
    listLoading.value = false
    fetchError.value = false
    dismissingPopup.value = false
  }

  return {
    // State
    announcements,
    loading,
    fetchError,
    dismissingPopup,
    currentPopup,
    // Getters
    unreadCount,
    summaryError,
    summaryLoading,
    popupError,
    detailCache,
    detailFor,
    isDetailLoading,
    detailErrorFor,
    // Actions
    fetchAnnouncements,
    ensureList: fetchAnnouncements,
    fetchSummary,
    fetchPopups,
    fetchDetail,
    dismissPopup,
    markAsRead,
    markAllAsRead,
    reset,
  }
})
