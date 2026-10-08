<template>
  <section class="external-shop">
    <header class="shop-heading">
      <div class="shop-heading__icon" aria-hidden="true"><Icon name="creditCard" size="lg" /></div>
      <div class="shop-heading__copy">
        <p class="shop-kicker" aria-hidden="true">04 / BILLING</p>
        <h1>{{ t('purchase.title') }}</h1>
        <p class="shop-description">{{ t('purchase.shopDescription') }}</p>
      </div>
    </header>
    <div class="shop-account">
      <div class="shop-account__identity">
        <span class="shop-account__icon" aria-hidden="true"><Icon name="userCircle" size="lg" /></span>
        <div class="shop-account__name"><p class="shop-kicker">{{ t('payment.rechargeAccount') }}</p><strong>{{ username }}</strong></div>
      </div>
      <div class="shop-balance"><span>{{ t('purchase.balanceUsd') }}</span><strong>${{ balance.toFixed(2) }}<small aria-hidden="true">USD</small></strong></div>
    </div>
    <div class="shop-layout">
      <section class="shop-amount">
        <div class="shop-section-heading">
          <h2><span class="shop-section-number" aria-hidden="true">01</span>{{ t('purchase.selectAmount') }}</h2>
          <span class="shop-currency" aria-hidden="true">CNY</span>
        </div>
        <p class="shop-amount-hint">{{ t('purchase.productAmountHint') }}</p>
        <div class="shop-tiers" role="group" :aria-label="t('purchase.referenceAmount')">
          <button v-for="tier in tiers" :key="tier" type="button" :aria-pressed="selectedAmount === tier" :disabled="!safeProducts[tier]"
            :class="['shop-tier', { 'shop-tier--selected': selectedAmount === tier }]" @click="amount = tier">
            <span class="shop-tier__amount">¥{{ tier }}</span>
            <Icon v-if="selectedAmount === tier" class="shop-tier__check" name="check" size="xs" aria-hidden="true" />
            <small v-if="!safeProducts[tier]">{{ t('purchase.tierNotConfigured') }}</small>
          </button>
        </div>
        <section class="shop-guide" :aria-label="t('purchase.howItWorks')">
          <h3><Icon name="arrowRight" size="sm" aria-hidden="true" />{{ t('purchase.howItWorks') }}</h3>
          <ol>
            <li><span class="shop-guide__number" aria-hidden="true">1</span><div><strong>{{ t('purchase.stepSelectTitle') }}</strong><p>{{ t('purchase.stepSelectDescription') }}</p></div></li>
            <li><span class="shop-guide__number" aria-hidden="true">2</span><div><strong>{{ t('purchase.stepBuyTitle') }}</strong><p>{{ t('purchase.stepBuyDescription') }}</p></div></li>
            <li><span class="shop-guide__number" aria-hidden="true">3</span><div><strong>{{ t('purchase.stepRedeemTitle') }}</strong><p>{{ t('purchase.stepRedeemDescription') }}</p></div></li>
          </ol>
        </section>
        <section class="shop-faq" :aria-label="t('purchase.faqTitle')">
          <h3><Icon name="questionCircle" size="sm" aria-hidden="true" />{{ t('purchase.faqTitle') }}</h3>
          <dl>
            <div><dt>{{ t('purchase.faqCreditQuestion') }}</dt><dd>{{ t('purchase.faqCreditAnswer') }}</dd></div>
            <div><dt>{{ t('purchase.faqValueQuestion') }}</dt><dd>{{ t('purchase.faqValueAnswer') }}</dd></div>
          </dl>
        </section>
      </section>
      <aside class="shop-summary">
        <div class="shop-summary-heading"><h2>{{ t('purchase.purchaseSummary') }}</h2><Icon name="document" size="md" aria-hidden="true" /></div>
        <div class="shop-reference" aria-live="polite" aria-atomic="true">
          <span>{{ t('purchase.referenceAmount') }}</span>
          <strong v-if="selectedAmount !== null">¥{{ selectedAmount.toFixed(2) }}<small aria-hidden="true">CNY</small></strong>
          <strong v-else class="shop-reference--empty">{{ t('purchase.chooseAmount') }}</strong>
        </div>
        <dl class="shop-order-details">
          <div><dt>{{ t('purchase.productType') }}</dt><dd>{{ t('purchase.codeProduct') }}</dd></div>
          <div><dt>{{ t('purchase.creditMethod') }}</dt><dd>{{ t('purchase.manualRedemption') }}</dd></div>
        </dl>
        <p class="shop-value-hint"><Icon name="infoCircle" size="sm" aria-hidden="true" />{{ t('purchase.codeValueHint') }}</p>
        <p v-if="!Object.keys(safeProducts).length" role="status" class="shop-warning">{{ t('purchase.invalidShopUrl') }}</p>
        <button type="button" class="shop-purchase" :disabled="!canPurchase" @click="openShop">
          {{ t('purchase.goToShop') }} <Icon name="externalLink" size="sm" aria-hidden="true" />
        </button>
        <p class="shop-purchase-hint"><Icon name="externalLink" size="xs" aria-hidden="true" />{{ t('purchase.redemptionRequired') }}</p>
        <section class="shop-reminders" :aria-label="t('purchase.remindersTitle')">
          <h3><Icon name="shield" size="sm" aria-hidden="true" />{{ t('purchase.remindersTitle') }}</h3>
          <ul>
            <li><Icon name="creditCard" size="sm" aria-hidden="true" /><span>{{ t('purchase.reminderProduct') }}</span></li>
            <li><Icon name="userCircle" size="sm" aria-hidden="true" /><span>{{ t('purchase.reminderAccount') }}</span></li>
            <li><Icon name="lock" size="sm" aria-hidden="true" /><span>{{ t('purchase.reminderCode') }}</span></li>
          </ul>
        </section>
        <div class="shop-redeem">
          <span>{{ t('purchase.haveCode') }}</span>
          <RouterLink to="/redeem"><span><Icon name="gift" size="sm" aria-hidden="true" />{{ t('purchase.redeemCode') }}</span><Icon name="arrowRight" size="sm" aria-hidden="true" /></RouterLink>
        </div>
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
.external-shop { display: grid; grid-template-rows: auto auto 1fr; gap: 20px; min-width: 0; container-type: inline-size; }
.shop-heading { display: flex; align-items: center; gap: 14px; padding-bottom: 20px; border-bottom: 1px solid var(--payment-line); }
.shop-heading__icon { display: grid; width: 48px; height: 48px; flex: 0 0 auto; place-items: center; border: 1px solid var(--payment-line); border-radius: 14px; color: var(--payment-accent); background: var(--payment-surface); box-shadow: 0 3px 8px #20242605; }
.shop-heading__copy { min-width: 0; }
.shop-heading h1 { margin: 3px 0 6px; font-size: 29px; font-weight: 700; line-height: 1.2; }
.shop-kicker { margin: 0; color: var(--payment-muted); font-size: 11px; font-weight: 600; line-height: 1.5; }
.shop-description { margin: 0; color: var(--payment-muted); font-size: 13px; line-height: 1.6; }
.shop-account { position: relative; display: flex; align-items: center; justify-content: space-between; gap: 20px; min-width: 0; padding: 18px 24px; border: 1px solid var(--payment-line); border-radius: 14px; background: var(--payment-surface); box-shadow: var(--signal-shadow, none); }
.shop-account::before { position: absolute; inset: 24px auto 24px -1px; width: 3px; border-radius: 2px; background: var(--payment-accent); content: ''; }
.shop-account__identity { display: flex; align-items: center; gap: 12px; min-width: 0; }
.shop-account__icon { display: grid; width: 42px; height: 42px; flex: 0 0 auto; place-items: center; border-radius: 12px; color: var(--payment-accent); background: var(--payment-accent-soft); }
.shop-account__name { min-width: 0; }
.shop-account__name strong { display: block; margin-top: 3px; font-size: 15px; font-weight: 650; overflow-wrap: anywhere; }
.shop-balance { display: grid; gap: 3px; flex: 0 0 auto; padding-left: 24px; border-left: 1px solid var(--payment-line); text-align: right; }
.shop-balance > span, .shop-reference > span { color: var(--payment-muted); font-size: 12px; }
.shop-balance strong { color: var(--payment-text); font-size: 28px; font-weight: 650; font-variant-numeric: tabular-nums; line-height: 1.3; }
.shop-balance small, .shop-reference small { margin-left: 7px; color: var(--payment-muted); font-size: 11px; font-weight: 500; }
.shop-layout { display: grid; grid-template-columns: minmax(0, 1fr) minmax(288px, 320px); align-items: stretch; gap: 20px; }
.shop-amount, .shop-summary { min-width: 0; padding: 22px; border: 1px solid var(--payment-line); border-radius: 16px; background: var(--payment-surface); box-shadow: var(--signal-shadow, none); }
.shop-amount { display: flex; flex-direction: column; }
.shop-section-heading { display: flex; align-items: center; justify-content: space-between; gap: 12px; }
.shop-section-heading h2, .shop-summary h2 { display: flex; align-items: center; gap: 10px; margin: 0; font-size: 16px; font-weight: 650; }
.shop-section-number { display: grid; width: 28px; height: 28px; flex: 0 0 auto; place-items: center; border-radius: 8px; color: var(--payment-accent); background: var(--payment-accent-soft); font-size: 11px; font-variant-numeric: tabular-nums; }
.shop-currency { padding: 4px 8px; border: 1px solid var(--payment-line); border-radius: 6px; color: var(--payment-muted); font-size: 10px; font-weight: 600; letter-spacing: .04em; }
.shop-amount-hint { margin: 8px 0 18px; color: var(--payment-muted); font-size: 12px; line-height: 1.6; }
.shop-tiers { display: grid; grid-template-columns: repeat(5, minmax(0, 1fr)); gap: 10px; }
.shop-tier { position: relative; min-width: 0; min-height: 84px; padding: 21px 5px; border: 1px solid var(--signal-control-line, var(--payment-line)); border-radius: 12px; color: var(--payment-text); background: var(--payment-surface); font-size: 24px; font-weight: 650; font-variant-numeric: tabular-nums; transition: border-color 140ms ease, background-color 140ms ease, box-shadow 140ms ease; }
.shop-tier:not(:disabled):hover { border-color: var(--payment-accent); background: var(--payment-accent-soft); }
.shop-tier--selected { border-color: var(--payment-accent); color: var(--payment-accent); background: var(--payment-accent-soft); box-shadow: inset 0 0 0 1px var(--payment-accent), 0 3px 8px #007db510; }
.shop-tier__check { position: absolute; top: 6px; right: 6px; padding: 2px; border-radius: 50%; color: var(--signal-on-accent, #fff); background: var(--payment-accent); }
.shop-tier:disabled { cursor: not-allowed; color: var(--payment-muted); background: var(--payment-bg); border-style: dashed; }
.shop-tier small { display: block; margin-top: 5px; font-size: 10px; font-weight: 400; overflow-wrap: anywhere; }
.shop-tier:focus-visible, .shop-purchase:focus-visible, .shop-redeem a:focus-visible { outline: 2px solid var(--payment-accent); outline-offset: 3px; }
.shop-guide { display: flex; flex: 1; flex-direction: column; margin-top: 24px; padding-top: 22px; border-top: 1px solid var(--payment-line); }
.shop-guide h3, .shop-faq h3 { display: flex; align-items: center; gap: 7px; margin: 0 0 16px; color: var(--payment-text); font-size: 13px; font-weight: 600; }
.shop-guide h3 :deep(svg), .shop-faq h3 :deep(svg) { color: var(--payment-muted); }
.shop-guide ol { display: grid; flex: 1; grid-template-columns: repeat(3, minmax(0, 1fr)); gap: 12px; margin: 0; padding: 0; list-style: none; }
.shop-guide li { position: relative; display: flex; flex-direction: column; align-items: flex-start; gap: 16px; min-width: 0; padding: 18px 14px; border: 1px solid var(--payment-line); border-radius: 12px; background: var(--payment-bg); }
.shop-guide__number { display: grid; width: 30px; height: 30px; flex: 0 0 auto; place-items: center; border: 1px solid var(--payment-line); border-radius: 50%; color: var(--payment-accent); background: var(--payment-surface); font-size: 12px; font-weight: 650; }
.shop-guide li:not(:last-child)::after { position: absolute; top: 33px; left: calc(100% - 10px); z-index: 1; width: 32px; border-top: 1px dashed var(--signal-control-line, var(--payment-line)); content: ''; }
.shop-guide strong { font-size: 13px; font-weight: 600; }
.shop-guide p { margin: 7px 0 0; color: var(--payment-muted); font-size: 12px; line-height: 1.7; }
.shop-faq { padding-top: 24px; }
.shop-faq dl { display: grid; grid-template-columns: repeat(2, minmax(0, 1fr)); gap: 18px; margin: 0; }
.shop-faq dl > div { min-width: 0; padding-left: 12px; border-left: 2px solid var(--payment-line); }
.shop-faq dt { color: var(--payment-text); font-size: 12px; font-weight: 600; line-height: 1.7; }
.shop-faq dd { margin: 6px 0 0; color: var(--payment-muted); font-size: 12px; line-height: 1.7; }
.shop-summary { display: flex; flex-direction: column; gap: 10px; }
.shop-summary-heading { display: flex; align-items: center; justify-content: space-between; gap: 12px; }
.shop-summary-heading > :deep(svg) { color: var(--payment-muted); }
.shop-reference { display: grid; gap: 10px; padding: 14px 16px; border: 1px solid var(--payment-line); border-radius: 12px; background: var(--payment-accent-soft); }
.shop-reference strong { min-width: 0; color: var(--payment-accent); font-size: 40px; font-weight: 650; font-variant-numeric: tabular-nums; overflow-wrap: anywhere; line-height: 1.2; }
.shop-reference .shop-reference--empty { padding: 5px 0; color: var(--payment-muted); font-size: 22px; font-weight: 500; }
.shop-order-details { display: grid; gap: 9px; margin: 0; padding: 3px 0 12px; border-bottom: 1px dashed var(--signal-control-line, var(--payment-line)); font-size: 12px; }
.shop-order-details > div { display: flex; justify-content: space-between; gap: 12px; min-width: 0; }
.shop-order-details dt { color: var(--payment-muted); }
.shop-order-details dd { min-width: 0; margin: 0; color: var(--payment-text); text-align: right; overflow-wrap: anywhere; }
.shop-value-hint, .shop-purchase-hint { display: flex; align-items: flex-start; gap: 6px; margin: 0; color: var(--payment-muted); font-size: 12px; line-height: 1.6; }
.shop-value-hint :deep(svg), .shop-purchase-hint :deep(svg) { flex: 0 0 auto; margin-top: 2px; }
.shop-purchase-hint { margin-top: -5px; font-size: 11px; }
.shop-warning { margin: 0; color: var(--payment-amber); font-size: 12px; line-height: 1.6; }
.shop-purchase { display: inline-flex; align-items: center; justify-content: center; gap: 10px; width: 100%; min-height: 48px; padding: 12px 16px; border: 1px solid var(--payment-accent); border-radius: 10px; color: var(--signal-on-accent, #fff); background: var(--payment-accent); font-size: 14px; font-weight: 600; transition: box-shadow 140ms ease, filter 140ms ease; }
.shop-purchase:not(:disabled):hover { filter: brightness(.94); box-shadow: 0 4px 12px #007db520; }
.shop-purchase:disabled { border-color: var(--payment-line); color: var(--payment-muted); background: var(--payment-bg); cursor: not-allowed; }
.shop-reminders { display: flex; flex: 1; flex-direction: column; padding: 16px; border: 1px solid var(--payment-line); border-radius: 12px; background: var(--payment-bg); }
.shop-reminders h3 { display: flex; align-items: center; gap: 7px; margin: 0; color: var(--payment-text); font-size: 12px; font-weight: 600; }
.shop-reminders h3 :deep(svg) { color: var(--payment-accent); }
.shop-reminders ul { display: grid; align-content: start; gap: 10px; margin: 14px 0 0; padding: 0; color: var(--payment-muted); font-size: 12px; line-height: 1.7; list-style: none; }
.shop-reminders li { display: flex; align-items: flex-start; gap: 9px; }
.shop-reminders li :deep(svg) { flex: 0 0 auto; margin-top: 2px; }
.shop-redeem { display: flex; align-items: center; justify-content: space-between; flex-wrap: wrap; gap: 6px 12px; padding: 9px 14px; border: 1px solid var(--payment-line); border-radius: 10px; background: var(--payment-surface); }
.shop-redeem > span { color: var(--payment-muted); font-size: 12px; }
.shop-redeem a { display: inline-flex; align-items: center; justify-content: space-between; gap: 10px; min-height: 44px; border-radius: 4px; color: var(--payment-accent); font-size: 13px; font-weight: 600; }
.shop-redeem a > span { display: inline-flex; align-items: center; gap: 8px; }
.shop-redeem a:hover { text-decoration: underline; text-underline-offset: 3px; }
@container (max-width: 740px) { .shop-layout { grid-template-columns: minmax(0, 1fr); } }
@container (max-width: 900px) {
  .shop-guide ol, .shop-faq dl { grid-template-columns: minmax(0, 1fr); }
  .shop-guide li { flex-direction: row; gap: 12px; padding: 14px; }
  .shop-guide li:not(:last-child)::after { display: none; }
}
@container (max-width: 480px) { .shop-tiers { grid-template-columns: repeat(3, minmax(0, 1fr)); } }
@media (max-width: 767px) {
  .external-shop { gap: 16px; }
  .shop-heading { flex-wrap: wrap; gap: 12px; padding-bottom: 16px; }
  .shop-heading__copy { flex: 1 1 calc(100% - 60px); }
  .shop-heading h1 { font-size: 23px; }
  .shop-layout { grid-template-columns: minmax(0, 1fr); gap: 16px; }
  .shop-account, .shop-amount, .shop-summary { padding: 18px; }
  .shop-tiers { grid-template-columns: repeat(3, minmax(0, 1fr)); }
  .shop-tier { min-height: 70px; padding: 18px 5px; font-size: 22px; }
}
@media (max-width: 374px) {
  .shop-account { flex-wrap: wrap; gap: 14px; }
  .shop-balance { width: 100%; padding: 12px 0 0; border-top: 1px solid var(--payment-line); border-left: 0; text-align: left; }
  .shop-account__icon { display: none; }
  .shop-tier { font-size: 19px; }
}
@media (prefers-reduced-motion: reduce) { .shop-tier, .shop-purchase { transition: none; } }
</style>
