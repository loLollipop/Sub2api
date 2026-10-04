<template>
  <div class="local-auth relative flex min-h-screen items-center justify-center overflow-hidden p-4">
    <!-- Background -->
    <div
      class="absolute inset-0 bg-[#f8f9fb] dark:bg-[#10141c]"
    ></div>

    <!-- Content Container -->
    <div class="relative z-10 w-full max-w-md">
      <!-- Logo/Brand -->
      <div class="mb-8 text-center">
        <!-- Custom Logo or Default Logo -->
        <template v-if="settingsLoaded">
          <div
            class="mb-4 inline-flex h-14 w-14 items-center justify-center overflow-hidden rounded-xl border border-gray-200 bg-white dark:border-dark-700 dark:bg-dark-800"
          >
            <img :src="siteLogo || '/logo.svg'" alt="Logo" class="h-full w-full object-contain" />
          </div>
          <h1 class="mb-2 text-2xl font-semibold tracking-tight text-gray-950 dark:text-white">
            {{ siteName }}
          </h1>
          <p class="text-sm text-gray-500 dark:text-dark-400">
            {{ siteSubtitle }}
          </p>
        </template>
      </div>

      <!-- Card Container -->
      <div class="rounded-xl border border-gray-200 bg-white p-6 shadow-sm dark:border-dark-700 dark:bg-dark-900 sm:p-8">
        <slot />
      </div>

      <!-- Footer Links -->
      <div class="mt-6 text-center text-sm">
        <slot name="footer" />
      </div>

      <div class="mt-5 text-gray-400 dark:text-dark-500">
        <LegalFooterLinks />
      </div>

      <!-- Copyright -->
      <div class="mt-8 text-center text-xs text-gray-400 dark:text-dark-500">
        &copy; {{ currentYear }} {{ siteName }}. All rights reserved.
      </div>
    </div>
  </div>
</template>

<script setup lang="ts">
import '@/styles/console-local.css'
import { computed, onMounted } from 'vue'
import { useAppStore } from '@/stores'
import LegalFooterLinks from '@/components/legal/LegalFooterLinks.vue'
import { sanitizeUrl } from '@/utils/url'

const appStore = useAppStore()

const siteName = computed(() => appStore.siteName || 'Sub2API')
const siteLogo = computed(() => sanitizeUrl(appStore.siteLogo || '', { allowRelative: true, allowDataUrl: true }))
const siteSubtitle = computed(() => appStore.cachedPublicSettings?.site_subtitle || 'Subscription to API Conversion Platform')
const settingsLoaded = computed(() => appStore.publicSettingsLoaded)

const currentYear = computed(() => new Date().getFullYear())

onMounted(() => {
  appStore.fetchPublicSettings()
})
</script>

<style scoped>
.text-gradient {
  @apply bg-gradient-to-r from-primary-600 to-primary-500 bg-clip-text text-transparent;
}

.auth-grid {
  background-image:
    linear-gradient(rgba(58, 83, 117, 0.07) 1px, transparent 1px),
    linear-gradient(90deg, rgba(58, 83, 117, 0.07) 1px, transparent 1px);
  background-size: 48px 48px;
  mask-image: radial-gradient(circle at center, black 0%, transparent 78%);
  animation: auth-grid-drift 28s linear infinite;
}

:global(html.dark) .auth-grid {
  background-image:
    linear-gradient(rgba(145, 169, 201, 0.11) 1px, transparent 1px),
    linear-gradient(90deg, rgba(145, 169, 201, 0.11) 1px, transparent 1px);
}

@keyframes auth-grid-drift {
  from { background-position: 0 0; }
  to { background-position: 48px 48px; }
}

@media (prefers-reduced-motion: reduce) {
  .auth-grid { animation: none; }
}
</style>
