import { beforeEach, describe, expect, it, vi } from 'vitest'
import type { ApiKey, Group } from '@/types'
import { canvasKeyName, ensureInfiniteCanvasApiKey, imageCanvasGroups } from '../infiniteCanvasSession'

const group = (id = 235, extra: Partial<Group> = {}) => ({ id, status: 'active', platform: 'openai', allow_image_generation: true, ...extra } as Group)
const key = (extra: Partial<ApiKey> = {}) => ({ id: 7, user_id: 1, name: canvasKeyName(235), key: 'sk-fixture', group_id: 235, group_ids: [235], status: 'active', quota: 0, quota_used: 0, expires_at: null, ...extra } as ApiKey)

describe('image canvas session', () => {
  const list = vi.fn()
  const create = vi.fn()
  const getAvailableGroups = vi.fn()
  const client = { list, create, getAvailableGroups }
  beforeEach(() => {
    vi.resetAllMocks()
    localStorage.clear()
    localStorage.setItem('auth_user', JSON.stringify({ id: 1 }))
    getAvailableGroups.mockResolvedValue([group()])
    list.mockResolvedValue({ items: [] })
    create.mockResolvedValue(key())
  })
  it('lists only active authorized image groups', () => {
    expect(imageCanvasGroups([group(), group(2, { allow_image_generation: false }), group(3, { status: 'inactive' }), group(4, { platform: 'anthropic' })]).map(g => g.id)).toEqual([235])
  })
  it('entering never creates a key', async () => {
    await expect(ensureInfiniteCanvasApiKey(client, { groupId: 235 })).rejects.toMatchObject({ code: 'not-configured' })
    expect(create).not.toHaveBeenCalled()
  })
  it('explicitly creates a single-group key with an idempotency token', async () => {
    expect(await ensureInfiniteCanvasApiKey(client, { groupId: 235, createIfMissing: true })).toMatchObject({ keyId: 7, groupIds: [235], created: true })
    expect(create).toHaveBeenCalledWith(canvasKeyName(235), 235, undefined, undefined, undefined, undefined, undefined, undefined, undefined, undefined, { idempotencyKey: expect.any(String) })
  })
  it('reuses an existing key without changing it', async () => {
    list.mockResolvedValue({ items: [key()] })
    expect(await ensureInfiniteCanvasApiKey(client, { groupId: 235 })).toMatchObject({ apiKey: 'sk-fixture', created: false })
    expect(create).not.toHaveBeenCalled()
  })
  it.each([
    [{ status: 'inactive' }, 'key-unavailable'],
    [{ expires_at: '2020-01-01T00:00:00Z' }, 'key-unavailable'],
    [{ quota: 5, quota_used: 5 }, 'key-unavailable'],
    [{ key: '' }, 'missing-key'],
  ] as const)('does not recreate an unavailable key (%j)', async (patch, code) => {
    list.mockResolvedValue({ items: [key(patch)] })
    await expect(ensureInfiniteCanvasApiKey(client, { groupId: 235, createIfMissing: true })).rejects.toMatchObject({ code })
    expect(create).not.toHaveBeenCalled()
  })
  it('does not borrow other users or multi-group keys', async () => {
    list.mockResolvedValue({ items: [key({ user_id: 2 }), key({ group_ids: [235, 1] })] })
    await expect(ensureInfiniteCanvasApiKey(client, { groupId: 235 })).rejects.toMatchObject({ code: 'not-configured' })
  })
  it('rejects revoked group access before reading or creating keys', async () => {
    getAvailableGroups.mockResolvedValue([])
    await expect(ensureInfiniteCanvasApiKey(client, { groupId: 235, createIfMissing: true })).rejects.toMatchObject({ code: 'no-groups' })
    expect(list).not.toHaveBeenCalled()
    expect(create).not.toHaveBeenCalled()
  })
  it('deduplicates concurrent explicit creation', async () => {
    const sessions = await Promise.all([1, 2].map(() => ensureInfiniteCanvasApiKey(client, { groupId: 235, createIfMissing: true })))
    expect(create).toHaveBeenCalledTimes(1)
    expect(sessions[0]).toEqual(sessions[1])
  })
  it('abandons a response after the signed-in user changes', async () => {
    list.mockImplementation(async () => {
      localStorage.setItem('auth_user', JSON.stringify({ id: 2 }))
      return { items: [key()] }
    })
    await expect(ensureInfiniteCanvasApiKey(client, { groupId: 235, createIfMissing: true })).rejects.toMatchObject({ code: 'request-failed' })
    expect(create).not.toHaveBeenCalled()
  })
})
