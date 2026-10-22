import { afterEach, describe, expect, it, vi } from 'vitest'
import { create } from '../keys'

const { post } = vi.hoisted(() => ({ post: vi.fn() }))
vi.mock('../client', () => ({ apiClient: { post } }))
afterEach(() => vi.resetAllMocks())

const createWithExpiry = (expiresAt: string, days?: number) => create(
  'Short lived', 7, undefined, undefined, undefined, undefined, days,
  undefined, undefined, undefined, { idempotencyKey: 'create-key' }, expiresAt
)

describe('API key creation expiry payload', () => {
  it('sends the exact absolute time and retains idempotency headers', async () => {
    const expiry = new Date(Date.now() + 30 * 60 * 1000).toISOString()
    post.mockResolvedValue({ data: { id: 1, expires_at: expiry } })
    expect(await createWithExpiry(expiry)).toEqual({ id: 1, expires_at: expiry })
    expect(post).toHaveBeenCalledTimes(1)
    expect(post).toHaveBeenCalledWith('/keys', { name: 'Short lived', group_id: 7, expires_at: expiry },
      { headers: { 'Idempotency-Key': 'create-key' } })
  })

  it('keeps legacy days-only creation', async () => {
    post.mockResolvedValue({ data: { id: 1 } })
    await create('Legacy', undefined, undefined, undefined, undefined, undefined, 7)
    expect(post).toHaveBeenCalledWith('/keys', { name: 'Legacy', expires_in_days: 7 }, undefined)
  })

  it.each(['invalid', '', '2000-01-01T00:00:00Z'])('rejects invalid/past absolute expiry %s before POST', async (expiry) => {
    await expect(createWithExpiry(expiry)).rejects.toThrow('valid future date')
    expect(post).not.toHaveBeenCalled()
  })

  it('rejects ambiguous absolute and relative expiry', async () => {
    await expect(createWithExpiry(new Date(Date.now() + 1800000).toISOString(), 1)).rejects.toThrow('mutually exclusive')
    expect(post).not.toHaveBeenCalled()
  })
})
