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

    <BaseDialog :show="isModalOpen" :title="t('announcements.title')" width="wide" :z-index="100" close-on-click-outside @close="closeModal">
      <div v-if="unreadCount > 0" class="mb-3 flex flex-wrap items-center justify-between gap-3 text-xs text-gray-500 dark:text-gray-400">
        <span>{{ unreadCount }} {{ t('announcements.unread') }}</span>
        <button type="button" class="btn btn-secondary" :disabled="loading" @click="markAllAsRead">
          {{ t('announcements.markAllRead') }}
        </button>
      </div>
      <div v-if="loading" class="flex justify-center py-10" role="status" :aria-label="t('common.loading')">
        <Icon name="refresh" size="lg" class="animate-spin text-gray-400" />
      </div>
      <div v-else-if="announcements.length" class="divide-y divide-gray-100 dark:divide-dark-700">
        <button
          v-for="item in announcements"
          :key="item.id"
          type="button"
          class="group relative flex w-full items-start gap-3 rounded-lg px-3 py-3 text-left transition-colors hover:bg-gray-50 focus-visible:outline focus-visible:outline-2 focus-visible:outline-primary-500 dark:hover:bg-dark-700/40"
          @click="openDetail(item)"
        >
          <span class="mt-0.5 flex h-8 w-8 shrink-0 items-center justify-center rounded-lg" :class="item.read_at ? 'bg-gray-100 text-gray-400 dark:bg-dark-700' : 'bg-primary-50 text-primary-600 dark:bg-primary-900/20 dark:text-primary-300'">
            <Icon :name="item.read_at ? 'checkCircleSimple' : 'infoCircleSimple'" size="sm" />
          </span>
          <span class="min-w-0 flex-1">
            <span class="block break-words text-sm font-medium text-gray-900 dark:text-white">{{ item.title }}</span>
            <span class="mt-1 flex flex-wrap items-center gap-2 text-xs text-gray-500 dark:text-gray-400">
              <time :datetime="item.created_at">{{ formatRelativeTime(item.created_at) }}</time>
              <span v-if="!item.read_at" class="text-primary-600 dark:text-primary-300">{{ t('announcements.unread') }}</span>
            </span>
          </span>
          <Icon name="chevronRightSmall" size="sm" class="mt-1 shrink-0 text-gray-400" />
        </button>
      </div>
      <div v-else class="py-10 text-center">
        <Icon name="inbox" size="xl" class="mx-auto mb-3 text-gray-400" />
        <p class="text-sm font-medium text-gray-900 dark:text-white">{{ t('announcements.empty') }}</p>
        <p class="mt-1 text-xs text-gray-500 dark:text-gray-400">{{ t('announcements.emptyDescription') }}</p>
      </div>
    </BaseDialog>

    <BaseDialog :show="detailModalOpen && !!selectedAnnouncement" :title="selectedAnnouncement?.title || t('announcements.title')" width="wide" :z-index="110" close-on-click-outside @close="closeDetail">
      <template v-if="selectedAnnouncement">
        <div class="mb-4 flex flex-wrap items-center gap-x-4 gap-y-1 text-xs text-gray-500 dark:text-gray-400">
          <time :datetime="selectedAnnouncement.created_at">{{ formatRelativeWithDateTime(selectedAnnouncement.created_at) }}</time>
          <span>{{ selectedAnnouncement.read_at ? t('announcements.read') : t('announcements.unread') }}</span>
        </div>
        <div class="markdown-body prose prose-sm max-w-none break-words dark:prose-invert" v-html="renderMarkdown(selectedAnnouncement.content)" />
      </template>
      <template #footer>
        <div v-if="selectedAnnouncement" class="flex flex-wrap items-center justify-between gap-3">
          <span class="text-xs text-gray-500 dark:text-gray-400">{{ selectedAnnouncement.read_at ? t('announcements.readStatus') : t('announcements.markReadHint') }}</span>
          <div class="ml-auto flex flex-wrap justify-end gap-2">
            <button type="button" class="btn btn-secondary" @click="closeDetail">{{ t('common.close') }}</button>
            <button v-if="!selectedAnnouncement.read_at" type="button" class="btn btn-primary" @click="markAsReadAndClose(selectedAnnouncement.id)">{{ t('announcements.markRead') }}</button>
          </div>
        </div>
      </template>
    </BaseDialog>
  </div>
</template>

<script setup lang="ts">
import { ref, computed, onBeforeUnmount, watch } from 'vue'
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
import { createDialogLayer } from './dialogStack'
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

// Methods
function renderMarkdown(content: string): string {
  if (!content) return ''
  const html = marked.parse(content) as string
  return DOMPurify.sanitize(html)
}

function openModal() {
  isModalOpen.value = true
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
    if (!marked) appStore.showError(t('common.unknownError'))
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
    await announcementStore.markAllAsRead()
    appStore.showSuccess(t('announcements.allMarkedAsRead'))
  } catch (err: any) {
    appStore.showError(err?.message || t('common.unknownError'))
  }
}

// Required announcement popups stay open until explicitly acknowledged.
const popupLayer = createDialogLayer(() => {})
watch(() => announcementStore.currentPopup, popup => popupLayer.update(!!popup, 120), { immediate: true, flush: 'sync' })

onBeforeUnmount(() => {
  popupLayer.remove()
})
</script>
