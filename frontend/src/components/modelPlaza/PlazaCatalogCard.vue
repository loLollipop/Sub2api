<template>
  <article class="plaza-catalog-card" :class="`plaza-catalog-card--${view}`" :data-model-key="entry.key">
    <header class="catalog-identity">
      <PlazaProviderIcon :platform="provider" size="lg" class="catalog-logo" />
      <div class="catalog-name">
        <h3>{{ entry.name }}</h3>
        <p class="catalog-muted">{{ providerName }} · {{ billingLabel }}</p>
      </div>
      <button
        type="button" class="catalog-copy" :aria-label="t(copied ? 'modelPlaza.list.copied' : 'modelPlaza.list.copyName')"
        :title="t(copied ? 'modelPlaza.list.copied' : 'modelPlaza.list.copyName')" @click="copyToClipboard(entry.name)"
      >
        <Icon :name="copied ? 'check' : 'copy'" size="sm" aria-hidden="true" />
      </button>
    </header>

    <div v-if="activeOffer" class="catalog-offer">
      <span v-if="!grouped" class="catalog-muted">{{ t('modelPlaza.list.groupLabel') }}</span>
      <Select
        v-if="entry.offers.length > 1" v-model="selectedGroup" :options="groupOptions"
        :aria-label="t('modelPlaza.list.groupLabel')" :searchable="false" class="catalog-group-select"
      />
      <span v-else-if="!grouped" class="catalog-group-name">{{ activeOffer.group.name }}</span>
      <span v-if="!isVideo" class="catalog-rate" :title="t('modelPlaza.table.rate')">
        <s v-if="hasCustomRate && !independentImage">{{ activeOffer.group.rate_multiplier }}x</s>
        {{ activeRate }}x
      </span>
    </div>

    <div class="catalog-pricing">
      <span v-if="activeOffer?.model.pricing && isToken" class="catalog-price-unit">{{ t('modelPlaza.table.unitPerMillion') }}</span>
      <p v-if="isVideo" class="catalog-muted">{{ t('modelPlaza.list.videoPricingUnavailable') }}</p>
      <dl v-else-if="activeOffer?.model.pricing" class="catalog-prices">
        <template v-if="isToken">
          <div>
            <dt>{{ t('modelPlaza.table.input') }}</dt>
            <dd data-price="input">{{ preview.input }}</dd>
          </div>
          <div>
            <dt>{{ t('modelPlaza.table.output') }}</dt>
            <dd data-price="output">{{ preview.output }}</dd>
          </div>
          <div v-if="preview.cacheRead != null" class="catalog-cache">
            <dt>{{ t('modelPlaza.list.cacheRead') }}</dt>
            <dd data-price="cache-read">{{ preview.cacheRead }}</dd>
          </div>
        </template>
        <div v-else class="catalog-request">
          <dt>{{ billingLabel }}</dt>
          <dd data-price="request">{{ preview.request }} <small>{{ perUnitSuffix(activeOffer.model) }}</small></dd>
        </div>
      </dl>
      <p v-else class="catalog-muted">{{ t('modelPlaza.list.noPricing') }}</p>
      <div v-if="preview.notes.length" class="catalog-price-notes">
        <span v-for="note in preview.notes" :key="note">{{ note }}</span>
      </div>
    </div>

    <footer class="catalog-footer">
      <span v-if="entry.offers.length > 1" class="catalog-muted">{{ t('modelPlaza.list.offers', { count: entry.offers.length }) }}</span>
      <button
        type="button" class="catalog-details" :disabled="!activeOffer"
        :aria-label="t('modelPlaza.list.viewPricing', { model: entry.name })" @click="detailsOpen = true"
      >
        {{ t('modelPlaza.list.details') }} <Icon name="chevronRight" size="xs" aria-hidden="true" />
      </button>
    </footer>

    <BaseDialog v-if="activeOffer" :show="detailsOpen" :title="entry.name" width="extra-wide" @close="detailsOpen = false">
      <div v-if="detailsOpen" class="catalog-dialog">
        <div class="catalog-dialog-heading">
          <span>{{ providerName }} · {{ billingLabel }}</span>
          <Select
            v-if="entry.offers.length > 1" v-model="selectedGroup" :options="groupOptions"
            :aria-label="t('modelPlaza.list.groupLabel')" :searchable="false" class="catalog-dialog-select"
          />
        </div>
        <p class="catalog-dialog-group">{{ t('modelPlaza.list.pricingFor', { group: activeOffer.group.name }) }}</p>
        <p v-if="activeOffer.group.description" class="catalog-description">{{ activeOffer.group.description }}</p>
        <div class="catalog-badges">
          <span v-if="activeOffer.group.is_exclusive">{{ t('modelPlaza.badges.exclusive') }}</span>
          <span v-if="activeOffer.group.subscription_type === 'subscription'">{{ t('modelPlaza.badges.subscription') }}</span>
          <span v-if="activeOffer.group.quality_status">{{ t(`modelPlaza.badges.quality.${activeOffer.group.quality_status}`) }}</span>
        </div>
        <p v-if="peakWindow" class="catalog-notice">{{ t('modelPlaza.detail.peakNote', { window: peakWindow, multiplier: activeOffer.group.peak_rate_multiplier }) }}</p>
        <p v-if="longContextDisabled" class="catalog-notice">{{ t('modelPlaza.detail.longContextDisabledNote') }}</p>
        <p v-if="isVideo" class="catalog-notice">{{ t('modelPlaza.list.videoPricingUnavailable') }}</p>
        <p v-else class="catalog-muted">{{ t('modelPlaza.list.officialReference') }}</p>
        <div v-if="!isVideo" class="catalog-table-scroll" role="region" :aria-label="t('modelPlaza.list.viewPricing', { model: entry.name })" tabindex="0">
          <PlazaModelPricingTable v-bind="pricingTableProps" />
        </div>
      </div>
    </BaseDialog>
  </article>
</template>

<script setup lang="ts">
import { computed, ref, watch } from 'vue'
import { useI18n } from 'vue-i18n'
import type { PlazaCatalogEntry } from './modelPlazaCatalog'
import { BILLING_MODE_TOKEN, BILLING_MODE_VIDEO } from '@/constants/channel'
import BaseDialog from '@/components/common/BaseDialog.vue'
import PlazaProviderIcon from './PlazaProviderIcon.vue'
import Select from '@/components/common/Select.vue'
import Icon from '@/components/icons/Icon.vue'
import { useClipboard } from '@/composables/useClipboard'
import { useAppStore } from '@/stores/app'
import { modelProvider, providerLabel } from './modelPlazaProviders'
import { formatPeakRateWindow, serverTimezoneLabel } from '@/utils/peak-rate'
import PlazaModelPricingTable from './PlazaModelPricingTable.vue'
import { useModelPlazaPricing, type ModelPlazaPricingProps } from './useModelPlazaPricing'

const props = withDefaults(defineProps<{ entry: PlazaCatalogEntry; view?: 'grid' | 'list'; grouped?: boolean }>(), { view: 'grid', grouped: false })
const { t } = useI18n()
const { copied, copyToClipboard } = useClipboard()
const appStore = useAppStore()
const detailsOpen = ref(false)
const selectedGroupId = ref<number | null>(null)
// Resolve from the current offers on every read, so a filtered or refreshed offer
// can never leave a stale model or price visible while the selection resets.
const activeOffer = computed(() => props.entry.offers.find(offer => offer.group.id === selectedGroupId.value) ?? props.entry.offers[0])
const provider = computed(() => props.entry.provider ?? modelProvider(activeOffer.value?.model ?? props.entry))
const providerName = computed(() => provider.value === 'other' ? t('modelPlaza.filters.otherProviders') : providerLabel(provider.value))
watch(() => [props.entry.key, props.entry.offers.map(offer => offer.group.id)] as const, ([key], previous) => {
  if (key !== previous?.[0] || !props.entry.offers.some(offer => offer.group.id === selectedGroupId.value)) {
    selectedGroupId.value = props.entry.offers[0]?.group.id ?? null
  }
}, { immediate: true, flush: 'sync' })
const selectedGroup = computed({
  get: () => activeOffer.value?.group.id ?? null,
  set: (value: string | number | boolean | null) => { selectedGroupId.value = typeof value === 'number' ? value : null }
})
const groupOptions = computed(() => props.entry.offers.map(({ group }) => ({ value: group.id, label: group.name })))
const peakWindow = computed(() => formatPeakRateWindow(activeOffer.value?.group, serverTimezoneLabel(appStore.cachedPublicSettings?.server_utc_offset)))
const pricingProps: ModelPlazaPricingProps = {
  get models() { return activeOffer.value ? [activeOffer.value.model] : [] },
  get platform() { return activeOffer.value?.group.platform },
  get rateMultiplier() { return activeOffer.value?.group.rate_multiplier ?? 1 },
  get userRateMultiplier() { return activeOffer.value?.group.user_rate_multiplier },
  get imageRateIndependent() { return activeOffer.value?.group.image_rate_independent },
  get imageRateMultiplier() { return activeOffer.value?.group.image_rate_multiplier },
  get peakWindow() { return peakWindow.value },
  get peakRateMultiplier() { return activeOffer.value?.group.peak_rate_multiplier }
}
// Give the child a fresh props snapshot when an offer changes. Passing the same
// getter object to v-bind would let Vue skip the child's prop update.
const pricingTableProps = computed(() => ({ ...pricingProps }))
const {
  billingMode, billingModeLabel, paidPerMillion, paidRequestPrice, perUnitSuffix,
  tokenIntervals, requestIntervals, tierLabel, timePeriods, usesIndependentImageRate,
  requestRate, hasCustomRate
} = useModelPlazaPricing(pricingProps)
const isToken = computed(() => !!activeOffer.value && billingMode(activeOffer.value.model) === BILLING_MODE_TOKEN)
const isVideo = computed(() => !!activeOffer.value && billingMode(activeOffer.value.model) === BILLING_MODE_VIDEO)
const billingLabel = computed(() => !activeOffer.value || isToken.value ? t('modelPlaza.list.tokenBilling') : billingModeLabel(activeOffer.value.model))
const independentImage = computed(() => !!activeOffer.value && usesIndependentImageRate(activeOffer.value.model))
const activeRate = computed(() => activeOffer.value ? requestRate(activeOffer.value.model) : 1)
const longContextDisabled = computed(() => activeOffer.value?.group.long_context_pricing_enabled === false && (activeOffer.value.model.official_pricing?.intervals?.length ?? 0) > 1)
const preview = computed(() => {
  const model = activeOffer.value?.model
  if (!model) return { input: '-', output: '-', request: '-', cacheRead: null, notes: [] as string[] }
  const tier = (isToken.value ? tokenIntervals(model) : requestIntervals(model))[0]
  const notes: string[] = []
  if (tier) notes.push(t('modelPlaza.list.tierPreview', { tier: tierLabel(tier) }))
  if (timePeriods(model).length) notes.push(t('modelPlaza.list.standardPrice'), t('modelPlaza.list.timePricing'))
  if (peakWindow.value) notes.push(t('modelPlaza.list.peakSummary', { window: peakWindow.value }))
  if (activeOffer.value?.group.quality_status === 'suspect') notes.push(t('modelPlaza.badges.quality.suspect'))
  if (model.long_context_basis === 'marginal') notes.push(t('modelPlaza.table.marginalBadge'))
  if (model.pricing?.max_reasoning_effort_multiplier) notes.push(t('modelPlaza.table.maxReasoningMultiplierBadge', { multiplier: model.pricing.max_reasoning_effort_multiplier }))
  if (usesIndependentImageRate(model)) notes.push(t('modelPlaza.list.imageRate', { rate: requestRate(model) }))
  const cacheRead = tier ? tier.cache_read_price : model.pricing?.cache_read_price
  return {
    input: paidPerMillion(model, tier ? tier.input_price : model.pricing?.input_price),
    output: paidPerMillion(model, tier ? tier.output_price : model.pricing?.output_price),
    request: paidRequestPrice(model, tier ? tier.per_request_price : model.pricing?.per_request_price),
    cacheRead: cacheRead == null ? null : paidPerMillion(model, cacheRead), notes
  }
})
</script>

<style scoped>
.plaza-catalog-card { display: flex; flex-direction: column; gap: 16px; min-width: 0; padding: 18px; border: 1px solid color-mix(in srgb, var(--signal-line, #dedee4) 80%, transparent); border-radius: 12px; background: color-mix(in srgb, var(--signal-surface, #fff) 78%, transparent); color: var(--signal-text, #1c1c1e); font-size: 13px; line-height: 1.5; }
.catalog-identity { display: flex; align-items: flex-start; gap: 11px; min-width: 0; }
.catalog-logo { width: 28px; height: 28px; flex-shrink: 0; margin-top: 2px; }
.catalog-name { min-width: 0; flex: 1; }
.catalog-name h3 { margin: 0; font-size: 13px; font-weight: 600; overflow-wrap: anywhere; }
.catalog-name p { margin-top: 3px; }
.catalog-muted, .catalog-prices dt, .catalog-prices small { color: var(--signal-muted, #63636c); font-size: 11px; }
.catalog-copy { display: inline-flex; align-items: center; justify-content: center; flex-shrink: 0; width: 28px; height: 28px; border-radius: 5px; color: var(--signal-muted, #63636c); }
.catalog-copy:hover, .catalog-details:hover { background: var(--signal-hover, #f3f3f6); color: var(--signal-text, #1c1c1e); }
.catalog-offer { display: flex; gap: 8px; align-items: center; min-width: 0; }
.catalog-offer > .catalog-muted { flex-shrink: 0; }
.catalog-group-select, .catalog-group-name { flex: 1; min-width: 0; overflow-wrap: anywhere; }
.catalog-group-select :deep(.select-trigger) { min-height: 32px; padding-block: 5px; font-size: 12px; }
.catalog-rate { flex-shrink: 0; font-size: 11px; font-variant-numeric: tabular-nums; }
.catalog-rate s { color: var(--signal-muted, #63636c); margin-right: 3px; }
.catalog-pricing { min-width: 0; flex: 1; }
.catalog-price-unit { display: block; color: var(--signal-muted, #63636c); font-size: 10px; margin-bottom: 9px; }
.catalog-prices { display: grid; grid-template-columns: repeat(3, minmax(0, 1fr)); gap: 12px; }
.catalog-prices > div { min-width: 0; }
.catalog-prices dd { margin: 3px 0 0; overflow-wrap: anywhere; font-family: ui-monospace, SFMono-Regular, Consolas, monospace; font-variant-numeric: tabular-nums; font-weight: 500; }
.catalog-prices small { font-weight: 400; white-space: normal; }
.catalog-request { grid-column: 1 / -1; }
.catalog-price-notes { display: flex; gap: 4px 10px; flex-wrap: wrap; margin-top: 10px; font-size: 10px; color: var(--signal-muted, #63636c); }
.catalog-footer { display: flex; align-items: center; justify-content: flex-end; gap: 8px; padding-top: 12px; border-top: 1px solid var(--signal-line, #dedee4); }
.catalog-footer > .catalog-muted { margin-right: auto; }
.catalog-details { display: inline-flex; align-items: center; gap: 4px; min-height: 28px; padding: 3px 5px; border-radius: 5px; font-size: 11px; }
.catalog-details:disabled { opacity: .5; cursor: default; }
.catalog-copy:focus-visible, .catalog-details:focus-visible, .catalog-table-scroll:focus-visible { outline: 2px solid var(--signal-accent, #0066d6); outline-offset: 3px; }
.catalog-dialog { display: grid; gap: 12px; min-width: 0; color: var(--signal-text, #1c1c1e); font-size: 13px; }
.catalog-dialog-heading { display: flex; gap: 12px; justify-content: space-between; align-items: center; flex-wrap: wrap; }
.catalog-dialog-select { width: min(100%, 280px); }
.catalog-dialog-group { font-weight: 600; }
.catalog-description { white-space: pre-line; overflow-wrap: anywhere; color: var(--signal-muted, #63636c); }
.catalog-badges { display: flex; flex-wrap: wrap; gap: 6px; }
.catalog-badges:empty { display: none; }
.catalog-badges span { padding: 2px 7px; border: 1px solid var(--signal-line, #dedee4); border-radius: 5px; font-size: 11px; }
.catalog-notice { font-size: 12px; color: var(--signal-muted, #63636c); overflow-wrap: anywhere; }
.catalog-table-scroll { min-width: 0; max-width: 100%; overflow-x: auto; overscroll-behavior-x: contain; }
.catalog-table-scroll :deep(.plaza-pricing-table) { overflow-x: visible; }
@media (min-width: 1100px) {
  .plaza-catalog-card--list { display: grid; grid-template-columns: minmax(0, 1fr) minmax(150px, .55fr) minmax(0, 1fr) auto; align-items: center; gap: 20px; }
  .plaza-catalog-card--list .catalog-offer { flex-wrap: wrap; }
  .plaza-catalog-card--list .catalog-offer > .catalog-muted { width: 100%; }
  .plaza-catalog-card--list .catalog-footer { flex-direction: column; align-items: flex-end; border: 0; padding: 0; }
}
@media (max-width: 480px) {
  .plaza-catalog-card { padding: 16px; }
  .catalog-prices { gap: 10px; }
  .catalog-copy, .catalog-details { min-width: 44px; min-height: 44px; }
  .catalog-group-select :deep(.select-trigger) { min-height: 44px; }
}
</style>
