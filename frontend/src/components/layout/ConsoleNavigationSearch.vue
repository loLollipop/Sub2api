<template>
  <button type="button" class="console-search-trigger btn btn-secondary" :class="expanded ? 'console-search-expanded' : 'btn-icon'" :title="t('common.search')" :aria-label="t('common.search')" aria-haspopup="dialog" @click="open = true">
    <Icon name="search" size="sm" />
    <span v-if="expanded" class="console-search-hint">{{ t('common.search') }}</span>
  </button>
  <BaseDialog :show="open" :title="t('common.search')" width="normal" :close-on-click-outside="true" @close="open = false">
    <div class="console-navigation-search">
      <div class="relative">
        <Icon name="search" size="md" class="pointer-events-none absolute left-3 top-3 text-gray-400" />
        <input ref="searchInput" v-model="query" class="input pl-10" :aria-label="t('common.search')" :placeholder="t('common.search')" autocomplete="off" @keydown.enter="handleEnter" />
      </div>
      <nav v-if="results.length" class="console-search-results" :aria-label="t('common.search')">
        <button v-for="item in results" :key="item.path" type="button" class="console-search-result" @click="navigate(item.path)">
          <span v-if="item.iconSvg" class="console-search-icon" aria-hidden="true" v-html="sanitizeSvg(item.iconSvg)"></span>
          <component v-else :is="item.icon" class="h-5 w-5 shrink-0" />
          <span class="console-search-label">{{ item.label }}</span>
          <Icon name="chevronRight" size="sm" />
        </button>
      </nav>
      <p v-else class="py-8 text-center text-sm text-gray-500">{{ t('common.noData') }}</p>
    </div>
  </BaseDialog>
</template>

<script setup lang="ts">
import { computed, nextTick, ref, watch } from 'vue'
import { useI18n } from 'vue-i18n'
import { useRouter } from 'vue-router'
import { useAuthStore } from '@/stores/auth'
import { useAppStore } from '@/stores/app'
import { useConsoleNavigation, type NavItem } from '@/composables/useConsoleNavigation'
import { sanitizeSvg } from '@/utils/sanitize'
import BaseDialog from '@/components/common/BaseDialog.vue'
import Icon from '@/components/icons/Icon.vue'

withDefaults(defineProps<{ expanded?: boolean }>(), { expanded: false })

const { t } = useI18n()
const router = useRouter()
const auth = useAuthStore()
const app = useAppStore()
const open = ref(false)
const query = ref('')
const searchInput = ref<HTMLInputElement | null>(null)
const { userNavItems, adminNavItems } = useConsoleNavigation()
function flattenDestinations(items: NavItem[]): NavItem[] {
  return items.flatMap(item => item.children?.length
    ? [...(item.expandOnly ? [] : [item]), ...flattenDestinations(item.children)]
    : [item])
}
const destinations = computed(() => {
  if (!auth.isAdmin) return app.backendModeEnabled ? [] : userNavItems.value
  return flattenDestinations([
    ...adminNavItems.value,
    ...(!auth.isSimpleMode ? userNavItems.value : []),
  ])
})
const results = computed(() => {
  const term = query.value.trim().toLocaleLowerCase()
  return destinations.value.filter(item => !term || (item.label+' '+item.path).toLocaleLowerCase().includes(term))
})

watch(open, async (value) => {
  if (!value) return
  query.value = ''
  await nextTick()
  searchInput.value?.focus()
})

function navigate(path?: string) {
  if (!path) return
  open.value = false
  void router.push(path)
}

function handleEnter(event: KeyboardEvent) {
  if (event.isComposing || event.keyCode === 229) return
  event.preventDefault()
  navigate(results.value[0]?.path)
}
</script>

<style scoped>
.console-search-results { display: grid; gap: 4px; margin-top: 14px; max-height: 360px; overflow-y: auto; }
.console-search-result { display: flex; align-items: center; gap: 12px; min-height: 44px; padding: 10px 12px; border: 1px solid transparent; border-radius: 6px; color: var(--signal-text); text-align: left; font-size: 14px; }
.console-search-result > svg:first-child { color: var(--signal-muted); flex-shrink: 0; }
.console-search-label { flex: 1; min-width: 0; }
.console-search-icon { flex: 0 0 20px; width: 20px; height: 20px; overflow: hidden; }
.console-search-icon :deep(svg) { display: block; width: 100%; height: 100%; }
.console-search-result > svg:last-child { color: var(--signal-muted); opacity: 0; }
.console-search-result:hover, .console-search-result:focus-visible { border-color: var(--signal-line); background: var(--signal-hover); }
.console-search-result:hover > svg:last-child, .console-search-result:focus-visible > svg:last-child { opacity: 1; }
</style>
