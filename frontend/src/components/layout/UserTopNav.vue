<template>
  <nav class="top-browse-nav" :aria-label="t('nav.dashboard')">
    <router-link to="/" class="top-browse-link">{{ homeLabel }}</router-link>
    <router-link :to="homePath" class="top-browse-link" :class="{ 'is-active': !isBrowse }" :aria-current="route.path === homePath ? 'page' : undefined">{{ consoleLabel }}</router-link>
    <router-link v-if="modelPlazaEnabled" to="/model-plaza?embedded=1" class="top-browse-link" :class="{ 'is-active': route.path === '/model-plaza' }" :aria-current="route.path === '/model-plaza' ? 'page' : undefined">{{ t('nav.modelPlaza') }}</router-link>
    <router-link v-for="item in browseItems" :key="item.path" :to="item.path" class="top-browse-link" :class="{ 'is-active': route.path === item.path }" :aria-current="route.path === item.path ? 'page' : undefined">{{ item.label }}</router-link>
    <a v-if="docUrl" :href="docUrl" target="_blank" rel="noopener noreferrer" class="top-browse-link">{{ t('nav.docs') }}<Icon name="arrowUpRight" size="xs" aria-hidden="true" /></a>
  </nav>
</template>

<script setup lang="ts">
import { computed } from 'vue'
import { useRoute } from 'vue-router'
import { useI18n } from 'vue-i18n'
import { useAppStore, useAuthStore } from '@/stores'
import { useConsoleNavigation } from '@/composables/useConsoleNavigation'
import { useConsoleWorkspace } from '@/composables/useConsoleWorkspace'
import { FeatureFlags, isFeatureFlagEnabled } from '@/utils/featureFlags'
import { sanitizeUrl } from '@/utils/url'
import Icon from '@/components/icons/Icon.vue'

const route = useRoute()
const { t, locale } = useI18n()
const app = useAppStore()
const auth = useAuthStore()
const { userNavItems } = useConsoleNavigation()
const { isAdminWorkspace } = useConsoleWorkspace()
const homePath = computed(() => isAdminWorkspace.value ? '/admin/dashboard' : '/dashboard')
const consoleLabel = computed(() => locale.value.startsWith('zh') ? '控制台' : 'Console')
const homeLabel = computed(() => locale.value.startsWith('zh') ? '首页' : 'Home')
const isBrowse = computed(() => ['/model-plaza', '/available-channels', '/monitor'].includes(route.path))
const modelPlazaEnabled = computed(() => isFeatureFlagEnabled(FeatureFlags.modelPlaza))
const docUrl = computed(() => sanitizeUrl(app.docUrl))
const browseItems = computed(() => {
  if ((isAdminWorkspace.value && auth.isSimpleMode) || (!auth.isAdmin && app.backendModeEnabled)) return []
  return userNavItems.value.filter(item => ['/available-channels', '/monitor'].includes(item.path))
})
</script>
