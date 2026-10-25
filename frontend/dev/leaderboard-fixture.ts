// Local preview data only. This module is never imported by the production app.
import type { LeaderboardPeriod, LeaderboardResponse } from '../src/api/leaderboard'

export function leaderboardFixture(period: LeaderboardPeriod, now = new Date()): LeaderboardResponse {
  const start = new Date(now)
  start.setUTCHours(0, 0, 0, 0)
  if (period === 'week') start.setUTCDate(start.getUTCDate() - (start.getUTCDay() + 6) % 7)
  if (period === 'month') start.setUTCDate(1)
  if (period === 'year') { start.setUTCDate(1); start.setUTCMonth(0) }
  const previousStart = new Date(start)
  if (period === 'day') previousStart.setUTCDate(start.getUTCDate() - 1)
  if (period === 'week') previousStart.setUTCDate(start.getUTCDate() - 7)
  if (period === 'month') previousStart.setUTCMonth(start.getUTCMonth() - 1)
  if (period === 'year') previousStart.setUTCFullYear(start.getUTCFullYear() - 1)
  const previousEnd = new Date(Math.min(start.getTime(), previousStart.getTime() + now.getTime() - start.getTime()))
  const granularity = period === 'day' ? 'hour' : period === 'year' ? 'month' : 'day'
  const names = ['gpt-6-astra', 'claude-opus-5-5', 'deepseek-v4-pro', 'gemini-3-flash', 'gpt-6.1-sol', 'glm-5.3', 'kimi-k2.5', 'qwen3.5-plus', 'claude-sonnet-4-6', 'grok-4', 'deepseek-chat', 'gemini-2.5-pro', 'minimax-m2.5', 'doubao-seed-2.0', 'gpt-4.1', 'qwen3-coder', 'claude-haiku-4-5', 'mistral-large', 'llama-4', 'moonshot-v1', 'hunyuan-turbo', 'custom-model']
  const trend: LeaderboardResponse['trend'] = []
  const totals = new Map<string, number>()
  let index = 0
  for (const cursor = new Date(start); cursor < now;) {
    for (const [i, model] of names.entries()) {
      const tokens = Math.round((1_800_000_000 / Math.pow(i + 1, 1.22)) * (0.75 + Math.sin(index * .8 + i) * .2) * (period === 'day' ? .07 : period === 'year' ? 7 : 1))
      trend.push({ date: cursor.toISOString(), model, total_tokens: tokens })
      totals.set(model, (totals.get(model) || 0) + tokens)
    }
    if (granularity === 'hour') cursor.setUTCHours(cursor.getUTCHours() + 1)
    else if (granularity === 'month') cursor.setUTCMonth(cursor.getUTCMonth() + 1)
    else cursor.setUTCDate(cursor.getUTCDate() + 1)
    ++index
  }
  const models = names.map((model, i) => {
    const tokens = totals.get(model) || 0
    return { model, total_tokens: tokens, requests: Math.round(tokens / 12500), previous_tokens: i === 8 ? 0 : Math.round(tokens * [1.24, .41, 1.52, .24, 2.1, .53, 1.15][i % 7]), previous_requests: Math.round(tokens / 13500) }
  })
  return { period, timezone: 'UTC', start: start.toISOString(), end: now.toISOString(), previous_start: previousStart.toISOString(), previous_end: previousEnd.toISOString(), updated_at: now.toISOString(), granularity, models, trend, total_tokens: models.reduce((s, m) => s + m.total_tokens, 0), total_requests: models.reduce((s, m) => s + m.requests, 0) }
}
