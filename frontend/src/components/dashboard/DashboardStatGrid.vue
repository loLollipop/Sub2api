<template><div class="overview-stat-grid"><slot /></div></template>

<style scoped>
.overview-stat-grid { display: grid; grid-template-columns: repeat(4, minmax(0, 1fr)); gap: 1px; min-width: 0; overflow: hidden; border: 1px solid var(--signal-line); border-radius: 12px; background: var(--signal-surface); }
.overview-stat-grid :deep(> *) { background: var(--signal-surface); box-shadow: 0 0 0 1px var(--signal-line); }
@media (max-width: 1023px) {
  .overview-stat-grid { grid-template-columns: repeat(2, minmax(0, 1fr)); gap: 24px; overflow: visible; border: 0; border-radius: 0; background: transparent; }
  .overview-stat-grid :deep(> *) { border: 1px solid var(--signal-line); border-radius: 12px; box-shadow: none; }
}
@media (max-width: 639px) { .overview-stat-grid { gap: 12px; } }
</style>
