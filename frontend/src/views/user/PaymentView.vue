<template>
  <AppLayout>
    <div class="signal-payment signal-payment--shop mx-auto w-full max-w-5xl" data-payment-mode="external" :aria-busy="loading">
      <div v-if="loading" class="signal-empty-state" role="status">{{ t('common.loading') }}</div>
      <div v-else-if="settingsFailed" class="signal-empty-state" role="alert">
        <p>{{ t('purchase.loadFailed') }}</p>
        <button type="button" class="btn btn-secondary mt-4" @click="loadSettings(true)">{{ t('common.tryAgain') }}</button>
      </div>
      <ExternalShopRecharge v-else-if="externalShopEnabled"
        :products="appStore.cachedPublicSettings?.purchase_subscription_products || {}"
        :username="authStore.user?.username || ''" :balance="authStore.user?.balance || 0" />
      <div v-else class="signal-empty-state" role="status">
        <h1>{{ t('purchase.notEnabledTitle') }}</h1>
        <p>{{ t('purchase.notEnabledDesc') }}</p>
      </div>
    </div>
  </AppLayout>
</template>

<script setup lang="ts">
import { computed, onMounted, ref } from 'vue'
import { useI18n } from 'vue-i18n'
import { useAppStore } from '@/stores/app'
import { useAuthStore } from '@/stores/auth'
import AppLayout from '@/components/layout/AppLayout.vue'
import ExternalShopRecharge from '@/components/payment/ExternalShopRecharge.vue'

const { t } = useI18n()
const appStore = useAppStore()
const authStore = useAuthStore()
const loading = ref(!appStore.publicSettingsLoaded)
const settingsFailed = ref(false)
const externalShopEnabled = computed(() => appStore.cachedPublicSettings?.purchase_subscription_enabled === true)

async function loadSettings(force = false) {
  loading.value = true
  settingsFailed.value = false
  try {
    if (force || !appStore.publicSettingsLoaded) await appStore.fetchPublicSettings(force)
    settingsFailed.value = !appStore.publicSettingsLoaded
  } catch {
    settingsFailed.value = true
  } finally {
    loading.value = false
  }
}

onMounted(() => loadSettings())
</script>

<style scoped>
.signal-payment {
  --payment-bg: var(--signal-bg, #f2f2f7);
  --payment-surface: var(--signal-surface, #ffffff);
  --payment-line: var(--signal-line, #dedee4);
  --payment-text: var(--signal-text, #1c1c1e);
  --payment-muted: var(--signal-muted, #63636c);
  --payment-accent: var(--signal-accent, #0066d6);
  --payment-accent-soft: var(--signal-accent-soft, #e3efff);
  --payment-amber: var(--signal-amber, #a86610);
  color: var(--payment-text);
}
.signal-empty-state { padding: 80px 24px; text-align: center; color: var(--payment-muted); }
.signal-empty-state h1 { margin-bottom: 12px; color: var(--payment-text); font-size: 20px; font-weight: 600; }
@media (min-width: 1024px) {
  .signal-payment--shop { min-height: calc(100dvh - var(--signal-shell-header-height, 64px) - 48px); }
}
</style>
