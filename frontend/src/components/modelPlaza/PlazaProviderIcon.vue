<template>
  <span class="plaza-provider-icon" :data-provider="provider" :style="dimensions" aria-hidden="true">
    <PlatformIcon v-if="nativeProviders.has(provider)" :platform="provider as GroupPlatform" :size="size" />
    <ModelIcon v-else-if="iconModel" :model="iconModel" size="100%" />
    <svg v-else viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="1.5">
      <rect x="4" y="4" width="16" height="16" rx="5" />
      <path d="M9 9h6v6H9zm0-7v2m6-2v2M9 20v2m6-2v2M2 9h2m-2 6h2m16-6h2m-2 6h2" />
    </svg>
  </span>
</template>

<script setup lang="ts">
import { computed } from 'vue'
import PlatformIcon from '@/components/common/PlatformIcon.vue'
import ModelIcon from '@/components/common/ModelIcon.vue'
import type { GroupPlatform } from '@/types'
import { modelProvider } from './modelPlazaProviders'

const props = withDefaults(defineProps<{ platform: string; model?: string; size?: 'xs' | 'sm' | 'md' | 'lg' }>(), { size: 'sm' })
const provider = computed(() => props.model ? modelProvider({ name: props.model, platform: props.platform }) : props.platform)
const nativeProviders = new Set(['openai', 'anthropic', 'gemini', 'grok', 'deepseek', 'kimi', 'zhipu', 'minimax', 'antigravity', 'cline', 'command_code'])
const iconModels: Record<string, string> = {
  qwen: 'qwen', doubao: 'doubao', baidu: 'ernie', hunyuan: 'hunyuan',
  yi: 'yi-model', spark: 'spark', mistral: 'mistral', meta: 'llama', cohere: 'command-r',
}
const iconModel = computed(() => iconModels[provider.value])
const dimensions = computed(() => {
  const size = { xs: '12px', sm: '14px', md: '16px', lg: '20px' }[props.size]
  return { '--plaza-logo-size': size }
})
</script>

<style scoped>
.plaza-provider-icon { display: inline-flex; align-items: center; justify-content: center; flex-shrink: 0; width: var(--plaza-logo-size); height: var(--plaza-logo-size); color: var(--signal-accent, #0066d6); }
.plaza-provider-icon > :deep(svg) { width: 100%; height: 100%; }
.plaza-provider-icon[data-provider='anthropic'] { color: #c47754; }
.plaza-provider-icon[data-provider='deepseek'] { color: #4d6bfe; }
.plaza-provider-icon[data-provider='gemini'] { color: #4285f4; }
.plaza-provider-icon[data-provider='qwen'] { color: #615eff; }
.plaza-provider-icon[data-provider='kimi'] { color: #3867ef; }
.plaza-provider-icon[data-provider='zhipu'] { color: #6366f1; }
.plaza-provider-icon[data-provider='minimax'] { color: #f43f5e; }
.plaza-provider-icon[data-provider='antigravity'] { color: #a855f7; }
.plaza-provider-icon[data-provider='cline'] { color: #8b5cf6; }
.plaza-provider-icon[data-provider='openai'],
.plaza-provider-icon[data-provider='grok'],
.plaza-provider-icon[data-provider='command_code'] { color: var(--signal-text, #1c1c1e); }
.plaza-provider-icon[data-provider='other'] { color: var(--signal-muted, #63636c); }
</style>
