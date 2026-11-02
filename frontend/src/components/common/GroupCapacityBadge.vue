<template>
  <div class="flex flex-col gap-1" :title="runtimeKnown ? t('admin.groups.capacityRuntimeHint') : t('admin.groups.capacityUnknownHint')">
    <!-- 并发槽位 -->
    <div class="flex items-center gap-1">
      <span
        :class="[
          'inline-flex items-center gap-1 rounded-md px-1.5 py-0.5 text-[10px] font-medium',
          capacityClass(concurrencyUsed, concurrencyMax)
        ]"
      >
        <Icon name="grid" size="none" class="h-2.5 w-2.5" :stroke-width="2" />
        <span class="font-mono">{{ runtimeKnown ? concurrencyUsed : '—' }}</span>
        <span class="text-gray-400 dark:text-gray-500">/</span>
        <span class="font-mono">{{ concurrencyMax }}</span>
      </span>
    </div>

    <!-- 会话数 -->
    <div v-if="sessionsMax > 0" class="flex items-center gap-1">
      <span
        :class="[
          'inline-flex items-center gap-1 rounded-md px-1.5 py-0.5 text-[10px] font-medium',
          capacityClass(sessionsUsed, sessionsMax)
        ]"
      >
        <Icon name="usersRounded" size="none" class="h-2.5 w-2.5" :stroke-width="2" />
        <span class="font-mono">{{ runtimeKnown ? sessionsUsed : '—' }}</span>
        <span class="text-gray-400 dark:text-gray-500">/</span>
        <span class="font-mono">{{ sessionsMax }}</span>
      </span>
    </div>

    <!-- RPM -->
    <div v-if="rpmMax > 0" class="flex items-center gap-1">
      <span
        :class="[
          'inline-flex items-center gap-1 rounded-md px-1.5 py-0.5 text-[10px] font-medium',
          capacityClass(rpmUsed, rpmMax)
        ]"
      >
        <Icon name="clock" size="none" class="h-2.5 w-2.5" :stroke-width="1.5" />
        <span class="font-mono">{{ runtimeKnown ? rpmUsed : '—' }}</span>
        <span class="text-gray-400 dark:text-gray-500">/</span>
        <span class="font-mono">{{ rpmMax }}</span>
      </span>
    </div>
  </div>
</template>

<script setup lang="ts">
import { useI18n } from 'vue-i18n'
import Icon from '@/components/icons/Icon.vue'
const { t } = useI18n()
interface Props {
  concurrencyUsed: number
  concurrencyMax: number
  sessionsUsed: number
  sessionsMax: number
  rpmUsed: number
  rpmMax: number
  runtimeKnown?: boolean
}

const props = withDefaults(defineProps<Props>(), {
  concurrencyUsed: 0,
  concurrencyMax: 0,
  sessionsUsed: 0,
  sessionsMax: 0,
  rpmUsed: 0,
  rpmMax: 0,
  runtimeKnown: true
})

function capacityClass(used: number, max: number): string {
  if (!props.runtimeKnown) return 'bg-gray-100 text-gray-600 dark:bg-gray-800 dark:text-gray-400'
  if (max > 0 && used >= max) {
    return 'bg-red-100 text-red-700 dark:bg-red-900/30 dark:text-red-400'
  }
  if (used > 0) {
    return 'bg-yellow-100 text-yellow-700 dark:bg-yellow-900/30 dark:text-yellow-400'
  }
  return 'bg-gray-100 text-gray-600 dark:bg-gray-800 dark:text-gray-400'
}
</script>
