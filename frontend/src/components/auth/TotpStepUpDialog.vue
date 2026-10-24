<template>
  <BaseDialog
    :show="controller.visible.value"
    :z-index="dialogZIndex"
    :title="t('stepUp.title')"
    width="narrow"
    :show-close-button="false"
    :close-on-escape="!verifying"
    :close-on-click-outside="!verifying"
    @close="handleCancel"
  >
    <div class="mb-6 text-center">
      <div class="mx-auto flex h-12 w-12 items-center justify-center rounded-full bg-primary-100 dark:bg-primary-900/30">
        <Icon name="lock" size="none" class="h-6 w-6 text-primary-600 dark:text-primary-400" :stroke-width="1.5" />
      </div>
      <p class="mt-2 text-sm text-gray-500 dark:text-gray-400">
        {{ t('stepUp.hint') }}
      </p>
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

    <button
      type="button"
      class="btn btn-secondary w-full"
      :disabled="verifying"
      @click="handleCancel"
    >
      {{ t('common.cancel') }}
    </button>
  </BaseDialog>
</template>

<script setup lang="ts">
import { ref, watch, nextTick, onBeforeUnmount } from 'vue'
import Icon from '@/components/icons/Icon.vue'
import { nextDialogZIndex } from '@/components/common/dialogStack'
import { extractApiErrorMessage } from '@/utils/apiError'
import { useI18n } from 'vue-i18n'
import { useAppStore } from '@/stores'
import { totpAPI } from '@/api'
import TotpCodeInput from '@/components/common/TotpCodeInput.vue'
import BaseDialog from '@/components/common/BaseDialog.vue'
import type { StepUpController } from '@/composables/useStepUp'

const props = defineProps<{
  controller: StepUpController
}>()

const { t } = useI18n()
const appStore = useAppStore()

const verifying = ref(false)
const dialogZIndex = ref(60)
let requestGeneration = 0
const code = ref('')
const codeInput = ref<InstanceType<typeof TotpCodeInput> | null>(null)

// Focus the first cell whenever the dialog opens.
watch(
  () => props.controller.visible.value,
  (open) => {
    ++requestGeneration
    verifying.value = false
    resetInputs()
    if (open) dialogZIndex.value = nextDialogZIndex(60)
  },
  { immediate: true, flush: 'sync' }
)

async function submit(otp: string) {
  if (!props.controller.visible.value || verifying.value || !/^[0-9]{6}$/.test(otp)) return
  const generation = requestGeneration
  verifying.value = true
  try {
    await totpAPI.stepUp(otp)
    if (generation !== requestGeneration || !props.controller.visible.value) return
    verifying.value = false
    resetInputs()
    props.controller.onVerified()
  } catch (err: unknown) {
    if (generation !== requestGeneration || !props.controller.visible.value) return
    verifying.value = false
    appStore.showError(extractApiErrorMessage(err, t('stepUp.verifyFailed')))
    resetInputs()
    nextTick(() => {
      if (generation === requestGeneration && props.controller.visible.value) codeInput.value?.focus()
    })
  }
}

function resetInputs() {
  code.value = ''
}

function handleCancel() {
  if (verifying.value) return
  props.controller.onCancel()
}

onBeforeUnmount(() => { ++requestGeneration })
</script>
