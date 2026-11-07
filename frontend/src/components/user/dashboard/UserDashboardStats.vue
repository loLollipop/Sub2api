<template>
  <section class="signal-stats" :aria-label="t('dashboard.title')">
    <DashboardStatGrid v-if="section !== 'platforms'">
      <DashboardStat v-if="!isSimple" class="overview-metric overview-metric--balance" icon="creditCard" tone="green" :label="`${t('dashboard.balance')} USD`">
        $<AnimatedNumber :value="balance" :format="formatBalance" />
        <template #detail><span>{{ t('dashboard.recentTotal') }} · {{ t('dashboard.actual') }} <strong>${{ formatCost(stats.total_actual_cost) }}</strong></span><span>{{ t('dashboard.standard') }} ${{ formatCost(stats.total_cost) }}</span></template>
      </DashboardStat>
      <DashboardStat class="overview-metric overview-metric--cost" icon="dollar" tone="purple" :label="t('dashboard.todayCost')">
        <span :title="t('dashboard.actual')">$<AnimatedNumber :value="stats.today_actual_cost" :format="formatCost" :duration="480" /></span>
        <template #detail><span>{{ t('dashboard.standard') }} <strong>${{ formatCost(stats.today_cost) }}</strong></span></template>
      </DashboardStat>
      <DashboardStat class="overview-metric overview-metric--tokens" icon="cube" :label="t('dashboard.todayTokens')">
        <AnimatedNumber :value="stats.today_tokens" :format="formatTokens" />
        <template #detail><span>{{ t('dashboard.input') }} {{ formatTokens(stats.today_input_tokens) }} / {{ t('dashboard.output') }} {{ formatTokens(stats.today_output_tokens) }}</span><span>{{ t('dashboard.cache') }} {{ formatTokens(stats.today_cache_creation_tokens + stats.today_cache_read_tokens) }}</span></template>
      </DashboardStat>
      <DashboardStat class="overview-metric" icon="chart" tone="blue" :label="t('dashboard.todayRequests')">
        <AnimatedNumber :value="stats.today_requests" :decimals="0" />
        <template #detail><span>{{ t('dashboard.recentTotal') }} <strong>{{ formatNumber(stats.total_requests) }}</strong></span></template>
      </DashboardStat>
      <DashboardStat class="overview-metric overview-metric--total" icon="database" tone="violet" :label="t('dashboard.recentTokens')">
        <AnimatedNumber :value="stats.total_tokens" :format="formatTokens" />
        <template #detail><span class="overview-token-split">{{ t('dashboard.input') }} {{ formatTokens(stats.total_input_tokens) }} / {{ t('dashboard.output') }} {{ formatTokens(stats.total_output_tokens) }} / {{ t('dashboard.cache') }} {{ formatTokens(stats.total_cache_creation_tokens + stats.total_cache_read_tokens) }}</span></template>
      </DashboardStat>
      <DashboardStat class="overview-metric" icon="key" tone="green" :label="t('dashboard.apiKeys')">
        <AnimatedNumber :value="stats.total_api_keys" :decimals="0" />
        <template #badge><span class="stat-badge-success">{{ stats.active_api_keys }} {{ t('common.active') }}</span></template>
      </DashboardStat>
      <DashboardStat class="overview-reading" icon="bolt" tone="blue" :label="t('dashboard.performance')">
        {{ formatTokens(stats.rpm) }} <small>RPM</small>
        <template #detail><span><strong>{{ formatTokens(stats.tpm) }}</strong> TPM</span></template>
      </DashboardStat>
      <DashboardStat class="overview-reading" icon="clock" tone="purple" :label="t('dashboard.avgResponse')">
        {{ formatDuration(stats.average_duration_ms) }}
      </DashboardStat>
    </DashboardStatGrid>

    <section v-if="section !== 'summary' && !isSimple && platformCards.length > 0" class="signal-platforms">
      <header class="signal-section-heading">
        <h2>{{ t('dashboard.last30Days') }}</h2>
        <span>{{ t('dashboard.platformCount', { count: platformCards.length }) }}</span>
      </header>
      <div class="signal-platform-grid">
        <article v-for="item in platformCards" :key="item.platform" class="signal-platform">
          <header class="signal-platform-heading">
            <span class="signal-platform-name">
              <PlatformIcon :platform="item.platform as GroupPlatform" size="lg" />
              <span>{{ platformLabel(item.platform) }}</span>
            </span>
            <span class="signal-spend" :title="t('dashboard.actual')">${{ formatCost(item.total_actual_cost) }}</span>
          </header>
          <dl class="signal-platform-ledger">
            <div><dt>{{ t('dashboard.todayCost') }}</dt><dd>${{ formatCost(item.today_actual_cost) }}</dd></div>
            <div><dt>{{ t('dashboard.requests') }}</dt><dd>{{ item.total_requests > 0 ? formatNumber(item.total_requests) : '-' }}</dd></div>
            <div><dt>{{ t('dashboard.tokens') }}</dt><dd>{{ item.total_tokens > 0 ? formatTokens(item.total_tokens) : '-' }}</dd></div>
          </dl>
          <div v-if="hasAnyLimit(item.quota)" class="signal-quota">
            <p class="signal-label">{{ t('dashboard.platformQuota.title') }}</p>
            <template v-for="w in (['daily', 'weekly', 'monthly'] as const)" :key="w">
              <div v-if="quotaVal(item.quota, `${w}_limit_usd`) != null" class="signal-quota-window">
                <template v-if="(quotaVal(item.quota, `${w}_limit_usd`) as number) === 0">
                  <div class="signal-quota-label"><span>{{ t(`dashboard.platformQuota.${w}`) }}</span><span class="signal-disabled">{{ t('dashboard.platformQuota.disabled') }}</span></div>
                  <div class="signal-quota-track"><div class="signal-quota-fill bg-red-500" style="width: 100%" /></div>
                </template>
                <template v-else>
                  <div class="signal-quota-label">
                    <span>{{ t(`dashboard.platformQuota.${w}`) }}</span>
                    <span>${{ formatUsd((quotaVal(item.quota, `${w}_usage_usd`) as number) ?? 0) }} / ${{ formatUsd(quotaVal(item.quota, `${w}_limit_usd`) as number) }}</span>
                  </div>
                  <div class="signal-quota-track">
                    <div class="signal-quota-fill"
                      :class="quotaBarClass(calcPercent((quotaVal(item.quota, `${w}_usage_usd`) as number) ?? 0, quotaVal(item.quota, `${w}_limit_usd`) as number))"
                      :style="{ width: calcPercent((quotaVal(item.quota, `${w}_usage_usd`) as number) ?? 0, quotaVal(item.quota, `${w}_limit_usd`) as number) + '%' }" />
                  </div>
                  <p v-if="quotaVal(item.quota, `${w}_window_resets_at`)" class="signal-reset">{{ t('dashboard.platformQuota.resetsAt', { time: formatResetTime(quotaVal(item.quota, `${w}_window_resets_at`) as string) }) }}</p>
                </template>
              </div>
            </template>
          </div>
        </article>
      </div>
    </section>
    <p v-else-if="section === 'platforms' && !isSimple" class="signal-platform-empty">{{ t('dashboard.platformBreakdownEmpty') }}</p>
  </section>
</template>

<script setup lang="ts">
import { computed } from 'vue'
import { useI18n } from 'vue-i18n'
import DashboardStat from '@/components/dashboard/DashboardStat.vue'
import DashboardStatGrid from '@/components/dashboard/DashboardStatGrid.vue'
import AnimatedNumber from '@/components/common/AnimatedNumber.vue'
import PlatformIcon from '@/components/common/PlatformIcon.vue'
import type { UserDashboardStats as UserStatsType } from '@/api/usage'
import type { PlatformQuotaItem, GroupPlatform } from '@/types'

interface FusedPlatformCard {
  platform: string
  total_actual_cost: number
  today_actual_cost: number
  total_requests: number
  total_tokens: number
  quota?: PlatformQuotaItem
}

const props = defineProps<{
  stats: UserStatsType
  balance: number
  isSimple: boolean
  platformQuotas?: PlatformQuotaItem[] | null
  section?: 'summary' | 'platforms'
}>()
const { t } = useI18n()

const PLATFORM_LABELS: Record<string, string> = {
  anthropic: 'Claude',
  openai: 'OpenAI',
  gemini: 'Gemini',
  antigravity: 'Antigravity',
  grok: 'Grok',
  kimi: 'Kimi',
  zhipu: 'Zhipu GLM',
  deepseek: 'DeepSeek',
  minimax: 'MiniMax',
}

const platformLabel = (p: string) => PLATFORM_LABELS[p] ?? p

const platformCards = computed<FusedPlatformCard[]>(() => {
  // 建立 by_platform Map
  const byPlat = new Map<string, NonNullable<UserStatsType['by_platform']>[number]>()
  for (const item of props.stats?.by_platform ?? []) byPlat.set(item.platform, item)

  // 建立 quota Map
  const byQuota = new Map<string, PlatformQuotaItem>()
  for (const q of props.platformQuotas ?? []) byQuota.set(q.platform, q)

  // Platform windows can differ from totals. Never synthesize spend from their difference.
  const platforms = new Set<string>([...byPlat.keys(), ...byQuota.keys()])

  const PLATFORM_ORDER = ['anthropic', 'openai', 'gemini', 'antigravity', 'grok']
  const cards: FusedPlatformCard[] = []

  for (const p of platforms) {
    const stat = byPlat.get(p)
    cards.push({
      platform: p,
      total_actual_cost: stat?.total_actual_cost ?? 0,
      today_actual_cost: stat?.today_actual_cost ?? 0,
      total_requests: stat?.total_requests ?? 0,
      total_tokens: stat?.total_tokens ?? 0,
      quota: byQuota.get(p),
    })
  }

  // 排序：按 PLATFORM_ORDER，未知平台按名称排序
  cards.sort((a, b) => {
    const ai = PLATFORM_ORDER.indexOf(a.platform)
    const bi = PLATFORM_ORDER.indexOf(b.platform)
    if (ai === -1 && bi === -1) return a.platform.localeCompare(b.platform)
    if (ai === -1) return 1
    if (bi === -1) return -1
    return ai - bi
  })

  return cards
})

// Quota helpers

type QuotaWindow = 'daily' | 'weekly' | 'monthly'
type QuotaField = `${QuotaWindow}_limit_usd` | `${QuotaWindow}_usage_usd` | `${QuotaWindow}_window_resets_at`

function quotaVal(q: PlatformQuotaItem | undefined, key: QuotaField): PlatformQuotaItem[QuotaField] {
  return q?.[key]
}

function hasAnyLimit(q: PlatformQuotaItem | undefined): boolean {
  if (!q) return false
  return q.daily_limit_usd != null || q.weekly_limit_usd != null || q.monthly_limit_usd != null
}

function calcPercent(usage: number, limit: number): number {
  if (!limit || limit <= 0) return 0
  return Math.min(100, Math.max(0, Math.round((usage / limit) * 100)))
}

function quotaBarClass(p: number): string {
  if (p >= 95) return 'bg-red-500'
  if (p >= 75) return 'bg-amber-500'
  return 'bg-green-500'
}

// 与 formatBalance 一致使用 Intl.NumberFormat 做半偶舍入，避免 toFixed 在不同 JS 引擎
// 下偶发截断而非四舍五入（与后端展示精度不一致）。
const usdFormatter = new Intl.NumberFormat('en-US', {
  minimumFractionDigits: 2,
  maximumFractionDigits: 2,
})
function formatUsd(n: number): string {
  if (!Number.isFinite(n)) return '0.00'
  return usdFormatter.format(n)
}

function formatResetTime(iso: string | null | undefined): string {
  if (!iso) return ''
  const d = new Date(iso)
  if (Number.isNaN(d.getTime())) return iso
  return d.toLocaleString(undefined, {
    month: 'numeric',
    day: 'numeric',
    hour: '2-digit',
    minute: '2-digit',
    hour12: false,
  })
}

const formatBalance = (b: number) =>
  new Intl.NumberFormat('en-US', {
    minimumFractionDigits: 2,
    maximumFractionDigits: 2
  }).format(b)

const formatNumber = (n: number) => n.toLocaleString()
const formatCost = (c: number) => c.toFixed(4)
const formatTokens = (t: number) => {
  if (t >= 1_000_000_000) return `${(t / 1_000_000_000).toFixed(2)}B`
  if (t >= 1_000_000) return `${(t / 1_000_000).toFixed(1)}M`
  if (t >= 1000) return `${(t / 1000).toFixed(1)}K`
  return t.toString()
}
const formatDuration = (ms: number) => ms >= 1000 ? `${(ms / 1000).toFixed(2)}s` : `${ms.toFixed(0)}ms`
</script>

<style scoped>
.signal-label { font-size: 12px; color: var(--signal-muted); }
.signal-spend { color: var(--signal-amber); }
.signal-stats { min-width: 0; }
.signal-platform-empty { padding: 48px 0; color: var(--signal-muted); font-size: 14px; text-align: center; }
.signal-section-heading { display: flex; justify-content: space-between; align-items: baseline; flex-wrap: wrap; gap: 8px; padding: 20px 0 12px; }
.signal-section-heading h2 { font-size: 14px; font-weight: 600; }
.signal-section-heading > span { font-size: 12px; color: var(--signal-muted, #65716a); }
.signal-platform-grid { display: grid; grid-template-columns: repeat(4, minmax(0, 1fr)); gap: 0 24px; }
.signal-platform { min-width: 0; padding: 16px 0; border-top: 1px solid var(--signal-line, #dce2df); }
.signal-platform--other { border-top-style: dashed; }
.signal-platform-heading { display: flex; justify-content: space-between; align-items: center; flex-wrap: wrap; gap: 10px; font-size: 13px; font-weight: 600; font-variant-numeric: tabular-nums; }
.signal-platform-name { display: inline-flex; align-items: center; gap: 8px; min-width: 0; overflow-wrap: anywhere; }
.signal-platform-name :deep(svg) { color: var(--signal-text, #202423); flex-shrink: 0; }
.signal-platform-ledger { margin-top: 12px; }
.signal-platform-ledger > div, .signal-quota-label { display: flex; align-items: baseline; justify-content: space-between; gap: 8px; font-size: 12px; line-height: 1.6; font-variant-numeric: tabular-nums; }
.signal-platform-ledger dt { color: var(--signal-muted, #65716a); }
.signal-platform-ledger dd, .signal-quota-label > span:last-child { text-align: right; overflow-wrap: anywhere; min-width: 0; }
.signal-quota { margin-top: 12px; border-top: 1px solid var(--signal-line, #dce2df); padding-top: 10px; }
.signal-quota-window { margin-top: 8px; }
.signal-quota-track { height: 4px; margin-top: 4px; overflow: hidden; background: var(--signal-line, #dce2df); }
.signal-quota-fill { height: 100%; }
.signal-quota-fill.bg-green-500 { background: var(--signal-accent, #087f68); }
.signal-quota-fill.bg-amber-500 { background: var(--signal-amber, #99640c); }
.signal-quota-fill.bg-red-500 { background: var(--signal-danger, #dc4545); }
.signal-disabled { color: var(--signal-danger, #dc4545); }
.signal-reset { margin-top: 4px; font-size: 11px; color: var(--signal-muted, #65716a); }
@media (max-width: 1100px) {
  .signal-platform-grid { grid-template-columns: repeat(2, minmax(0, 1fr)); }
}
@media (max-width: 640px) {
  .signal-platform-grid { gap: 0 16px; }
}
@media (max-width: 420px) {
  .signal-platform-grid { grid-template-columns: minmax(0, 1fr); }
  .signal-platform-ledger { display: grid; grid-template-columns: repeat(3, minmax(0, 1fr)); gap: 10px; }
  .signal-platform-ledger > div { display: block; }
  .signal-platform-ledger dd { text-align: left; }
}
</style>
