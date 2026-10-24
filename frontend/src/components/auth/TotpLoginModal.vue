<template>
  <BaseDialog
    :show="true"
    :title="t('profile.totp.loginTitle')"
    width="narrow"
    :show-close-button="false"
    :close-on-escape="!verifying"
    @close="emit('cancel')"
  >
    <div class="mb-6 text-center">
      <div class="mx-auto flex h-12 w-12 items-center justify-center rounded-full bg-primary-100 dark:bg-primary-900/30">
        <Icon name="shield" size="none" class="h-6 w-6 text-primary-600 dark:text-primary-400" :stroke-width="1.5" />
      </div>
      <p class="mt-2 text-sm text-gray-500 dark:text-gray-400">{{ t('profile.totp.loginHint') }}</p>
      <p v-if="userEmailMasked" class="mt-1 text-sm font-medium text-gray-700 dark:text-gray-300">{{ userEmailMasked }}</p>
    </div>
    <div class="mb-6">
      <TotpCodeInput
        ref="codeInput"
        v-model="code"
        :label="t('profile.totp.enterCode')"
        :disabled="verifying"
        @complete="submit"
      />
      <div v-if="verifying" class="mt-3 flex items-center justify-center gap-2 text-sm text-gray-500">
        <div class="animate-spin rounded-full h-4 w-4 border-b-2 border-primary-500"></div>
        {{ t('common.verifying') }}
      </div>
    </div>
    <button type="button" class="btn btn-secondary w-full" :disabled="verifying" @click="emit('cancel')">
      {{ t('common.cancel') }}
    </button>
  </BaseDialog>
</template>

<script setup lang="ts">
import Icon from '@/components/icons/Icon.vue'
import BaseDialog from '@/components/common/BaseDialog.vue'
import TotpCodeInput from '@/components/common/TotpCodeInput.vue'
import { ref, nextTick } from 'vue'
import { useI18n } from 'vue-i18n'
import { useAppStore } from '@/stores'

defineProps<{ tempToken: string; userEmailMasked?: string }>()
const emit = defineEmits<{ verify: [code: string]; cancel: [] }>()
const { t } = useI18n()
const appStore = useAppStore()
const verifying = ref(false)
const code = ref('')
const codeInput = ref<InstanceType<typeof TotpCodeInput> | null>(null)

function submit(otp: string) {
  if (verifying.value || !/^[0-9]{6}$/.test(otp)) return
  verifying.value = true
  emit('verify', otp)
}

defineExpose({
  setVerifying: (value: boolean) => { verifying.value = value },
  setError: (message: string) => {
    verifying.value = false
    if (message) appStore.showError(message)
    code.value = ''
    nextTick(() => codeInput.value?.focus())
  }
})
</script>
