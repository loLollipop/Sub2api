<template>
  <BaseDialog :show="show" :title="t('admin.accounts.editAccount')" width="wide" @close="$emit('close')">
    <div data-testid="edit-account-shell" class="space-y-5" aria-busy="true" :aria-label="t('admin.accounts.editLoading')">
      <div v-for="height in ['h-4 w-32', 'h-10 w-full', 'h-4 w-24', 'h-24 w-full', 'h-10 w-full']" :key="height" class="animate-pulse rounded bg-gray-100 motion-reduce:animate-none dark:bg-dark-700" :class="height" />
      <p class="text-sm text-gray-500 dark:text-gray-400">{{ t('admin.accounts.editLoading') }}</p>
    </div>
  </BaseDialog>
</template>

<script setup lang="ts">
import { useI18n } from 'vue-i18n'
import BaseDialog from '@/components/common/BaseDialog.vue'
defineProps<{ show: boolean }>()
defineEmits<{ close: [] }>()
const { t } = useI18n()
</script>
