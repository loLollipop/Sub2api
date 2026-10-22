<template>
  <BaseDialog
    :show="!!displayedAnnouncement"
    :title="displayedAnnouncement?.title || t('announcements.title')"
    width="wide"
    :z-index="120"
    :show-close-button="preview"
    :close-on-escape="preview"
    @close="handleDismiss"
  >
    <template v-if="displayedAnnouncement">
      <div class="mb-4 flex flex-wrap items-center gap-3 text-xs text-gray-500 dark:text-gray-400">
        <span class="rounded-md bg-primary-50 px-2 py-1 font-medium text-primary-700 dark:bg-primary-900/20 dark:text-primary-300">{{ t('announcements.title') }}</span>
        <time :datetime="displayedAnnouncement.created_at">{{ formatRelativeWithDateTime(displayedAnnouncement.created_at) }}</time>
      </div>
      <div class="markdown-body prose prose-sm max-w-none break-words dark:prose-invert" v-html="renderedContent" />
    </template>
    <template #footer>
      <div class="flex justify-end">
        <button type="button" data-testid="announcement-popup-dismiss" class="btn btn-primary" @click="handleDismiss">
          {{ preview ? t('common.close') : t('announcements.markRead') }}
        </button>
      </div>
    </template>
  </BaseDialog>
</template>

<script setup lang="ts">
import BaseDialog from './BaseDialog.vue'
import { computed } from 'vue'
import { useI18n } from 'vue-i18n'
import { marked } from 'marked'
import DOMPurify from 'dompurify'
import { useAnnouncementStore } from '@/stores/announcements'
import { formatRelativeWithDateTime } from '@/utils/format'
import type { Announcement, UserAnnouncement } from '@/types'
import '@/styles/announcement-markdown.css'

type PreviewAnnouncement = Pick<Announcement | UserAnnouncement, 'title' | 'content' | 'created_at'>

const props = withDefaults(defineProps<{
  announcement?: PreviewAnnouncement | null
  preview?: boolean
}>(), {
  announcement: null,
  preview: false,
})

const emit = defineEmits<{
  close: []
}>()

const { t } = useI18n()
const announcementStore = useAnnouncementStore()
const displayedAnnouncement = computed(() => (
  props.preview ? props.announcement : announcementStore.currentPopup
))

marked.setOptions({
  breaks: true,
  gfm: true,
})

const renderedContent = computed(() => {
  const content = displayedAnnouncement.value?.content
  if (!content) return ''
  const html = marked.parse(content) as string
  return DOMPurify.sanitize(html)
})

function handleDismiss() {
  if (props.preview) {
    emit('close')
    return
  }
  announcementStore.dismissPopup()
}

</script>
