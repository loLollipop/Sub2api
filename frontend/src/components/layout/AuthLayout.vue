<template>
  <div class="auth-shell">
    <div class="auth-brand-block">
      <RouterLink v-if="settingsLoaded" to="/" class="auth-brand">
        <span class="auth-logo">
          <img :src="siteLogo || '/logo.svg'" alt="" class="h-full w-full object-contain" />
        </span>
        <h1>{{ siteName }}</h1>
      </RouterLink>
    </div>

    <div class="auth-content">
      <!-- Card Container -->
      <div class="auth-card">
        <slot />
      </div>

      <!-- Footer Links -->
      <div v-if="$slots.footer" class="auth-footer">
        <slot name="footer" />
      </div>

      <div class="auth-legal">
        <LegalFooterLinks />
      </div>

      <!-- Copyright -->
      <div class="auth-copyright">
        &copy; {{ currentYear }} {{ siteName }}. All rights reserved.
      </div>
    </div>
  </div>
</template>

<script setup lang="ts">
import { computed, onMounted } from 'vue'
import { useAppStore } from '@/stores'
import LegalFooterLinks from '@/components/legal/LegalFooterLinks.vue'
import { sanitizeUrl } from '@/utils/url'
import '@/styles/auth-shell.css'

const appStore = useAppStore()

const siteName = computed(() => appStore.siteName || 'Sub2API')
const siteLogo = computed(() => sanitizeUrl(appStore.siteLogo || '', { allowRelative: true, allowDataUrl: true }))
const settingsLoaded = computed(() => appStore.publicSettingsLoaded)

const currentYear = computed(() => new Date().getFullYear())

onMounted(() => {
  appStore.fetchPublicSettings()
})
</script>
