import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import type { MonitorMatrixBucket, MonitorMatrixResponse, MonitorMatrixRow } from '@/api/channelMonitorV2'
import { bucketStatusClass, createGroupStatusLoader, latestBucket } from '../groupStatusDot'

const { getMatrix } = vi.hoisted(() => ({ getMatrix: vi.fn() }))
vi.mock('@/api/channelMonitorV2', () => ({ getMatrix }))

function bucket(overall = 'healthy', errorRate = 0, start = '2026-11-14T12:00:00Z'): MonitorMatrixBucket {
  return {
    bucket_start: start,
    health: { overall },
    metrics: { error_rate: errorRate },
  } as MonitorMatrixBucket
}

function row(groupId: number, buckets: MonitorMatrixBucket[]): MonitorMatrixRow {
  return { group_id: groupId, buckets } as MonitorMatrixRow
}

describe('group status dots', () => {
  beforeEach(() => {
    vi.useFakeTimers()
    vi.setSystemTime(new Date('2026-11-14T12:00:00Z'))
    getMatrix.mockReset()
  })
  afterEach(() => vi.useRealTimers())

  it.each([
    [0, 'bg-emerald-600'],
    [0.15, 'bg-emerald-400'],
    [0.3, 'bg-yellow-300'],
    [0.45, 'bg-amber-400'],
    [0.6, 'bg-red-500'],
    [0.8, 'bg-gray-950'],
  ])('uses the monitor availability band for error rate %s', (errorRate, color) => {
    expect(bucketStatusClass(bucket('healthy', errorRate))).toContain(color)
  })

  it('keeps unknown buckets grey and sorts the newest real bucket without mutating rows', () => {
    const older = bucket('healthy', 0, '2026-11-14T11:55:00Z')
    const newest = bucket('unknown', 1)
    const source = row(7, [newest, older])
    expect(bucketStatusClass(newest)).toBe('bg-gray-300 dark:bg-dark-600')
    expect(latestBucket(source)).toBe(newest)
    expect(source.buckets).toEqual([newest, older])
    expect(latestBucket(row(8, []))).toBeNull()
  })

  it('loads on demand, merges concurrent requests and caches for exactly sixty seconds', async () => {
    let resolve!: (matrix: Pick<MonitorMatrixResponse, 'items'>) => void
    getMatrix.mockImplementationOnce(() => new Promise(done => { resolve = done }))
    const load = createGroupStatusLoader()
    expect(getMatrix).not.toHaveBeenCalled()
    const first = load()
    expect(load()).toBe(first)
    expect(getMatrix).toHaveBeenCalledWith({ range: '90m', platforms: [], groupIds: [], models: [] }, 'platform_group', false)
    const newest = bucket()
    resolve({ items: [row(7, [newest]), row(8, [])] })
    expect(await first).toEqual({ 7: newest })
    await vi.advanceTimersByTimeAsync(59_999)
    expect(await load()).toEqual({ 7: newest })
    expect(getMatrix).toHaveBeenCalledTimes(1)
    await vi.advanceTimersByTimeAsync(1)
    getMatrix.mockResolvedValueOnce({ items: [] })
    expect(await load()).toEqual({})
    expect(getMatrix).toHaveBeenCalledTimes(2)
  })

  it.each(['empty', 'failure'])('backs off after an %s result', async result => {
    if (result === 'empty') getMatrix.mockResolvedValue({ items: [] })
    else getMatrix.mockRejectedValue(new Error('monitor unavailable'))
    const load = createGroupStatusLoader()
    expect(await load()).toEqual({})
    expect(await load()).toEqual({})
    await vi.advanceTimersByTimeAsync(59_999)
    expect(await load()).toEqual({})
    expect(getMatrix).toHaveBeenCalledTimes(1)
    await vi.advanceTimersByTimeAsync(1)
    expect(await load()).toEqual({})
    expect(getMatrix).toHaveBeenCalledTimes(2)
  })

  it('clears previously healthy colours on failure and keeps caches local to each page instance', async () => {
    const newest = bucket()
    getMatrix.mockResolvedValueOnce({ items: [row(7, [newest])] })
    const firstPage = createGroupStatusLoader()
    expect(await firstPage()).toEqual({ 7: newest })
    getMatrix.mockRejectedValue(new Error('monitor unavailable'))
    const nextPage = createGroupStatusLoader()
    expect(await nextPage()).toEqual({})
    expect(getMatrix).toHaveBeenCalledTimes(2)
    await vi.advanceTimersByTimeAsync(60_000)
    expect(await firstPage()).toEqual({})
    expect(await firstPage()).toEqual({})
    expect(getMatrix).toHaveBeenCalledTimes(3)
  })
})
