<template>
  <section class="dashboard-chart-card">
    <header class="dashboard-chart-header">
      <div class="dashboard-chart-heading">
        <h2>{{ title }}</h2>
        <p v-if="value !== undefined || $slots.value" class="dashboard-chart-value"><slot name="value">{{ value }}</slot></p>
      </div>
      <div v-if="$slots.actions" class="dashboard-chart-actions"><slot name="actions" /></div>
    </header>
    <slot />
  </section>
</template>

<script setup lang="ts">
defineProps<{ title: string; value?: string | number }>()
</script>

<style scoped>
.dashboard-chart-card { min-width: 0; overflow: hidden; border: 1px solid var(--signal-line); border-radius: 12px; background: var(--signal-surface); }
.dashboard-chart-header { display: flex; flex-wrap: wrap; align-items: flex-start; justify-content: space-between; gap: 12px; padding: 20px; border-bottom: 1px solid var(--signal-line); }
.dashboard-chart-heading { min-width: 0; }
.dashboard-chart-heading h2 { color: var(--signal-muted); font-size: 12px; line-height: 20px; font-weight: 400; text-transform: uppercase; }
.dashboard-chart-value { margin-top: 6px; color: var(--signal-text); font-size: 30px; line-height: 36px; font-weight: 600; font-variant-numeric: tabular-nums; letter-spacing: -.5px; }
.dashboard-chart-actions { display: flex; flex-wrap: wrap; align-items: center; gap: 8px; min-width: 0; }
</style>
