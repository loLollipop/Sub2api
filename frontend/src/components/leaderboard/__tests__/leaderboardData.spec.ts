import { describe, expect, it } from 'vitest'
import type { LeaderboardModel, LeaderboardResponse } from '@/api/leaderboard'
import { bucketLabels, PROVIDER_COLORS, providerOf, providerShares, rankModels, trendBuckets } from '../leaderboardData'

const model = (name: string, current: number, previous: number): LeaderboardModel => ({ model: name, total_tokens: current, previous_tokens: previous, requests: 1, previous_requests: 1 })
function response(overrides: Partial<LeaderboardResponse> = {}): LeaderboardResponse {
  return { period: 'week', timezone: 'Asia/Shanghai', start: '2026-10-19T00:00:00+08:00', end: '2026-10-21T10:00:00+08:00', previous_start: '', previous_end: '', updated_at: '', granularity: 'day', total_tokens: 0, total_requests: 0, models: [], trend: [], ...overrides }
}

describe('leaderboard data semantics', () => {
  it('identifies Chinese providers by model names without assuming an OpenAI protocol brand', () => {
    expect(providerOf('openai/deepseek-v4-pro')).toBe('deepseek')
    expect(providerOf('Qwen/Qwen3.5-Plus')).toBe('qwen')
    expect(providerOf('gpt-6-astra')).toBe('openai')
    expect(providerOf('custom-alias')).toBe('other')
    const shares = providerShares([model('deepseek-chat', 60, 0), model('deepseek-reasoner', 20, 0), model('gpt-5', 20, 0), model('old', 0, 100)])
    expect(shares).toEqual([{ provider: 'deepseek', tokens: 80, share: 80 }, { provider: 'openai', tokens: 20, share: 20 }])
  })

  it('keeps previous-only models in the previous ranking and distinguishes new models from growth', () => {
    const ranked = rankModels([model('gpt-5', 120, 30), model('deepseek-chat', 90, 100), model('claude-sonnet-4-6', 80, 0), model('retired', 0, 60)])
    expect(ranked.map(m => [m.model, m.rank, m.rankChange, m.changePercent])).toEqual([
      ['gpt-5', 1, 2, 300], ['deepseek-chat', 2, -1, -10], ['claude-sonnet-4-6', 3, null, null],
    ])
  })

  it('breaks token ties deterministically without depending on the API row order', () => {
    const a = model('gpt-5', 100, 100), b = model('claude-sonnet-4-6', 100, 100)
    expect(rankModels([a, b])).toEqual(rankModels([b, a]))
  })

  it('fills missing local dates with zero and combines model points rather than dropping them', () => {
    const buckets = trendBuckets(response({ trend: [
      { date: '2026-10-18T16:00:00Z', model: 'gpt-5', total_tokens: 10 },
      { date: '2026-10-19T00:00:00+08:00', model: 'gpt-5', total_tokens: 5 },
      { date: '2026-10-20T16:00:00Z', model: 'deepseek-chat', total_tokens: 20 },
    ] }))
    expect(buckets.map(b => b.key)).toEqual(['2026-10-19', '2026-10-20', '2026-10-21'])
    expect(buckets[0].models.get('gpt-5')).toBe(15)
    expect(buckets[1].models.size).toBe(0)
    expect(buckets[2].models.get('deepseek-chat')).toBe(20)
  })

  it('preserves all 25 hours of a DST fall-back day and excludes the next day', () => {
    const data = response({ period: 'day', timezone: 'America/New_York', granularity: 'hour', start: '2026-11-01T00:00:00-04:00', end: '2026-11-02T00:00:00-05:00' })
    const buckets = trendBuckets(data)
    expect(buckets).toHaveLength(25)
    expect(new Set(buckets.map(b => b.key)).size).toBe(25)
    expect(buckets.at(-1)?.key).toBe('2026-11-02T04:00:00.000Z')
    const labels = bucketLabels(data, buckets.map(bucket => bucket.key), 'en')
    expect(labels[1]).toContain('GMT-4')
    expect(labels[2]).toContain('GMT-5')
    expect(new Set(labels).size).toBe(25)
  })

  it('assigns a distinct color to every recognized provider and to unknown models', () => {
    const colors = Object.values(PROVIDER_COLORS)
    expect(new Set(colors).size).toBe(colors.length)
    for (const family of ['gpt-5', 'claude-opus', 'gemini-pro', 'deepseek-chat', 'qwen3', 'glm-5', 'mistral-large', 'llama-4', 'hunyuan-turbo', 'ernie-4', 'yi-model', 'spark-v4', 'command-r']) {
      expect(PROVIDER_COLORS[providerOf(family)]).toBeDefined()
    }
  })
})
