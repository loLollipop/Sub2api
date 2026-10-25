<template>
  <div class="model-orbit" aria-hidden="true">
    <svg class="model-orbit__lines" viewBox="0 0 1440 720" fill="none">
      <ellipse cx="720" cy="366" rx="641" ry="230" transform="rotate(12 720 366)" />
      <ellipse cx="720" cy="352" rx="521" ry="228" transform="rotate(-17 720 352)" />
      <circle cx="272" cy="143" r="3" /><circle cx="1181" cy="493" r="4" />
      <circle cx="1044" cy="155" r="3" /><circle cx="425" cy="578" r="4" />
    </svg>
    <div v-for="(brand, index) in brands" :key="brand.platform" class="model-orbit__brand" :class="`model-orbit__brand--${brand.platform}`" :style="{ '--drift-delay': `${index * -1.8}s` }">
      <PlazaProviderIcon :platform="brand.platform" size="lg" />
      <span>{{ brand.label }}</span>
    </div>
    <span class="model-orbit__spark model-orbit__spark--one"></span>
    <span class="model-orbit__spark model-orbit__spark--two"></span>
  </div>
</template>

<script setup lang="ts">
import PlazaProviderIcon from '@/components/modelPlaza/PlazaProviderIcon.vue'
defineProps<{ brands: readonly { platform: string; label: string }[] }>()
</script>

<style scoped>
.model-orbit { position: absolute; inset: 0; pointer-events: none; user-select: none; }
.model-orbit__lines { position: absolute; width: min(1440px, 112vw); height: 100%; top: 0; left: 50%; transform: translateX(-50%); overflow: visible; }
.model-orbit__lines ellipse { stroke: var(--home-orbit-line); stroke-width: 1; }
.model-orbit__lines ellipse + ellipse { stroke-dasharray: 4 7; opacity: .6; }
.model-orbit__lines circle { fill: var(--home-orbit-point); stroke: var(--home-orbit-halo); stroke-width: 12; paint-order: stroke; }
.model-orbit__brand { position: absolute; display: flex; align-items: center; gap: 9px; padding: 10px 15px; border: 1px solid var(--home-brand-line); border-radius: 12px; background: var(--home-brand-surface); color: var(--home-ink); font-size: 12px; font-weight: 550; box-shadow: 0 6px 22px -12px var(--home-shadow); backdrop-filter: blur(12px); animation: brand-drift 9s ease-in-out infinite; animation-delay: var(--drift-delay); }
.model-orbit__brand--openai { top: 21%; left: 15%; transform: rotate(-9deg); }
.model-orbit__brand--anthropic { top: 68%; right: 12%; transform: rotate(8deg); }
.model-orbit__brand--gemini { top: 66%; left: 10%; transform: rotate(9deg); }
.model-orbit__brand--deepseek { top: 36%; right: 6%; transform: rotate(-8deg); }
.model-orbit__brand--grok { top: 8%; left: 38%; transform: rotate(5deg); opacity: .75; }
.model-orbit__brand--kimi { top: 12%; right: 25%; transform: rotate(-6deg); opacity: .75; }
.model-orbit__brand--qwen { bottom: 10%; left: 36%; transform: rotate(-8deg); }
.model-orbit__brand--zhipu { top: 39%; left: 4%; transform: rotate(-5deg); opacity: .8; }
.model-orbit__spark { position: absolute; width: 3px; height: 3px; border-radius: 50%; background: var(--home-orbit-point); opacity: .5; }
.model-orbit__spark--one { left: 24%; top: 46%; }
.model-orbit__spark--two { left: 68%; top: 20%; }
@keyframes brand-drift { 0%, 100% { translate: 0 0; } 50% { translate: 0 -9px; } }
@media (max-width: 1100px) {
  .model-orbit__brand { padding: 8px 11px; font-size: 11px; gap: 7px; }
  .model-orbit__brand--openai { left: 9%; top: 18%; }
  .model-orbit__brand--anthropic { right: 7%; top: 74%; }
  .model-orbit__brand--gemini { left: 7%; top: 72%; }
  .model-orbit__brand--deepseek { top: 29%; right: 4%; }
  .model-orbit__brand--zhipu { top: 36%; left: 3%; }
  .model-orbit__brand--grok { left: 35%; }
}
@media (max-width: 900px) {
  .model-orbit__brand--openai { top: 11%; left: 8%; }
  .model-orbit__brand--deepseek { top: 12%; right: 7%; }
  .model-orbit__brand--gemini { top: auto; bottom: 13%; left: 7%; }
  .model-orbit__brand--anthropic { top: auto; bottom: 12%; right: 7%; }
  .model-orbit__brand--grok, .model-orbit__brand--kimi, .model-orbit__brand--zhipu, .model-orbit__brand--qwen { display: none; }
}
@media (max-width: 640px) {
  .model-orbit__lines { width: 980px; opacity: .7; }
  .model-orbit__brand { padding: 7px 10px; border-radius: 10px; font-size: 10px; }
  .model-orbit__brand :deep(.plaza-provider-icon) { width: 16px; height: 16px; }
}
@media (prefers-reduced-motion: reduce) { .model-orbit__brand { animation: none; } }
</style>
