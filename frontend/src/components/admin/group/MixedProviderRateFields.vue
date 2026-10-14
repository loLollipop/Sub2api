<template>
  <fieldset class="space-y-3">
    <legend class="input-label">{{ t('common.mixedProviderRates') }}</legend>
    <p class="input-hint">{{ t('common.mixedProviderRatesHint') }}</p>
    <div class="grid grid-cols-2 gap-3 sm:grid-cols-3">
      <label v-for="vendor in mixedModelVendors" :key="vendor" class="space-y-1 text-sm text-gray-700 dark:text-gray-300">
        <span>{{ modelVendorLabel(vendor, t('common.mixedVendorOther')) }}</span>
        <input :value="modelValue[vendor]" type="number" min="0" max="999999.9999" step="any" required class="input" :aria-label="modelVendorLabel(vendor, t('common.mixedVendorOther'))" @input="update(vendor, $event)" />
      </label>
    </div>
  </fieldset>
</template>

<script setup lang="ts">
import { useI18n } from 'vue-i18n'
import { mixedModelVendors, modelVendorLabel, type MixedModelVendor, type MixedProviderRates } from '@/utils/mixed-provider-rates'
const props = defineProps<{ modelValue: MixedProviderRates }>()
const emit = defineEmits<{ 'update:modelValue': [value: MixedProviderRates] }>()
const { t } = useI18n()
function update(vendor: MixedModelVendor, event: Event) {
  emit('update:modelValue', { ...props.modelValue, [vendor]: (event.target as HTMLInputElement).valueAsNumber })
}
</script>
