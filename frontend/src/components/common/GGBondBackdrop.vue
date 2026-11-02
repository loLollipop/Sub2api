<template>
  <div class="ggbond-backdrop" :class="`ggbond-backdrop--${variant}`" aria-hidden="true">
    <div class="ggbond-backdrop__light" />
    <svg class="ggbond-backdrop__crest" viewBox="0 0 480 540" fill="none">
      <!-- Original pig-hero insignia: a helmet, a snout and a protective shield. -->
      <path class="crest-field" d="M240 28 426 108v156c0 110-79 187-186 242C133 451 54 374 54 264V108Z" />
      <path class="crest-rim" d="m240 48 166 72v144c0 96-68 166-166 220C142 430 74 360 74 264V120Z" />
      <path class="crest-ear" d="m112 190-5-82 83 43m100 0 83-43-5 82" />
      <path class="crest-helmet" d="M116 250c0-84 49-139 124-139s124 55 124 139v37c0 75-51 123-124 123s-124-48-124-123Z" />
      <path class="crest-brow" d="m132 218 71 20 37-34 37 34 71-20-20 61h-50l-38-21-38 21h-50Z" />
      <rect class="crest-snout" x="173" y="294" width="134" height="83" rx="41.5" />
      <path class="crest-nostrils" d="M214 326v18m52-18v18" />
      <path class="crest-energy" d="m249 138-31 38h25l-12 28 35-42h-25Z" />
      <path class="crest-orbit" d="M28 274C7 187 57 89 151 52m181 4c91 44 137 137 116 225M118 449l31 22m186-22-31 22" />
      <circle class="crest-node" cx="42" cy="203" r="5" />
      <circle class="crest-node" cx="430" cy="162" r="4" />
    </svg>
    <svg class="ggbond-backdrop__trail" viewBox="0 0 1000 600" fill="none" preserveAspectRatio="xMinYMax slice">
      <path d="M-90 558 132 336h176L566 78h226L976-106M-90 596l260-222h172L598 116h216L998-68" />
      <path d="M-100 432H42l168-168h176L610 40" />
      <circle cx="308" cy="336" r="5" /><circle cx="386" cy="264" r="4" />
      <path class="trail-spark" d="m150 410 7 13 14 7-14 7-7 14-7-14-14-7 14-7Z" />
    </svg>
  </div>
</template>

<script setup lang="ts">
withDefaults(defineProps<{ variant?: 'shell' | 'home' | 'auth' }>(), { variant: 'shell' })
</script>

<style scoped>
.ggbond-backdrop {
  --hero-red: #c9573e;
  --hero-gold: #c5913e;
  --hero-glow: #f0cf9b6b;
  --hero-blush: #eeb8a74a;
  position: fixed;
  inset: 0;
  z-index: -1;
  overflow: hidden;
  contain: strict;
  pointer-events: none;
  user-select: none;
}
.dark .ggbond-backdrop {
  --hero-red: #cc7968;
  --hero-gold: #c5a477;
  --hero-glow: #b65e2608;
  --hero-blush: #963a3006;
}
.dark .ggbond-backdrop__crest { opacity: .85; }
.dark .ggbond-backdrop__trail { opacity: .8; stroke-opacity: .14; }
.dark .crest-field { fill: none; stroke-opacity: .19; }
.dark .crest-rim { stroke-opacity: .32; }
.dark :is(.crest-ear, .crest-helmet) { fill: none; stroke-opacity: .26; }
.dark .crest-brow { fill-opacity: .025; stroke-opacity: .32; }
.dark .crest-snout { fill: none; stroke-opacity: .3; }
.ggbond-backdrop__light {
  position: absolute;
  inset: 0;
  background:
    radial-gradient(ellipse 76% 68% at 100% 0%, var(--hero-glow), transparent 80%),
    radial-gradient(ellipse 52% 70% at 0% 84%, var(--hero-blush), transparent 76%);
}
.ggbond-backdrop__crest {
  position: absolute;
  top: 11vh;
  right: -156px;
  width: clamp(420px, 41vw, 640px);
  color: var(--hero-red);
  transform: rotate(12deg);
}
.crest-field { fill: currentColor; fill-opacity: .026; stroke: currentColor; stroke-opacity: .1; stroke-width: 1; }
.crest-rim { stroke: var(--hero-gold); stroke-opacity: .24; stroke-width: 1; }
.crest-ear, .crest-helmet { fill: currentColor; fill-opacity: .025; stroke: currentColor; stroke-opacity: .17; stroke-width: 1.5; stroke-linejoin: round; }
.crest-brow { fill: var(--hero-gold); fill-opacity: .075; stroke: var(--hero-gold); stroke-opacity: .25; stroke-width: 1.5; }
.crest-snout { fill: var(--hero-gold); fill-opacity: .06; stroke: currentColor; stroke-opacity: .2; stroke-width: 1.5; }
.crest-nostrils { stroke: currentColor; stroke-opacity: .22; stroke-width: 7; stroke-linecap: round; }
.crest-energy { fill: var(--hero-gold); fill-opacity: .3; }
.crest-orbit { stroke: var(--hero-gold); stroke-opacity: .2; stroke-width: 1; stroke-linecap: round; }
.crest-node { fill: var(--hero-gold); fill-opacity: .35; }
.ggbond-backdrop__trail {
  position: absolute;
  bottom: -80px;
  left: -90px;
  width: min(76vw, 1000px);
  height: 620px;
  color: var(--hero-red);
  stroke: currentColor;
  stroke-width: 1;
  stroke-opacity: .085;
}
.ggbond-backdrop__trail circle, .trail-spark { fill: var(--hero-gold); fill-opacity: .22; stroke: none; }
.ggbond-backdrop--home { position: absolute; }
.ggbond-backdrop--home .ggbond-backdrop__crest { top: 98px; right: -132px; width: clamp(420px, 43vw, 680px); }
.ggbond-backdrop--home .ggbond-backdrop__trail { bottom: 40px; }
.ggbond-backdrop--auth .ggbond-backdrop__crest { right: -75px; top: 15vh; }
@media (max-width: 767px) {
  .ggbond-backdrop__crest, .ggbond-backdrop--home .ggbond-backdrop__crest, .ggbond-backdrop--auth .ggbond-backdrop__crest { top: 120px; right: -208px; width: 420px; }
  .ggbond-backdrop__trail { width: 650px; height: 450px; left: -220px; opacity: .65; }
}
@media (forced-colors: active), print { .ggbond-backdrop { display: none; } }
</style>
