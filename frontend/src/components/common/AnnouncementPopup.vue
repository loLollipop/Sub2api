<template>
  <BaseDialog
    :show="!!displayedAnnouncement"
    :title="displayedAnnouncement?.title || t('announcements.title')"
    width="wide"
    panel-class="announcement-popup-panel"
    overlay-class="announcement-overlay"
    :z-index="120"
    :show-close-button="preview"
    :close-on-escape="preview"
    @close="handleDismiss"
  >
    <template v-if="displayedAnnouncement" #description>
      <div class="announcement-meta flex flex-wrap items-center gap-3 text-xs text-gray-500 dark:text-gray-400">
        <span v-if="!preview" class="rounded-md bg-primary-50 px-2 py-1 font-medium text-primary-700 dark:bg-primary-900/20 dark:text-primary-300">{{ t('announcements.unread') }}</span>
        <span class="inline-flex items-center gap-1.5"><Icon name="clock" size="sm" /><time :datetime="displayedAnnouncement.created_at">{{ formatRelativeWithDateTime(displayedAnnouncement.created_at) }}</time></span>
      </div>
    </template>
    <template v-if="displayedAnnouncement">
      <div v-if="detailFailed" class="py-10 text-center" role="alert">
        <p>{{ t('announcements.loadFailed') }}</p>
        <button type="button" class="btn btn-secondary mt-3" @click="loadDetail(true)">{{ t('common.refresh') }}</button>
      </div>
      <div v-else-if="detailPending" class="flex justify-center py-10" role="status" :aria-label="t('common.loading')">
        <Icon name="refresh" size="lg" class="animate-spin text-gray-400" />
      </div>
      <div v-else class="markdown-body prose prose-sm max-w-none break-words dark:prose-invert" v-html="renderedContent" />
    </template>
    <template #footer>
      <div class="flex flex-wrap items-center justify-end gap-3">
        <p v-if="readFailed" role="alert" class="mr-auto text-sm text-red-600 dark:text-red-400">{{ t('announcements.readFailed') }}</p>
        <button type="button" data-testid="announcement-popup-dismiss" class="btn btn-primary" :disabled="announcementStore.dismissingPopup || detailPending || detailFailed" @click="handleDismiss">
          {{ preview ? t('common.close') : t('announcements.markRead') }}
        </button>
      </div>
    </template>
  </BaseDialog>
</template>

<script setup lang="ts">
import BaseDialog from './BaseDialog.vue'
import Icon from '@/components/icons/Icon.vue'
import { computed, ref, watch } from 'vue'
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
const readFailed = ref(false)
const popupId = computed(() => props.preview ? null : announcementStore.currentPopup?.id)
const detail = computed(() => announcementStore.detailFor(popupId.value))
const content = computed(() => detail.value?.content ?? displayedAnnouncement.value?.content)
const detailFailed = computed(() => !props.preview && announcementStore.detailErrorFor(popupId.value))
const detailPending = computed(() => !props.preview && (typeof content.value !== 'string' ||
  (!!popupId.value && announcementStore.isDetailLoading(popupId.value))) && !detailFailed.value)
function loadDetail(force = false) {
  if (popupId.value) void announcementStore.fetchDetail(popupId.value, force)
}
watch(displayedAnnouncement, () => {
  readFailed.value = false
  if (!props.preview && typeof content.value !== 'string') loadDetail()
}, { immediate: true })

marked.setOptions({
  breaks: true,
  gfm: true,
})

const renderedContent = computed(() => {
  if (!content.value) return ''
  const html = marked.parse(content.value) as string
  return DOMPurify.sanitize(html)
})

async function handleDismiss() {
  if (props.preview) {
    emit('close')
    return
  }
  readFailed.value = false
  const announcement = displayedAnnouncement.value
  const marked = await announcementStore.dismissPopup()
  if (displayedAnnouncement.value === announcement && marked === false) readFailed.value = true
}

</script>
