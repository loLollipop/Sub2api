import { keysAPI } from '@/api/keys'
import { userGroupsAPI } from '@/api/groups'
import type { ApiKey, Group } from '@/types'
import { INFINITE_CANVAS_KEY_NAME, isUsableCanvasKey } from './infiniteCanvas'

export class InfiniteCanvasSetupError extends Error {
  constructor(readonly code: 'not-configured' | 'not-installed' | 'key-unavailable' | 'no-groups' | 'missing-key' | 'request-failed', message: string) {
    super(message)
    this.name = 'InfiniteCanvasSetupError'
  }
}

export const canvasKeyName = (groupId: number) => INFINITE_CANVAS_KEY_NAME + ' · ' + groupId
export const imageCanvasGroups = (groups: Group[]) => groups.filter(group => group.status === 'active'
  && group.allow_image_generation && ['openai', 'grok'].includes(group.platform))

export interface InfiniteCanvasSession {
  apiKey: string
  groupIds: number[]
  created: boolean
  keyId: number
}
export interface InfiniteCanvasKeyClient {
  list: typeof keysAPI.list
  create: typeof keysAPI.create
  getAvailableGroups: typeof userGroupsAPI.getAvailable
}
const defaultClient: InfiniteCanvasKeyClient = { list: keysAPI.list, create: keysAPI.create, getAvailableGroups: userGroupsAPI.getAvailable }
const operations = new WeakMap<InfiniteCanvasKeyClient, Map<string, Promise<InfiniteCanvasSession>>>()

function ownerScope(): number | null {
  try { return (JSON.parse(localStorage.getItem('auth_user') || 'null') as { id?: number } | null)?.id || null }
  catch { return null }
}
function assertOwner(owner: number | null) {
  if (!owner || ownerScope() !== owner) throw new InfiniteCanvasSetupError('request-failed', 'The signed-in user changed')
}
function toSession(key: ApiKey, groupId: number, created: boolean): InfiniteCanvasSession {
  if (!isUsableCanvasKey(key)) throw new InfiniteCanvasSetupError('key-unavailable', 'Canvas key is inactive, expired or exhausted')
  if (!key.key) throw new InfiniteCanvasSetupError('missing-key', 'Canvas key plaintext is unavailable')
  return { apiKey: key.key, groupIds: [groupId], created, keyId: key.id }
}

async function prepare(client: InfiniteCanvasKeyClient, owner: number | null, groupId: number, create: boolean): Promise<InfiniteCanvasSession> {
  try {
    assertOwner(owner)
    const groups = imageCanvasGroups(await client.getAvailableGroups())
    assertOwner(owner)
    if (!groups.some(group => group.id === groupId)) throw new InfiniteCanvasSetupError('no-groups', 'No authorized image group')
    const name = canvasKeyName(groupId)
    const listed = await client.list(1, 100, { search: name, sort_by: 'created_at', sort_order: 'desc' })
    assertOwner(owner)
    const matches = listed.items.filter(key => {
      const ids = key.group_ids?.length ? key.group_ids : [key.group_id]
      return key.user_id === owner && key.name === name && ids.length === 1 && ids[0] === groupId
    })
    const existing = matches.find(isUsableCanvasKey) || matches[0]
    if (existing) return toSession(existing, groupId, false)
    if (!create) throw new InfiniteCanvasSetupError('not-configured', 'No canvas key configured')
    // Confirmed creation binds one image group. Never rotate or widen a key.
    const key = await client.create(name, groupId, undefined, undefined, undefined, undefined, undefined, undefined,
      undefined, undefined, { idempotencyKey: crypto.randomUUID() })
    assertOwner(owner)
    return toSession(key, groupId, true)
  } catch (error) {
    if (error instanceof InfiniteCanvasSetupError) throw error
    throw new InfiniteCanvasSetupError('request-failed', 'Failed to prepare canvas key')
  }
}

export function ensureInfiniteCanvasApiKey(client: InfiniteCanvasKeyClient = defaultClient, options: { groupId: number; createIfMissing?: boolean }): Promise<InfiniteCanvasSession> {
  const owner = ownerScope()
  const operation = JSON.stringify([owner, options.groupId, options.createIfMissing === true])
  let pending = operations.get(client)
  if (!pending) { pending = new Map(); operations.set(client, pending) }
  const existing = pending.get(operation)
  if (existing) return existing
  const promise = prepare(client, owner, options.groupId, options.createIfMissing === true)
  pending.set(operation, promise)
  void promise.then(() => pending.delete(operation), () => pending.delete(operation))
  return promise
}
