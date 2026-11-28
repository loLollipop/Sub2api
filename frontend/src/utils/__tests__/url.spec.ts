import { describe, expect, it } from 'vitest'
import { sanitizeInternalRedirectPath, sanitizeUrl } from '../url'

describe('internal redirects', () => {
  it.each([
    '//evil.example/path', '/\\evil.example', '/\t/evil.example',
    '/\n/evil.example', '/\r/evil.example', '/bad\u0000path', '/bad\u007fpath',
    'https://evil.example', 'javascript:alert(1)', '/https://evil.example',
    null, undefined, 42, {}, [], [null, 42]
  ])('rejects unsafe or non-path input %j', (value) => {
    expect(sanitizeInternalRedirectPath(value)).toBe('/dashboard')
  })

  it('preserves paths, queries, fragments and repeated query semantics', () => {
    expect(sanitizeInternalRedirectPath(' /profile?tab=security#passkeys ')).toBe('/profile?tab=security#passkeys')
    expect(sanitizeInternalRedirectPath([null, '', ' /billing?plan=pro ', '/other'])).toBe('/billing?plan=pro')
    expect(sanitizeInternalRedirectPath(['//evil.example', '/safe'])).toBe('/dashboard')
    expect(sanitizeInternalRedirectPath(undefined, '/profile')).toBe('/profile')
  })

  it('keeps valid paths on the current origin', () => {
    const value = sanitizeInternalRedirectPath('/profile?tab=security')
    expect(new URL(value, window.location.origin).origin).toBe(window.location.origin)
  })
})

describe('URL sanitization', () => {
  it('rejects backslashes and control characters in relative URLs', () => {
    for (const value of ['/\\evil.example', '/\t/evil.example', '/bad\u0000path']) {
      expect(sanitizeUrl(value, { allowRelative: true })).toBe('')
    }
    expect(sanitizeUrl('/images/example.png', { allowRelative: true })).toBe('/images/example.png')
    expect(sanitizeUrl('https://example.com')).toBe('https://example.com/')
    expect(sanitizeUrl('javascript:alert(1)')).toBe('')
    expect(sanitizeUrl('data:image/png;base64,abc', { allowDataUrl: true })).toBe('data:image/png;base64,abc')
  })
})
