<template>
  <div class="fixed inset-0 z-50 overflow-y-auto" @click.self="$emit('close')">
    <div class="flex min-h-full items-center justify-center p-4">
      <div class="fixed inset-0 bg-black/50 transition-opacity" @click="$emit('close')"></div>

      <div class="relative w-full max-w-md transform rounded-xl bg-white p-6 shadow-xl transition-all dark:bg-dark-800">
        <!-- Header -->
        <div class="mb-6">
          <div class="mx-auto flex h-12 w-12 items-center justify-center rounded-full bg-red-100 dark:bg-red-900/30">
            <Icon name="exclamationTriangleRounded" size="none" class="h-6 w-6 text-red-600 dark:text-red-400" :stroke-width="1.5" />
          </div>
          <h3 class="mt-4 text-center text-xl font-semibold text-gray-900 dark:text-white">
            {{ t('profile.totp.disableTitle') }}
          </h3>
          <p class="mt-2 text-center text-sm text-gray-500 dark:text-gray-400">
            {{ t('profile.totp.disableWarning') }}
          </p>
        </div>

        <!-- Loading verification method -->
        <div v-if="methodLoading" class="flex items-center justify-center py-8">
          <div class="animate-spin rounded-full h-8 w-8 border-b-2 border-primary-500"></div>
        </div>

        <form v-else @submit.prevent="handleDisable" class="space-y-4">
          <div v-if="verificationMethod === 'totp'">
            <label for="totp-disable-code" class="input-label">{{ t('profile.totp.authenticatorCode') }}</label>
            <input
              id="totp-disable-code"
              v-model="form.totpCode"
              type="text"
              inputmode="numeric"
              autocomplete="one-time-code"
              maxlength="6"
              pattern="[0-9]{6}"
              class="input"
              data-testid="totp-disable-code"
              :placeholder="t('profile.totp.enterCode')"
            />
            <p class="mt-2 text-sm text-gray-500 dark:text-gray-400">{{ t('profile.totp.disableTotpHint') }}</p>
          </div>

          <!-- Actions -->
          <div class="flex justify-end gap-3 pt-4">
            <button type="button" class="btn btn-secondary" @click="$emit('close')">
              {{ t('common.cancel') }}
            </button>
            <button
              type="submit"
              class="btn btn-danger"
              :disabled="loading || !canSubmit"
            >
              {{ loading ? t('common.processing') : t('profile.totp.confirmDisable') }}
            </button>
          </div>
        </form>
      </div>
    </div>
  </div>
</template>

<script setup lang="ts">
import Icon from '@/components/icons/Icon.vue'
import { ref, onMounted, computed } from 'vue'
import { useI18n } from 'vue-i18n'
import { useAppStore } from '@/stores/app'
import { totpAPI } from '@/api'
import { extractApiErrorMessage } from '@/utils/apiError'
import type { TotpVerificationMethod } from '@/types'

const emit = defineEmits<{
  close: []
  success: []
}>()

const { t } = useI18n()
const appStore = useAppStore()

const methodLoading = ref(true)
const verificationMethod = ref<TotpVerificationMethod['method'] | null>(null)
const loading = ref(false)
const form = ref({ totpCode: '' })

const canSubmit = computed(() => {
  if (methodLoading.value) return false
  if (verificationMethod.value === 'totp') {
    return /^\d{6}$/.test(form.value.totpCode.trim())
  }
  return false
})

const loadVerificationMethod = async () => {
  methodLoading.value = true
  try {
    const method = await totpAPI.getVerificationMethod('disable')
    if (method.method !== 'totp') throw new Error(t('profile.totp.disableFailed'))
    verificationMethod.value = method.method
  } catch (err: unknown) {
    appStore.showError(extractApiErrorMessage(err, t('common.error')))
    emit('close')
  } finally {
    methodLoading.value = false
  }
}

const handleDisable = async () => {
  if (!canSubmit.value || loading.value) return

  loading.value = true

  try {
    await totpAPI.disable({ totp_code: form.value.totpCode.trim() })
    appStore.showSuccess(t('profile.totp.disableSuccess'))
    emit('success')
  } catch (err: unknown) {
    appStore.showError(extractApiErrorMessage(err, t('profile.totp.disableFailed')))
  } finally {
    form.value.totpCode = ''
    loading.value = false
  }
}

onMounted(() => {
  loadVerificationMethod()
})

</script>
