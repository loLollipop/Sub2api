<template>
  <div class="plaza-filters" :class="{ 'plaza-filters-sidebar': sidebar }">
    <div class="plaza-toolbar">
      <div v-if="!hideSearch" class="plaza-search">
        <Icon name="search" size="sm" class="plaza-search-icon" />
        <input
          :value="search"
          type="text"
          :placeholder="t('modelPlaza.filters.searchPlaceholder')"
          :aria-label="t('modelPlaza.filters.modelLabel')"
          class="input plaza-search-input"
          @input="$emit('update:search', ($event.target as HTMLInputElement).value)"
        />
        <button
          v-if="search"
          type="button"
          class="plaza-search-clear"
          :aria-label="t('common.clear')"
          @click="$emit('update:search', '')"
        >
          <Icon name="x" size="xs" class="h-3.5 w-3.5" />
        </button>
      </div>
      <p v-if="sidebar" class="plaza-filter-label">{{ t('modelPlaza.filters.groupLabel') }}</p>
      <Select
        :model-value="groupId"
        :options="groupSelectOptions"
        :aria-label="t('modelPlaza.filters.groupLabel')"
        class="plaza-select"
        @update:model-value="onGroupChange"
      />
      <p v-if="sidebar" class="plaza-filter-label">{{ t('modelPlaza.filters.rateLabel') }}</p>
      <Select
        :model-value="rate"
        :options="rateSelectOptions"
        :aria-label="t('modelPlaza.filters.rateLabel')"
        class="plaza-select plaza-select-rate"
        @update:model-value="onRateChange"
      />
    </div>

    <div class="plaza-platform-toolbar">
      <p v-if="sidebar" class="plaza-filter-label">{{ t('modelPlaza.filters.platformLabel') }}</p>
      <div class="plaza-seg-wrap">
        <div
          class="plaza-seg"
          role="radiogroup"
          :aria-label="t('modelPlaza.filters.platformLabel')"
          @keydown="plazaRadioKeyboard"
        >
          <button
            v-for="p in platformChoices"
            :key="`platform-${p}`"
            type="button"
            role="radio"
            class="plaza-seg-item"
            :aria-checked="platform === p"
            :tabindex="platform === p ? 0 : -1"
            :disabled="p !== 'all' && !platformEnabled(p)"
            @click="$emit('update:platform', p)"
          >
            <PlazaProviderIcon v-if="p !== 'all'" :platform="p" size="xs" />
            {{ p === 'all' ? t('modelPlaza.filters.all') : platformLabel(p) }}
          </button>
        </div>
      </div>
      <button v-if="hasActiveFilters && !sidebar" type="button" class="plaza-reset" data-testid="plaza-clear-filters" @click="$emit('reset')">
        {{ t('modelPlaza.filters.clearAll') }}
      </button>
    </div>
  </div>
</template>

<script setup lang="ts">
import { computed } from 'vue'
import { useI18n } from 'vue-i18n'
import Icon from '@/components/icons/Icon.vue'
import PlazaProviderIcon from './PlazaProviderIcon.vue'
import Select, { type SelectOption } from '@/components/common/Select.vue'
import { platformLabel } from '@/utils/platformColors'
import { plazaRadioKeyboard } from './plazaRadioKeyboard'

const props = defineProps<{
  /** 数据中出现的平台(去重排序后)。 */
  platforms: string[]
  /** 全量分组(含平台与生效倍率),三个维度的置灰联动由此推导。 */
  groups: Array<{ id: number; name: string; platform: string; platforms?: string[]; rate: number; rates?: number[] }>
  /** 全量生效倍率去重升序。 */
  rates: number[]
  platform: string
  groupId: number | 'all'
  rate: number | 'all'
  /** 模型名搜索词(纯前端过滤)。 */
  search: string
  hideSearch?: boolean
  sidebar?: boolean
}>()

const emit = defineEmits<{
  'update:platform': [value: string]
  'update:groupId': [value: number | 'all']
  'update:rate': [value: number | 'all']
  'update:search': [value: string]
  reset: []
}>()

const { t } = useI18n()

const hasActiveFilters = computed(() => props.platform !== 'all' || props.groupId !== 'all' || props.rate !== 'all' || props.search.trim() !== '')

const platformChoices = computed(() => ['all', ...props.platforms])

const groupSelectOptions = computed<SelectOption[]>(() => [
  { value: 'all', label: t('modelPlaza.filters.groupLabel') },
  ...props.groups.map((g) => ({
    value: g.id,
    label: g.name,
    disabled: !groupEnabled(g)
  }))
])

const rateSelectOptions = computed<SelectOption[]>(() => [
  { value: 'all', label: t('modelPlaza.filters.rateLabel') },
  ...props.rates.map((r) => ({
    value: r,
    label: `${r}x`,
    disabled: !rateEnabled(r)
  }))
])

/**
 * 三个维度互为约束(faceted):某选项可点 ⟺ 在「其他两维」当前选择下仍有分组命中。
 * 「全部」永远可点,作为解除本维约束的出口;可点项组合恒有结果,无需选择修正。
 */
function matchesRate(g: { rate: number; rates?: number[] }, rate: number): boolean {
  return (g.rates ?? [g.rate]).includes(rate)
}

function platformEnabled(p: string): boolean {
  return props.groups.some(
    (g) =>
      (g.platforms ?? [g.platform]).includes(p) &&
      (props.groupId === 'all' || g.id === props.groupId) &&
      (props.rate === 'all' || matchesRate(g, props.rate))
  )
}

function groupEnabled(g: { platform: string; platforms?: string[]; rate: number; rates?: number[] }): boolean {
  return (
    (props.platform === 'all' || (g.platforms ?? [g.platform]).includes(props.platform)) &&
    (props.rate === 'all' || matchesRate(g, props.rate))
  )
}

function rateEnabled(r: number): boolean {
  return props.groups.some(
    (g) =>
      matchesRate(g, r) &&
      (props.platform === 'all' || (g.platforms ?? [g.platform]).includes(props.platform)) &&
      (props.groupId === 'all' || g.id === props.groupId)
  )
}

function onGroupChange(value: string | number | boolean | null) {
  emit('update:groupId', value === 'all' || value === null ? 'all' : Number(value))
}

function onRateChange(value: string | number | boolean | null) {
  emit('update:rate', value === 'all' || value === null ? 'all' : Number(value))
}
</script>

<style scoped>
.plaza-filters {
  display: flex;
  flex-direction: column;
  gap: 10px;
  min-width: 0;
}

.plaza-toolbar {
  display: grid;
  grid-template-columns: minmax(0, 1fr) minmax(168px, 220px) minmax(112px, 150px);
  gap: 8px;
  align-items: stretch;
}

.plaza-search {
  position: relative;
  min-width: 0;
}

.plaza-search-icon {
  position: absolute;
  top: 50%;
  left: 12px;
  z-index: 1;
  color: var(--signal-muted, #63636c);
  transform: translateY(-50%);
  pointer-events: none;
}

.plaza-search-input {
  width: 100%;
  min-height: 36px;
  padding: 8px 36px 8px 36px;
  border-radius: 8px;
  background: color-mix(in srgb, var(--signal-muted, #63636c) 4%, transparent);
}

.plaza-search-clear {
  position: absolute;
  top: 50%;
  right: 8px;
  display: grid;
  place-items: center;
  width: 24px;
  height: 24px;
  border: 0;
  border-radius: 8px;
  background: transparent;
  color: var(--signal-muted, #63636c);
  transform: translateY(-50%);
}

.plaza-search-clear:hover {
  color: var(--signal-text, #1c1c1e);
}

.plaza-select {
  min-width: 0;
}

.plaza-platform-toolbar { display: flex; align-items: center; gap: 10px; min-width: 0; }
.plaza-platform-toolbar > .plaza-seg-wrap { flex: 1; }
.plaza-reset { flex-shrink: 0; min-height: 36px; padding: 6px 8px; border-radius: 7px; color: var(--signal-muted, #63636c); font-size: 12px; }
.plaza-reset:hover { color: var(--signal-text, #202426); background: var(--signal-hover, #f3f4f6); }
.plaza-reset:focus-visible { outline: 2px solid var(--signal-accent, #007db5); outline-offset: 2px; }

.plaza-seg-wrap {
  min-width: 0;
  overflow-x: auto;
  -webkit-overflow-scrolling: touch;
}

.plaza-seg {
  display: inline-flex;
  min-height: 36px;
  padding: 2px 0;
  border-bottom: 1px solid var(--signal-line, #dde2e4);
  gap: 6px;
}

.plaza-seg-item {
  display: inline-flex;
  align-items: center;
  gap: 6px;
  min-height: 32px;
  padding: 0 12px;
  border: 0;
  border-bottom: 2px solid transparent;
  border-radius: 0;
  background: transparent;
  color: var(--signal-muted, #63636c);
  font-size: 13px;
  line-height: 18px;
  white-space: nowrap;
}

.plaza-seg-item[aria-checked='true'] {
  border-bottom-color: var(--signal-accent, #007db5);
  color: var(--signal-text, #202426);
}

.plaza-seg-item:disabled {
  cursor: not-allowed;
  opacity: 0.4;
}

.plaza-filters-sidebar { gap: 22px; }
.plaza-filters-sidebar .plaza-toolbar { display: flex; flex-direction: column; order: 2; gap: 8px; }
.plaza-filter-label { color: var(--signal-text, #1c1c1e); font-size: 12px; font-weight: 500; line-height: 18px; }
.plaza-filters-sidebar .plaza-filter-label:not(:first-child) { margin-top: 12px; }
.plaza-filters-sidebar .plaza-platform-toolbar { display: flex; flex-direction: column; align-items: stretch; gap: 10px; }
.plaza-filters-sidebar .plaza-seg { display: grid; width: 100%; gap: 3px; border-bottom: 0; padding: 0; }
.plaza-filters-sidebar .plaza-seg-item { justify-content: flex-start; gap: 10px; min-height: 34px; padding: 7px 10px; border: 1px solid transparent; border-radius: 7px; font-size: 12px; }
.plaza-filters-sidebar .plaza-seg-item[aria-checked='true'] { background: var(--signal-hover, #e9e9ef); border-color: color-mix(in srgb, var(--signal-line, #dedee4) 60%, transparent); font-weight: 500; }
.plaza-filters-sidebar .plaza-seg-item:not(:disabled):hover { background: color-mix(in srgb, var(--signal-hover, #e9e9ef) 60%, transparent); }
.plaza-seg-item:focus-visible { outline: 2px solid var(--signal-accent, #0066d6); outline-offset: -2px; }

@media (max-width: 767px) {
  .plaza-toolbar {
    grid-template-columns: minmax(0, 1fr) minmax(0, 1fr);
  }

  .plaza-search {
    grid-column: 1 / -1;
  }

  .plaza-seg-item {
    min-height: 44px;
  }
  .plaza-filters-sidebar .plaza-seg { display: flex; flex-wrap: wrap; }
  .plaza-filters-sidebar .plaza-seg-item { min-height: 44px; }
}
</style>
