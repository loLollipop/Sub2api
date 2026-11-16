<template>
  <BaseDialog
    :show="true"
    :title="t('keys.bulkEdit.title', { count: selectedIds.length })"
    width="normal"
    :show-close-button="!saving"
    :close-on-escape="!saving"
    :close-on-click-outside="!saving"
    @close="close"
  >
    <template #description>{{ t('keys.bulkEdit.hint') }}</template>
    <form id="bulk-edit-keys-form" class="space-y-4" @submit.prevent="apply">
      <fieldset :disabled="saving" class="min-w-0 space-y-4">
        <section>
          <label class="flex items-center gap-2">
            <input v-model="enabled.group" type="checkbox" data-test="enable-group" />
            {{ t('keys.bulkEdit.fields.group') }}
          </label>
          <div v-if="enabled.group" class="mt-2 space-y-2">
            <Select v-model="form.groupId" :options="groupOptions" :disabled="saving" :aria-label="t('keys.bulkEdit.fields.group')" data-test="bulk-edit-group" />
            <p class="text-xs text-gray-500 dark:text-dark-400">{{ t('keys.bulkEdit.groupHint') }}</p>
          </div>
        </section>
        <section>
          <label class="flex items-center gap-2">
            <input v-model="enabled.status" type="checkbox" data-test="enable-status" />
            {{ t('keys.bulkEdit.fields.status') }}
          </label>
          <Select v-if="enabled.status" v-model="form.status" :options="statusOptions" :disabled="saving" :aria-label="t('keys.bulkEdit.fields.status')" class="mt-2" data-test="bulk-edit-status" />
        </section>
        <section v-for="field in numericFields" :key="field.key">
          <label :for="`enable-${field.key}`" class="flex items-center gap-2">
            <input :id="`enable-${field.key}`" v-model="enabled[field.key]" type="checkbox" :data-test="`enable-${field.key}`" />
            {{ t(field.label) }}
          </label>
          <input
            v-if="enabled[field.key]"
            v-model="form[field.key]"
            type="number" min="0" step="0.01"
            :aria-label="t(field.label)"
            :placeholder="t('keys.bulkEdit.limitPlaceholder')"
            class="input mt-2 w-full"
            :data-test="`bulk-edit-${field.key}`"
          />
        </section>
        <section>
          <label class="flex items-center gap-2">
            <input v-model="enabled.expiresAt" type="checkbox" data-test="enable-expiresAt" />
            {{ t('keys.bulkEdit.fields.expiresAt') }}
          </label>
          <div v-if="enabled.expiresAt" class="mt-2 space-y-2">
            <input v-model="form.expiresAt" type="datetime-local" :aria-label="t('keys.bulkEdit.fields.expiresAt')" class="input w-full min-w-0" data-test="bulk-edit-expires" />
            <p class="text-xs text-gray-500 dark:text-dark-400">{{ t('keys.bulkEdit.expiresHint') }}</p>
          </div>
        </section>
        <section>
          <label class="flex items-center gap-2">
            <input v-model="enabled.ipWhitelist" type="checkbox" data-test="enable-ipWhitelist" />
            {{ t('keys.bulkEdit.fields.ipWhitelist') }}
          </label>
          <textarea v-if="enabled.ipWhitelist" v-model="form.ipWhitelist" :aria-label="t('keys.bulkEdit.fields.ipWhitelist')" :placeholder="t('keys.bulkEdit.ipPlaceholder')" rows="3" class="input mt-2 w-full" data-test="bulk-edit-ip" />
        </section>
      </fieldset>
      <p v-if="errorMessage" role="alert" class="text-sm text-red-600 dark:text-red-400" data-test="bulk-edit-error">{{ errorMessage }}</p>
    </form>
    <template #footer>
      <button type="button" class="btn btn-secondary" :disabled="saving" @click="close">{{ t('common.cancel') }}</button>
      <button type="submit" form="bulk-edit-keys-form" class="btn btn-primary" :disabled="saving || !hasSelection || selectedIds.length === 0" data-test="bulk-edit-apply">
        {{ saving ? t('common.saving') : t('keys.bulkEdit.apply', { count: selectedIds.length }) }}
      </button>
    </template>
  </BaseDialog>
</template>

<script setup lang="ts">
import { computed, reactive, ref } from 'vue'
import { useI18n } from 'vue-i18n'
import { keysAPI } from '@/api'
import BaseDialog from '@/components/common/BaseDialog.vue'
import Select from '@/components/common/Select.vue'
import type { UpdateApiKeyRequest } from '@/types'

const props = defineProps<{
  selectedIds: number[]
  groupOptions: { value: number; label: string }[]
}>()
const emit = defineEmits<{
  close: []
  applied: [result: { updated: number; failed: number }]
}>()
const { t } = useI18n()
const saving = ref(false)
const errorMessage = ref('')
const enabled = reactive({
  group: false, status: false, quota: false, rateLimit5h: false,
  rateLimit1d: false, rateLimit7d: false, expiresAt: false, ipWhitelist: false
})
const form = reactive({
  groupId: '' as number | string | boolean | null,
  status: 'active' as string | number | boolean | null,
  quota: '', rateLimit5h: '', rateLimit1d: '', rateLimit7d: '', expiresAt: '', ipWhitelist: ''
})
const numericFields = [
  { key: 'quota', field: 'quota', label: 'keys.bulkEdit.fields.quota' },
  { key: 'rateLimit5h', field: 'rate_limit_5h', label: 'keys.bulkEdit.fields.rateLimit5h' },
  { key: 'rateLimit1d', field: 'rate_limit_1d', label: 'keys.bulkEdit.fields.rateLimit1d' },
  { key: 'rateLimit7d', field: 'rate_limit_7d', label: 'keys.bulkEdit.fields.rateLimit7d' }
] as const
const statusOptions = computed(() => [
  { value: 'active', label: t('keys.bulkEdit.statusActive') },
  { value: 'inactive', label: t('keys.bulkEdit.statusInactive') }
])
const hasSelection = computed(() => Object.values(enabled).some(Boolean))

function close() {
  if (!saving.value) emit('close')
}

async function apply() {
  if (saving.value || props.selectedIds.length === 0) return
  const payload: UpdateApiKeyRequest = {}
  if (enabled.group) {
    if (typeof form.groupId !== 'number' || !props.groupOptions.some(g => g.value === form.groupId)) {
      errorMessage.value = t('keys.bulkEdit.invalidGroup')
      return
    }
    // Explicitly replace the route set, including smart-routing keys.
    payload.group_ids = [form.groupId]
  }
  if (enabled.status && (form.status === 'active' || form.status === 'inactive')) payload.status = form.status
  for (const field of numericFields) {
    if (!enabled[field.key]) continue
    const value = Number(form[field.key])
    if (String(form[field.key]).trim() === '' || !Number.isFinite(value) || value < 0) {
      errorMessage.value = t('keys.bulkEdit.invalidNumber')
      return
    }
    payload[field.field] = value
  }
  if (enabled.expiresAt) {
    const date = new Date(form.expiresAt)
    if (form.expiresAt && (!Number.isFinite(date.getTime()) || date.getTime() <= Date.now())) {
      errorMessage.value = t('keys.bulkEdit.invalidExpiry')
      return
    }
    payload.expires_at = form.expiresAt ? date.toISOString() : ''
  }
  if (enabled.ipWhitelist) payload.ip_whitelist = form.ipWhitelist.split(/[\s,]+/).filter(Boolean)
  if (Object.keys(payload).length === 0) {
    errorMessage.value = t('keys.bulkEdit.emptySelection')
    return
  }
  const ids = [...new Set(props.selectedIds)]
  saving.value = true
  errorMessage.value = ''
  let updated = 0
  let failed = 0
  for (const id of ids) {
    try {
      await keysAPI.update(id, payload)
      updated++
    } catch {
      failed++
    }
  }
  saving.value = false
  emit('applied', { updated, failed })
}
</script>
