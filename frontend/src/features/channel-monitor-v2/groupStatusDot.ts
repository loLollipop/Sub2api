import { getMatrix, type MonitorMatrixBucket, type MonitorMatrixRow } from '@/api/channelMonitorV2'
import { availabilityBarClass } from './monitorFormat'

/** Same availability bands as the monitor timeline; unknown samples stay grey. */
export function bucketStatusClass(bucket: MonitorMatrixBucket): string {
  if (bucket.health.overall === 'unknown') return 'bg-gray-300 dark:bg-dark-600'
  return availabilityBarClass((1 - bucket.metrics.error_rate) * 100)
}

export function latestBucket(row: MonitorMatrixRow): MonitorMatrixBucket | null {
  return [...row.buckets]
    .sort((a, b) => Date.parse(a.bucket_start) - Date.parse(b.bucket_start))
    .at(-1) ?? null
}

export type GroupStatusBuckets = Record<number, MonitorMatrixBucket>

/**
 * Each key page owns its cache, so results never cross login sessions or ranges.
 * Empty responses and failures also back off for a minute. This optional
 * decoration disappears on failure rather than keeping an old healthy colour.
 */
export function createGroupStatusLoader() {
  let cachedAt: number | null = null
  let cached: GroupStatusBuckets = {}
  let inFlight: Promise<GroupStatusBuckets> | null = null

  return function load(): Promise<GroupStatusBuckets> {
    if (inFlight) return inFlight
    if (cachedAt !== null && Date.now() - cachedAt < 60_000) return Promise.resolve(cached)

    inFlight = (async () => {
      try {
        const matrix = await getMatrix({ range: '90m', platforms: [], groupIds: [], models: [] }, 'platform_group', false)
        const next: GroupStatusBuckets = {}
        for (const row of matrix.items) {
          const bucket = latestBucket(row)
          if (row.group_id != null && bucket) next[row.group_id] = bucket
        }
        cached = next
      } catch {
        // Optional monitor data must not prevent creating or editing a key.
        cached = {}
      } finally {
        cachedAt = Date.now()
        inFlight = null
      }
      return cached
    })()
    return inFlight
  }
}
