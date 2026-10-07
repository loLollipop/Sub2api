<template>
  <section class="external-shop">
    <header class="shop-heading">
      <div class="shop-heading__icon" aria-hidden="true"><Icon name="creditCard" size="lg" /></div>
      <div>
        <p class="shop-kicker" aria-hidden="true">04 / BILLING</p>
        <h1>{{ t('nav.buySubscription') }}</h1>
      </div>
    </header>
    <p class="shop-description">{{ t('purchase.shopDescription') }}</p>
    <div class="shop-layout">
      <section class="shop-amount">
        <h2><span aria-hidden="true">01</span>{{ t('purchase.referenceAmount') }}</h2>
        <div class="shop-tiers" role="group" :aria-label="t('purchase.referenceAmount')">
          <button v-for="tier in tiers" :key="tier" type="button" :aria-pressed="selectedAmount === tier" :disabled="!safeProducts[tier]"
            :class="['shop-tier', { 'shop-tier--selected': selectedAmount === tier }]" @click="amount = tier">¥{{ tier }}<small v-if="!safeProducts[tier]">{{ t('purchase.tierNotConfigured') }}</small></button>
        </div>
      </section>
      <aside class="shop-summary">
        <div class="shop-account">
          <div><p class="shop-kicker">{{ t('payment.rechargeAccount') }}</p><strong>{{ username }}</strong></div>
          <div class="shop-balance"><span>{{ t('payment.currentBalance') }}</span><strong>${{ balance.toFixed(2) }}</strong></div>
        </div>
        <div class="shop-reference"><span>{{ t('purchase.referenceAmount') }}</span><strong>¥{{ (selectedAmount || 0).toFixed(2) }}</strong></div>
        <p v-if="!Object.keys(safeProducts).length" role="status" class="shop-warning">{{ t('purchase.invalidShopUrl') }}</p>
        <button type="button" class="btn btn-primary w-full py-3" :disabled="!canPurchase" @click="openShop">
          {{ t('purchase.goToShop') }} <Icon name="externalLink" size="sm" aria-hidden="true" />
        </button>
        <RouterLink to="/redeem" class="btn btn-secondary w-full">{{ t('purchase.redeemCode') }}</RouterLink>
      </aside>
    </div>
  </section>
</template>

<script setup lang="ts">
import { computed, ref, watch } from 'vue'
import { useI18n } from 'vue-i18n'
import { RouterLink } from 'vue-router'
import Icon from '@/components/icons/Icon.vue'

const props = defineProps<{ products: Record<string, string>; username: string; balance: number }>()
const { t } = useI18n()
const tiers = [10, 20, 30, 50, 100] as const
const amount = ref<number | null>(null)

function hasUnsafeProductUrlCharacters(raw: string): boolean {
  return Array.from(raw).some(char => {
    const code = char.charCodeAt(0)
    return code < 32 || (code >= 127 && code <= 159) || char === '\\'
  })
}

function safeProductUrl(raw: string): string {
  if (!raw || raw !== raw.trim() || raw.length > 2048 || (/\s/u.test(raw) || hasUnsafeProductUrlCharacters(raw)) || !/^https?:\/\//i.test(raw)) return ''
  try {
    const url = new URL(raw)
    const authority = raw.split('://')[1]?.split(/[/?#]/)[0] || ''
    const host = authority.startsWith('[') ? url.hostname : authority.split(':')[0] || ''
    if (!['http:', 'https:'].includes(url.protocol) || !url.hostname || url.username || url.password || (url.port && Number(url.port) < 1) || authority.endsWith(':')) return ''
    if (/^[\d.]+$/.test(host) && (host.split('.').length !== 4 || host.split('.').some(part => String(Number(part)) !== part || Number(part) > 255))) return ''
    if (!authority.startsWith('[') && (host.length > 253 || /^[\d.]+$/.test(host) && !/^\d+\.\d+\.\d+\.\d+$/.test(host)
      || host.replace(/\.$/, '').split('.').some(label => !/^[\p{L}\p{N}](?:[\p{L}\p{N}-]{0,61}[\p{L}\p{N}])?$/u.test(label)))) return ''
    return raw
  } catch {
    return ''
  }
}

const safeProducts = computed<Record<number, string>>(() => Object.fromEntries(tiers.flatMap(tier => {
  const url = safeProductUrl(props.products[String(tier)] || '')
  return url ? [[tier, url]] : []
})))
const selectedAmount = computed(() => amount.value !== null && tiers.some(tier => tier === amount.value)
  && safeProducts.value[amount.value] ? amount.value : null)
const selectedUrl = computed(() => selectedAmount.value === null ? '' : safeProducts.value[selectedAmount.value] || '')
const canPurchase = computed(() => Boolean(selectedUrl.value))
// A changed or removed product requires a fresh selection before opening it.
watch(safeProducts, (products, oldProducts) => {
  if (amount.value !== null && products[amount.value] !== oldProducts[amount.value]) amount.value = null
}, { flush: 'sync' })

function openShop() {
  if (canPurchase.value) window.open(selectedUrl.value, '_blank', 'noopener,noreferrer')
}
</script>

<style scoped>
.external-shop { display: grid; gap: 18px; min-width: 0; }
.shop-heading { display: flex; align-items: center; gap: 14px; padding-bottom: 18px; border-bottom: 1px solid var(--payment-line); }
.shop-heading__icon { display: grid; width: 38px; height: 38px; place-items: center; border: 1px solid var(--payment-line); border-radius: 6px; color: var(--payment-accent); background: var(--payment-surface); }
.shop-heading h1 { margin: 0; font-size: 24px; font-weight: 700; }
.shop-kicker { margin: 0; color: var(--payment-muted); font-size: 10px; font-weight: 700; letter-spacing: .12em; }
.shop-description { margin: 0; color: var(--payment-muted); font-size: 14px; line-height: 1.7; }
.shop-layout { display: grid; grid-template-columns: minmax(0, 1fr) minmax(280px, 360px); gap: 24px; }
.shop-amount { padding: 24px; border: 1px solid var(--payment-line); border-radius: 8px; background: var(--payment-surface); }
.shop-amount h2 { display: flex; align-items: center; gap: 12px; margin: 0 0 22px; font-size: 16px; font-weight: 700; }
.shop-amount h2 span { color: var(--payment-accent); font-size: 12px; font-variant-numeric: tabular-nums; }
.shop-tiers { display: grid; grid-template-columns: repeat(3, minmax(0, 1fr)); gap: 12px; }
.shop-tier { min-height: 64px; padding: 12px; border: 2px solid var(--payment-line); border-radius: 6px; background: var(--payment-surface); font-size: 20px; font-weight: 600; font-variant-numeric: tabular-nums; }
.shop-tier:not(:disabled):hover, .shop-tier--selected { border-color: var(--payment-accent); }
.shop-tier--selected { color: var(--payment-accent); background: var(--payment-accent-soft); }
.shop-tier:disabled { cursor: not-allowed; opacity: .5; }
.shop-tier small { display: block; margin-top: 5px; font-size: 11px; font-weight: 400; }
.shop-tier:focus-visible { outline: 2px solid var(--payment-accent); outline-offset: 3px; }
.shop-summary { display: grid; align-content: start; gap: 16px; padding: 24px; border: 1px solid var(--payment-line); border-radius: 8px; background: var(--payment-surface); }
.shop-account { display: flex; align-items: center; justify-content: space-between; gap: 16px; padding-bottom: 18px; border-bottom: 1px solid var(--payment-line); overflow-wrap: anywhere; }
.shop-balance { display: grid; gap: 4px; text-align: right; flex-shrink: 0; }
.shop-balance span, .shop-reference span { color: var(--payment-muted); font-size: 12px; }
.shop-reference { display: flex; justify-content: space-between; align-items: baseline; gap: 12px; }
.shop-reference strong { color: var(--payment-accent); font-size: 28px; font-variant-numeric: tabular-nums; overflow-wrap: anywhere; min-width: 0; }
.shop-warning { margin: 0; color: var(--payment-amber); font-size: 13px; line-height: 1.6; }
.shop-summary .btn { display: inline-flex; align-items: center; justify-content: center; gap: 8px; min-height: 44px; }
@media (max-width: 767px) { .shop-layout { grid-template-columns: 1fr; gap: 16px; } .shop-amount, .shop-summary { padding: 18px; } }
</style>
