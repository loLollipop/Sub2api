<template>
  <AppLayout>
    <div data-testid="profile-shell" class="signal-account-page signal-profile">
      <ProfileInfoCard :user="user" :oidc-provider-name="oidcOAuthProviderName" />

      <div class="profile-settings-grid">
        <section data-testid="profile-auth-bindings-panel" class="card profile-settings-panel profile-login-panel">
          <ProfileIdentityBindingsSection
            :user="user"
            :linuxdo-enabled="linuxdoOAuthEnabled"
            :dingtalk-enabled="dingtalkOAuthEnabled"
            :oidc-enabled="oidcOAuthEnabled"
            :oidc-provider-name="oidcOAuthProviderName"
            :wechat-enabled="wechatOAuthEnabled"
            :wechat-open-enabled="wechatOAuthOpenEnabled"
            :wechat-mp-enabled="wechatOAuthMPEnabled"
            embedded compact
          />
        </section>

        <section class="card profile-settings-panel profile-security-panel" aria-labelledby="profile-security-title">
          <header class="profile-section-heading">
            <h2 id="profile-security-title">{{ t('profile.securityTitle') }}</h2>
            <p>{{ t('profile.securityDescription') }}</p>
          </header>
          <div class="profile-security-row">
            <span class="profile-setting-icon"><Icon name="lock" size="sm" /></span>
            <div class="profile-setting-copy">
              <h3>{{ t('profile.loginPassword') }}</h3>
              <p>{{ t('profile.passwordSummary') }}</p>
            </div>
            <button type="button" class="btn btn-secondary btn-sm" data-testid="profile-password-open" :aria-label="t('profile.changePassword')" aria-haspopup="dialog" @click="passwordOpen = true">
              {{ t('common.edit') }}
            </button>
          </div>
          <ProfileTotpCard embedded />
          <ProfilePasskeyCard :enabled="passkeyEnabled" embedded />
        </section>

        <ProfileBalanceNotifyCard
          v-if="user && balanceLowNotifyEnabled"
          class="profile-notifications-panel"
          :enabled="user.balance_notify_enabled ?? true"
          :threshold="user.balance_notify_threshold"
          :extra-emails="user.balance_notify_extra_emails ?? []"
          :system-default-threshold="systemDefaultThreshold"
          :user-email="user.email"
        />
      </div>

      <div v-if="contactInfo" class="profile-support">
        <Icon name="chat" size="sm" />
        <span>{{ t('common.contactSupport') }}</span><p>{{ contactInfo }}</p>
      </div>

      <BaseDialog
        :show="passwordOpen"
        :title="t('profile.changePassword')"
        width="narrow"
        :close-on-escape="!passwordBusy"
        :show-close-button="!passwordBusy"
        @close="closePassword"
      >
        <div class="profile-dialog-form">
          <ProfilePasswordForm
            v-if="passwordOpen"
            embedded in-dialog
            @busy="passwordBusy = $event"
            @saved="passwordOpen = false"
            @cancel="closePassword"
          />
        </div>
      </BaseDialog>
    </div>
  </AppLayout>
</template>
<script setup lang="ts">
import { computed, onMounted, ref } from 'vue'
import { useI18n } from 'vue-i18n'
import BaseDialog from '@/components/common/BaseDialog.vue'
import ProfileIdentityBindingsSection from '@/components/user/profile/ProfileIdentityBindingsSection.vue'
import { Icon } from '@/components/icons'
import AppLayout from '@/components/layout/AppLayout.vue'
import ProfileBalanceNotifyCard from '@/components/user/profile/ProfileBalanceNotifyCard.vue'
import ProfileInfoCard from '@/components/user/profile/ProfileInfoCard.vue'
import ProfilePasswordForm from '@/components/user/profile/ProfilePasswordForm.vue'
import ProfileTotpCard from '@/components/user/profile/ProfileTotpCard.vue'
import ProfilePasskeyCard from '@/components/user/profile/ProfilePasskeyCard.vue'
import { isWeChatWebOAuthEnabled } from '@/api/auth'
import { useAppStore } from '@/stores/app'
import { useAuthStore } from '@/stores/auth'

const { t } = useI18n()
const appStore = useAppStore()
const authStore = useAuthStore()
const user = computed(() => authStore.user)
const passwordOpen = ref(false)
const passwordBusy = ref(false)
function closePassword() { if (!passwordBusy.value) passwordOpen.value = false }

const contactInfo = ref('')
const balanceLowNotifyEnabled = ref(false)
const systemDefaultThreshold = ref(0)
const linuxdoOAuthEnabled = ref(false)
const dingtalkOAuthEnabled = ref(false)
const wechatOAuthEnabled = ref(false)
const wechatOAuthOpenEnabled = ref<boolean | undefined>(undefined)
const wechatOAuthMPEnabled = ref<boolean | undefined>(undefined)
const oidcOAuthEnabled = ref(false)
const oidcOAuthProviderName = ref('OIDC')
const passkeyEnabled = ref(false)

onMounted(async () => {
  const profileRefresh = authStore.refreshUser().catch((error) => {
    console.error('Failed to refresh profile:', error)
  })

  const settingsLoad = appStore.fetchPublicSettings()
    .then((settings) => {
      if (!settings) {
        return
      }
      contactInfo.value = settings.contact_info || ''
      balanceLowNotifyEnabled.value = settings.balance_low_notify_enabled ?? false
      systemDefaultThreshold.value = settings.balance_low_notify_threshold ?? 0
      linuxdoOAuthEnabled.value = settings.linuxdo_oauth_enabled ?? false
      dingtalkOAuthEnabled.value = settings.dingtalk_oauth_enabled ?? false
      wechatOAuthEnabled.value = isWeChatWebOAuthEnabled(settings)
      wechatOAuthOpenEnabled.value = typeof settings.wechat_oauth_open_enabled === 'boolean'
        ? settings.wechat_oauth_open_enabled
        : undefined
      wechatOAuthMPEnabled.value = typeof settings.wechat_oauth_mp_enabled === 'boolean'
        ? settings.wechat_oauth_mp_enabled
        : undefined
      oidcOAuthEnabled.value = settings.oidc_oauth_enabled ?? false
      oidcOAuthProviderName.value = settings.oidc_oauth_provider_name || 'OIDC'
      passkeyEnabled.value = settings.passkey_enabled === true
    })
    .catch((error) => {
      console.error('Failed to load settings:', error)
    })

  await Promise.all([profileRefresh, settingsLoad])
})
</script>
