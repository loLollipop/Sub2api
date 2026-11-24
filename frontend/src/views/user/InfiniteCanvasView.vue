<template>
  <AppLayout>
    <div class="canvas-page-layout">
      <div v-if="groups.length" class="mb-3 flex items-center gap-3">
        <label for="canvas-image-group" class="text-sm text-gray-500">{{ t('infiniteCanvas.group') }}</label>
        <select id="canvas-image-group" v-model.number="groupId" class="input max-w-xs" :disabled="loading || canvasBusy">
          <option v-for="group in groups" :key="group.id" :value="group.id">{{ group.name }}</option>
        </select>
        <span class="hidden text-xs text-gray-400 sm:inline">{{ t('infiniteCanvas.imageOnly') }}</span>
      </div>
      <div class="card flex-1 min-h-0 overflow-hidden">
        <div v-if="loading" class="flex h-full flex-col items-center justify-center gap-3 py-12">
          <div class="h-8 w-8 animate-spin rounded-full border-2 border-primary-500 border-t-transparent"></div>
          <p class="text-sm text-gray-500 dark:text-dark-400">{{ t('infiniteCanvas.preparing') }}</p>
        </div>

        <div v-else-if="errorCode === 'not-configured'" class="flex h-full items-center justify-center p-6 text-center">
          <div class="max-w-md">
            <h3 class="text-lg font-semibold text-gray-900 dark:text-white">{{ t('infiniteCanvas.notConfigured') }}</h3>
            <button type="button" class="btn btn-primary mt-6" @click="prepareSession(true)">
              <Icon name="plus" size="sm" class="mr-2" />
              {{ t('infiniteCanvas.createAndEnter') }}
            </button>
          </div>
        </div>

        <div v-else-if="errorMessage" class="flex h-full items-center justify-center p-10 text-center">
          <div class="max-w-md">
            <div class="mx-auto mb-4 flex h-12 w-12 items-center justify-center rounded-full bg-gray-100 dark:bg-dark-700">
              <Icon name="exclamationTriangle" size="lg" class="text-amber-500" />
            </div>
            <h3 class="text-lg font-semibold text-gray-900 dark:text-white">{{ t('infiniteCanvas.errorTitle') }}</h3>
            <p class="mt-2 text-sm text-gray-500 dark:text-dark-400">{{ errorMessage }}</p>
            <RouterLink v-if="errorCode === 'key-unavailable' || errorCode === 'missing-key'" to="/keys" class="btn btn-primary mt-6">
              {{ t('infiniteCanvas.manageKeys') }}
            </RouterLink>
            <button v-else type="button" class="btn btn-primary mt-6" @click="prepareSession()">
              {{ t('infiniteCanvas.retry') }}
            </button>
          </div>
        </div>

        <div v-else class="canvas-embed-shell">
          <iframe
            v-if="embedUrl"
            ref="iframeRef"
            :src="embedUrl"
            @load="sendSession"
            class="canvas-embed-frame"
            allow="clipboard-read; clipboard-write; fullscreen"
            allowfullscreen
            :title="t('infiniteCanvas.title')"
          ></iframe>
        </div>
      </div>
    </div>
  </AppLayout>
</template>

<script setup lang="ts">
import { computed, onMounted, onUnmounted, ref, watch } from 'vue'
import { RouterLink, onBeforeRouteLeave } from 'vue-router'
import { useI18n } from 'vue-i18n'
import AppLayout from '@/components/layout/AppLayout.vue'
import Icon from '@/components/icons/Icon.vue'
import { useAppStore, useAuthStore } from '@/stores'
import { userGroupsAPI } from '@/api/groups'
import type { Group } from '@/types'
import { detectTheme } from '@/utils/embedded-url'
import { buildInfiniteCanvasImportUrl, resolveInfiniteCanvasBaseUrl } from '@/utils/infiniteCanvas'
import { InfiniteCanvasSetupError, ensureInfiniteCanvasApiKey, imageCanvasGroups, type InfiniteCanvasSession } from '@/utils/infiniteCanvasSession'

const { t, locale } = useI18n()
const appStore = useAppStore()
const auth = useAuthStore()
const groups = ref<Group[]>([])
const groupId = ref(0)
const iframeRef = ref<HTMLIFrameElement>()
const canvasBusy = ref(false)
const loading = ref(true)
const errorCode = ref<InfiniteCanvasSetupError['code'] | null>(null)
const embedUrl = ref('')
let session: InfiniteCanvasSession | null = null
let disposed = false
let sequence = 0
const controller = new AbortController()

const errorMessage = computed(() => {
  if (errorCode.value === 'not-installed') return t('infiniteCanvas.runtimePending')
  if (errorCode.value === 'no-groups') return t('infiniteCanvas.noGroups')
  if (errorCode.value === 'key-unavailable') return t('infiniteCanvas.keyUnavailable')
  if (errorCode.value === 'missing-key') return t('infiniteCanvas.missingKey')
  if (errorCode.value === 'request-failed') return t('infiniteCanvas.requestFailed')
  return ''
})

function sendSession() {
  if (!session || disposed || !auth.user?.id) return
  iframeRef.value?.contentWindow?.postMessage({
    type: 'sub2api-canvas-session', ownerId: auth.user.id, groupId: groupId.value,
    groupName: groups.value.find(group => group.id === groupId.value)?.name || '',
    apiKey: session.apiKey, baseUrl: window.location.origin, theme: detectTheme(),
    lang: String(locale.value).toLowerCase().startsWith('zh') ? 'zh-CN' : 'en-US',
  }, window.location.origin)
}
function onCanvasMessage(event: MessageEvent) {
  if (event.origin !== window.location.origin || event.source !== iframeRef.value?.contentWindow) return
  if (event.data?.type === 'sub2api-canvas-ready') sendSession()
  if (event.data?.type === 'sub2api-canvas-busy') canvasBusy.value = event.data.busy === true
}
async function loadGroups() {
  const owner = auth.user?.id
  if (!owner) return
  loading.value = true
  try {
    const available = imageCanvasGroups(await userGroupsAPI.getAvailable())
    if (disposed || auth.user?.id !== owner) return
    groups.value = available
    if (!available.length) { errorCode.value = 'no-groups'; loading.value = false; return }
    if (groupId.value === available[0].id) await prepareSession()
    else groupId.value = available[0].id
  } catch {
    if (!disposed && auth.user?.id === owner) { errorCode.value = 'request-failed'; loading.value = false }
  }
}
async function prepareSession(createIfMissing = false) {
  const current = ++sequence
  loading.value = true
  errorCode.value = null
  session = null
  embedUrl.value = ''
  canvasBusy.value = false
  try {
    if (!groupId.value) { await loadGroups(); return }
    if (!appStore.publicSettingsLoaded) await appStore.fetchPublicSettings()
    const runtime = await fetch(resolveInfiniteCanvasBaseUrl(window.location.origin), {
      method: 'HEAD', credentials: 'omit', cache: 'no-store', signal: controller.signal,
    })
    if (disposed || current !== sequence) return
    if (runtime.status === 404) { errorCode.value = 'not-installed'; return }
    if (!runtime.ok || !runtime.headers.get('Content-Type')?.includes('text/html')) throw new Error('Canvas is unavailable')
    const prepared = await ensureInfiniteCanvasApiKey(undefined, { groupId: groupId.value, createIfMissing })
    if (disposed || current !== sequence) return
    session = prepared
    embedUrl.value = buildInfiniteCanvasImportUrl({
      canvasBaseUrl: resolveInfiniteCanvasBaseUrl(window.location.origin), pageOrigin: window.location.origin,
      theme: detectTheme(), lang: String(locale.value).toLowerCase().startsWith('zh') ? 'zh-CN' : 'en-US',
    })
    if (prepared.created) appStore.showSuccess(t('infiniteCanvas.keyCreated'))
  } catch (error) {
    if (!disposed && current === sequence) errorCode.value = error instanceof InfiniteCanvasSetupError ? error.code : 'request-failed'
  } finally { if (!disposed && current === sequence) loading.value = false }
}
function beforeUnload(event: BeforeUnloadEvent) {
  if (canvasBusy.value) { event.preventDefault(); event.returnValue = '' }
}
watch(groupId, (value) => { if (value > 0 && auth.user?.id) void prepareSession() })
watch(() => auth.user?.id, () => {
  sequence++
  session = null
  embedUrl.value = ''
  groupId.value = 0
  if (auth.user?.id) void loadGroups()
})
onBeforeRouteLeave(() => !canvasBusy.value || window.confirm(t('infiniteCanvas.leaveWarning')))
onMounted(() => {
  window.addEventListener('message', onCanvasMessage)
  window.addEventListener('beforeunload', beforeUnload)
  void loadGroups()
})
onUnmounted(() => {
  disposed = true
  session = null
  controller.abort()
  window.removeEventListener('message', onCanvasMessage)
  window.removeEventListener('beforeunload', beforeUnload)
})
</script>

<style scoped>
.canvas-page-layout {
  @apply flex flex-col;
  height: calc(100vh - 64px - 4rem);
}

.canvas-embed-shell {
  @apply relative h-full min-h-0 overflow-hidden bg-gray-50 dark:bg-dark-900;
}

.canvas-embed-frame {
  @apply h-full w-full border-0 bg-white dark:bg-dark-900;
}
</style>
