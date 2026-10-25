<template>
  <div class="ranking-page" :aria-busy="loading">
    <header class="ranking-heading">
      <div>
        <span class="ranking-eyebrow">{{ t('leaderboard.eyebrow') }}</span>
        <h1>{{ t('leaderboard.title') }}</h1>
        <p>{{ t('leaderboard.description') }}</p>
      </div>
      <div class="ranking-update">
        <span v-if="data">{{ t('leaderboard.updated', { time: updatedTime }) }}</span>
        <button type="button" :aria-label="t('leaderboard.refresh')" :disabled="loading" @click="$emit('refresh')"><Icon name="refresh" size="sm" :class="{ 'animate-spin': loading }" /></button>
      </div>
    </header>

    <div class="ranking-periods" role="group" :aria-label="t('leaderboard.periodLabel')">
      <button v-for="item in periods" :key="item" type="button" :aria-pressed="period === item" @click="$emit('update:period', item)">{{ t(`leaderboard.periods.${item}`) }}</button>
      <span v-if="data" class="ranking-timezone">{{ data.timezone }}</span>
    </div>

    <div v-if="loading" class="ranking-loading" role="status">
      <div class="ranking-skeleton-header" /><div class="ranking-skeleton-bars"><i v-for="i in 12" :key="i" :style="{ height: `${30 + (i * 17) % 65}%` }" /></div>
      <span class="sr-only">{{ t('leaderboard.loading') }}</span>
    </div>
    <div v-else-if="error || !data || !ranked.length" class="ranking-state" role="status">
      <Icon :name="error ? 'infoCircle' : 'chart'" size="lg" />
      <h2>{{ t(error === 'disabled' ? 'leaderboard.disabled' : error ? 'leaderboard.error' : 'leaderboard.empty') }}</h2>
      <p>{{ t(error === 'disabled' ? 'leaderboard.disabledHint' : error ? 'leaderboard.errorHint' : 'leaderboard.emptyHint') }}</p>
      <button v-if="error && error !== 'disabled'" type="button" class="btn btn-secondary" @click="$emit('refresh')">{{ t('leaderboard.retry') }}</button>
    </div>
    <template v-else>
      <section class="ranking-panel" aria-labelledby="ranking-popular">
        <header class="ranking-panel-heading">
          <div><h2 id="ranking-popular"><Icon name="chart" size="sm" />{{ t('leaderboard.popular') }}</h2><p>{{ t('leaderboard.popularHint') }}</p></div>
          <div class="ranking-total"><strong>{{ compactTokens(data.total_tokens) }}</strong><span>{{ t('leaderboard.tokens') }}</span></div>
        </header>
        <div class="ranking-chart-area"><LeaderboardChart :data="data" /></div>
        <div class="ranking-legend" aria-hidden="true"><span v-for="(model, i) in ranked.slice(0, 8)" :key="model.model"><i :style="{ background: SERIES_COLORS[i] }" />{{ model.model }}</span><span v-if="ranked.length > 8"><i :style="{ background: SERIES_COLORS[8] }" />{{ t('leaderboard.otherModels') }}</span></div>
        <div class="ranking-list-section">
          <div class="ranking-section-heading"><div><h2><Icon name="trophy" size="sm" />{{ t('leaderboard.modelRanking') }}</h2><p>{{ t('leaderboard.rankingHint') }}</p></div><span>{{ t('leaderboard.activeModels') }} <b>{{ ranked.length }}</b></span></div>
          <ol class="ranking-model-list" :style="{ '--ranking-rows': Math.ceil(Math.min(20, ranked.length) / 2) }">
            <li v-for="model in ranked.slice(0, 20)" :key="model.model" :data-rank="model.rank">
              <span class="ranking-position" :class="{ 'ranking-podium': model.rank <= 3 }">{{ String(model.rank).padStart(2, '0') }}</span>
              <PlazaProviderIcon :platform="model.provider" size="lg" />
              <div class="ranking-model-name"><strong :title="model.model">{{ model.model }}</strong><span>{{ model.provider === 'other' ? t('leaderboard.other') : providerLabel(model.provider) }}</span></div>
              <div class="ranking-model-value"><strong>{{ compactTokens(model.total_tokens) }} <small>Token</small></strong><span v-if="model.changePercent === null" class="ranking-new">{{ t('leaderboard.newEntry') }}</span><span v-else :class="changeClass(model.changePercent)">{{ model.changePercent > 0 ? '↗' : model.changePercent < 0 ? '↘' : '—' }} {{ Math.abs(model.changePercent).toLocaleString(locale, { maximumFractionDigits: 1 }) }}%</span></div>
            </li>
          </ol>
        </div>
      </section>

      <section class="ranking-panel" aria-labelledby="ranking-share">
        <header class="ranking-panel-heading"><div><h2 id="ranking-share"><Icon name="chart" size="sm" />{{ t('leaderboard.shareTitle') }}</h2><p>{{ t('leaderboard.shareHint') }}</p></div></header>
        <div class="ranking-chart-area"><LeaderboardChart :data="data" share /></div>
        <div class="ranking-list-section">
          <div class="ranking-section-heading"><h2>{{ t('leaderboard.providerRanking') }}</h2><span>{{ t('leaderboard.totalRequests') }} <b>{{ compactTokens(data.total_requests) }}</b></span></div>
          <ol class="ranking-provider-list">
            <li v-for="(provider, i) in providers" :key="provider.provider">
              <span class="ranking-position">{{ String(i + 1).padStart(2, '0') }}</span>
              <i class="ranking-provider-dot" :style="{ background: PROVIDER_COLORS[provider.provider] || '#94a3b8' }" />
              <span class="ranking-provider-name">{{ provider.provider === 'other' ? t('leaderboard.other') : providerLabel(provider.provider) }}</span>
              <div class="ranking-model-value"><strong>{{ compactTokens(provider.tokens) }}</strong><span>{{ provider.share.toLocaleString(locale, { maximumFractionDigits: 1 }) }}%</span></div>
            </li>
          </ol>
        </div>
      </section>

      <div class="ranking-movers">
        <section v-for="direction in directions" :key="direction" class="ranking-panel" :aria-labelledby="`ranking-${direction}`">
          <header class="ranking-panel-heading"><div><h2 :id="`ranking-${direction}`"><Icon :name="direction === 'rising' ? 'trendingUp' : 'trendingDown'" size="sm" :class="direction === 'rising' ? 'ranking-positive' : 'ranking-negative'" />{{ t(`leaderboard.${direction}`) }}</h2><p>{{ t(`leaderboard.${direction}Hint`) }}</p></div></header>
          <ul v-if="movers[direction].length" class="ranking-mover-list">
            <li v-for="model in movers[direction]" :key="model.model">
              <PlazaProviderIcon :platform="model.provider" size="lg" /><div class="ranking-model-name"><strong :title="model.model">{{ model.model }}</strong><span>#{{ model.rank }} · {{ model.provider === 'other' ? t('leaderboard.other') : providerLabel(model.provider) }}</span></div>
              <span :class="direction === 'rising' ? 'ranking-positive' : 'ranking-negative'">{{ direction === 'rising' ? '↗' : '↘' }} {{ t('leaderboard.places', { count: Math.abs(model.rankChange || 0) }) }}</span>
            </li>
          </ul>
          <p v-else class="ranking-no-movers">{{ t('leaderboard.noMovers') }}</p>
        </section>
      </div>
      <footer class="ranking-footnote"><p>{{ t('leaderboard.scope') }} {{ t('leaderboard.timezone', { timezone: data.timezone }) }}</p><p>{{ t('leaderboard.comparison') }}</p><p>{{ t('leaderboard.retention') }}</p></footer>
    </template>
  </div>
</template>

<script setup lang="ts">
import { computed } from 'vue'
import { useI18n } from 'vue-i18n'
import Icon from '@/components/icons/Icon.vue'
import PlazaProviderIcon from '@/components/modelPlaza/PlazaProviderIcon.vue'
import { providerLabel } from '@/components/modelPlaza/modelPlazaProviders'
import type { LeaderboardPeriod, LeaderboardResponse } from '@/api/leaderboard'
import LeaderboardChart from './LeaderboardChart.vue'
import { rankModels, providerShares, compactTokens, SERIES_COLORS, PROVIDER_COLORS } from './leaderboardData'
import './leaderboard.css'

const props = defineProps<{ data: LeaderboardResponse | null; period: LeaderboardPeriod; loading: boolean; error: 'failed' | 'disabled' | null }>()
defineEmits<{ 'update:period': [LeaderboardPeriod]; refresh: [] }>()
const { t, locale } = useI18n()
const periods: LeaderboardPeriod[] = ['day', 'week', 'month', 'year']
const directions = ['rising', 'falling'] as const
const ranked = computed(() => rankModels(props.data?.models || []))
const providers = computed(() => providerShares(props.data?.models || []))
const movers = computed(() => ({
  rising: ranked.value.filter(m => m.rankChange !== null && m.rankChange > 0).sort((a, b) => b.rankChange! - a.rankChange! || a.rank - b.rank).slice(0, 6),
  falling: ranked.value.filter(m => m.rankChange !== null && m.rankChange < 0).sort((a, b) => a.rankChange! - b.rankChange! || a.rank - b.rank).slice(0, 6),
}))
const updatedTime = computed(() => props.data ? new Intl.DateTimeFormat(locale.value, { hour: '2-digit', minute: '2-digit', hourCycle: 'h23', timeZone: props.data.timezone }).format(new Date(props.data.updated_at)) : '')
const changeClass = (value: number) => value > 0 ? 'ranking-positive' : value < 0 ? 'ranking-negative' : ''
</script>
