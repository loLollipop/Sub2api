<template>
  <header class="workspace-page-header">
    <div class="workspace-page-copy">
      <div class="workspace-page-title">
        <h1>{{ title }}</h1>
        <span v-if="count !== undefined" class="workspace-page-count">{{ count.toLocaleString() }}</span>
      </div>
      <p v-if="description" class="workspace-page-description">{{ description }}</p>
    </div>
    <div v-if="$slots.actions" class="workspace-page-actions"><slot name="actions" /></div>
  </header>
</template>

<script setup lang="ts">
defineProps<{ title: string; description?: string; count?: number }>()
</script>

<style scoped>
.workspace-page-header { display: flex; align-items: center; justify-content: space-between; flex-wrap: wrap; gap: 12px 20px; min-width: 0; }
.workspace-page-copy { min-width: 0; }
.workspace-page-title { display: flex; align-items: center; flex-wrap: wrap; gap: 10px; }
.workspace-page-title h1 { margin: 0; color: var(--signal-text, #1f2937); font-size: 20px; line-height: 28px; font-weight: 600; letter-spacing: -.025em; overflow-wrap: anywhere; }
.workspace-page-count { padding: 1px 7px; border: 1px solid var(--signal-line, #e5e7eb); border-radius: 6px; color: var(--signal-muted, #6b7280); font-size: 12px; line-height: 20px; font-weight: 500; font-variant-numeric: tabular-nums; }
.workspace-page-description { margin: 5px 0 0; color: var(--signal-muted, #6b7280); font-size: 13px; line-height: 21px; }
.workspace-page-actions { display: flex; align-items: center; flex-wrap: wrap; gap: 8px; min-width: 0; }
@media (max-width: 639px) { .workspace-page-header { gap: 12px; } .workspace-page-title h1 { font-size: 19px; } }
</style>
