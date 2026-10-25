<template>
  <AppLayout :public-browse="!auth.isAuthenticated">
    <LeaderboardContent :data="data" :period="period" :loading="loading" :error="error" @update:period="setPeriod" @refresh="load" />
  </AppLayout>
</template>

<script setup lang="ts">
import { onBeforeUnmount, ref, watch } from 'vue'
import { useRoute, useRouter } from 'vue-router'
import AppLayout from '@/components/layout/AppLayout.vue'
import LeaderboardContent from '@/components/leaderboard/LeaderboardContent.vue'
import { getLeaderboard, type LeaderboardPeriod, type LeaderboardResponse } from '@/api/leaderboard'
import { useAuthStore } from '@/stores/auth'

const route = useRoute()
const router = useRouter()
const auth = useAuthStore()
const periods = new Set(['day', 'week', 'month', 'year'])
const period = ref<LeaderboardPeriod>('week')
const data = ref<LeaderboardResponse | null>(null)
const loading = ref(true)
const error = ref<'failed' | 'disabled' | null>(null)
let pending: AbortController | undefined
let requestSequence = 0

async function load() {
  pending?.abort()
  const controller = new AbortController()
  pending = controller
  const sequence = ++requestSequence
  loading.value = true
  error.value = null
  // Avoid showing another period's numbers alongside the newly selected tab.
  data.value = null
  try {
    const response = await getLeaderboard(period.value, controller.signal)
    if (sequence === requestSequence) data.value = response
  } catch (cause) {
    if (controller.signal.aborted || sequence !== requestSequence) return
    const status = (cause as { status?: number; response?: { status?: number } }).status ?? (cause as { response?: { status?: number } }).response?.status
    if (status === 401) {
      void router.replace({ path: '/login', query: { redirect: route.fullPath } })
    } else {
      error.value = status === 404 ? 'disabled' : 'failed'
    }
  } finally {
    if (sequence === requestSequence) loading.value = false
  }
}

function setPeriod(value: LeaderboardPeriod) {
  if (value !== period.value) void router.replace({ query: { ...route.query, period: value } })
}

watch(() => route.query.period, value => {
  period.value = typeof value === 'string' && periods.has(value) ? value as LeaderboardPeriod : 'week'
  void load()
}, { immediate: true })
onBeforeUnmount(() => { ++requestSequence; pending?.abort() })
</script>
