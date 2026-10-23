<template>
  <section data-testid="profile-overview-hero" class="card profile-overview">
    <div class="profile-identity">
      <button
        type="button"
        class="profile-avatar-trigger"
        data-testid="profile-avatar-open"
        :aria-label="t('profile.avatar.open')"
        :title="t('profile.avatar.open')"
        aria-haspopup="dialog"
        :disabled="!user"
        @click="avatarOpen = true"
      >
        <span class="profile-identity-avatar">
          <img v-if="avatarUrl" :src="avatarUrl" :alt="displayName" class="h-full w-full object-cover">
          <span v-else>{{ avatarInitial }}</span>
        </span>
        <span class="profile-avatar-edit" aria-hidden="true"><Icon name="editSimple" size="sm" /></span>
      </button>
      <div class="profile-identity-copy">
        <h1 class="profile-eyebrow">{{ t('profile.title') }}</h1>
        <div class="profile-name-line">
          <h2>{{ displayName }}</h2>
          <button
            type="button"
            class="profile-name-edit"
            data-testid="profile-username-open"
            :aria-label="t('profile.editUsername')"
            :title="t('profile.editUsername')"
            aria-haspopup="dialog"
            :disabled="!user"
            @click="usernameOpen = true"
          ><Icon name="editSimple" size="sm" /></button>
        </div>
        <div class="profile-identity-meta">
          <span v-if="primaryEmailDisplay">{{ primaryEmailDisplay }}</span>
          <span v-if="user" data-testid="profile-meta-concurrency">{{ t('profile.concurrencyLimit') }} · {{ user.concurrency }}</span>
          <span v-if="user" data-testid="profile-meta-member-since">{{ t('profile.memberSince') }} · {{ memberSinceLabel }}</span>
        </div>
        <div v-if="sourceHints.length" class="profile-source-hints">
          <span v-for="hint in sourceHints" :key="hint.key"><Icon name="link" size="sm" />{{ hint.text }}</span>
        </div>
      </div>
      <div v-if="user" class="profile-account-meta">
        <div class="profile-account-badges">
          <span :class="['badge', user.role === 'admin' ? 'badge-primary' : 'badge-gray']">
            {{ user.role === 'admin' ? t('profile.administrator') : t('profile.user') }}
          </span>
          <span :class="['badge', user.status === 'active' ? 'badge-success' : 'badge-danger']">
            {{ user.status === 'active' ? t('common.active') : t('common.disabled') }}
          </span>
        </div>
        <span>{{ t('profile.accountId', { id: user.id }) }}</span>
      </div>
    </div>
    <dl class="profile-identity-metrics">
      <div data-testid="profile-overview-metric-balance">
        <dt><Icon name="creditCard" size="sm" />{{ t('profile.accountBalance') }}</dt>
        <dd>{{ user ? formatCurrency(user.balance) : '—' }}</dd>
        <dd class="profile-metric-hint">{{ user ? t('profile.balanceHint') : t('common.loading') }}</dd>
      </div>
      <div data-testid="profile-overview-metric-spending" :aria-busy="statsState === 'loading'">
        <dt><Icon name="chart" size="sm" />{{ t('profile.totalSpent') }}</dt>
        <dd>{{ totalSpentLabel }}</dd>
        <dd class="profile-metric-hint">{{ statsHint || t('profile.totalSpentHint') }}</dd>
      </div>
      <div data-testid="profile-overview-metric-requests" :aria-busy="statsState === 'loading'">
        <dt><Icon name="terminal" size="sm" />{{ t('profile.totalRequests') }}</dt>
        <dd>{{ totalRequestsLabel }}</dd>
        <dd class="profile-metric-hint">{{ statsHint || t('profile.totalRequestsHint') }}</dd>
      </div>
    </dl>
  </section>

  <BaseDialog
    :show="avatarOpen"
    :title="t('profile.avatar.open')"
    width="narrow"
    :close-on-escape="!avatarBusy"
    :show-close-button="!avatarBusy"
    @close="closeAvatar"
  >
    <div class="profile-dialog-form">
      <ProfileAvatarCard
        v-if="avatarOpen" :user="user"
        embedded in-dialog
        @busy="avatarBusy = $event"
        @saved="avatarOpen = false"
        @cancel="closeAvatar"
      />
    </div>
  </BaseDialog>
  <BaseDialog
    :show="usernameOpen"
    :title="t('profile.editUsername')"
    width="narrow"
    :close-on-escape="!usernameBusy"
    :show-close-button="!usernameBusy"
    @close="closeUsername"
  >
    <div class="profile-dialog-form" @keydown.esc="handleUsernameEscape">
      <ProfileEditForm
        v-if="usernameOpen" :initial-username="user?.username || ''"
        embedded in-dialog
        @busy="usernameBusy = $event"
        @saved="usernameOpen = false"
        @cancel="closeUsername"
      />
    </div>
  </BaseDialog>
</template>
<script setup lang="ts">
import { computed, ref } from 'vue'
import { useI18n } from 'vue-i18n'
import Icon from '@/components/icons/Icon.vue'
import ProfileAvatarCard from '@/components/user/profile/ProfileAvatarCard.vue'
import ProfileEditForm from '@/components/user/profile/ProfileEditForm.vue'
import BaseDialog from '@/components/common/BaseDialog.vue'
import type { UserDashboardStats } from '@/api/usage'
import type { User, UserAuthBindingStatus, UserAuthProvider, UserProfileSourceContext } from '@/types'

const props = withDefaults(defineProps<{
  user: User | null
  oidcProviderName?: string
  stats?: Pick<UserDashboardStats, 'total_actual_cost' | 'total_requests'> | null
  statsState?: 'loading' | 'ready' | 'error'
}>(), {
  oidcProviderName: 'OIDC',
  stats: null,
  statsState: 'loading',
})

const { t, locale } = useI18n()
const statsHint = computed(() => props.statsState === 'loading'
  ? t('profile.statsLoading')
  : props.statsState === 'error' || !props.stats ? t('profile.statsUnavailable') : '')
const totalSpentLabel = computed(() => props.statsState === 'ready' && props.stats
  ? formatCurrency(props.stats.total_actual_cost) : '—')
const totalRequestsLabel = computed(() => props.statsState === 'ready' && props.stats && Number.isFinite(props.stats.total_requests)
  ? new Intl.NumberFormat(locale?.value).format(props.stats.total_requests) : '—')
const avatarOpen = ref(false)
const avatarBusy = ref(false)
const usernameOpen = ref(false)
const usernameBusy = ref(false)
function closeAvatar() { if (!avatarBusy.value) avatarOpen.value = false }
function closeUsername() { if (!usernameBusy.value) usernameOpen.value = false }
function handleUsernameEscape(event: KeyboardEvent) {
  if (event.isComposing || event.keyCode === 229) event.stopPropagation()
}

function normalizeBindingStatus(binding: boolean | UserAuthBindingStatus | undefined): boolean | null {
  if (typeof binding === 'boolean') {
    return binding
  }
  if (!binding) {
    return null
  }
  if (typeof binding.bound === 'boolean') {
    return binding.bound
  }
  return Boolean(binding.provider_subject || binding.issuer || binding.provider_key)
}

function isEmailBound(user: User | null | undefined): boolean {
  if (typeof user?.email_bound === 'boolean') {
    return user.email_bound
  }

  const nested = user?.auth_bindings?.email ?? user?.identity_bindings?.email
  const normalized = normalizeBindingStatus(nested)
  return normalized ?? false
}

const avatarUrl = computed(() => props.user?.avatar_url?.trim() || '')
const displayName = computed(() => props.user?.username?.trim() || props.user?.email?.trim() || t('profile.user'))
const primaryEmailDisplay = computed(() => {
  const email = props.user?.email?.trim() || ''
  if (!email) {
    return ''
  }
  if (email.endsWith('.invalid') && !isEmailBound(props.user)) {
    return ''
  }
  return email
})
const avatarInitial = computed(() => displayName.value.charAt(0).toUpperCase() || 'U')
const memberSinceLabel = computed(() => {
  const raw = props.user?.created_at?.trim()
  if (!raw) {
    return '-'
  }

  const date = new Date(raw)
  if (Number.isNaN(date.getTime())) {
    return '-'
  }

  return new Intl.DateTimeFormat(undefined, {
    year: 'numeric',
    month: 'short',
  }).format(date)
})

const providerLabels = computed<Record<UserAuthProvider, string>>(() => ({
  email: t('profile.authBindings.providers.email'),
  linuxdo: t('profile.authBindings.providers.linuxdo'),
  dingtalk: t('profile.authBindings.providers.dingtalk'),
  oidc: t('profile.authBindings.providers.oidc', { providerName: props.oidcProviderName }),
  wechat: t('profile.authBindings.providers.wechat'),
  github: 'GitHub',
  google: 'Google'
}))

function formatCurrency(value: number): string {
  return Number.isFinite(value)
    ? new Intl.NumberFormat(locale?.value, { style: 'currency', currency: 'USD', currencyDisplay: 'narrowSymbol', minimumFractionDigits: 2, maximumFractionDigits: 4 }).format(value)
    : '—'
}

function normalizeProvider(value: string): UserAuthProvider | null {
  const normalized = value.trim().toLowerCase()
  if (
    normalized === 'email' ||
    normalized === 'linuxdo' ||
    normalized === 'wechat' ||
    normalized === 'github' ||
    normalized === 'google'
  ) {
    return normalized
  }
  if (normalized === 'oidc' || normalized.startsWith('oidc:') || normalized.startsWith('oidc/')) {
    return 'oidc'
  }
  return null
}

function readObjectString(source: Record<string, unknown>, ...keys: string[]): string {
  for (const key of keys) {
    const value = source[key]
    if (typeof value === 'string' && value.trim()) {
      return value.trim()
    }
  }
  return ''
}

function resolveThirdPartySource(
  rawSource: string | UserProfileSourceContext | null | undefined
): { provider: UserAuthProvider; label: string } | null {
  if (!rawSource) {
    return null
  }

  if (typeof rawSource === 'string') {
    const provider = normalizeProvider(rawSource)
    if (!provider || provider === 'email') {
      return null
    }
    return {
      provider,
      label: providerLabels.value[provider]
    }
  }

  const sourceRecord = rawSource as Record<string, unknown>
  const provider = normalizeProvider(
    readObjectString(sourceRecord, 'provider', 'source', 'provider_type', 'auth_provider')
  )
  if (!provider || provider === 'email') {
    return null
  }

  const explicitLabel = readObjectString(
    sourceRecord,
    'provider_label',
    'label',
    'provider_name',
    'providerName'
  )

  return {
    provider,
    label: explicitLabel || providerLabels.value[provider]
  }
}

const sourceHints = computed(() => {
  const currentUser = props.user
  if (!currentUser) {
    return []
  }

  const hints: Array<{ key: string; text: string }> = []
  const avatarSource = resolveThirdPartySource(
    currentUser.profile_sources?.avatar ?? currentUser.avatar_source
  )
  const usernameSource = resolveThirdPartySource(
    currentUser.profile_sources?.username ??
      currentUser.profile_sources?.display_name ??
      currentUser.profile_sources?.nickname ??
      currentUser.display_name_source ??
      currentUser.username_source ??
      currentUser.nickname_source
  )

  if (avatarSource) {
    hints.push({
      key: 'avatar',
      text: t('profile.authBindings.source.avatar', { providerName: avatarSource.label })
    })
  }

  if (usernameSource) {
    hints.push({
      key: 'username',
      text: t('profile.authBindings.source.username', { providerName: usernameSource.label })
    })
  }

  return hints
})
</script>
