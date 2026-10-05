<template>
  <div class="home-landing" :class="{ 'home-landing--dark': isDark }" data-testid="default-home">
    <header class="landing-header">
      <nav class="landing-nav landing-container" :aria-label="t('home.landing.navigation')">
        <router-link to="/" class="landing-brand" :title="siteName">
          <img :src="siteLogo || '/logo.svg'" alt="" width="34" height="34" />
          <span>{{ siteName }}</span>
        </router-link>
        <div v-if="docUrl" class="landing-nav__links">
          <a :href="docUrl" target="_blank" rel="noopener noreferrer">{{ t('home.docs') }}</a>
        </div>
        <div class="landing-nav__actions">
          <LocaleSwitcher />
          <button
            type="button"
            class="landing-theme"
            :aria-label="isDark ? t('home.switchToLight') : t('home.switchToDark')"
            :title="isDark ? t('home.switchToLight') : t('home.switchToDark')"
            @click="$emit('toggleTheme')"
          >
            <Icon :name="isDark ? 'sun' : 'moon'" size="md" aria-hidden="true" />
          </button>
          <router-link :to="entryPath" class="landing-login" data-testid="home-account-entry">
            <span v-if="isAuthenticated && userInitial" class="landing-account-mark" aria-hidden="true">{{ userInitial }}</span>
            <span>{{ isAuthenticated ? t('home.dashboard') : t('home.login') }}</span>
            <Icon name="arrowUpRight" size="sm" aria-hidden="true" />
          </router-link>
        </div>
      </nav>
    </header>

    <main class="landing-container landing-main">
      <div class="landing-stage">
        <section class="landing-hero" aria-labelledby="landing-title">
          <span class="landing-eyebrow">AI / API GATEWAY</span>
          <h1 id="landing-title"><span class="landing-brand-title">{{ siteName }}</span><span class="landing-brand-dot" aria-hidden="true">.</span></h1>
          <p v-if="siteSubtitle.trim()" class="landing-subtitle">{{ siteSubtitle }}</p>
          <div class="landing-hero__actions">
            <router-link :to="entryPath" class="landing-button" data-testid="home-primary-cta">
              {{ isAuthenticated ? t('home.goToDashboard') : t('home.getStarted') }}
              <Icon name="arrowRight" size="md" aria-hidden="true" />
            </router-link>
            <a v-if="docUrl" :href="docUrl" target="_blank" rel="noopener noreferrer" class="landing-text-link"><Icon name="book" size="sm" aria-hidden="true" />{{ t('home.docs') }}<Icon name="arrowUpRight" size="sm" aria-hidden="true" /></a>
          </div>
        </section>
        <div id="integration" class="landing-integration"><HomeApiPreview :api-base-url="apiBaseUrl" /></div>
      </div>

      <section class="landing-features" :aria-label="t('home.landing.capabilities')" data-testid="home-capabilities">
        <article v-for="feature in features" :key="feature.key" class="landing-feature" :class="`landing-feature--${feature.key}`">
          <div class="home-feature-mark"><Icon :name="feature.icon" size="lg" aria-hidden="true" /></div>
          <div><h2>{{ t(`home.landing.${feature.key}Title`) }}</h2><p>{{ t(`home.landing.${feature.key}Description`) }}</p></div>
        </article>
      </section>
    </main>

    <footer class="landing-footer">
      <div class="landing-container">
        <div class="landing-footer__bottom">
          <p>&copy; {{ new Date().getFullYear() }} {{ siteName }}</p>
          <a :href="githubUrl" target="_blank" rel="noopener noreferrer">GitHub <Icon name="arrowUpRight" size="xs" aria-hidden="true" /></a>
        </div>
        <LegalFooterLinks />
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
import HomeApiPreview from './HomeApiPreview.vue'

const props = defineProps<{
  siteName: string
  siteLogo: string
  siteSubtitle: string
  docUrl: string
  apiBaseUrl: string
  githubUrl: string
  isDark: boolean
  isAuthenticated: boolean
  userInitial: string
  dashboardPath: string
}>()
defineEmits<{ toggleTheme: [] }>()
const { t } = useI18n()
const entryPath = computed(() => props.isAuthenticated ? props.dashboardPath : '/login')
const features = [
  { key: 'connect', icon: 'server' },
  { key: 'routing', icon: 'shield' },
  { key: 'usage', icon: 'chart' },
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
  /* Same neutral surfaces / blue accent as the application's SIGNAL console. */
  --home-bg: #f6f7fb;
  --home-panel: #ffffff;
  --home-code-bg: #f2f4f8;
  --home-ink: #1c1c1e;
  --home-muted: #63636c;
  --home-line: #dedee4;
  --home-accent: #0066d6;
  --home-accent-soft: #e3efff;
  --home-on-accent: #ffffff;
  --home-shadow: #1f528540;
  --home-title-gradient: linear-gradient(110deg, #17396c 0%, #0066d6 56%, #087e9e 100%);
  --syntax-command: #7c3db5;
  --syntax-url: #0066d6;
  --syntax-key: #a05213;
  --syntax-string: #087b68;
  --syntax-placeholder: #475b80;
  display: flex;
  flex-direction: column;
  min-height: 100vh;
  background: radial-gradient(ellipse at 79% 36%, #dbeaff99, transparent 48%), linear-gradient(180deg, #fafbfe, var(--home-bg) 74%, #fafbfe);
  color: var(--home-ink);
  -webkit-font-smoothing: antialiased;
}
.home-landing--dark {
  --home-bg: #111113;
  --home-panel: #1c1c1e;
  --home-code-bg: #222226;
  --home-ink: #f2f2f7;
  --home-muted: #aeaeb2;
  --home-line: #38383b;
  --home-accent: #78b4ff;
  --home-accent-soft: #22364e;
  --home-on-accent: #10253d;
  --home-shadow: #00000070;
  --home-title-gradient: linear-gradient(110deg, #e1ecff 0%, #84baff 56%, #66d1d8 100%);
  --syntax-command: #d6a7ff;
  --syntax-url: #78b4ff;
  --syntax-key: #edb477;
  --syntax-string: #81d3b6;
  --syntax-placeholder: #b8c9ea;
  background: radial-gradient(ellipse at 79% 36%, #183d642b, transparent 48%), var(--home-bg);
}
.landing-container { width: min(1200px, calc(100% - 96px)); margin-inline: auto; }
.landing-header { position: relative; z-index: 10; border-bottom: 1px solid var(--home-line); background: var(--home-panel); }
.landing-nav { display: flex; min-height: 80px; align-items: center; justify-content: space-between; gap: 24px; }
.landing-brand { display: flex; align-items: center; gap: 10px; min-width: 0; font-size: 19px; font-weight: 700; letter-spacing: -0.035em; }
.landing-brand img { flex-shrink: 0; border-radius: 8px; object-fit: contain; }
.landing-brand span { max-width: 240px; overflow: hidden; text-overflow: ellipsis; white-space: nowrap; }
.landing-nav__links { display: flex; align-items: center; gap: 27px; color: var(--home-muted); font-size: 13px; }
.landing-nav__links a { display: inline-flex; align-items: center; min-height: 40px; }
.landing-nav a:hover, .landing-text-link:hover { color: var(--home-accent); }
.landing-nav__actions { display: flex; align-items: center; gap: 12px; flex-shrink: 0; }
.landing-theme { display: grid; width: 38px; height: 40px; place-items: center; border-radius: 8px; color: var(--home-muted); }
.landing-theme:hover { background: var(--home-line); }
.landing-login { display: inline-flex; align-items: center; gap: 8px; min-height: 40px; padding-inline: 15px; border: 1px solid var(--home-line); border-radius: 8px; font-size: 13px; font-weight: 600; }
.landing-account-mark { display: grid; flex-shrink: 0; place-items: center; width: 24px; height: 24px; margin-left: -7px; border-radius: 6px; background: var(--home-accent); color: var(--home-on-accent); font-size: 12px; font-weight: 650; }
.landing-main { display: flex; flex: 1; flex-direction: column; justify-content: center; gap: 68px; padding-block: 76px 70px; }
.landing-stage { display: grid; grid-template-columns: 1fr 1.1fr; align-items: center; gap: 75px; min-width: 0; }
.landing-hero { min-width: 0; padding-block: 25px; }
.landing-eyebrow { display: inline-flex; align-items: center; gap: 10px; padding: 7px 11px; border: 1px solid var(--home-line); border-radius: 6px; background: var(--home-panel); color: var(--home-accent); font: 500 10px/1.5 ui-monospace, monospace; letter-spacing: 0.11em; }
.landing-eyebrow::before { content: ''; width: 5px; height: 5px; border-radius: 50%; background: var(--home-accent); }
.landing-hero h1 { margin: 28px 0 0; font-family: 'Home Outfit', 'Segoe UI', 'PingFang SC', 'Microsoft YaHei', sans-serif; font-size: clamp(62px, 8vw, 112px); font-weight: 700; letter-spacing: -0.06em; line-height: 1.08; overflow-wrap: anywhere; }
.landing-brand-title { color: var(--home-accent); }
@supports (background-clip: text) or (-webkit-background-clip: text) {
  .landing-brand-title { background: var(--home-title-gradient); -webkit-background-clip: text; background-clip: text; color: transparent; }
}
.landing-brand-dot { color: var(--home-accent); }
.landing-subtitle { max-width: 450px; margin-top: 20px; color: var(--home-muted); font-size: 20px; font-weight: 400; letter-spacing: 0.015em; line-height: 1.65; white-space: pre-wrap; overflow-wrap: anywhere; }
.landing-hero__actions { display: flex; align-items: center; flex-wrap: wrap; gap: 14px; margin-top: 34px; }
.landing-button { display: inline-flex; justify-content: center; align-items: center; gap: 26px; min-height: 50px; padding: 13px 22px; border: 1px solid var(--home-accent); border-radius: 8px; background: var(--home-accent); color: var(--home-on-accent); box-shadow: 0 5px 14px #0066d626; font-size: 14px; font-weight: 600; transition: box-shadow 150ms, filter 150ms; }
.landing-button:hover { filter: brightness(1.08); box-shadow: 0 6px 20px #0066d638; }
.landing-text-link { display: inline-flex; align-items: center; justify-content: center; gap: 9px; min-height: 50px; padding-inline: 17px; border: 1px solid var(--home-line); border-radius: 8px; background: var(--home-panel); color: var(--home-ink); font-size: 13px; }
.landing-integration { min-width: 0; scroll-margin-top: 25px; }
.landing-features { display: grid; grid-template-columns: repeat(3, minmax(0, 1fr)); gap: 30px; padding: 28px 32px; border: 1px solid var(--home-line); border-radius: 14px; background: var(--home-panel); box-shadow: 0 4px 18px #1c1c1e03; }
.landing-feature { display: flex; align-items: center; gap: 17px; min-width: 0; }
.landing-feature + .landing-feature { border-left: 1px solid var(--home-line); padding-left: 30px; }
.home-feature-mark { display: grid; place-items: center; width: 45px; height: 45px; flex-shrink: 0; border-radius: 10px; background: var(--home-accent-soft); color: var(--home-accent); }
.landing-feature--routing .home-feature-mark { color: #087b68; background: #e5f4ee; }
.landing-feature--usage .home-feature-mark { color: #946214; background: #faf0da; }
.home-landing--dark .landing-feature--routing .home-feature-mark { color: #81d3b6; background: #233d34; }
.home-landing--dark .landing-feature--usage .home-feature-mark { color: #edbc73; background: #3b3224; }
.landing-feature h2 { font-size: 14px; font-weight: 600; }
.landing-feature p { margin-top: 5px; color: var(--home-muted); font-size: 12px; line-height: 1.7; }
.landing-footer { padding-block: 22px; border-top: 1px solid var(--home-line); color: var(--home-muted); }
.landing-footer__bottom { display: flex; justify-content: space-between; align-items: center; gap: 20px; margin-bottom: 15px; font-size: 11px; }
.landing-footer__bottom > p { overflow-wrap: anywhere; }
.landing-footer__bottom a { display: inline-flex; align-items: center; gap: 5px; min-height: 30px; }
.landing-footer__risk { max-width: 850px; margin: 12px auto 0; text-align: center; font-size: 10px; line-height: 1.7; }
.home-landing :is(a, button):focus-visible { outline: 2px solid var(--home-accent); outline-offset: 4px; }
@media (max-width: 1050px) {
  .landing-container { width: calc(100% - 48px); }
  .landing-nav { flex-wrap: wrap; gap: 8px 16px; padding-block: 15px; }
  .landing-brand { flex: 1; }
  .landing-nav__links { order: 3; justify-content: center; width: 100%; gap: 26px; }
  .landing-stage { gap: 30px; grid-template-columns: 1fr 1.1fr; }
  .landing-hero h1 { font-size: clamp(60px, 8vw, 82px); }
  .landing-feature + .landing-feature { padding-left: 20px; }
  .landing-features { gap: 20px; padding-inline: 24px; }
}
@media (max-width: 760px) {
  .landing-container { width: calc(100% - 36px); }
  .landing-brand { gap: 8px; font-size: 17px; }
  .landing-brand span { max-width: min(160px, 36vw); }
  .landing-brand img { width: 30px; height: 30px; }
  .landing-nav__actions { gap: 3px; }
  .landing-login { padding-inline: 6px; font-size: 12px; }
  .landing-account-mark { width: 20px; height: 20px; margin-left: 0; font-size: 11px; }
  .landing-login svg { display: none; }
  .landing-main { gap: 30px; padding-block: 36px; }
  .landing-stage { grid-template-columns: 1fr; gap: 30px; }
  .landing-hero { padding-block: 12px; }
  .landing-hero h1 { font-size: clamp(56px, 13vw, 80px); }
  .landing-subtitle { margin-top: 18px; font-size: 16px; }
  .landing-hero__actions { margin-top: 24px; }
  .landing-features { grid-template-columns: 1fr; gap: 20px; padding: 24px; }
  .landing-feature + .landing-feature { border-left: none; border-top: 1px solid var(--home-line); padding: 20px 0 0; }
  .landing-footer { padding-block: 16px; }
}
@media (prefers-reduced-motion: reduce) { .home-landing * { transition: none !important; scroll-behavior: auto !important; } }
@media (forced-colors: active) { .landing-brand-title { background: none; color: CanvasText; } }
@media print { .landing-brand-title { background: none; color: var(--home-ink); } }
</style>
