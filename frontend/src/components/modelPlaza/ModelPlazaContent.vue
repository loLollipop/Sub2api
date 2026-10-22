<template>
  <div class="model-plaza-content space-y-5">
    <div class="plaza-heading signal-editorial-heading" :class="{ 'plaza-heading-embedded': embedded }">
      <div class="signal-editorial-copy">
        <span class="signal-editorial-index" aria-hidden="true">02 / MODELS</span>
        <h1>{{ t('modelPlaza.title') }}</h1>
        <p>{{ t('modelPlaza.description') }}</p>
      </div>
    </div>

    <!-- 全局价格说明(管理员配置,Markdown) -->
    <div
      v-if="descriptionHtml"
      class="plaza-description px-1 py-3 text-sm"
      v-html="descriptionHtml"
    ></div>

    <!-- 未登录提示 -->
    <p
      v-if="!isAuthenticated"
      class="plaza-hint"
    >
      <Icon name="infoCircle" size="xs" class="h-3.5 w-3.5" />
      {{ t('modelPlaza.anonymousHint') }}
    </p>

    <!-- 加载/错误/空 -->
    <div v-if="loading" class="flex min-h-[240px] items-center justify-center">
      <div class="h-8 w-8 animate-spin rounded-full border-2 border-primary-600/25 border-t-primary-600 dark:border-primary-400/25 dark:border-t-primary-400"></div>
    </div>
    <div
      v-else-if="error"
      class="plaza-status plaza-status-error"
    >
      {{ t('modelPlaza.loadFailed') }}
    </div>
    <template v-else>
      <div class="plaza-catalog-search">
        <Icon name="search" size="sm" aria-hidden="true" />
        <input v-model="searchQuery" type="search" :placeholder="t('modelPlaza.list.searchPlaceholder')" :aria-label="t('modelPlaza.filters.modelLabel')" />
      </div>

      <div class="plaza-catalog-layout">
        <button type="button" class="plaza-mobile-filter-toggle" :aria-expanded="filtersOpen" aria-controls="plaza-catalog-filters" @click="filtersOpen = !filtersOpen">
          <Icon name="filter" size="sm" aria-hidden="true" />
          {{ t('modelPlaza.list.filters') }}
          <span v-if="hasActiveFilters" class="plaza-filter-dot" aria-hidden="true"></span>
          <Icon name="chevronDown" size="xs" :class="{ 'rotate-180': filtersOpen }" aria-hidden="true" />
        </button>
        <aside id="plaza-catalog-filters" class="plaza-catalog-sidebar" :class="{ 'is-open': filtersOpen }" :aria-label="t('modelPlaza.list.filters')">
          <div class="plaza-filter-heading">
            <h2>{{ t('modelPlaza.list.filters') }}</h2>
            <button v-if="hasActiveFilters" type="button" class="plaza-catalog-reset" data-testid="plaza-clear-filters" @click="resetFilters">{{ t('modelPlaza.filters.clearAll') }}</button>
          </div>
          <PlazaFilterBar
            :platforms="platforms"
            :groups="groupOptions"
            :rates="rates"
            :platform="selectedPlatform"
            :group-id="selectedGroupId"
            :rate="selectedRate"
            :search="searchQuery"
            hide-search
            sidebar
            @update:platform="selectedPlatform = $event"
            @update:group-id="selectedGroupId = $event"
            @update:rate="selectedRate = $event"
            @update:search="searchQuery = $event"
            @reset="resetFilters"
          />
          <div class="plaza-billing-filters">
            <h3>{{ t('modelPlaza.list.billingType') }}</h3>
            <div role="radiogroup" :aria-label="t('modelPlaza.list.billingType')" @keydown="plazaRadioKeyboard">
              <button v-for="mode in billingChoices" :key="mode.value" type="button" role="radio" :aria-checked="selectedBilling === mode.value" :tabindex="selectedBilling === mode.value ? 0 : -1" @click="selectedBilling = mode.value">{{ mode.label }}</button>
            </div>
          </div>
        </aside>
        <div class="plaza-catalog-results">
          <div ref="resultsToolbar" class="plaza-catalog-toolbar">
            <p class="plaza-catalog-count" aria-live="polite" role="status">{{ t('modelPlaza.list.catalogCount', { count: filteredEntries.length, total: allEntries.length }) }}</p>
            <div class="plaza-catalog-controls">
              <Select v-model="sortOrder" :options="sortOptions" :aria-label="t('modelPlaza.list.sort')" class="plaza-sort" />
              <div class="plaza-view-toggle" role="group" :aria-label="t('modelPlaza.list.view')">
                <button type="button" :aria-pressed="viewMode === 'grid'" :aria-label="t('modelPlaza.list.gridView')" @click="viewMode = 'grid'"><Icon name="grid" size="sm" /></button>
                <button type="button" :aria-pressed="viewMode === 'list'" :aria-label="t('modelPlaza.list.listView')" @click="viewMode = 'list'"><svg viewBox="0 0 20 20" fill="none" stroke="currentColor" stroke-width="1.5" aria-hidden="true"><path d="M3 4h14M3 10h14M3 16h14" /></svg></button>
              </div>
            </div>
          </div>
          <p class="plaza-catalog-price-hint">{{ t('modelPlaza.list.catalogPriceHint') }}</p>
          <div v-if="pageEntries.length" class="plaza-catalog" :class="`plaza-catalog-${viewMode}`">
            <PlazaCatalogCard v-for="entry in pageEntries" :key="entry.key" :entry="entry" :view="viewMode" />
          </div>
          <div v-else class="plaza-status">
            <p>{{ hasActiveFilters ? t('modelPlaza.noSearchResult') : t('modelPlaza.empty') }}</p>
            <button v-if="hasActiveFilters" type="button" class="btn btn-secondary mt-3" data-testid="plaza-empty-clear-filters" @click="resetFilters">{{ t('modelPlaza.filters.clearAll') }}</button>
          </div>
          <Pagination v-if="filteredEntries.length > PAGE_SIZE" :total="filteredEntries.length" :page="page" :page-size="PAGE_SIZE" :show-page-size-selector="false" @update:page="changePage" />
        </div>
      </div>
    </template>
  </div>
</template>

<script setup lang="ts">
import { computed, ref, watch } from 'vue'
import { useI18n } from 'vue-i18n'
import { marked } from 'marked'
import DOMPurify from 'dompurify'
import Icon from '@/components/icons/Icon.vue'
import PlazaFilterBar from './PlazaFilterBar.vue'
import PlazaCatalogCard from './PlazaCatalogCard.vue'
import Select from '@/components/common/Select.vue'
import Pagination from '@/components/common/Pagination.vue'
import { platformLabel } from '@/utils/platformColors'
import { buildModelCatalog } from './modelPlazaCatalog'
import { plazaRadioKeyboard } from './plazaRadioKeyboard'
import type { ModelPlazaGroup, ModelPlazaResponse } from '@/api/modelPlaza'
import { useAuthStore } from '@/stores/auth'

const props = defineProps<{
  response: ModelPlazaResponse | null
  loading: boolean
  error?: boolean
  /** 后台内嵌形态(AppLayout 内):隐藏页头。 */
  embedded?: boolean
}>()

const { t } = useI18n()
const authStore = useAuthStore()
const isAuthenticated = computed(() => authStore.isAuthenticated)

const selectedPlatform = ref<string>('all')
const selectedGroupId = ref<number | 'all'>('all')
const selectedRate = ref<number | 'all'>('all')
const searchQuery = ref('')
const selectedBilling = ref('all')
const viewMode = ref<'grid' | 'list'>('grid')
const sortOrder = ref('name')
const filtersOpen = ref(false)
const page = ref(1)
const PAGE_SIZE = 24
const resultsToolbar = ref<HTMLElement | null>(null)
const billingChoices = computed(() => [
  { value: 'all', label: t('modelPlaza.filters.all') },
  { value: 'token', label: t('modelPlaza.list.tokenBilling') },
  { value: 'per_request', label: t('modelPlaza.table.perRequest') },
  { value: 'image', label: t('modelPlaza.table.perImage') },
  { value: 'video', label: t('modelPlaza.list.videoBilling') }
])
const sortOptions = computed(() => [
  { value: 'name', label: t('modelPlaza.list.nameAsc') },
  { value: 'name-desc', label: t('modelPlaza.list.nameDesc') },
  { value: 'provider', label: t('modelPlaza.list.byProvider') }
])

const hasActiveFilters = computed(() => selectedPlatform.value !== 'all' || selectedGroupId.value !== 'all' || selectedRate.value !== 'all' || searchQuery.value.trim() !== '' || selectedBilling.value !== 'all')
const resetFilters = () => {
  selectedPlatform.value = 'all'
  selectedGroupId.value = 'all'
  selectedRate.value = 'all'
  searchQuery.value = ''
  selectedBilling.value = 'all'
}

const descriptionHtml = computed(() => {
  const md = props.response?.description?.trim()
  if (!md) return ''
  return DOMPurify.sanitize(marked.parse(md) as string)
})

/** 生效倍率 = 用户专属倍率 ?? 分组默认倍率。 */
function effectiveRate(g: ModelPlazaGroup): number {
  return g.user_rate_multiplier ?? g.rate_multiplier
}

const platforms = computed(() =>
  [...new Set((props.response?.groups ?? []).flatMap(g => g.models.map(m => m.platform)).filter(Boolean))].sort()
)

const groupOptions = computed(() =>
  (props.response?.groups ?? []).map((g) => ({
    id: g.id,
    name: g.name,
    platform: g.platform,
    platforms: [...new Set(g.models.map(m => m.platform))],
    rate: effectiveRate(g),
    rates: [effectiveRate(g)]
  }))
)

/** 全量生效倍率;当前组合下不可用的项由 FilterBar 置灰而非隐藏。 */
const rates = computed(() =>
  [...new Set((props.response?.groups ?? []).map(effectiveRate))].sort((a, b) => a - b)
)

/** 数据刷新后选中的倍率可能不复存在,重置为全部。 */
watch(rates, (list) => {
  if (selectedRate.value !== 'all' && !list.includes(selectedRate.value)) {
    selectedRate.value = 'all'
  }
})

const filteredGroups = computed(() => {
  let groups = props.response?.groups ?? []
  if (selectedPlatform.value !== 'all') {
    groups = groups.map(g => ({ ...g, models: g.models.filter(m => m.platform === selectedPlatform.value) })).filter(g => g.models.length > 0)
  }
  if (selectedGroupId.value !== 'all') {
    groups = groups.filter((g) => g.id === selectedGroupId.value)
  }
  if (selectedRate.value !== 'all') {
    groups = groups.filter((g) => effectiveRate(g) === selectedRate.value)
  }
  // 模型名搜索:分组内只留命中的模型,整组无命中则隐藏该分组。
  const q = searchQuery.value.trim().toLowerCase()
  if (q) {
    groups = groups
      .map((g) => ({ ...g, models: g.models.filter((m) => m.name.toLowerCase().includes(q) || platformLabel(m.platform).toLowerCase().includes(q)) }))
      .filter((g) => g.models.length > 0)
  }
  if (selectedBilling.value !== 'all') {
    groups = groups.map(g => ({ ...g, models: g.models.filter(m => (m.pricing?.billing_mode || 'token') === selectedBilling.value) })).filter(g => g.models.length > 0)
  }
  // 专属倍率会改变生效值,不能只依赖后端按默认倍率的排序。
  return [...groups].sort(
    (a, b) => effectiveRate(a) - effectiveRate(b) || a.name.localeCompare(b.name)
  )
})

const allEntries = computed(() => buildModelCatalog(props.response?.groups ?? []))
const filteredEntries = computed(() => buildModelCatalog(filteredGroups.value).sort((a, b) => {
  const nameCompare = a.name.localeCompare(b.name, undefined, { numeric: true })
  if (sortOrder.value === 'provider') return a.platform.localeCompare(b.platform) || nameCompare
  return (sortOrder.value === 'name-desc' ? -nameCompare : nameCompare) || a.platform.localeCompare(b.platform)
}))
const pageEntries = computed(() => filteredEntries.value.slice((page.value - 1) * PAGE_SIZE, page.value * PAGE_SIZE))
watch([selectedPlatform, selectedGroupId, selectedRate, selectedBilling, searchQuery, sortOrder, () => props.response], () => { page.value = 1 }, { flush: 'sync' })

function changePage(next: number) {
  page.value = next
  resultsToolbar.value?.scrollIntoView({ block: 'start' })
}
</script>

<style scoped>
.model-plaza-content {
  min-width: 0;
  color: var(--signal-text, #1c1c1e);
}

.plaza-heading h1 {
  font-size: 28px;
  line-height: 34px;
  font-weight: 600;
  letter-spacing: 0;
}

.plaza-heading p,
.plaza-hint {
  margin-top: 6px;
  color: var(--signal-muted, #63636c);
  font-size: 13px;
}

.plaza-hint {
  display: flex;
  align-items: center;
  gap: 6px;
}

.plaza-status {
  padding: 32px 20px;
  border: 1px dashed var(--signal-line, #dedee4);
  border-radius: 8px;
  color: var(--signal-muted, #63636c);
  text-align: center;
  font-size: 14px;
}

.plaza-status-error {
  border-style: solid;
  border-color: #eb4d3d55;
  background: #eb4d3d0f;
  color: #c4291c;
}

.plaza-catalog-search { position: relative; width: min(100%, 660px); margin: 0 auto 28px; }
.plaza-catalog-search > svg { position: absolute; top: 50%; left: 15px; transform: translateY(-50%); color: var(--signal-muted, #63636c); pointer-events: none; }
.plaza-catalog-search input { display: block; width: 100%; min-height: 42px; padding: 10px 16px 10px 42px; border: 1px solid color-mix(in srgb, var(--signal-line, #dedee4) 80%, transparent); border-radius: 10px; background: color-mix(in srgb, var(--signal-muted, #63636c) 4%, transparent); color: var(--signal-text, #1c1c1e); font-size: 13px; }
.plaza-catalog-search input::placeholder { color: var(--signal-muted, #63636c); }
.plaza-catalog-search input:focus-visible { outline: 2px solid var(--signal-accent, #0066d6); outline-offset: 2px; }
.plaza-catalog-layout { display: grid; grid-template-columns: 210px minmax(0, 1fr); align-items: start; gap: 28px; }
.plaza-catalog-sidebar { display: grid; gap: 22px; position: sticky; top: 84px; max-height: calc(100dvh - 104px); overflow-y: auto; overscroll-behavior: contain; min-width: 0; padding: 16px; border: 1px solid color-mix(in srgb, var(--signal-line, #dedee4) 70%, transparent); border-radius: 12px; background: color-mix(in srgb, var(--signal-surface, white) 55%, transparent); }
.plaza-filter-heading { display: flex; justify-content: space-between; align-items: center; gap: 8px; }
.plaza-filter-heading h2 { font-size: 13px; font-weight: 600; }
.plaza-catalog-reset { font-size: 11px; color: var(--signal-muted, #63636c); min-height: 28px; }
.plaza-catalog-reset:hover { color: var(--signal-text, #1c1c1e); }
.plaza-billing-filters { display: grid; gap: 10px; padding-top: 18px; border-top: 1px solid var(--signal-line, #dedee4); }
.plaza-billing-filters h3 { font-size: 12px; font-weight: 500; }
.plaza-billing-filters > div { display: flex; flex-wrap: wrap; gap: 6px; }
.plaza-billing-filters button { min-height: 28px; padding: 4px 8px; border: 1px solid var(--signal-line, #dedee4); border-radius: 6px; color: var(--signal-muted, #63636c); font-size: 11px; }
.plaza-billing-filters button[aria-checked='true'] { background: var(--signal-hover, #e9e9ef); color: var(--signal-text, #1c1c1e); border-color: var(--signal-control-line, #c7c7ce); }
.plaza-catalog-results { min-width: 0; }
.plaza-catalog-toolbar { display: flex; justify-content: space-between; align-items: center; gap: 12px; min-height: 34px; scroll-margin-top: 90px; }
.plaza-catalog-count { color: var(--signal-text, #1c1c1e); font-size: 12px; }
.plaza-catalog-controls { display: flex; align-items: center; gap: 10px; }
.plaza-sort { width: 142px; }
.plaza-sort :deep(.select-trigger) { min-height: 32px; padding-block: 5px; font-size: 12px; border-radius: 7px; background: transparent; }
.plaza-view-toggle { display: flex; gap: 2px; padding: 2px; border: 1px solid var(--signal-line, #dedee4); border-radius: 8px; }
.plaza-view-toggle button { display: grid; place-items: center; width: 28px; height: 28px; border-radius: 5px; color: var(--signal-muted, #63636c); }
.plaza-view-toggle button[aria-pressed='true'] { background: var(--signal-hover, #e9e9ef); color: var(--signal-text, #1c1c1e); }
.plaza-view-toggle svg { width: 14px; height: 14px; }
.plaza-catalog-price-hint { margin: 12px 0 16px; color: var(--signal-muted, #63636c); font-size: 11px; line-height: 18px; }
.plaza-catalog { display: grid; gap: 14px; }
.plaza-catalog-grid { grid-template-columns: repeat(2, minmax(0, 1fr)); }
.plaza-catalog-list { grid-template-columns: minmax(0, 1fr); gap: 8px; }
.plaza-mobile-filter-toggle { display: none; }
.plaza-filter-dot { width: 6px; height: 6px; border-radius: 50%; background: var(--signal-accent, #0066d6); }
.plaza-catalog-results :deep(.table-pagination) { margin-top: 22px; padding-inline: 0; border-top: 0; background: transparent; }
.plaza-catalog-layout button:focus-visible { outline: 2px solid var(--signal-accent, #0066d6); outline-offset: 2px; }
@media (min-width: 1280px) {
  .plaza-catalog-grid { grid-template-columns: repeat(3, minmax(0, 1fr)); }
}
@media (max-width: 1023px) {
  .plaza-catalog-layout { grid-template-columns: minmax(0, 1fr); gap: 16px; }
  .plaza-mobile-filter-toggle { display: inline-flex; justify-self: start; align-items: center; gap: 8px; min-height: 44px; padding: 8px 12px; border: 1px solid var(--signal-line, #dedee4); border-radius: 8px; color: var(--signal-text, #1c1c1e); font-size: 12px; }
  .plaza-catalog-sidebar { display: none; position: static; max-height: none; }
  .plaza-catalog-sidebar.is-open { display: grid; }
}
@media (max-width: 639px) {
  .plaza-catalog-grid { grid-template-columns: minmax(0, 1fr); }
  .plaza-catalog-controls { gap: 6px; }
  .plaza-catalog-toolbar { flex-wrap: wrap; }
  .plaza-sort { width: 134px; }
  .plaza-catalog-search { margin-bottom: 20px; }
  .plaza-catalog-search input { min-height: 44px; }
  .plaza-view-toggle button { width: 40px; height: 40px; }
  .plaza-sort :deep(.select-trigger) { min-height: 44px; }
  .plaza-billing-filters button, .plaza-catalog-reset { min-height: 44px; }
}



.plaza-description {
  border-bottom: 1px solid var(--signal-line, #dedee4);
  line-height: 1.7;
  overflow-wrap: anywhere;
}

.plaza-description :deep(h1),
.plaza-description :deep(h2),
.plaza-description :deep(h3) {
  @apply mb-2 mt-3 font-semibold text-gray-900 first:mt-0 dark:text-white;
}

.plaza-description :deep(p) {
  @apply mb-2 text-gray-700 last:mb-0 dark:text-dark-200;
}

.plaza-description :deep(a) {
  @apply text-primary-600 underline underline-offset-4 hover:text-primary-700 dark:text-primary-300;
}

.plaza-description :deep(ul) {
  @apply mb-2 list-disc pl-5;
}

.plaza-description :deep(ol) {
  @apply mb-2 list-decimal pl-5;
}

.plaza-description :deep(li) {
  @apply mb-0.5 text-gray-700 dark:text-dark-200;
}

.plaza-description :deep(code) {
  @apply rounded bg-gray-100 px-1.5 py-0.5 font-mono text-xs dark:bg-dark-800;
}

.plaza-description :deep(blockquote) {
  @apply my-2 border-l-4 border-gray-300 pl-3 text-gray-600 dark:border-dark-600 dark:text-dark-300;
}
</style>
