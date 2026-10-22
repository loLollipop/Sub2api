<template>
  <aside
    id="mobile-console-navigation"
    ref="sidebarRef"
    class="sidebar"
    :inert="!isDesktop && !mobileOpen ? true : undefined"
    :aria-hidden="!isDesktop && !mobileOpen ? true : undefined"
    tabindex="-1"
    :class="[
      sidebarCollapsed ? 'w-[72px]' : 'w-64',
      { '-translate-x-full lg:translate-x-0': !mobileOpen, 'signal-sidebar': isConsoleSignal }
    ]"
  >
    <!-- Logo/Brand -->
    <div class="sidebar-header" :class="{ 'sidebar-header-collapsed': sidebarCollapsed }">
      <!-- Custom Logo or Default Logo -->
      <router-link
        to="/"
        class="sidebar-logo flex h-9 w-9 items-center justify-center overflow-hidden rounded-xl shadow-glow transition-opacity hover:opacity-80"
        @click="handleMenuItemClick('/')"
      >
        <img v-if="settingsLoaded" :src="siteLogo || '/logo.svg'" alt="Logo" class="h-full w-full object-contain" />
      </router-link>
      <div class="sidebar-brand" :class="{ 'sidebar-brand-collapsed': sidebarCollapsed }" :aria-hidden="sidebarCollapsed ? 'true' : 'false'">
        <router-link
          to="/"
          class="sidebar-brand-title brand-gradient text-lg font-bold"
          @click="handleMenuItemClick('/')"
        >
          {{ siteName }}
        </router-link>
        <!-- Version Badge -->
        <VersionBadge :version="siteVersion" />
      </div>
    </div>

    <!-- Navigation -->
    <nav ref="sidebarNavRef" class="sidebar-nav scrollbar-hide">
      <!-- Admin View: Admin menu first, then personal menu -->
      <template v-if="isAdmin">
        <!-- Admin Section -->
        <div class="sidebar-section">
          <template v-for="item in adminNavItems" :key="item.path">
            <!-- Branches open inline; the icon rail keeps just the parent. -->
            <template v-if="item.children?.length">
              <button
                type="button"
                class="sidebar-link sidebar-branch mb-1 w-full text-left"
                :class="{ 'sidebar-link-active': item.children.some(child => isActive(child.path)), 'sidebar-link-collapsed': sidebarCollapsed, 'sidebar-branch-open': isBranchOpen(item.path) }"
                :title="sidebarCollapsed ? item.label : undefined"
                :aria-label="sidebarCollapsed ? item.label : undefined"
                :aria-expanded="isBranchOpen(item.path)"
                :aria-controls="branchId(item.path)"
                :data-branch="item.path"
                @click="toggleBranch(item.path)"
              >
                <component :is="item.icon" class="h-5 w-5 flex-shrink-0" />
                <span class="sidebar-label" :class="{ 'sidebar-label-collapsed': sidebarCollapsed }" :aria-hidden="sidebarCollapsed ? 'true' : 'false'">{{ item.label }}</span>
              </button>
              <div
                :id="branchId(item.path)"
                class="sidebar-drawer"
                :class="{ 'sidebar-drawer-open': isBranchOpen(item.path) }"
                :inert="!isBranchOpen(item.path) ? true : undefined"
                :aria-hidden="!isBranchOpen(item.path) ? true : undefined"
              >
                <div class="sidebar-drawer-clip">
                  <div class="sidebar-children">
                    <router-link
                      v-for="child in item.children"
                      :key="child.path"
                      :to="child.path"
                      class="sidebar-link sidebar-sublink"
                      :class="{ 'sidebar-link-active': isActive(child.path) }"
                      :id="child.path === '/admin/redeem' ? 'sidebar-wallet' : undefined"
                      @click="handleMenuItemClick(child.path)"
                    >
                      <span class="sidebar-child-dot" aria-hidden="true"></span>
                      <span class="sidebar-label">{{ child.label }}</span>
                    </router-link>
                  </div>
                </div>
              </div>
            </template>
            <!-- Normal item (no children) -->
            <router-link
              v-else-if="!item.expandOnly"
              :to="item.path"
              class="sidebar-link mb-1"
              :class="{ 'sidebar-link-active': isActive(item.path), 'sidebar-link-collapsed': sidebarCollapsed }"
              :title="sidebarCollapsed ? item.label : undefined"
              :aria-label="sidebarCollapsed ? item.label : undefined"
              :data-tour="item.path === '/keys' ? 'sidebar-my-keys' : undefined"
              :id="
                item.path === '/admin/accounts'
                  ? 'sidebar-channel-manage'
                  : item.path === '/admin/groups'
                    ? 'sidebar-group-manage'
                    : item.path === '/admin/redeem'
                      ? 'sidebar-wallet'
                      : undefined
              "
              @click="handleMenuItemClick(item.path)"
            >
              <span v-if="item.iconSvg" class="h-5 w-5 flex-shrink-0 sidebar-svg-icon" v-html="sanitizeSvg(item.iconSvg)"></span>
              <component v-else :is="item.icon" class="h-5 w-5 flex-shrink-0" />
              <span class="sidebar-label" :class="{ 'sidebar-label-collapsed': sidebarCollapsed }" :aria-hidden="sidebarCollapsed ? 'true' : 'false'">{{ item.label }}</span>
            </router-link>
          </template>
        </div>

        <!-- Personal tools stay inside the admin workspace. -->
        <div class="sidebar-section">
          <div class="sidebar-section-title" :class="{ 'sidebar-section-title-collapsed': sidebarCollapsed }" :aria-hidden="sidebarCollapsed ? 'true' : 'false'">
            <span class="sidebar-section-title-text" :class="{ 'sidebar-section-title-text-collapsed': sidebarCollapsed }">
              {{ t('nav.myAccount') }}
            </span>
          </div>

          <router-link
            v-for="item in personalNavItems"
            :key="item.path"
            :to="item.path"
            class="sidebar-link mb-1"
            :class="{ 'sidebar-link-active': isActive(item.path), 'sidebar-link-collapsed': sidebarCollapsed }"
            :title="sidebarCollapsed ? item.label : undefined"
            :aria-label="sidebarCollapsed ? item.label : undefined"
            :data-tour="item.path === '/admin/keys' ? 'sidebar-my-keys' : undefined"
            @click="handleMenuItemClick(item.path)"
          >
            <span v-if="item.iconSvg" class="h-5 w-5 flex-shrink-0 sidebar-svg-icon" v-html="sanitizeSvg(item.iconSvg)"></span>
            <component v-else :is="item.icon" class="h-5 w-5 flex-shrink-0" />
            <span class="sidebar-label" :class="{ 'sidebar-label-collapsed': sidebarCollapsed }" :aria-hidden="sidebarCollapsed ? 'true' : 'false'">{{ item.label }}</span>
          </router-link>
        </div>
      </template>

      <!-- Regular User View -->
      <template v-else-if="!appStore.backendModeEnabled || authStore.isAdmin">
        <div class="sidebar-section">
          <router-link
            v-for="item in visibleUserItems"
            :key="item.path"
            :to="item.path"
            class="sidebar-link mb-1"
            :class="{ 'sidebar-link-active': isActive(item.path), 'sidebar-link-collapsed': sidebarCollapsed }"
            :title="sidebarCollapsed ? item.label : undefined"
            :aria-label="sidebarCollapsed ? item.label : undefined"
            :data-tour="item.path === '/keys' ? 'sidebar-my-keys' : undefined"
            @click="handleMenuItemClick(item.path)"
          >
            <span v-if="item.iconSvg" class="h-5 w-5 flex-shrink-0 sidebar-svg-icon" v-html="sanitizeSvg(item.iconSvg)"></span>
            <component v-else :is="item.icon" class="h-5 w-5 flex-shrink-0" />
            <span class="sidebar-label" :class="{ 'sidebar-label-collapsed': sidebarCollapsed }" :aria-hidden="sidebarCollapsed ? 'true' : 'false'">{{ item.label }}</span>
          </router-link>
        </div>
      </template>
    </nav>

  </aside>

  <!-- Mobile Overlay -->
  <transition name="fade">
    <div
      v-if="mobileOpen"
      class="fixed inset-0 z-40 bg-black/50 lg:hidden"
      @click="closeMobile"
    ></div>
  </transition>
</template>

<script setup lang="ts">
import { computed, nextTick, onBeforeUnmount, onMounted, ref, watch } from 'vue'
import { useRoute } from 'vue-router'
import { useI18n } from 'vue-i18n'
import { useAdminSettingsStore, useAppStore, useAuthStore, useOnboardingStore } from '@/stores'
import VersionBadge from '@/components/common/VersionBadge.vue'
import { sanitizeSvg } from '@/utils/sanitize'
import { sanitizeUrl } from '@/utils/url'
import { useConsoleNavigation } from '@/composables/useConsoleNavigation'
import { useMediaQuery } from '@vueuse/core'
import { useConsoleSignal } from '@/composables/useConsoleSignal'
import { useConsoleWorkspace } from '@/composables/useConsoleWorkspace'

const { t } = useI18n()

const route = useRoute()
const { isConsoleSignal } = useConsoleSignal()
const appStore = useAppStore()
const authStore = useAuthStore()
const onboardingStore = useOnboardingStore()
const adminSettingsStore = useAdminSettingsStore()
const { userNavItems, personalNavItems, adminNavItems, refreshBatchImageAccess } = useConsoleNavigation()

const isDesktop = useMediaQuery('(min-width: 1024px)')
const sidebarCollapsed = computed(() => isDesktop.value && appStore.sidebarCollapsed)
const sidebarRef = ref<HTMLElement | null>(null)
const mobileOpen = computed(() => appStore.mobileOpen)
const { isAdminWorkspace: isAdmin } = useConsoleWorkspace()
const sidebarNavRef = ref<HTMLElement | null>(null)


const browsePaths = new Set(['/available-channels', '/monitor'])
const visibleUserItems = computed(() => userNavItems.value.filter(item => !isDesktop.value || !browsePaths.has(item.path)))

function branchId(path: string) {
  return `sidebar-branch-${path.split('/').filter(Boolean).join('-')}`
}

function isBranchOpen(path: string) {
  return !sidebarCollapsed.value && appStore.sidebarExpandedGroups[path] === true
}

function toggleBranch(path: string) {
  if (sidebarCollapsed.value) {
    appStore.sidebarCollapsed = false
    appStore.sidebarExpandedGroups[path] = true
    return
  }
  appStore.sidebarExpandedGroups[path] = !isBranchOpen(path)
}

watch([() => route.path, adminNavItems], () => {
  if (!isAdmin.value) return
  for (const item of adminNavItems.value) {
    if (item.children?.some(child => isActive(child.path))) {
      appStore.sidebarExpandedGroups[item.path] = true
    }
  }
}, { immediate: true })

// Site settings from appStore (cached, no flicker)
const siteName = computed(() => appStore.siteName)
const siteLogo = computed(() => sanitizeUrl(appStore.siteLogo || '', { allowRelative: true, allowDataUrl: true }))
const siteVersion = computed(() => appStore.siteVersion)
const settingsLoaded = computed(() => appStore.publicSettingsLoaded)

function closeMobile() {
  appStore.setMobileOpen(false)
  void nextTick(() => document.getElementById('mobile-navigation-trigger')?.focus())
}

function handleMenuItemClick(itemPath: string) {
  if (mobileOpen.value) {
    closeMobile()
  }

  // Map paths to tour selectors
  const pathToSelector: Record<string, string> = {
    '/admin/groups': '#sidebar-group-manage',
    '/admin/accounts': '#sidebar-channel-manage',
    '/keys': '[data-tour="sidebar-my-keys"]',
    '/admin/keys': '[data-tour="sidebar-my-keys"]'
  }

  const selector = pathToSelector[itemPath]
  if (selector && onboardingStore.isCurrentStep(selector)) {
    onboardingStore.nextStep(500)
  }
}

function isActive(path: string): boolean {
  return route.path === path || route.path.startsWith(path + '/')
}

function mobileFocusTargets() {
  return Array.from(sidebarRef.value?.querySelectorAll<HTMLElement>('a[href], button:not(:disabled)') ?? [])
    .filter(item => !item.closest('[inert]'))
}

function handleMobileKeydown(event: KeyboardEvent) {
  if (isDesktop.value || !mobileOpen.value || event.defaultPrevented ||
      document.body.classList.contains('modal-open') || event.isComposing) return
  if (event.key === 'Escape') {
    event.preventDefault()
    closeMobile()
    return
  }
  if (event.key !== 'Tab') return
  const items = mobileFocusTargets()
  const first = items[0]
  const last = items.at(-1)
  if (!first || !last) return
  if (event.shiftKey && (document.activeElement === first || !sidebarRef.value?.contains(document.activeElement))) {
    event.preventDefault()
    last.focus()
  } else if (!event.shiftKey && (document.activeElement === last || !sidebarRef.value?.contains(document.activeElement))) {
    event.preventDefault()
    first.focus()
  }
}

watch([mobileOpen, isDesktop], async ([open, desktop]) => {
  if (!open || desktop) return
  await nextTick()
  if (mobileOpen.value && !isDesktop.value) mobileFocusTargets()[0]?.focus()
}, { immediate: true })
watch(() => route.fullPath, () => {
  if (mobileOpen.value && !isDesktop.value) closeMobile()
})

// Fetch admin settings (for feature-gated nav items like Ops).
watch(
  () => authStore.isAdmin,
  (v) => {
    if (v) {
      adminSettingsStore.fetch()
    }
  },
  { immediate: true }
)

onMounted(() => {
  document.addEventListener('keydown', handleMobileKeydown)
  void refreshBatchImageAccess()
  if (isAdmin.value) {
    adminSettingsStore.fetch()
  }
  // Restore sidebar scroll position after route change re-mounts the component
  if (appStore.sidebarScrollTop > 0 && sidebarNavRef.value) {
    void nextTick(() => {
      if (sidebarNavRef.value) {
        sidebarNavRef.value.scrollTop = appStore.sidebarScrollTop
      }
    })
  }
})

onBeforeUnmount(() => {
  document.removeEventListener('keydown', handleMobileKeydown)
  if (sidebarNavRef.value) {
    appStore.sidebarScrollTop = sidebarNavRef.value.scrollTop
  }
})
</script>

<style scoped>
.sidebar-drawer { display: grid; grid-template-rows: 0fr; opacity: 0; transition: grid-template-rows 200ms ease, opacity 160ms ease; }
.sidebar-drawer-open { grid-template-rows: 1fr; opacity: 1; }
.sidebar-drawer-clip { min-height: 0; overflow: hidden; }
.sidebar-children { display: grid; gap: 2px; margin: 0 0 6px 23px; padding: 3px; border-radius: 8px; background: var(--signal-hover); }
.sidebar.signal-sidebar .sidebar-children .sidebar-link.sidebar-sublink { min-height: 32px; gap: 9px; padding: 6px 10px; border-radius: 5px; font-size: 12px; font-weight: 400; color: var(--signal-muted); }
.sidebar.signal-sidebar .sidebar-children .sidebar-sublink.sidebar-link-active { color: var(--signal-accent); background: var(--signal-accent-soft); font-weight: 500; }
.sidebar-child-dot { flex: 0 0 4px; width: 4px; height: 4px; border-radius: 50%; background: currentColor; opacity: .55; }
.sidebar-link-active .sidebar-child-dot { opacity: 1; }
.sidebar-branch:focus-visible { outline: 2px solid var(--signal-accent); outline-offset: 2px; }
@media (max-width: 1023px) {
  .sidebar.signal-sidebar .sidebar-children .sidebar-link.sidebar-sublink { min-height: 44px; }
}
@media (prefers-reduced-motion: reduce) {
  .sidebar-drawer { transition: none; }
}

.sidebar-logo {
  flex: 0 0 2.25rem;
  min-width: 2.25rem;
}

.sidebar-header-collapsed {
  gap: 0;
  padding-left: 1.125rem;
  padding-right: 1.125rem;
}

.sidebar-brand {
  min-width: 0;
  flex: 1 1 auto;
  white-space: nowrap;
  transition:
    max-width 0.22s ease,
    opacity 0.14s ease,
    transform 0.14s ease;
  max-width: 12rem;
}

.sidebar-brand-collapsed {
  max-width: 0;
  overflow: hidden;
  opacity: 0;
  transform: translateX(-4px);
  pointer-events: none;
}

.sidebar-brand-title {
  display: block;
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
}

.sidebar-link-collapsed {
  gap: 0;
  padding-left: 0.875rem;
  padding-right: 0.875rem;
}

.sidebar-section-title {
  position: relative;
  display: flex;
  align-items: center;
  min-height: 1.25rem;
  overflow: hidden;
  white-space: nowrap;
}

.sidebar-section-title-text {
  display: block;
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
  transition:
    opacity 0.16s ease,
    transform 0.16s ease;
}

.sidebar-section-title::after {
  content: '';
  position: absolute;
  left: 0.75rem;
  right: 0.75rem;
  top: 50%;
  height: 1px;
  background: rgb(229 231 235);
  opacity: 0;
  transform: translateY(-50%);
  transition: opacity 0.18s ease;
}

.dark .sidebar-section-title::after {
  background: rgb(55 65 81);
}

.sidebar-section-title-text-collapsed {
  opacity: 0;
  transform: translateX(-4px);
}

.sidebar-section-title-collapsed::after {
  opacity: 1;
  transition-delay: 0.08s;
}

.sidebar-label {
  display: block;
  min-width: 0;
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
  transition:
    max-width 0.2s ease,
    opacity 0.12s ease,
    transform 0.12s ease;
  max-width: 12rem;
}

.sidebar-label-flex {
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: 0.5rem;
}

.sidebar-label-collapsed {
  max-width: 0;
  opacity: 0;
  transform: translateX(-4px);
  pointer-events: none;
}

/* Custom SVG icon in sidebar: constrain size without overriding uploaded SVG colors */
.sidebar-svg-icon {
  color: currentColor;
}

.sidebar-svg-icon :deep(svg) {
  display: block;
  width: 1.25rem;
  height: 1.25rem;
}
</style>
