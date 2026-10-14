<template>
  <span class="flex min-w-0 flex-wrap items-center gap-1.5 whitespace-normal text-xs" data-testid="mixed-provider-rates">
    <span v-for="entry in rates" :key="`${entry.vendor}:${entry.rate}`" class="rounded bg-black/5 px-1.5 py-0.5 dark:bg-white/10">
      {{ modelVendorLabel(entry.vendor, t('common.mixedVendorOther')) }} <span class="font-semibold tabular-nums">{{ entry.rate }}x</span>
    </span>
    <span v-if="rates.length === 0" class="text-gray-500 dark:text-gray-400">{{ t('common.mixedRatesNoModels') }}</span>
    <span v-if="userOverride" class="w-full text-gray-500 dark:text-gray-400">{{ t('common.mixedRatesUserOverride') }}</span>
  </span>
</template>

<script setup lang="ts">
import { useI18n } from 'vue-i18n'
import { modelVendorLabel, type VendorRate } from '@/utils/mixed-provider-rates'
defineProps<{ rates: VendorRate[]; userOverride?: boolean }>()
const { t } = useI18n()
</script>
