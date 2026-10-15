<template>
  <BaseDialog :show="show" :title="t('admin.groups.rpmOverridesTitle')" width="wide" @close="handleClose">
    <div v-if="group" class="space-y-4">
      <!-- 分组信息 -->
      <div class="flex flex-wrap items-center gap-3 rounded-lg bg-gray-50 px-4 py-2.5 text-sm dark:bg-dark-700">
        <span class="inline-flex items-center gap-1.5" :class="platformColorClass">
          <PlatformIcon :platform="group.platform" size="sm" />
          {{ t('admin.groups.platforms.' + group.platform) }}
        </span>
        <span class="text-gray-400">|</span>
        <span class="font-medium text-gray-900 dark:text-white">{{ group.name }}</span>
        <span class="text-gray-400">|</span>
        <span class="text-gray-600 dark:text-gray-400">
          {{ t('admin.groups.groupRpmDefault') }}: {{ group.rpm_limit || 0 }}
        </span>
      </div>

      <!-- 操作区：添加用户 -->
      <div class="rounded-lg border border-gray-200 p-3 dark:border-dark-600">
        <h4 class="mb-2 text-sm font-medium text-gray-700 dark:text-gray-300">
          {{ t('admin.groups.addUserRpm') }}
        </h4>
        <div class="flex items-end gap-2">
          <div class="relative flex-1">
            <input
              :value="searchQuery"
              :disabled="!canEditDraft"
              type="text"
              autocomplete="off"
              class="input w-full"
              :placeholder="t('admin.groups.searchUserPlaceholder')"
              @input="handleSearchUsers"
              @focus="showDropdown = canEditDraft"
            />
            <div
              v-if="showDropdown && searchResults.length > 0"
              class="absolute left-0 right-0 top-full z-10 mt-1 max-h-48 overflow-y-auto rounded-lg border border-gray-200 bg-white shadow-lg dark:border-dark-500 dark:bg-dark-700"
            >
              <button
                v-for="user in searchResults"
                :key="user.id"
                type="button"
                :disabled="!canEditDraft"
                class="flex w-full items-center gap-2 px-3 py-1.5 text-left text-sm hover:bg-gray-50 dark:hover:bg-dark-600"
                @click="selectUser(user)"
              >
                <span class="text-gray-400">#{{ user.id }}</span>
                <span class="text-gray-900 dark:text-white">{{ user.username || user.email }}</span>
                <span v-if="user.username" class="text-xs text-gray-400">{{ user.email }}</span>
              </button>
            </div>
          </div>
          <div class="w-24">
            <input
              :value="newRpm"
              :disabled="!canEditDraft"
              type="number"
              step="1"
              min="0"
              autocomplete="off"
              class="hide-spinner input w-full"
              placeholder="100"
              @input="updateNewRpm(($event.target as HTMLInputElement).value)"
            />
          </div>
          <button
            type="button"
            class="btn btn-primary shrink-0"
            :disabled="!canEditDraft || !selectedUser || !validRpm(newRpm)"
            @click="handleAddLocal"
          >
            {{ t('common.add') }}
          </button>
        </div>

        <div v-if="localEntries.length > 0" class="mt-3 flex items-center justify-end border-t border-gray-100 pt-3 dark:border-dark-600">
          <button
            type="button"
            :disabled="!canEditDraft || saving || clearing"
            class="rounded-lg border border-red-200 bg-red-50 px-3 py-1.5 text-sm font-medium text-red-600 transition-colors hover:bg-red-100 disabled:opacity-50 dark:border-red-800 dark:bg-red-900/20 dark:text-red-400 dark:hover:bg-red-900/40"
            @click="clearAllLocal"
          >
            <Icon v-if="clearing" name="refresh" size="sm" class="mr-1 inline animate-spin" />
            {{ t('admin.groups.clearAll') }}
          </button>
        </div>
      </div>

      <!-- 加载状态 -->
      <div v-if="loading" class="flex justify-center py-6">
        <svg class="h-6 w-6 animate-spin text-primary-500" fill="none" viewBox="0 0 24 24">
          <circle class="opacity-25" cx="12" cy="12" r="10" stroke="currentColor" stroke-width="4"></circle>
          <path class="opacity-75" fill="currentColor" d="M4 12a8 8 0 018-8V0C5.373 0 0 5.373 0 12h4zm2 5.291A7.962 7.962 0 014 12H0c0 3.042 1.135 5.824 3 7.938l3-2.647z"></path>
        </svg>
      </div>

      <!-- 列表 -->
      <div v-else>
        <h4 class="mb-2 text-sm font-medium text-gray-700 dark:text-gray-300">
          {{ t('admin.groups.rpmOverrides') }} ({{ localEntries.length }})
        </h4>

        <div v-if="localEntries.length === 0" class="py-6 text-center text-sm text-gray-400 dark:text-gray-500">
          {{ t('admin.groups.noRpmOverrides') }}
        </div>

        <div v-else>
          <div class="overflow-hidden rounded-lg border border-gray-200 dark:border-dark-600">
            <div class="max-h-[420px] overflow-auto">
              <table class="w-full min-w-max text-sm">
                <thead class="sticky top-0 z-[1]">
                  <tr class="border-b border-gray-200 bg-gray-50 dark:border-dark-600 dark:bg-dark-700">
                    <th class="px-3 py-2 text-left text-xs font-medium text-gray-500 dark:text-gray-400">{{ t('admin.groups.columns.userEmail') }}</th>
                    <th class="px-3 py-2 text-left text-xs font-medium text-gray-500 dark:text-gray-400">ID</th>
                    <th class="px-3 py-2 text-left text-xs font-medium text-gray-500 dark:text-gray-400">{{ t('admin.groups.columns.userName') }}</th>
                    <th class="px-3 py-2 text-left text-xs font-medium text-gray-500 dark:text-gray-400">{{ t('admin.groups.columns.userNotes') }}</th>
                    <th class="px-3 py-2 text-left text-xs font-medium text-gray-500 dark:text-gray-400">{{ t('admin.groups.columns.userStatus') }}</th>
                    <th class="px-3 py-2 text-left text-xs font-medium text-gray-500 dark:text-gray-400" :title="t('admin.groups.columns.rpmOverrideHint')">{{ t('admin.groups.columns.rpmOverride') }}</th>
                    <th class="w-10 px-2 py-2"></th>
                  </tr>
                </thead>
                <tbody class="divide-y divide-gray-100 dark:divide-dark-600">
                  <tr
                    v-for="entry in paginatedLocalEntries"
                    :key="entry.user_id"
                    class="hover:bg-gray-50 dark:hover:bg-dark-700/50"
                  >
                    <td class="px-3 py-2 text-gray-600 dark:text-gray-400">{{ entry.user_email }}</td>
                    <td class="whitespace-nowrap px-3 py-2 text-gray-400 dark:text-gray-500">{{ entry.user_id }}</td>
                    <td class="whitespace-nowrap px-3 py-2 text-gray-900 dark:text-white">{{ entry.user_name || '-' }}</td>
                    <td class="max-w-[160px] truncate px-3 py-2 text-gray-500 dark:text-gray-400" :title="entry.user_notes">{{ entry.user_notes || '-' }}</td>
                    <td class="whitespace-nowrap px-3 py-2">
                      <span
                        :class="[
                          'inline-flex rounded-full px-2 py-0.5 text-xs font-medium',
                          entry.user_status === 'active'
                            ? 'bg-green-100 text-green-700 dark:bg-green-900/30 dark:text-green-400'
                            : 'bg-gray-100 text-gray-600 dark:bg-dark-600 dark:text-gray-400'
                        ]"
                      >
                        {{ entry.user_status }}
                      </span>
                    </td>
                    <td class="whitespace-nowrap px-3 py-2">
                      <input
                        type="number"
                        :disabled="!canEditDraft"
                        step="1"
                        min="0"
                        autocomplete="off"
                        :value="entry.rpm_override"
                        class="hide-spinner w-20 rounded border border-gray-200 bg-white px-2 py-1 text-center text-sm font-medium transition-colors focus:border-primary-500 focus:outline-none focus:ring-1 focus:ring-primary-500/20 dark:border-dark-500 dark:bg-dark-700 dark:focus:border-primary-500"
                        @change="updateLocalRpm(entry.user_id, ($event.target as HTMLInputElement).value)"
                      />
                    </td>
                    <td class="px-2 py-2">
                      <button
                        type="button"
                        :disabled="!canEditDraft"
                        class="rounded p-1 text-gray-400 transition-colors hover:bg-red-50 hover:text-red-600 dark:hover:bg-red-900/20 dark:hover:text-red-400"
                        @click="removeLocal(entry.user_id)"
                      >
                        <Icon name="trash" size="sm" />
                      </button>
                    </td>
                  </tr>
                </tbody>
              </table>
            </div>
          </div>

          <Pagination
            :total="localEntries.length"
            :page="currentPage"
            :page-size="pageSize"
            @update:page="currentPage = $event"
            @update:pageSize="handlePageSizeChange"
          />
        </div>
      </div>

      <!-- 底部 -->
      <div class="flex items-center gap-3 border-t border-gray-200 pt-4 dark:border-dark-600">
        <template v-if="isDirty">
          <span class="text-xs text-amber-600 dark:text-amber-400">{{ t('admin.groups.unsavedChanges') }}</span>
          <button
            type="button"
            class="text-xs font-medium text-primary-600 hover:text-primary-700 dark:text-primary-400 dark:hover:text-primary-300"
            :disabled="!canEditDraft"
            @click="handleCancel"
          >
            {{ t('admin.groups.revertChanges') }}
          </button>
        </template>
        <div class="ml-auto flex items-center gap-3">
          <button type="button" class="btn btn-sm px-4 py-1.5" @click="handleClose">
            {{ t('common.close') }}
          </button>
          <button
            v-if="isDirty"
            type="button"
            class="btn btn-primary btn-sm px-4 py-1.5"
            :disabled="!canEditDraft || saving || clearing"
            @click="handleSave"
          >
            <Icon v-if="saving" name="refresh" size="sm" class="mr-1 animate-spin" />
            {{ t('common.save') }}
          </button>
        </div>
      </div>
    </div>
  </BaseDialog>
</template>

<script setup lang="ts">
import { ref, computed, watch, onUnmounted } from 'vue'
import { useI18n } from 'vue-i18n'
import { useAppStore } from '@/stores/app'
import { adminAPI } from '@/api/admin'
import type { GroupRPMOverrideEntry } from '@/api/admin/groups'
import type { AdminGroup, AdminUser } from '@/types'
import BaseDialog from '@/components/common/BaseDialog.vue'
import Pagination from '@/components/common/Pagination.vue'
import Icon from '@/components/icons/Icon.vue'
import PlatformIcon from '@/components/common/PlatformIcon.vue'

interface LocalEntry extends GroupRPMOverrideEntry {}

const props = defineProps<{
  show: boolean
  group: AdminGroup | null
}>()

const emit = defineEmits<{
  close: []
  success: []
}>()

const { t } = useI18n()
const appStore = useAppStore()

const loading = ref(false)
const entriesLoaded = ref(false)
const saving = ref(false)
const clearing = ref(false)
const serverEntries = ref<GroupRPMOverrideEntry[]>([])
const localEntries = ref<LocalEntry[]>([])
const searchQuery = ref('')
const searchResults = ref<AdminUser[]>([])
const showDropdown = ref(false)
const selectedUser = ref<AdminUser | null>(null)
const newRpm = ref<number | null>(null)
const currentPage = ref(1)
const pageSize = ref(10)
const canEditDraft = computed(() => props.show && !!props.group && entriesLoaded.value && !loading.value && !saving.value && !clearing.value)
const validRpm = (rpm: number | null): rpm is number =>
  typeof rpm === 'number' && Number.isInteger(rpm) && rpm >= 0 && rpm <= 2147483647

let searchTimeout: ReturnType<typeof setTimeout>
let loadVersion = 0
let searchVersion = 0

const resetDraft = () => {
  loadVersion++
  searchVersion++
  clearTimeout(searchTimeout)
  entriesLoaded.value = false
  loading.value = false
  serverEntries.value = []
  localEntries.value = []
  currentPage.value = 1
  searchQuery.value = ''
  searchResults.value = []
  showDropdown.value = false
  selectedUser.value = null
  newRpm.value = null
}

const platformColorClass = computed(() => {
  switch (props.group?.platform) {
    case 'anthropic': return 'text-orange-700 dark:text-orange-400'
    case 'openai': return 'text-emerald-700 dark:text-emerald-400'
    case 'antigravity': return 'text-purple-700 dark:text-purple-400'
    default: return 'text-blue-700 dark:text-blue-400'
  }
})

const isDirty = computed(() => {
  if (localEntries.value.length !== serverEntries.value.length) return true
  const serverMap = new Map(serverEntries.value.map(e => [e.user_id, e.rpm_override]))
  return localEntries.value.some(e => serverMap.get(e.user_id) !== e.rpm_override)
})

const paginatedLocalEntries = computed(() => {
  const start = (currentPage.value - 1) * pageSize.value
  return localEntries.value.slice(start, start + pageSize.value)
})

const cloneEntries = (entries: GroupRPMOverrideEntry[]): LocalEntry[] => {
  return entries.map(e => ({ ...e }))
}

const loadEntries = async () => {
  if (!props.show || !props.group) return
  const groupId = props.group.id
  const version = loadVersion
  loading.value = true
  try {
    const entries = await adminAPI.groups.getGroupRPMOverrides(groupId)
    if (version !== loadVersion || !props.show || props.group?.id !== groupId) return
    serverEntries.value = entries
    localEntries.value = cloneEntries(serverEntries.value)
    entriesLoaded.value = true
    adjustPage()
  } catch (error) {
    if (version !== loadVersion) return
    appStore.showError(t('admin.groups.failedToLoad'))
    console.error('Error loading RPM overrides:', error)
  } finally {
    if (version === loadVersion) loading.value = false
  }
}

const adjustPage = () => {
  const totalPages = Math.max(1, Math.ceil(localEntries.value.length / pageSize.value))
  if (currentPage.value > totalPages) currentPage.value = totalPages
}

watch([() => props.show, () => props.group?.id], ([show]) => {
  resetDraft()
  if (show && props.group) loadEntries()
}, { immediate: true, flush: 'sync' })

const handlePageSizeChange = (newSize: number) => {
  pageSize.value = newSize
  currentPage.value = 1
}

const handleSearchUsers = (event: Event) => {
  if (!canEditDraft.value) return
  searchQuery.value = (event.target as HTMLInputElement).value
  clearTimeout(searchTimeout)
  const version = ++searchVersion
  const query = searchQuery.value.trim()
  selectedUser.value = null
  if (!query) {
    searchResults.value = []
    showDropdown.value = false
    return
  }
  searchTimeout = setTimeout(async () => {
    if (version !== searchVersion || !canEditDraft.value) return
    try {
      const res = await adminAPI.users.list(1, 10, { search: query })
      if (version !== searchVersion || !canEditDraft.value) return
      searchResults.value = res.items
      showDropdown.value = true
    } catch {
      if (version === searchVersion && canEditDraft.value) searchResults.value = []
    }
  }, 300)
}

const selectUser = (user: AdminUser) => {
  if (!canEditDraft.value) return
  selectedUser.value = user
  searchQuery.value = user.email
  showDropdown.value = false
  searchResults.value = []
}

const updateNewRpm = (value: string) => {
  if (!canEditDraft.value) return
  newRpm.value = value.trim() ? Number(value) : null
}

const handleAddLocal = () => {
  if (!canEditDraft.value || !selectedUser.value || !validRpm(newRpm.value)) return
  const user = selectedUser.value
  const idx = localEntries.value.findIndex(e => e.user_id === user.id)
  const entry: LocalEntry = {
    user_id: user.id,
    user_name: user.username || '',
    user_email: user.email,
    user_notes: user.notes || '',
    user_status: user.status || 'active',
    rpm_override: newRpm.value
  }
  if (idx >= 0) {
    localEntries.value[idx] = entry
  } else {
    localEntries.value.push(entry)
  }
  searchQuery.value = ''
  selectedUser.value = null
  newRpm.value = null
  adjustPage()
}

const updateLocalRpm = (userId: number, value: string) => {
  if (!value.trim()) return
  const num = Number(value)
  if (!canEditDraft.value || !validRpm(num)) return
  const entry = localEntries.value.find(e => e.user_id === userId)
  if (entry) entry.rpm_override = num
}

const removeLocal = (userId: number) => {
  if (!canEditDraft.value) return
  localEntries.value = localEntries.value.filter(e => e.user_id !== userId)
  adjustPage()
}

const clearAllLocal = async () => {
  if (!canEditDraft.value || !props.group || clearing.value || saving.value) return
  const groupId = props.group.id
  const version = loadVersion
  clearing.value = true
  try {
    await adminAPI.groups.clearGroupRPMOverrides(groupId)
    if (version !== loadVersion || !props.show || props.group?.id !== groupId) return
    localEntries.value = []
    serverEntries.value = []
    appStore.showSuccess(t('admin.groups.rpmSaved'))
  } catch (error) {
    if (version !== loadVersion || !props.show || props.group?.id !== groupId) return
    appStore.showError(t('admin.groups.failedToSave'))
    console.error('Error clearing RPM overrides:', error)
  } finally {
    clearing.value = false
  }
}

const handleCancel = () => {
  if (!canEditDraft.value) return
  localEntries.value = cloneEntries(serverEntries.value)
  adjustPage()
}

const handleSave = async () => {
  if (!canEditDraft.value || !props.group || saving.value || clearing.value) return
  if (localEntries.value.some(entry => !validRpm(entry.rpm_override))) {
    appStore.showError(t('admin.groups.failedToSave'))
    return
  }
  const groupId = props.group.id
  const version = loadVersion
  saving.value = true
  try {
    const entries = localEntries.value.map(e => ({
      user_id: e.user_id,
      rpm_override: e.rpm_override
    }))
    await adminAPI.groups.batchSetGroupRPMOverrides(groupId, entries)
    if (version !== loadVersion || !props.show || props.group?.id !== groupId) return
    appStore.showSuccess(t('admin.groups.rpmSaved'))
    emit('success')
    emit('close')
  } catch (error) {
    if (version !== loadVersion || !props.show || props.group?.id !== groupId) return
    appStore.showError(t('admin.groups.failedToSave'))
    console.error('Error saving RPM overrides:', error)
  } finally {
    saving.value = false
  }
}

const handleClose = () => {
  resetDraft()
  emit('close')
}

const handleClickOutside = () => { showDropdown.value = false }
if (typeof document !== 'undefined') {
  document.addEventListener('click', handleClickOutside)
}
onUnmounted(() => {
  resetDraft()
  if (typeof document !== 'undefined') document.removeEventListener('click', handleClickOutside)
})
</script>

<style scoped>
.hide-spinner::-webkit-outer-spin-button,
.hide-spinner::-webkit-inner-spin-button {
  -webkit-appearance: none;
  margin: 0;
}
.hide-spinner {
  -moz-appearance: textfield;
}
</style>
