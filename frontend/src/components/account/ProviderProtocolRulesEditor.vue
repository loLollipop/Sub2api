<template>
  <div class="space-y-2" data-testid="provider-protocol-rules">
    <label class="input-label">{{ t('admin.accounts.providerRules.title') }}</label>
    <p class="input-hint">{{ t('admin.accounts.providerRules.hint') }}</p>
    <div v-for="(row, index) in modelValue" :key="index" class="flex flex-wrap items-center gap-2">
      <input :value="row.pattern" class="input min-w-32 flex-1" :aria-label="t('admin.accounts.providerRules.pattern')" @input="update(index, { pattern: ($event.target as HTMLInputElement).value })" />
      <select :value="row.protocol" class="input w-auto" :aria-label="t('admin.accounts.cnProviders.apiProtocol.title')" @change="update(index, { protocol: ($event.target as HTMLSelectElement).value as CnNativeApiProtocol })">
        <option v-for="protocol in protocols" :key="protocol" :value="protocol">{{ protocol }}</option>
      </select>
      <label v-for="protocol in protocols.filter(value => value !== row.protocol)" :key="protocol" class="flex items-center gap-1 text-xs">
        <input type="checkbox" :checked="row.protocols?.includes(protocol)" @change="toggle(index, protocol)" />
        {{ protocol }}
      </label>
      <button type="button" class="btn btn-secondary btn-sm" @click="emit('update:modelValue', modelValue.filter((_, i) => i !== index))">{{ t('common.delete') }}</button>
    </div>
    <div class="flex gap-2">
      <button type="button" class="btn btn-secondary btn-sm" @click="emit('update:modelValue', [...modelValue, { pattern: '', protocol: protocols[0] || 'chat_completions' }])">{{ t('common.add') }}</button>
      <button type="button" class="btn btn-secondary btn-sm" @click="emit('update:modelValue', providerProtocolRules(platform, undefined, mode))">{{ t('admin.accounts.providerRules.restore') }}</button>
    </div>
  </div>
</template>

<script setup lang="ts">
import { computed } from 'vue'
import { useI18n } from 'vue-i18n'
import { providerNativeProtocols, providerProtocolRules, type CnNativeApiProtocol, type ProviderProtocolRuleRow } from './credentialsBuilder'
const props = defineProps<{ modelValue: ProviderProtocolRuleRow[]; platform: string; mode?: string }>()
const emit = defineEmits<{ 'update:modelValue': [value: ProviderProtocolRuleRow[]] }>()
const { t } = useI18n()
const protocols = computed(() => providerNativeProtocols(props.platform, props.mode))
function update(index: number, patch: Partial<ProviderProtocolRuleRow>) {
  emit('update:modelValue', props.modelValue.map((row, i) => i === index ? { ...row, ...patch } : row))
}
function toggle(index: number, protocol: CnNativeApiProtocol) {
  const row = props.modelValue[index]
  if (!row) return
  const current = row.protocols ?? [row.protocol]
  const values = current.includes(protocol) ? current.filter(value => value !== protocol) : [...current, protocol]
  update(index, { protocols: [...new Set([row.protocol, ...values])] })
}
</script>
