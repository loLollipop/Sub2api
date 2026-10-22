import { computed } from 'vue'
import { useRoute } from 'vue-router'
import { useAdminSettingsStore, useAuthStore } from '@/stores'

/** Authorization stays role-based; navigation follows the workspace in the URL. */
export function useConsoleWorkspace() {
  const route = useRoute()
  const auth = useAuthStore()
  const adminSettings = useAdminSettingsStore()
  const isAdminWorkspace = computed(() => auth.isAdmin && (
    route.path === '/admin' || route.path.startsWith('/admin/') ||
    (route.path.startsWith('/custom/') && adminSettings.customMenuItems.some(item =>
      item.id === route.params.id && item.visibility === 'admin'
    ))
  ))
  return { isAdminWorkspace }
}
