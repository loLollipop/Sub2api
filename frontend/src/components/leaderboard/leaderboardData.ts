import type { LeaderboardModel, LeaderboardResponse } from '@/api/leaderboard'
import { modelProvider } from '@/components/modelPlaza/modelPlazaProviders'

export const SERIES_COLORS = ['#f97316', '#4d6bfe', '#10b981', '#8b5cf6', '#0ea5e9', '#eab308', '#ec4899', '#14b8a6', '#94a3b8']
export const PROVIDER_COLORS: Record<string, string> = {
  openai: '#10b981', anthropic: '#c47754', gemini: '#4285f4', grok: '#64748b',
  deepseek: '#4d6bfe', qwen: '#8b5cf6', kimi: '#2563eb', zhipu: '#06b6d4',
  minimax: '#f43f5e', doubao: '#eab308', other: '#94a3b8',
  mistral: '#f59e0b', meta: '#0866ff', hunyuan: '#0f766e', baidu: '#dc2626',
  yi: '#7c3aed', spark: '#c026d3', cohere: '#65a30d',
}

export function providerOf(model: string): string {
  return modelProvider({ name: model, platform: '' })
}

function order(a: { model: string; tokens: number }, b: { model: string; tokens: number }): number {
  return b.tokens - a.tokens || (a.model < b.model ? -1 : a.model > b.model ? 1 : 0)
}

export function rankModels(models: LeaderboardModel[]) {
  const previous = models.filter(m => m.previous_tokens > 0)
    .map(m => ({ model: m.model, tokens: m.previous_tokens })).sort(order)
  const previousRanks = new Map(previous.map((m, i) => [m.model, i + 1]))
  return models.filter(m => m.total_tokens > 0)
    .map(m => ({ ...m, tokens: m.total_tokens })).sort(order)
    .map((m, i) => ({
      ...m, rank: i + 1, provider: providerOf(m.model),
      rankChange: previousRanks.has(m.model) ? previousRanks.get(m.model)! - (i + 1) : null,
      changePercent: m.previous_tokens > 0 ? (m.total_tokens - m.previous_tokens) / m.previous_tokens * 100 : null,
    }))
}

export function providerShares(models: LeaderboardModel[]) {
  const totals = new Map<string, number>()
  for (const model of models) {
    if (model.total_tokens <= 0) continue
    const provider = providerOf(model.model)
    totals.set(provider, (totals.get(provider) || 0) + model.total_tokens)
  }
  const sum = [...totals.values()].reduce((a, b) => a + b, 0)
  return [...totals].map(([provider, tokens]) => ({ provider, tokens, share: sum > 0 ? tokens / sum * 100 : 0 }))
    .sort((a, b) => b.tokens - a.tokens || a.provider.localeCompare(b.provider))
}

// Bucket keys are local calendar dates, so DST days need not contain 24 hours.
export function trendBuckets(data: LeaderboardResponse) {
  const parts = (date: Date) => {
    const values = new Intl.DateTimeFormat('en-US', { timeZone: data.timezone, year: 'numeric', month: '2-digit', day: '2-digit' }).formatToParts(date)
    const value = (type: string) => Number(values.find(p => p.type === type)?.value)
    return { year: value('year'), month: value('month'), day: value('day') }
  }
  const keyOf = (date: Date) => {
    if (data.granularity === 'hour') return date.toISOString()
    const p = parts(date)
    return `${p.year}-${String(p.month).padStart(2, '0')}${data.granularity === 'day' ? `-${String(p.day).padStart(2, '0')}` : ''}`
  }
  const start = new Date(data.start), end = new Date(data.end)
  const keys: string[] = []
  if (data.granularity === 'hour') {
    for (let time = start.getTime(); time < end.getTime(); time += 3_600_000) keys.push(new Date(time).toISOString())
  } else {
    const p = parts(start), last = parts(new Date(end.getTime() - 1))
    const cursor = new Date(Date.UTC(p.year, p.month - 1, data.granularity === 'month' ? 1 : p.day))
    const finish = Date.UTC(last.year, last.month - 1, data.granularity === 'month' ? 1 : last.day)
    while (cursor.getTime() <= finish) {
      keys.push(cursor.toISOString().slice(0, data.granularity === 'month' ? 7 : 10))
      if (data.granularity === 'month') cursor.setUTCMonth(cursor.getUTCMonth() + 1)
      else cursor.setUTCDate(cursor.getUTCDate() + 1)
    }
  }
  const buckets = new Map(keys.map(key => [key, new Map<string, number>()]))
  for (const point of data.trend) {
    const bucket = buckets.get(keyOf(new Date(point.date)))
    if (bucket) bucket.set(point.model, (bucket.get(point.model) || 0) + point.total_tokens)
  }
  return [...buckets].map(([key, models]) => ({ key, models }))
}

export function bucketLabels(data: LeaderboardResponse, keys: string[], locale: string): string[] {
  if (data.granularity === 'hour') {
    const options = { hour: '2-digit', minute: '2-digit', hourCycle: 'h23', timeZone: data.timezone } as const
    const labels = keys.map(key => new Intl.DateTimeFormat(locale, options).format(new Date(key)))
    const counts = new Map<string, number>()
    for (const label of labels) counts.set(label, (counts.get(label) || 0) + 1)
    return keys.map((key, i) => counts.get(labels[i])! > 1
      ? new Intl.DateTimeFormat(locale, { ...options, timeZoneName: 'shortOffset' }).format(new Date(key))
      : labels[i])
  }
  return keys.map(key => {
    const date = new Date(`${key}${data.granularity === 'month' ? '-01' : ''}T00:00:00Z`)
    return new Intl.DateTimeFormat(locale, { month: 'short', ...(data.granularity === 'day' ? { day: 'numeric' } as const : {}), timeZone: 'UTC' }).format(date)
  })
}

export function compactTokens(value: number): string {
  const unit = value >= 1e12 ? [1e12, 'T'] : value >= 1e9 ? [1e9, 'B'] : value >= 1e6 ? [1e6, 'M'] : value >= 1e3 ? [1e3, 'K'] : null
  if (!unit) return Math.round(value).toLocaleString()
  return `${(value / Number(unit[0])).toFixed(1).replace(/\.0$/, '')}${unit[1]}`
}
