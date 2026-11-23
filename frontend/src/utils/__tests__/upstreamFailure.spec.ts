import { describe, expect, it } from 'vitest'
import { classifyUpstreamFailureBody } from '../upstreamFailure'

describe('upstream failure labels', () => {
  it('labels a supplier body-budget rejection as capacity despite its misleading model code', () => {
    const body = JSON.stringify({ error: { message: 'Request body buffering capacity is exhausted; retry later', code: 'model_not_found' } })
    expect(classifyUpstreamFailureBody(body, 503)).toBe('capacity-cooling')
  })

  it.each([
    ['{"error":{"code":"model_not_found"}}', 404, 'model_not_found'],
    ['Too many requests', 429, 'rate-limit'],
    ['Invalid API key', 401, 'credential-forbidden'],
    ['', 503, '']
  ] as const)('retains the existing category for %s', (body, status, expected) => {
    expect(classifyUpstreamFailureBody(body, status)).toBe(expected)
  })
})
