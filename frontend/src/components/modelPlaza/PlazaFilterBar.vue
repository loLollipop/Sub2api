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
      <div v-if="sidebar" class="plaza-group-options" role="radiogroup" :aria-label="t('modelPlaza.filters.groupLabel')" @keydown="plazaRadioKeyboard">
        <button
          v-for="option in groupChoices" :key="option.value" type="button" role="radio"
          class="plaza-group-option" :aria-checked="groupId === option.value"
          :tabindex="groupTabStop === option.value ? 0 : -1"
          @click="onGroupChange(option.value)"
        >
          <span>{{ option.label }} <span v-if="option.rate !== undefined" class="plaza-group-rate">{{ option.rate }}x</span></span>
          <span class="plaza-group-count">{{ option.count }}</span>
        </button>
      </div>
      <Select
        v-else
        :model-value="groupId"
        :options="groupSelectOptions"
        :aria-label="t('modelPlaza.filters.groupLabel')"
        class="plaza-select"
        @update:model-value="onGroupChange"
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
            :tabindex="platformTabStop === p ? 0 : -1"
            @click="$emit('update:platform', p)"
          >
            <PlazaProviderIcon v-if="p !== 'all'" :platform="p" size="xs" />
            {{ p === 'all' ? t('modelPlaza.filters.all') : p === 'other' ? t('modelPlaza.filters.otherProviders') : providerLabel(p) }}
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
import { providerLabel } from './modelPlazaProviders'
import { plazaRadioKeyboard } from './plazaRadioKeyboard'

const props = defineProps<{
  /** 当前分组中出现的真实模型品牌，独立于接入协议。 */
  platforms: string[]
  /** 所有分组始终可选，倍率仅在名称旁展示。 */
  groups: Array<{ id: number; name: string; platforms: string[]; rate: number; modelCount?: number }>
  platform: string
  groupId: number | 'all'
  /** 模型名搜索词(纯前端过滤)。 */
  search: string
  hideSearch?: boolean
  sidebar?: boolean
}>()

const emit = defineEmits<{
  'update:platform': [value: string]
  'update:groupId': [value: number | 'all']
  'update:search': [value: string]
  reset: []
}>()

const { t } = useI18n()

const hasActiveFilters = computed(() => props.platform !== 'all' || props.groupId !== 'all' || props.search.trim() !== '')

const platformChoices = computed(() => ['all', ...props.platforms.filter(platform => props.groupId === 'all' ||
  props.groups.some(group => group.id === props.groupId && group.platforms.includes(platform)))])

const groupChoices = computed(() => [
  { value: 'all' as const, label: t('modelPlaza.filters.all'), count: props.groups.reduce((count, group) => count + (group.modelCount ?? 0), 0), rate: undefined },
  ...props.groups.map(group => ({ value: group.id, label: group.name, count: group.modelCount ?? 0, rate: group.rate }))
])

const groupTabStop = computed(() => groupChoices.value.some(option => option.value === props.groupId) ? props.groupId : 'all')
const platformTabStop = computed(() => platformChoices.value.includes(props.platform) ? props.platform : 'all')

const groupSelectOptions = computed<SelectOption[]>(() => [
  { value: 'all', label: t('modelPlaza.filters.groupLabel') },
  ...props.groups.map((g) => ({
    value: g.id,
    label: `${g.name} ${g.rate}x`
  }))
])

function onGroupChange(value: string | number | boolean | null) {
  emit('update:groupId', value === 'all' || value === null ? 'all' : Number(value))
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
  grid-template-columns: minmax(0, 1fr) minmax(168px, 260px);
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

.plaza-filters-sidebar { gap: 22px; }
.plaza-filters-sidebar .plaza-toolbar { display: flex; flex-direction: column; gap: 8px; }
.plaza-group-options { display: grid; gap: 3px; }
.plaza-group-option { display: flex; justify-content: space-between; align-items: center; gap: 10px; min-height: 36px; padding: 7px 10px; border: 1px solid transparent; border-radius: 7px; font-size: 12px; text-align: left; color: var(--signal-muted, #63636c); }
.plaza-group-option > span:first-child { min-width: 0; overflow-wrap: anywhere; }
.plaza-group-option[aria-checked='true'] { background: var(--signal-hover, #e9e9ef); color: var(--signal-text, #1c1c1e); border-color: color-mix(in srgb, var(--signal-line, #dedee4) 60%, transparent); font-weight: 500; }
.plaza-group-option:not(:disabled):hover { background: color-mix(in srgb, var(--signal-hover, #e9e9ef) 60%, transparent); }
.plaza-group-option:focus-visible { outline: 2px solid var(--signal-accent, #0066d6); outline-offset: -2px; }
.plaza-group-count { flex-shrink: 0; color: var(--signal-muted, #63636c); font-size: 11px; font-variant-numeric: tabular-nums; }
.plaza-group-rate { margin-left: 4px; color: var(--signal-muted, #63636c); font-size: 11px; font-weight: 400; white-space: nowrap; }
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
  .plaza-group-option { min-height: 44px; }
}
</style>
