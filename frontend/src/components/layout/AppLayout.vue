<template>
  <div
    class="min-h-screen"
    :class="[isConsoleSignal ? 'console-signal console-local' : 'bg-gray-50 dark:bg-dark-950', { 'signal-admin': isAdminSignal }]"
  >
    <AppHeader local-console />
    <AppSidebar horizontal />

    <div class="signal-frame local-frame">
      <main id="console-main" class="signal-main local-main p-4 md:p-6 lg:p-8">
        <slot />
      </main>
    </div>
  </div>
</template>

<script setup lang="ts">
import '@/styles/onboarding.css'
import '@/styles/console-signal.css'
import '@/styles/console-account.css'
import '@/styles/console-studio.css'
import '@/styles/console-local.css'
import { computed, onMounted } from 'vue'
import { useAuthStore } from '@/stores/auth'
import { useOnboardingTour } from '@/composables/useOnboardingTour'
import { useOnboardingStore } from '@/stores/onboarding'
import { useConsoleSignal } from '@/composables/useConsoleSignal'
import AppSidebar from './AppSidebar.vue'
import AppHeader from './AppHeader.vue'

const authStore = useAuthStore()
const { isConsoleSignal, isAdminSignal } = useConsoleSignal()
const isAdmin = computed(() => authStore.user?.role === 'admin')

const { replayTour } = useOnboardingTour({
  storageKey: isAdmin.value ? 'admin_guide' : 'user_guide',
  autoStart: true
})

const onboardingStore = useOnboardingStore()

onMounted(() => {
  onboardingStore.setReplayCallback(replayTour)
})

defineExpose({ replayTour })
</script>
