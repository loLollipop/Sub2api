import { describe, expect, it } from 'vitest'
import { cnBalanceCellVisible, cnQuotaCellVisible, providerNativeProtocols, providerProtocolRules, serializeProviderProtocolRules } from '../credentialsBuilder'
import { BUILTIN_PLATFORM_CATALOG } from '@/constants/platformCatalog'

describe('provider catalog capabilities', () => {

  it('registers TypeSafe only as a native System One gateway', () => {
    const platform = BUILTIN_PLATFORM_CATALOG.platforms.find(spec => spec.id === 'typesafe')
    expect(platform?.gateway).toBe('systemone')
    expect(platform?.cn_provider).toBe(false)
    expect(platform?.multi_protocol).toBeUndefined()
    expect(providerNativeProtocols('typesafe')).toEqual([])
    expect(providerProtocolRules('typesafe')).toEqual([])
  })

  it('retains model protocol sets and explicit empty rule overrides', () => {
    expect(providerProtocolRules('command_code').some(rule => rule.pattern === 'gpt-*' && rule.protocols?.includes('chat_completions'))).toBe(true)
    expect(providerProtocolRules('command_code', [])).toEqual([])
    expect(serializeProviderProtocolRules([{ pattern: ' GPT-* ', protocol: 'responses', protocols: ['responses', 'chat_completions'] }])).toEqual([
      { pattern: 'gpt-*', protocol: 'responses', protocols: ['responses', 'chat_completions'] }
    ])
  })

  it.each(['command_code', 'cline'])('gates %s probes on every official credential endpoint', platform => {
    const account = { platform, type: 'apikey', credentials: {} as Record<string, unknown> }
    expect(cnQuotaCellVisible(account)).toBe(true)
    expect(cnBalanceCellVisible(account)).toBe(true)
    const host = platform === 'cline' ? 'api.cline.bot' : 'api.commandcode.ai'
    for (const url of ['https://relay.example/v1', `http://${host}/v1`, `https://${host}:444/v1`, `https://user:pass@${host}/v1`, `https://${host}/v1?q=1`, `https://${host}/v1?`, `https://${host}/v1#`, `https://${host}/%76%31`]) {
      account.credentials = { base_url: url }
      expect(cnQuotaCellVisible(account)).toBe(false)
      expect(cnBalanceCellVisible(account)).toBe(false)
    }
    account.credentials = { api_base_urls: { anthropic: 'https://relay.example' } }
    expect(cnQuotaCellVisible(account)).toBe(false)
    expect(cnBalanceCellVisible({ ...account, type: 'oauth', credentials: {} })).toBe(false)
  })
})
