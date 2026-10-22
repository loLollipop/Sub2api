<template>
  <div
    class="min-h-screen top-layout"
    :class="[isConsoleSignal ? 'console-signal' : 'bg-gray-50 dark:bg-dark-950', { 'signal-admin': isAdminSignal, 'signal-nav-collapsed': sidebarCollapsed, 'consumer-layout': !isAdmin, 'browse-layout': isBrowse }]"
  >
    <!-- Background Decoration -->
    <div v-if="!isConsoleSignal" class="pointer-events-none fixed inset-0 bg-mesh-gradient"></div>

    <AppHeader :browse="isBrowse" :inert="mobileDrawerOpen || undefined" />

    <div class="shell-workspace">
      <AppSidebar />
      <div class="relative min-w-0 flex-1" :class="{ 'signal-frame': isConsoleSignal }" :inert="mobileDrawerOpen || undefined">
        <main class="p-4 md:p-6 lg:p-8" :class="{ 'signal-main': isConsoleSignal }">
          <slot />
        </main>
      </div>
    </div>
  </div>
</template>

<script setup lang="ts">
import '@/styles/onboarding.css'
import '@/styles/console-signal.css'
import '@/styles/console-account.css'
import '@/styles/console-studio.css'
import '@/styles/console-consumer.css'
import '@/styles/console-components.css'
import '@/styles/console-profile.css'
import '@/styles/console-workflows.css'
import { computed, nextTick, onMounted, onUnmounted } from 'vue'
import { useMediaQuery } from '@vueuse/core'
import { useRoute, useRouter } from 'vue-router'
import { useAppStore } from '@/stores'
import { useAuthStore } from '@/stores/auth'
import { useOnboardingTour } from '@/composables/useOnboardingTour'
import { useOnboardingStore } from '@/stores/onboarding'
import { useConsoleSignal } from '@/composables/useConsoleSignal'
import { useConsoleWorkspace } from '@/composables/useConsoleWorkspace'
import AppSidebar from './AppSidebar.vue'
import AppHeader from './AppHeader.vue'

const appStore = useAppStore()
const authStore = useAuthStore()
const isDesktop = useMediaQuery('(min-width: 1024px)')
const mobileDrawerOpen = computed(() => appStore.mobileOpen && !isDesktop.value)
const route = useRoute()
const router = useRouter()
const isBrowse = computed(() => ['/model-plaza', '/available-channels', '/monitor'].includes(route.path))
const { isConsoleSignal, isAdminSignal } = useConsoleSignal()
const sidebarCollapsed = computed(() => appStore.sidebarCollapsed)
const { isAdminWorkspace: isAdmin } = useConsoleWorkspace()

const { replayTour: startReplay } = useOnboardingTour({
  storageKey: authStore.isAdmin ? 'admin_guide' : 'user_guide',
  autoStart: () => !isBrowse.value && isAdmin.value
})

const onboardingStore = useOnboardingStore()
let replayPending = false
let unmounted = false

const replayTour = async () => {
  if (unmounted || replayPending) return
  if (!isBrowse.value && (!authStore.isAdmin || isAdmin.value)) {
    startReplay()
    return
  }

  replayPending = true
  const destination = authStore.isAdmin ? '/admin/dashboard' : '/dashboard'
  try {
    const failure = await router.push(destination)
    await nextTick()
    if (!failure && route.path === destination) {
      // Navigation may replace this layout. Use the callback of the mounted layout.
      replayPending = false
      onboardingStore.replay()
    }
  } catch {
    // A failed navigation must not start a tour against hidden browse targets.
  } finally {
    replayPending = false
  }
}

onMounted(() => {
  onboardingStore.setReplayCallback(replayTour)
})

onUnmounted(() => {
  unmounted = true
  onboardingStore.setReplayCallback(null)
})

defineExpose({ replayTour })
</script>
