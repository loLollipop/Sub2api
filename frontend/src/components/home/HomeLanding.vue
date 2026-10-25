<template>
  <div class="home-landing" :class="{ 'home-landing--dark': isDark }" data-testid="default-home">
    <header class="landing-header">
      <nav class="landing-nav landing-container" :aria-label="t('home.landing.navigation')">
        <router-link to="/" class="landing-brand" :title="siteName">
          <img :src="siteLogo || '/logo.svg'" alt="" width="32" height="32" />
          <span class="brand-gradient">{{ siteName }}</span>
        </router-link>
        <div class="landing-nav__right">
          <div class="landing-nav__links">
            <a href="#home" aria-current="page">{{ t('home.landing.home') }}</a>
            <router-link :to="dashboardPath">{{ t('home.dashboard') }}</router-link>
            <router-link v-if="modelPlazaEnabled" to="/model-plaza">{{ t('nav.modelPlaza') }}</router-link>
            <router-link v-if="leaderboardEnabled" to="/leaderboard">{{ t('leaderboard.title') }}</router-link>
            <a href="#integration">{{ t('home.landing.integration') }}</a>
            <a v-if="docUrl" :href="docUrl" target="_blank" rel="noopener noreferrer">{{ t('home.docs') }}<Icon name="arrowUpRight" size="xs" aria-hidden="true" /></a>
          </div>
          <div class="landing-nav__actions">
            <LocaleSwitcher compact />
            <button
              type="button"
              class="landing-theme"
              :aria-label="isDark ? t('home.switchToLight') : t('home.switchToDark')"
              :title="isDark ? t('home.switchToLight') : t('home.switchToDark')"
              @click="$emit('toggleTheme')"
            >
              <Icon :name="isDark ? 'sun' : 'moon'" size="md" aria-hidden="true" />
            </button>
            <AccountMenu v-if="isAuthenticated" compact :show-onboarding="false" data-testid="home-account-menu" />
            <router-link v-else to="/login" class="landing-login" data-testid="home-account-entry">{{ t('home.login') }}</router-link>
          </div>
        </div>
      </nav>
    </header>

    <main>
      <section id="home" class="landing-stage" aria-labelledby="landing-title">
        <div class="landing-stage__glow" aria-hidden="true"></div>
        <HomeModelOrbit :brands="brands" />
        <div class="landing-hero">
          <p class="landing-eyebrow"><span aria-hidden="true"></span>{{ t('home.landing.orbitEyebrow') }}</p>
          <h1 id="landing-title"><span class="landing-brand-title brand-gradient">{{ siteName }}</span></h1>
          <p v-if="siteSubtitle.trim()" class="landing-subtitle">{{ siteSubtitle }}</p>
          <p class="landing-description">{{ t('home.landing.orbitDescription') }}</p>
          <div class="landing-hero__actions">
            <router-link :to="entryPath" class="landing-button" data-testid="home-primary-cta">
              {{ isAuthenticated ? t('home.goToDashboard') : t('home.getStarted') }}
              <Icon name="arrowRight" size="sm" aria-hidden="true" />
            </router-link>
            <a href="#integration" class="landing-secondary">{{ t('home.landing.exploreIntegration') }}<Icon name="arrowDown" size="sm" aria-hidden="true" /></a>
          </div>
        </div>
        <a href="#integration" class="landing-explore"><span>{{ t('home.landing.scrollToExplore') }}</span><Icon name="arrowDown" size="xs" aria-hidden="true" /></a>
      </section>

      <section class="landing-ecosystem" :aria-label="t('home.landing.modelEcosystem')">
        <div class="landing-container landing-ecosystem__inner">
          <p>{{ t('home.landing.modelEcosystem') }}<span aria-hidden="true">/</span></p>
          <ul class="landing-models">
            <li v-for="brand in brands" :key="brand.platform"><PlazaProviderIcon :platform="brand.platform" size="md" /><span>{{ brand.label }}</span></li>
          </ul>
        </div>
      </section>

      <section id="integration" class="landing-integration landing-container" aria-labelledby="integration-title">
        <div class="landing-integration__copy">
          <p class="landing-section-label"><span aria-hidden="true">01 /</span>{{ t('home.landing.integration') }}</p>
          <h2 id="integration-title">{{ t('home.landing.integrationTitle') }}</h2>
          <p class="landing-integration__description">{{ t('home.landing.integrationDescription') }}</p>
          <div class="landing-features" :aria-label="t('home.landing.capabilities')" data-testid="home-capabilities">
            <article v-for="feature in features" :key="feature.key" class="landing-feature">
              <div class="home-feature-mark"><Icon :name="feature.icon" size="sm" aria-hidden="true" /></div>
              <div><h3>{{ t(`home.landing.${feature.key}Title`) }}</h3><p>{{ t(`home.landing.${feature.key}Description`) }}</p></div>
            </article>
          </div>
          <a v-if="docUrl" :href="docUrl" target="_blank" rel="noopener noreferrer" class="landing-doc-link">{{ t('home.viewDocs') }}<Icon name="arrowUpRight" size="sm" aria-hidden="true" /></a>
        </div>
        <div class="landing-integration__preview"><HomeApiPreview :api-base-url="apiBaseUrl" /></div>
      </section>
    </main>

    <footer class="landing-footer">
      <div class="landing-container">
        <div class="landing-footer__bottom">
          <p>&copy; {{ new Date().getFullYear() }} {{ siteName }}</p>
          <LegalFooterLinks />
          <a :href="githubUrl" target="_blank" rel="noopener noreferrer">GitHub <Icon name="arrowUpRight" size="xs" aria-hidden="true" /></a>
        </div>
        <p class="landing-footer__risk">{{ t('home.footer.riskStrip') }}</p>
      </div>
    </footer>
  </div>
</template>

<script setup lang="ts">
import { computed } from 'vue'
import { useI18n } from 'vue-i18n'
import Icon from '@/components/icons/Icon.vue'
import LocaleSwitcher from '@/components/common/LocaleSwitcher.vue'
import LegalFooterLinks from '@/components/legal/LegalFooterLinks.vue'
import AccountMenu from '@/components/layout/AccountMenu.vue'
import PlazaProviderIcon from '@/components/modelPlaza/PlazaProviderIcon.vue'
import { FeatureFlags, isFeatureFlagEnabled } from '@/utils/featureFlags'
import HomeApiPreview from './HomeApiPreview.vue'
import HomeModelOrbit from './HomeModelOrbit.vue'

const props = defineProps<{
  siteName: string
  siteLogo: string
  siteSubtitle: string
  docUrl: string
  apiBaseUrl: string
  githubUrl: string
  isDark: boolean
  isAuthenticated: boolean
  dashboardPath: string
}>()
defineEmits<{ toggleTheme: [] }>()
const { t } = useI18n()
const entryPath = computed(() => props.isAuthenticated ? props.dashboardPath : '/login')
const modelPlazaEnabled = computed(() => isFeatureFlagEnabled(FeatureFlags.modelPlaza))
const leaderboardEnabled = computed(() => isFeatureFlagEnabled(FeatureFlags.leaderboard))
const features = [
  { key: 'connect', icon: 'server' },
  { key: 'routing', icon: 'shield' },
  { key: 'usage', icon: 'chart' },
] as const
// Brand examples for the visual composition, not a live availability response.
const brands = [
  { platform: 'openai', label: 'OpenAI' }, { platform: 'anthropic', label: 'Claude' },
  { platform: 'gemini', label: 'Gemini' }, { platform: 'deepseek', label: 'DeepSeek' },
  { platform: 'grok', label: 'Grok' }, { platform: 'kimi', label: 'Kimi' },
  { platform: 'qwen', label: 'Qwen' }, { platform: 'zhipu', label: 'GLM' },
] as const
</script>

<style scoped>
@font-face {
  font-family: 'Home Outfit';
  src: url('../../assets/fonts/outfit/outfit-latin.woff2') format('woff2');
  font-weight: 100 900;
  font-style: normal;
  font-display: swap;
}
.home-landing {
  --home-bg: #faf9f6;
  --home-panel: #fffefa;
  --home-code-bg: #f4f2ee;
  --home-ink: #302d29;
  --home-muted: #706960;
  --home-line: #e7e2da;
  --home-accent: #b75324;
  --home-accent-soft: #f5e8db;
  --home-on-accent: #fffaf5;
  --home-shadow: #76503230;
  --home-orbit-line: #bd8d6840;
  --home-orbit-point: #c5824d;
  --home-orbit-halo: #c5824d0d;
  --home-brand-line: #e4dcd0;
  --home-brand-surface: #fffefaad;
  --home-glow: #edd9b975;
  --home-sage: #dce2d85c;
  --syntax-command: #885394;
  --syntax-url: #b04d22;
  --syntax-key: #a05213;
  --syntax-string: #32806d;
  --syntax-placeholder: #536885;
  --signal-text: var(--home-ink);
  display: flex;
  flex-direction: column;
  min-height: 100svh;
  background: var(--home-bg);
  color: var(--home-ink);
  -webkit-font-smoothing: antialiased;
}
.home-landing--dark {
  --home-bg: #171816;
  --home-panel: #21221f;
  --home-code-bg: #292a25;
  --home-ink: #ede9e1;
  --home-muted: #b0aaa0;
  --home-line: #383930;
  --home-accent: #eea06c;
  --home-accent-soft: #382c23;
  --home-on-accent: #2a1c12;
  --home-shadow: #00000090;
  --home-orbit-line: #bd936640;
  --home-orbit-point: #d49663;
  --home-orbit-halo: #d4966310;
  --home-brand-line: #4b453a;
  --home-brand-surface: #252620ad;
  --home-glow: #b16a2e1a;
  --home-sage: #87987714;
  --syntax-command: #d6a7ff;
  --syntax-url: #eea06c;
  --syntax-key: #edb477;
  --syntax-string: #9ac6b0;
  --syntax-placeholder: #b8c9ea;
}
.landing-container { width: min(1240px, calc(100% - 96px)); margin-inline: auto; }
.landing-header { position: relative; z-index: 10; }
.landing-nav { display: flex; min-height: 82px; align-items: center; justify-content: space-between; gap: 24px; }
.landing-brand { display: flex; align-items: center; gap: 10px; min-width: 0; font-family: 'Home Outfit', sans-serif; font-size: 20px; font-weight: 650; letter-spacing: -.035em; }
.landing-brand img { flex-shrink: 0; border-radius: 50%; object-fit: contain; }
.landing-brand span { max-width: 260px; overflow: hidden; text-overflow: ellipsis; white-space: nowrap; }
.landing-nav__right, .landing-nav__links, .landing-nav__actions { display: flex; align-items: center; }
.landing-nav__right { gap: 28px; flex-shrink: 0; }
.landing-nav__links { gap: 24px; color: var(--home-muted); font-size: 13px; }
.landing-nav__links a { display: inline-flex; align-items: center; gap: 3px; min-height: 40px; white-space: nowrap; transition: color 150ms; }
.landing-nav__links a[aria-current], .landing-nav__links a:hover { color: var(--home-accent); }
.landing-nav__actions { gap: 6px; padding-left: 22px; border-left: 1px solid var(--home-line); }
.landing-theme { display: grid; width: 36px; height: 36px; place-items: center; border-radius: 50%; color: var(--home-muted); transition: background 150ms, color 150ms; }
.landing-theme:hover { background: var(--home-accent-soft); color: var(--home-accent); }
.landing-login { display: inline-flex; align-items: center; min-height: 34px; padding-inline: 18px; margin-left: 8px; border: 1px solid var(--home-line); border-radius: 999px; font-size: 12px; font-weight: 600; transition: background 150ms; }
.landing-login:hover { background: var(--home-accent-soft); }
.landing-nav :deep(.header-user-button) { border-radius: 50%; }
.landing-nav :deep(.header-avatar) { width: 32px; height: 32px; border: 1px solid var(--home-line); border-radius: 50%; background: var(--home-accent-soft); color: var(--home-accent); box-shadow: none; }
.landing-nav :deep(.header-identity > div:first-child) { color: var(--home-ink); }
.landing-nav :deep(.header-identity > div + div) { color: var(--home-muted); }
.landing-nav :deep(.avatar-only) { padding: 2px; margin-left: 8px; }
.landing-nav :deep(.dropdown) { max-width: calc(100vw - 36px); border-color: var(--home-line); background: var(--home-panel); box-shadow: 0 12px 36px -16px var(--home-shadow); }
.landing-nav :deep(.dropdown-item) { min-height: 36px; font-size: 12px; }
.landing-stage { position: relative; display: grid; place-items: center; min-height: clamp(650px, calc(100svh - 164px), 820px); isolation: isolate; overflow: hidden; }
.landing-stage__glow { position: absolute; inset: -82px 0 0; z-index: -1; background: radial-gradient(ellipse at 45% 36%, var(--home-glow), transparent 52%), radial-gradient(ellipse at 91% 56%, var(--home-sage), transparent 38%); pointer-events: none; }
.landing-stage__glow::after { content: ''; position: absolute; inset: 12% 8%; background-image: radial-gradient(var(--home-orbit-line) .7px, transparent .7px); background-size: 12px 12px; mask-image: linear-gradient(110deg, #0006, transparent 35%, transparent 65%, #0006); }
.landing-hero { position: relative; z-index: 2; width: min(720px, calc(100% - 40px)); padding-block: 120px; text-align: center; }
.landing-eyebrow { display: inline-flex; align-items: center; gap: 10px; color: var(--home-muted); font-size: 12px; letter-spacing: .13em; }
.landing-eyebrow > span { width: 6px; height: 6px; border-radius: 50%; background: var(--home-accent); box-shadow: 0 0 0 4px var(--home-accent-soft); }
.landing-hero h1 { margin: 20px 0 0; font-family: 'Home Outfit', 'PingFang SC', 'Microsoft YaHei', sans-serif; font-size: clamp(68px, 7.7vw, 112px); font-weight: 650; letter-spacing: -.065em; line-height: 1.14; overflow-wrap: anywhere; }
.landing-subtitle { margin-top: 14px; color: var(--home-ink); font-family: 'Home Outfit', 'PingFang SC', 'Microsoft YaHei', sans-serif; font-size: 20px; letter-spacing: .015em; line-height: 1.6; white-space: pre-wrap; overflow-wrap: anywhere; }
.landing-description { max-width: 500px; margin: 20px auto 0; color: var(--home-muted); font-size: 14px; line-height: 1.95; white-space: pre-line; text-wrap: balance; }
.landing-hero__actions { display: flex; align-items: center; justify-content: center; flex-wrap: wrap; gap: 12px; margin-top: 30px; }
.landing-button, .landing-secondary { display: inline-flex; align-items: center; justify-content: center; gap: 18px; min-height: 46px; padding: 12px 21px; border: 1px solid transparent; border-radius: 10px; font-size: 13px; font-weight: 550; transition: background 180ms, box-shadow 180ms, translate 180ms; }
.landing-button { border-color: var(--home-accent); background: var(--home-accent); color: var(--home-on-accent); box-shadow: 0 6px 18px -8px var(--home-accent); }
.landing-button:hover { translate: 0 -2px; box-shadow: 0 9px 24px -10px var(--home-accent); }
.landing-secondary { border-color: var(--home-line); background: var(--home-brand-surface); color: var(--home-ink); backdrop-filter: blur(8px); }
.landing-secondary:hover { background: var(--home-accent-soft); }
.landing-explore { position: absolute; z-index: 2; bottom: 24px; display: flex; align-items: center; gap: 10px; padding: 8px; color: var(--home-muted); font-size: 10px; letter-spacing: .12em; }
.landing-ecosystem { position: relative; padding-block: 28px; border-block: 1px solid var(--home-line); background: var(--home-panel); }
.landing-ecosystem__inner { display: flex; align-items: center; gap: 32px; }
.landing-ecosystem__inner > p { display: flex; align-items: center; gap: 25px; flex-shrink: 0; color: var(--home-muted); font-size: 11px; letter-spacing: .06em; }
.landing-ecosystem__inner > p > span { opacity: .4; }
.landing-models { display: flex; flex: 1; flex-wrap: wrap; align-items: center; justify-content: space-between; gap: 20px; min-width: 0; }
.landing-models li { display: flex; align-items: center; gap: 8px; font-family: 'Home Outfit', sans-serif; font-size: 13px; font-weight: 500; }
.landing-models :deep(.plaza-provider-icon) { flex-shrink: 0; }
.landing-integration { display: grid; grid-template-columns: .9fr 1.1fr; align-items: center; gap: clamp(40px, 6vw, 96px); padding-block: 100px 110px; scroll-margin-top: 24px; }
.landing-integration__copy, .landing-integration__preview { min-width: 0; }
.landing-section-label { display: flex; align-items: center; gap: 13px; color: var(--home-muted); font-size: 11px; letter-spacing: .05em; }
.landing-section-label > span { color: var(--home-accent); font-family: 'Home Outfit', sans-serif; }
.landing-integration h2 { margin-top: 23px; font-size: clamp(26px, 2.7vw, 38px); font-weight: 550; line-height: 1.55; letter-spacing: -.04em; white-space: pre-line; text-wrap: balance; }
.landing-integration__description { max-width: 365px; margin-top: 19px; color: var(--home-muted); font-size: 13px; line-height: 1.95; }
.landing-features { display: flex; flex-direction: column; gap: 23px; margin-top: 32px; }
.landing-feature { display: flex; align-items: center; gap: 13px; }
.home-feature-mark { display: grid; place-items: center; width: 34px; height: 34px; flex-shrink: 0; border: 1px solid var(--home-line); border-radius: 9px; background: var(--home-panel); color: var(--home-accent); }
.landing-feature h3 { font-size: 12px; font-weight: 600; }
.landing-feature p { margin-top: 4px; color: var(--home-muted); font-size: 11px; line-height: 1.7; }
.landing-doc-link { display: inline-flex; align-items: center; gap: 8px; min-height: 40px; margin-top: 22px; color: var(--home-accent); font-size: 12px; }
.landing-doc-link:hover { text-decoration: underline; text-underline-offset: 5px; }
.landing-footer { padding-block: 22px; border-top: 1px solid var(--home-line); color: var(--home-muted); }
.landing-footer__bottom { display: grid; grid-template-columns: 1fr auto 1fr; align-items: center; gap: 20px; font-size: 11px; }
.landing-footer__bottom > p { overflow-wrap: anywhere; }
.landing-footer__bottom > a { display: inline-flex; justify-self: end; align-items: center; gap: 5px; min-height: 30px; }
.landing-footer__risk { max-width: 850px; margin: 10px auto 0; text-align: center; font-size: 10px; line-height: 1.8; }
.home-landing :is(a, button):focus-visible { outline: 2px solid var(--home-accent); outline-offset: 4px; }
@media (max-width: 1100px) {
  .landing-container { width: calc(100% - 64px); }
  .landing-nav__right { gap: 22px; }
  .landing-nav__links { gap: 20px; }
  .landing-hero { width: min(620px, calc(100% - 40px)); }
  .landing-ecosystem__inner { gap: 24px; }
  .landing-ecosystem__inner > p { display: none; }
  .landing-integration { gap: 40px; padding-block: 80px; }
}
@media (max-width: 900px) {
  .landing-nav { flex-wrap: wrap; gap: 0; padding-top: 16px; }
  .landing-brand { flex: 1; }
  .landing-nav__right { display: contents; }
  .landing-nav__actions { border: 0; padding-left: 0; }
  .landing-nav__links { order: 3; width: 100%; min-width: 0; gap: 22px; overflow-x: auto; padding-block: 8px; scrollbar-width: none; }
  .landing-nav__links::-webkit-scrollbar { display: none; }
  .landing-hero { padding-block: 150px; }
  .landing-integration { grid-template-columns: 1fr; gap: 36px; }
  .landing-integration__copy { display: grid; grid-template-columns: 1fr 1fr; column-gap: 40px; }
  .landing-section-label, .landing-integration h2, .landing-integration__description, .landing-doc-link { grid-column: 1; }
  .landing-features { grid-column: 2; grid-row: 2 / 5; align-self: center; margin-top: 0; }
}
@media (max-width: 640px) {
  .landing-container { width: calc(100% - 36px); }
  .landing-nav { min-height: 72px; gap: 0; padding-top: 16px; }
  .landing-brand { gap: 8px; font-size: 18px; }
  .landing-brand img { width: 28px; height: 28px; }
  .landing-brand span { max-width: min(150px, 35vw); }
  .landing-nav__links { gap: 20px; font-size: 12px; }
  .landing-nav__actions { gap: 2px; padding-left: 0; border: 0; }
  .landing-login { min-height: 32px; padding-inline: 13px; margin-left: 4px; }
  .landing-nav :deep(.avatar-only) { margin-left: 4px; }
  .landing-stage { min-height: 660px; }
  .landing-hero { padding-block: 140px 155px; }
  .landing-eyebrow { font-size: 10px; }
  .landing-hero h1 { margin-top: 24px; font-size: clamp(53px, 14vw, 82px); }
  .landing-subtitle { margin-top: 12px; font-size: 17px; }
  .landing-description { max-width: 340px; margin-top: 20px; font-size: 12px; }
  .landing-hero__actions { gap: 10px; margin-top: 26px; }
  .landing-button, .landing-secondary { min-height: 44px; gap: 12px; padding-inline: 16px; font-size: 12px; }
  .landing-explore { bottom: 20px; font-size: 9px; }
  .landing-ecosystem { padding-block: 22px; }
  .landing-models { display: grid; grid-template-columns: repeat(4, minmax(0, 1fr)); gap: 20px 12px; }
  .landing-models li { justify-content: center; gap: 5px; font-size: 10px; }
  .landing-models :deep(.plaza-provider-icon) { width: 17px; height: 17px; }
  .landing-integration { padding-block: 55px 60px; gap: 32px; }
  .landing-integration__copy { display: block; }
  .landing-integration h2 { font-size: 29px; }
  .landing-integration__description { font-size: 12px; }
  .landing-features { gap: 20px; margin-top: 26px; }
  .landing-doc-link { margin-top: 16px; }
  .landing-footer__bottom { grid-template-columns: 1fr auto; gap: 8px 16px; }
  .landing-footer__bottom :deep(.legal-footer-links) { grid-column: 1 / -1; grid-row: 2; }
  .landing-footer__risk { font-size: 9px; }
}
@media (prefers-reduced-motion: reduce) {
  :global(html:has(.home-landing)) { scroll-behavior: auto; }
  .home-landing * { transition: none !important; scroll-behavior: auto !important; }
}
@media (forced-colors: active) { .landing-brand-title, .landing-brand span { background: none; color: CanvasText; -webkit-text-fill-color: CanvasText; } }
@media print { .landing-brand-title, .landing-brand span { background: none; color: var(--home-ink); -webkit-text-fill-color: currentColor; } }
</style>
