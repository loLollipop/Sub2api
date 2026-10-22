<template>
  <AppLayout>
    <div class="space-y-6">
      <header class="page-header">
        <h1 class="page-title text-xl font-semibold text-gray-900 dark:text-white">{{ t('admin.affiliates.title') }}</h1>
        <p class="page-description mt-2 text-sm text-gray-500 dark:text-dark-400">{{ t('admin.affiliates.description') }}</p>
      </header>

      <ConsoleTabs
        id="admin-affiliates"
        :label="t('admin.affiliates.title')"
        :model-value="activeTab"
        :items="tabs"
        @update:model-value="selectTab"
      />

      <section
        :id="`admin-affiliates-panel-${activeTab}`"
        class="affiliates-panel"
        role="tabpanel"
        :aria-labelledby="`admin-affiliates-tab-${activeTab}`"
        tabindex="0"
      >
        <AdminAffiliateRecordsTable :key="activeTab" :type="activeTab" />
      </section>
    </div>
  </AppLayout>
</template>

<script setup lang="ts">
import { computed } from 'vue'
import { useI18n } from 'vue-i18n'
import { useRoute, useRouter } from 'vue-router'
import ConsoleTabs from '@/components/common/ConsoleTabs.vue'
import AppLayout from '@/components/layout/AppLayout.vue'
import AdminAffiliateRecordsTable from './AdminAffiliateRecordsTable.vue'

type AffiliateTab = 'invites' | 'rebates' | 'transfers'

const { t } = useI18n()
const route = useRoute()
const router = useRouter()
const tabs = computed(() => [
  { key: 'invites', label: t('nav.affiliateInviteRecords') },
  { key: 'rebates', label: t('nav.affiliateRebateRecords') },
  { key: 'transfers', label: t('nav.affiliateTransferRecords') },
])
const activeTab = computed<AffiliateTab>(() => {
  const tab = route.query.tab
  return tab === 'rebates' || tab === 'transfers' ? tab : 'invites'
})

function selectTab(tab: string) {
  if (tab === activeTab.value || !tabs.value.some(item => item.key === tab)) return
  void router.push({
    path: route.path,
    query: { ...route.query, tab: tab === 'invites' ? undefined : tab },
    hash: route.hash,
  })
}
</script>

<style scoped>
.affiliates-panel :deep(.table-page-layout) {
  height: auto;
  min-height: 26rem;
}

@media (min-width: 1024px) {
  .affiliates-panel :deep(.table-page-layout) {
    height: calc(100dvh - 19rem);
  }
}
</style>
