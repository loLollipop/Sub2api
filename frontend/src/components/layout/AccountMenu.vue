<template>
  <div v-if="user" ref="dropdownRef" class="account-menu relative" @keydown.esc.stop.prevent="closeDropdown(true)" @focusout="handleFocusOut">
    <button
      ref="triggerRef"
      type="button"
      class="header-user-button flex items-center gap-2 rounded-xl p-1.5 transition-colors hover:bg-gray-100 dark:hover:bg-dark-800"
      :class="{ 'avatar-only': compact }"
      :aria-label="t('common.userMenu')"
      :aria-expanded="dropdownOpen"
      :aria-controls="dropdownOpen ? menuId : undefined"
      @click="dropdownOpen = !dropdownOpen"
      @keydown.down.prevent="openAndFocus(0)"
      @keydown.up.prevent="openAndFocus(-1)"
    >
      <div class="header-avatar flex h-8 w-8 shrink-0 items-center justify-center overflow-hidden rounded-xl bg-gradient-to-br from-primary-500 to-primary-600 text-sm font-medium text-white shadow-sm">
        <img v-if="avatarUrl" :src="avatarUrl" :alt="displayName" class="h-full w-full object-cover" />
        <span v-else>{{ userInitials }}</span>
      </div>
      <div v-if="!compact" class="header-identity hidden text-left md:block">
        <div class="truncate text-sm font-medium text-gray-900 dark:text-white">{{ displayName }}</div>
        <div class="text-xs text-gray-500 dark:text-dark-400">{{ t('admin.users.roles.' + user.role) }}</div>
      </div>
      <Icon v-if="!compact" name="chevronDown" size="sm" class="hidden text-gray-400 md:block" />
    </button>

    <transition name="account-dropdown">
      <div v-if="dropdownOpen || utilities" v-show="dropdownOpen" :id="menuId" ref="menuRef" class="dropdown right-0 mt-2 w-56" :class="{ 'account-utilities-panel': utilities }" @keydown="handleMenuKeydown">
        <div class="account-identity border-b border-gray-100 px-4 py-3 dark:border-dark-700">
          <div class="truncate text-sm font-medium text-gray-900 dark:text-white">{{ displayName }}</div>
          <div class="break-words text-xs text-gray-500 dark:text-dark-400">{{ user.email }}</div>
        </div>

        <dl v-if="utilities" class="account-balance">
          <div class="account-balance-row">
            <dt>{{ t('common.availableBalance') }}</dt>
            <dd>{{ formatMoney(availableBalance) }}</dd>
          </div>
          <div class="account-balance-row account-balance-frozen">
            <dt>{{ t('common.frozenBalance') }}</dt>
            <dd>{{ formatMoney(frozenBalance) }}</dd>
          </div>
          <div class="account-balance-row account-balance-total">
            <dt>{{ t('common.totalBalance') }}</dt>
            <dd>{{ formatMoney(totalBalance) }}</dd>
          </div>
        </dl>

        <div class="py-1">
          <router-link to="/profile" class="dropdown-item" @click="closeDropdown()">
            <Icon name="user" size="sm" />{{ t('nav.profile') }}
          </router-link>
          <router-link to="/keys" class="dropdown-item" @click="closeDropdown()">
            <Icon name="key" size="sm" />{{ t('nav.apiKeys') }}
          </router-link>
          <a v-if="authStore.isAdmin" href="https://github.com/loLollipop/Sub2api" target="_blank" rel="noopener noreferrer" class="dropdown-item" @click="closeDropdown()">
            <svg class="h-4 w-4" fill="currentColor" viewBox="0 0 24 24" aria-hidden="true">
              <path fill-rule="evenodd" clip-rule="evenodd" d="M12 2C6.477 2 2 6.477 2 12c0 4.42 2.865 8.17 6.839 9.49.5.092.682-.217.682-.482 0-.237-.008-.866-.013-1.7-2.782.604-3.369-1.34-3.369-1.34-.454-1.156-1.11-1.464-1.11-1.464-.908-.62.069-.608.069-.608 1.003.07 1.531 1.03 1.531 1.03.892 1.529 2.341 1.087 2.91.831.092-.646.35-1.086.636-1.336-2.22-.253-4.555-1.11-4.555-4.943 0-1.091.39-1.984 1.029-2.683-.103-.253-.446-1.27.098-2.647 0 0 .84-.269 2.75 1.025A9.578 9.578 0 0112 6.836c.85.004 1.705.114 2.504.336 1.909-1.294 2.747-1.025 2.747-1.025.546 1.377.203 2.394.1 2.647.64.699 1.028 1.592 1.028 2.683 0 3.842-2.339 4.687-4.566 4.935.359.309.678.919.678 1.852 0 1.336-.012 2.415-.012 2.743 0 .267.18.578.688.48C19.138 20.167 22 16.418 22 12c0-5.523-4.477-10-10-10z" />
            </svg>
            {{ t('nav.github') }}
          </a>
        </div>

        <div v-if="contactInfo" class="border-t border-gray-100 px-4 py-2.5 dark:border-dark-700">
          <div class="flex items-center gap-2 text-xs text-gray-500 dark:text-gray-400">
            <Icon name="chatBubbles" size="none" class="h-3.5 w-3.5 shrink-0" :stroke-width="1.5" />
            <span>{{ t('common.contactSupport') }}:</span>
            <span class="min-w-0 break-words font-medium text-gray-700 dark:text-gray-300">{{ contactInfo }}</span>
          </div>
        </div>

        <div v-if="showOnboardingButton" class="border-t border-gray-100 py-1 dark:border-dark-700">
          <button type="button" class="dropdown-item w-full" @click="handleReplayGuide">
            <Icon name="questionCircleSolidLarge" size="none" class="h-4 w-4" :stroke-width="1" />
            {{ t('onboarding.restartTour') }}
          </button>
        </div>
        <div class="border-t border-gray-100 py-1 dark:border-dark-700">
          <button type="button" class="header-logout dropdown-item w-full text-red-600 hover:bg-red-50 dark:text-red-400 dark:hover:bg-red-900/20" @click="handleLogout">
            <Icon name="logout" size="none" class="h-4 w-4" :stroke-width="1.5" />{{ t('nav.logout') }}
          </button>
        </div>
      </div>
    </transition>
  </div>
</template>

<script setup lang="ts">
import { computed, nextTick, onBeforeUnmount, onMounted, ref, useId } from 'vue'
import { useRouter } from 'vue-router'
import { useI18n } from 'vue-i18n'
import { useAppStore, useAuthStore, useOnboardingStore } from '@/stores'
import Icon from '@/components/icons/Icon.vue'

const props = withDefaults(defineProps<{ showOnboarding?: boolean; compact?: boolean; utilities?: boolean }>(), {
  showOnboarding: true,
  compact: false,
  utilities: false,
})
const { t } = useI18n()
const router = useRouter()
const authStore = useAuthStore()
const appStore = useAppStore()
const onboardingStore = useOnboardingStore()
const user = computed(() => authStore.user)
const dropdownOpen = ref(false)
const dropdownRef = ref<HTMLElement | null>(null)
const triggerRef = ref<HTMLButtonElement | null>(null)
const menuRef = ref<HTMLElement | null>(null)
const menuId = useId()
const avatarUrl = computed(() => user.value?.avatar_url?.trim() || '')
const displayName = computed(() => user.value?.username || user.value?.email?.split('@')[0] || '')
const userInitials = computed(() => Array.from(displayName.value).slice(0, 2).join('').toUpperCase())
const contactInfo = computed(() => appStore.contactInfo)
const availableBalance = computed(() => Number(user.value?.balance ?? 0))
const frozenBalance = computed(() => Number(user.value?.frozen_balance ?? 0))
const totalBalance = computed(() => availableBalance.value + frozenBalance.value)
const showOnboardingButton = computed(() => props.showOnboarding && !authStore.isSimpleMode && user.value?.role === 'admin')

function formatMoney(value: number) {
  return Number.isFinite(value) ? `$${value.toFixed(2)}` : '—'
}

function closeDropdown(restoreFocus = false) {
  dropdownOpen.value = false
  if (restoreFocus) triggerRef.value?.focus()
}

function menuItems() {
  return Array.from(menuRef.value?.querySelectorAll<HTMLElement>('a, button:not(:disabled)') ?? [])
    .filter(item => item.getAttribute('tabindex') !== '-1' && !item.closest('[style*="display: none"]'))
}
async function openAndFocus(index: number) {
  dropdownOpen.value = true
  await nextTick()
  menuItems().at(index)?.focus()
}
function handleMenuKeydown(event: KeyboardEvent) {
  const items = menuItems()
  if (!items.length) return
  const index = items.indexOf(document.activeElement as HTMLElement)
  const target = event.key === 'ArrowDown' ? (index + 1) % items.length : event.key === 'ArrowUp' ? (index - 1 + items.length) % items.length : event.key === 'Home' ? 0 : event.key === 'End' ? items.length - 1 : -1
  if (target < 0) return
  event.preventDefault()
  items[target]?.focus()
}

function handleClickOutside(event: MouseEvent) {
  if (event.target instanceof Node && !dropdownRef.value?.contains(event.target)) closeDropdown()
}

function handleFocusOut(event: FocusEvent) {
  if (!(event.relatedTarget instanceof Node) || !dropdownRef.value?.contains(event.relatedTarget)) closeDropdown()
}

function handleReplayGuide() {
  closeDropdown()
  onboardingStore.replay()
}

async function handleLogout() {
  closeDropdown()
  try {
    await authStore.logout()
  } catch (error) {
    // Keep the console's existing redirect even when the logout request fails.
    console.error('Logout error:', error)
  }
  await router.push('/login')
}

onMounted(() => document.addEventListener('click', handleClickOutside))
onBeforeUnmount(() => document.removeEventListener('click', handleClickOutside))
</script>

<style scoped>
.header-identity { max-width: 144px; }
.account-dropdown-enter-active, .account-dropdown-leave-active { transition: opacity 150ms ease, transform 150ms ease; }
.account-dropdown-enter-from, .account-dropdown-leave-to { opacity: 0; transform: scale(0.95) translateY(-4px); }
@media (prefers-reduced-motion: reduce) {
  .dropdown { animation: none; }
  .account-dropdown-enter-active, .account-dropdown-leave-active { transition: none; }
}
</style>
