<template>
  <PlatformIcon :platform="platform as GroupPlatform" :size="size" class="plaza-provider-icon" :data-provider="platform" aria-hidden="true" />
</template>

<script setup lang="ts">
import PlatformIcon from '@/components/common/PlatformIcon.vue'
import type { GroupPlatform } from '@/types'

withDefaults(defineProps<{ platform: string; size?: 'xs' | 'sm' | 'md' | 'lg' }>(), { size: 'sm' })
</script>

<style scoped>
.plaza-provider-icon { color: var(--signal-accent, #0066d6); }
.plaza-provider-icon[data-provider='anthropic'] { color: #c47754; }
.plaza-provider-icon[data-provider='deepseek'] { color: #4d6bfe; }
.plaza-provider-icon[data-provider='gemini'] { color: #4285f4; }
.plaza-provider-icon[data-provider='kimi'] { color: #3867ef; }
.plaza-provider-icon[data-provider='zhipu'] { color: #6366f1; }
.plaza-provider-icon[data-provider='minimax'] { color: #f43f5e; }
.plaza-provider-icon[data-provider='antigravity'] { color: #a855f7; }
.plaza-provider-icon[data-provider='cline'] { color: #8b5cf6; }
.plaza-provider-icon[data-provider='openai'],
.plaza-provider-icon[data-provider='grok'],
.plaza-provider-icon[data-provider='command_code'] { color: var(--signal-text, #1c1c1e); }
</style>
