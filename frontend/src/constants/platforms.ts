import type { AccountPlatform, GroupPlatform } from '@/types'
import { listPlatforms } from './platformCatalog'
import { i18n } from '@/i18n'
import { reactive, watchSyncEffect } from 'vue'

export interface PlatformOption<T extends string = string> {
  [key: string]: unknown
  value: T
  label: string
}

/**
 * Concrete upstream platforms supported by accounts and request routing.
 * Keep platform selectors derived from this catalog so newly added providers
 * do not silently disappear from list filters.
 */
export const CONCRETE_PLATFORM_OPTIONS: PlatformOption<AccountPlatform>[] = reactive([])

/** Platforms that can own a group. */
export const GROUP_PLATFORM_OPTIONS: PlatformOption<GroupPlatform>[] = reactive([])

// Preserve the full catalog for editing and filters of existing accounts/groups.
export const GROUP_CREATION_PLATFORM_OPTIONS: PlatformOption<GroupPlatform>[] = reactive([])

watchSyncEffect(() => {
  const concrete = listPlatforms().map(spec => ({
    value: spec.id as AccountPlatform,
    get label() {
      return spec.id === 'mixed' ? i18n.global.t('admin.accounts.platforms.mixed') : spec.display_name
    }
  }))
  CONCRETE_PLATFORM_OPTIONS.splice(0, CONCRETE_PLATFORM_OPTIONS.length, ...concrete)
  GROUP_PLATFORM_OPTIONS.splice(0, GROUP_PLATFORM_OPTIONS.length, ...concrete, {
    value: 'composite',
    get label() { return i18n.global.t('admin.groups.platforms.composite') }
  })
  GROUP_CREATION_PLATFORM_OPTIONS.splice(0, GROUP_CREATION_PLATFORM_OPTIONS.length,
    ...GROUP_PLATFORM_OPTIONS.filter(option => !['kimi', 'zhipu', 'deepseek', 'minimax'].includes(option.value)))
})
