<template>
  <nav class="signal-actions" :aria-label="t('dashboard.quickActions')">
    <div class="signal-actions-list" :class="{ 'signal-actions-list--five': showAffiliate && canUseBatchImage }">
      <RouterLink class="signal-action" to="/keys">
        <Icon class="signal-action-icon" name="key" size="sm" />
        <span class="signal-action-label">{{ t('dashboard.createApiKey') }}</span>
      </RouterLink>
      <RouterLink class="signal-action" to="/usage">
        <Icon class="signal-action-icon" name="chart" size="sm" />
        <span class="signal-action-label">{{ t('dashboard.viewUsage') }}</span>
      </RouterLink>
      <RouterLink v-if="showAffiliate" class="signal-action" to="/affiliate">
        <Icon class="signal-action-icon" name="users" size="sm" />
        <span class="signal-action-label">{{ t('dashboard.inviteRecords') }}</span>
      </RouterLink>
      <RouterLink class="signal-action" to="/redeem">
        <Icon class="signal-action-icon" name="gift" size="sm" />
        <span class="signal-action-label">{{ t('dashboard.redeemCode') }}</span>
      </RouterLink>
      <RouterLink v-if="canUseBatchImage" class="signal-action" to="/batch-image">
        <Icon class="signal-action-icon" name="sparkles" size="sm" />
        <span class="signal-action-label">{{ t('dashboard.batchImageAgent') }}</span>
      </RouterLink>
    </div>
  </nav>
</template>

<script setup lang="ts">
import { computed, onMounted } from 'vue'
import { RouterLink } from 'vue-router'
import { useI18n } from 'vue-i18n'
import Icon from '@/components/icons/Icon.vue'
import { useBatchImageAccess } from '@/composables/useBatchImageAccess'
import { useAuthStore } from '@/stores/auth'
import { useAppStore } from '@/stores/app'
import { FeatureFlags, isFeatureFlagEnabled } from '@/utils/featureFlags'
const { t } = useI18n()
const { canUseBatchImage, refreshBatchImageAccess } = useBatchImageAccess()
const authStore = useAuthStore()
const appStore = useAppStore()
const showAffiliate = computed(() => !authStore.isSimpleMode && !appStore.backendModeEnabled && isFeatureFlagEnabled(FeatureFlags.affiliate))

onMounted(() => {
  void refreshBatchImageAccess()
})
</script>

<style scoped>
.signal-actions { display: grid; gap: 10px; min-width: 0; }
.signal-actions-list { display: grid; grid-template-columns: repeat(4, minmax(0, 1fr)); gap: 12px; min-width: 0; }
.signal-actions-list--five { grid-template-columns: repeat(5, minmax(0, 1fr)); }
.signal-action { display: flex; align-items: center; gap: 12px; min-width: 0; min-height: 46px; padding: 12px; border: 1px solid var(--signal-line); border-radius: 10px; background: var(--signal-surface); color: var(--signal-text); text-align: left; transition: background-color 140ms ease; }
.signal-action-icon { width: 16px; height: 16px; flex: 0 0 16px; color: var(--signal-muted); }
.signal-action-label { min-width: 0; font-size: 13px; line-height: 20px; font-weight: 500; overflow: hidden; text-overflow: ellipsis; white-space: nowrap; }
.signal-action:hover { background: var(--signal-hover); }
.signal-action:focus-visible { outline: 2px solid var(--signal-accent); outline-offset: 3px; }
@media (max-width: 1023px) {
  .signal-actions-list { grid-template-columns: repeat(2, minmax(0, 1fr)); }
  .signal-action { gap: 8px; }
}
@media (prefers-reduced-motion: reduce) {
  .signal-action { transition: none; }
}
</style>
