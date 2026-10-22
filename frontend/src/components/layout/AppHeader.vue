<template>
  <div class="top-header-spacer">
    <header class="top-header" :class="{ 'is-compact': compact, 'is-scrolled': scrolled && !browse }">
      <div class="top-header-primary">
        <div class="top-brand-area">
          <button v-if="!browse || !isDesktop" id="mobile-navigation-trigger" type="button" class="top-sidebar-toggle btn-ghost btn-icon" :aria-label="isDesktop ? (appStore.sidebarCollapsed ? t('nav.expand') : t('nav.collapse')) : t('common.toggleMenu')" :aria-expanded="isDesktop ? !appStore.sidebarCollapsed : appStore.mobileOpen" aria-controls="mobile-console-navigation" @click="isDesktop ? appStore.toggleSidebar() : appStore.toggleMobileSidebar()">
            <svg width="24" height="24" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="1.75" aria-hidden="true"><rect x="3" y="4" width="18" height="16" rx="3" /><path d="M9 4v16m-3-9h.01M6 14h.01" /></svg>
          </button>
          <router-link to="/" class="top-brand" :title="appStore.siteName">
            <img :src="siteLogo || '/logo.svg'" alt="" class="top-brand-logo" />
            <span>{{ appStore.siteName }}</span>
          </router-link>
        </div>
        <UserTopNav />
        <div class="top-account-area">
          <ConsoleNavigationSearch expanded />
          <SubscriptionProgressMini class="header-subscriptions" />
          <ThemeControls />
          <AnnouncementBell class="top-announcements" />
          <LocaleSwitcher compact class="top-locale" />
          <AccountMenu compact utilities />
        </div>
      </div>
    </header>
  </div>
</template>

<script setup lang="ts">
import { computed, onBeforeUnmount, onMounted, ref, watch } from 'vue'
import { useI18n } from 'vue-i18n'
import { useAppStore } from '@/stores'
import { useMediaQuery } from '@vueuse/core'
import AnnouncementBell from '@/components/common/AnnouncementBell.vue'
import LocaleSwitcher from '@/components/common/LocaleSwitcher.vue'
import SubscriptionProgressMini from '@/components/common/SubscriptionProgressMini.vue'
import ConsoleNavigationSearch from './ConsoleNavigationSearch.vue'
import ThemeControls from './ThemeControls.vue'
import { sanitizeUrl } from '@/utils/url'
import AccountMenu from './AccountMenu.vue'
import UserTopNav from './UserTopNav.vue'

const props = defineProps<{ browse?: boolean }>()
const { t } = useI18n()
const appStore = useAppStore()
const isDesktop = useMediaQuery('(min-width: 1024px)')
const compact = ref(false)
const scrolled = ref(false)
const siteLogo = computed(() => sanitizeUrl(appStore.siteLogo || '', { allowRelative: true, allowDataUrl: true }))
let scrollFrame = 0
function updateScrollState() {
  scrolled.value = window.scrollY > 8
  // Separate entry/exit thresholds prevent flicker near the transition.
  compact.value = Boolean(props.browse) && window.scrollY >= (compact.value ? 40 : 88)
}
function updateScroll() {
  if (scrollFrame) return
  scrollFrame = requestAnimationFrame(() => {
    scrollFrame = 0
    updateScrollState()
  })
}
watch(() => props.browse, updateScrollState)
onMounted(() => {
  updateScrollState()
  window.addEventListener('scroll', updateScroll, { passive: true })
})
onBeforeUnmount(() => {
  window.removeEventListener('scroll', updateScroll)
  cancelAnimationFrame(scrollFrame)
})
</script>
