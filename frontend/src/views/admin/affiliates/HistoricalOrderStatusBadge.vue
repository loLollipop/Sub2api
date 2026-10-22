<template>
  <span
    class="affiliate-order-status inline-flex items-center rounded-full px-2.5 py-0.5 text-xs font-medium"
    :data-status="status"
    :class="statusClass"
  >
    {{ statusLabel }}
  </span>
</template>

<script setup lang="ts">
import { computed } from 'vue'
import { useI18n } from 'vue-i18n'
type OrderStatus = 'PENDING' | 'PAID' | 'RECHARGING' | 'COMPLETED' | 'EXPIRED' | 'CANCELLED' | 'FAILED' | 'REFUND_REQUESTED' | 'REFUNDING' | 'REFUND_PENDING' | 'REFUNDED' | 'PARTIALLY_REFUNDED' | 'REFUND_FAILED'

const props = defineProps<{
  status: OrderStatus
}>()

const { t } = useI18n()

const statusMap: Record<OrderStatus, { key: string; class: string }> = {
  PENDING: { key: 'admin.affiliates.orderStatus.pending', class: 'bg-yellow-100 text-yellow-800 dark:bg-yellow-900/30 dark:text-yellow-400' },
  PAID: { key: 'admin.affiliates.orderStatus.paid', class: 'bg-blue-100 text-blue-800 dark:bg-blue-900/30 dark:text-blue-400' },
  RECHARGING: { key: 'admin.affiliates.orderStatus.recharging', class: 'bg-blue-100 text-blue-800 dark:bg-blue-900/30 dark:text-blue-400' },
  COMPLETED: { key: 'admin.affiliates.orderStatus.completed', class: 'bg-green-100 text-green-800 dark:bg-green-900/30 dark:text-green-400' },
  EXPIRED: { key: 'admin.affiliates.orderStatus.expired', class: 'bg-gray-100 text-gray-800 dark:bg-gray-900/30 dark:text-gray-400' },
  CANCELLED: { key: 'admin.affiliates.orderStatus.cancelled', class: 'bg-gray-100 text-gray-800 dark:bg-gray-900/30 dark:text-gray-400' },
  FAILED: { key: 'admin.affiliates.orderStatus.failed', class: 'bg-red-100 text-red-800 dark:bg-red-900/30 dark:text-red-400' },
  REFUND_REQUESTED: { key: 'admin.affiliates.orderStatus.refund_requested', class: 'bg-orange-100 text-orange-800 dark:bg-orange-900/30 dark:text-orange-400' },
  REFUNDING: { key: 'admin.affiliates.orderStatus.refunding', class: 'bg-orange-100 text-orange-800 dark:bg-orange-900/30 dark:text-orange-400' },
  REFUND_PENDING: { key: 'admin.affiliates.orderStatus.refund_pending', class: 'bg-orange-100 text-orange-800 dark:bg-orange-900/30 dark:text-orange-400' },
  REFUNDED: { key: 'admin.affiliates.orderStatus.refunded', class: 'bg-purple-100 text-purple-800 dark:bg-purple-900/30 dark:text-purple-400' },
  PARTIALLY_REFUNDED: { key: 'admin.affiliates.orderStatus.partially_refunded', class: 'bg-purple-100 text-purple-800 dark:bg-purple-900/30 dark:text-purple-400' },
  REFUND_FAILED: { key: 'admin.affiliates.orderStatus.refund_failed', class: 'bg-red-100 text-red-800 dark:bg-red-900/30 dark:text-red-400' },
}

const statusLabel = computed(() => {
  const entry = statusMap[props.status]
  return entry ? t(entry.key) : props.status
})

const statusClass = computed(() => {
  const entry = statusMap[props.status]
  return entry?.class ?? 'bg-gray-100 text-gray-800 dark:bg-gray-900/30 dark:text-gray-400'
})
</script>
