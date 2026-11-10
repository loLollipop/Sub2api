<template>
  <div>
    <button
      type="button"
      class="relative flex h-9 w-9 items-center justify-center rounded-lg text-gray-600 transition-colors hover:bg-gray-100 dark:text-gray-400 dark:hover:bg-dark-800"
      :aria-label="t('announcements.title')"
      aria-haspopup="dialog"
      :aria-expanded="isModalOpen"
      @click="openModal"
    >
      <Icon name="bell" size="md" />
      <span v-if="unreadCount > 0" class="absolute right-1 top-1 h-1.5 w-1.5 rounded-full bg-primary-500" />
    </button>

    <BaseDialog :show="isModalOpen" :title="t('announcements.title')" placement="right" width="narrow" overlay-class="announcement-overlay announcement-overlay--drawer" :z-index="100" close-on-click-outside @close="closeModal">
      <template #description><span v-if="unreadCount > 0">{{ unreadCount }} {{ t('announcements.unread') }}</span></template>
      <template #actions>
        <button v-if="unreadCount > 0" type="button" class="announcement-mark-all" :disabled="loading" @click="markAllAsRead">
          {{ t('announcements.markAllRead') }}
        </button>
      </template>
      <div v-if="loading" class="flex justify-center py-10" role="status" :aria-label="t('common.loading')">
        <Icon name="refresh" size="lg" class="animate-spin text-gray-400" />
      </div>
      <div v-else-if="announcementStore.fetchError" class="announcement-list-error" role="alert">
        <Icon name="exclamationCircle" size="lg" />
        <p>{{ t('announcements.loadFailed') }}</p>
        <button class="btn btn-secondary" @click="announcementStore.fetchAnnouncements(true)">{{ t('common.refresh') }}</button>
      </div>
      <div v-else-if="announcements.length" class="announcement-timeline">
        <button
          v-for="item in announcements"
          :key="item.id"
          type="button"
          class="group relative announcement-row"
          @click="openDetail(item)"
        >
          <span class="announcement-row-icon" :class="item.read_at ? 'bg-gray-100 text-gray-400 dark:bg-dark-700' : 'bg-primary-50 text-primary-600 dark:bg-primary-900/20 dark:text-primary-300'">
            <Icon :name="item.read_at ? 'checkCircleSimple' : 'infoCircleSimple'" size="sm" />
            <span v-if="!item.read_at" class="announcement-unread-chip" :aria-label="t('announcements.unread')" />
          </span>
          <span class="min-w-0 flex-1">
            <span class="announcement-row-heading"><span class="announcement-row-title">{{ item.title }}</span><time :datetime="item.created_at">{{ formatRelativeTime(item.created_at) }}</time></span>
            <span class="announcement-row-excerpt">{{ excerpt(item.content) }}</span>
          </span>
        </button>
      </div>
      <div v-else class="py-10 text-center">
        <Icon name="inbox" size="xl" class="mx-auto mb-3 text-gray-400" />
        <p class="text-sm font-medium text-gray-900 dark:text-white">{{ t('announcements.empty') }}</p>
        <p class="mt-1 text-xs text-gray-500 dark:text-gray-400">{{ t('announcements.emptyDescription') }}</p>
      </div>
    </BaseDialog>

    <BaseDialog :show="detailModalOpen && !!selectedAnnouncement" :title="selectedAnnouncement?.title || t('announcements.title')" width="wide" panel-class="announcement-detail-panel" overlay-class="announcement-overlay" :z-index="110" close-on-click-outside @close="closeDetail">
      <template v-if="selectedAnnouncement" #description>
        <div class="announcement-meta flex flex-wrap items-center gap-x-4 gap-y-1 text-xs text-gray-500 dark:text-gray-400">
          <span class="inline-flex items-center gap-1.5"><Icon name="clock" size="sm" /><time :datetime="selectedAnnouncement.created_at">{{ formatRelativeWithDateTime(selectedAnnouncement.created_at) }}</time></span>
          <span class="rounded-md px-2 py-1" :class="selectedRead ? 'bg-gray-100 text-gray-600 dark:bg-dark-700 dark:text-gray-300' : 'bg-primary-50 text-primary-700 dark:bg-primary-900/20 dark:text-primary-300'">{{ selectedRead ? t('announcements.read') : t('announcements.unread') }}</span>
        </div>
      </template>
      <template v-if="selectedAnnouncement">
        <div class="markdown-body prose prose-sm max-w-none break-words dark:prose-invert" v-html="renderMarkdown(selectedAnnouncement.content)" />
      </template>
      <template #footer>
        <div v-if="selectedAnnouncement" class="flex w-full flex-wrap items-center justify-between gap-3">
          <span class="text-xs text-gray-500 dark:text-gray-400">{{ selectedRead ? t('announcements.readStatus') : t('announcements.markReadHint') }}</span>
          <div class="ml-auto flex flex-wrap justify-end gap-2">
            <button type="button" class="btn btn-secondary" @click="closeDetail">{{ t('common.close') }}</button>
            <button v-if="!selectedRead" type="button" class="btn btn-primary" @click="markAsReadAndClose(selectedAnnouncement.id)">{{ t('announcements.markRead') }}</button>
          </div>
        </div>
      </template>
    </BaseDialog>
  </div>
</template>

<script setup lang="ts">
import { ref, computed } from 'vue'
import { useI18n } from 'vue-i18n'
import { storeToRefs } from 'pinia'
import { marked } from 'marked'
import DOMPurify from 'dompurify'
import { useAppStore } from '@/stores/app'
import { useAnnouncementStore } from '@/stores/announcements'
import { formatRelativeTime, formatRelativeWithDateTime } from '@/utils/format'
import type { UserAnnouncement } from '@/types'
import Icon from '@/components/icons/Icon.vue'
import BaseDialog from './BaseDialog.vue'
import '@/styles/announcement-markdown.css'

const { t } = useI18n()
const appStore = useAppStore()
const announcementStore = useAnnouncementStore()

// Configure marked
marked.setOptions({
  breaks: true,
  gfm: true,
})

// Use store state (storeToRefs for reactivity)
const { announcements, loading } = storeToRefs(announcementStore)
const unreadCount = computed(() => announcementStore.unreadCount)

// Local modal state
const isModalOpen = ref(false)
const detailModalOpen = ref(false)
const selectedAnnouncement = ref<UserAnnouncement | null>(null)
const selectedRead = computed(() => {
  const selected = selectedAnnouncement.value
  return !!(announcements.value.find(item => item.id === selected?.id) ?? selected)?.read_at
})

// Plain-text timeline previews share the sanitized Markdown renderer.
function excerpt(content: string): string {
  const text = new DOMParser().parseFromString(renderMarkdown(content), 'text/html').body.textContent || ''
  return text.replace(/\s+/g, ' ').trim().slice(0, 140)
}

// Methods
function renderMarkdown(content: string): string {
  if (!content) return ''
  const html = marked.parse(content) as string
  return DOMPurify.sanitize(html)
}

function openModal() {
  isModalOpen.value = true
  if (!announcements.value.length || announcementStore.fetchError) void announcementStore.fetchAnnouncements()
}

function closeModal() {
  isModalOpen.value = false
}

function openDetail(announcement: UserAnnouncement) {
  selectedAnnouncement.value = announcement
  detailModalOpen.value = true
  if (!announcement.read_at) {
    markAsRead(announcement.id)
  }
}

function closeDetail() {
  detailModalOpen.value = false
  selectedAnnouncement.value = null
}

async function markAsRead(id: number) {
  try {
    const marked = await announcementStore.markAsRead(id)
    if (marked === false) appStore.showError(t('common.unknownError'))
    return marked
  } catch (err: any) {
    appStore.showError(err?.message || t('common.unknownError'))
    return false
  }
}

async function markAsReadAndClose(id: number) {
  if (!await markAsRead(id)) return
  appStore.showSuccess(t('announcements.markedAsRead'))
  closeDetail()
}

async function markAllAsRead() {
  try {
    if (await announcementStore.markAllAsRead()) appStore.showSuccess(t('announcements.allMarkedAsRead'))
  } catch (err: any) {
    appStore.showError(err?.message || t('common.unknownError'))
  }
}

</script>

<style scoped>
.announcement-timeline { display: grid; gap: 2px; margin: -12px; }
.announcement-row { display: flex; width: 100%; align-items: center; gap: 12px; padding: 10px 12px; border-radius: 8px; text-align: left; transition: background-color 140ms ease; }
.announcement-row:hover { background: var(--signal-hover, #f8fafc); }
.announcement-row:focus-visible { outline: 2px solid var(--signal-accent, var(--theme-accent)); outline-offset: -2px; }
.announcement-row-icon { position: relative; display: grid; place-items: center; flex-shrink: 0; width: 32px; height: 32px; border-radius: 50%; }
.announcement-unread-chip { position: absolute; top: 0; right: 0; width: 7px; height: 7px; border-radius: 50%; background: #ef4444; box-shadow: 0 0 0 2px var(--signal-surface, #fff); }
.announcement-row-heading { display: flex; align-items: center; justify-content: space-between; gap: 12px; }
.announcement-row-heading time { flex-shrink: 0; font-size: 12px; color: var(--signal-muted, #64748b); }
.announcement-row-title { min-width: 0; overflow: hidden; text-overflow: ellipsis; white-space: nowrap; font-size: 13px; line-height: 21px; font-weight: 500; color: var(--signal-text, #111827); }
.announcement-row-excerpt { display: block; overflow: hidden; text-overflow: ellipsis; white-space: nowrap; color: var(--signal-muted, #64748b); font-size: 12px; line-height: 19px; }
.announcement-mark-all { padding: 5px 8px; border-radius: 6px; background: var(--signal-inset, #f8fafc); font-size: 12px; color: var(--signal-text, #111827); }
.announcement-mark-all:hover { background: var(--signal-hover, #f1f5f9); }
.announcement-mark-all:disabled { opacity: .5; }
.announcement-mark-all:focus-visible { outline: 2px solid var(--signal-accent, var(--theme-accent)); outline-offset: 2px; }
.announcement-list-error { display: grid; justify-items: center; gap: 14px; padding-block: 48px; text-align: center; font-size: 13px; color: var(--signal-muted, #64748b); }
</style>
