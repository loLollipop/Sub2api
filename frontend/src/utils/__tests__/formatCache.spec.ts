import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'

const locale = vi.hoisted(() => ({ value: 'en-US' }))
vi.mock('@/i18n', () => ({ getLocale: () => locale.value, i18n: { global: { t: (key: string) => key } } }))

beforeEach(() => { vi.resetModules(); locale.value = 'en-US' })
afterEach(() => vi.restoreAllMocks())

describe('cached Intl formatters', () => {
  it('reuses common formatters while preserving notation, currency, precision and locale', async () => {
    const { formatNumber, formatCurrency } = await import('../format')
    const constructor = vi.spyOn(Intl, 'NumberFormat')
    expect(formatNumber(1234)).toBe('1,234')
    expect(formatNumber(5678)).toBe('5,678')
    expect(constructor).toHaveBeenCalledTimes(1)
    expect(formatNumber(12000)).toBe('12K')
    expect(formatCurrency(1)).toBe('$1.00')
    expect(formatCurrency(2)).toBe('$2.00')
    expect(formatCurrency(0.001)).toBe('$0.001000')
    expect(formatCurrency(1, 'EUR')).toBe('€1.00')
    expect(constructor).toHaveBeenCalledTimes(5)
    locale.value = 'de-DE'
    expect(formatNumber(1234)).toBe('1.234')
    expect(formatCurrency(1)).toContain('1,00')
    expect(constructor).toHaveBeenCalledTimes(7)
  })

  it('reuses equivalent date options, keeps time zones separate and observes option mutations', async () => {
    const { formatDate } = await import('../format')
    const constructor = vi.spyOn(Intl, 'DateTimeFormat')
    const date = '2026-11-03T01:00:00Z'
    const options: Intl.DateTimeFormatOptions = { hour: '2-digit', hourCycle: 'h23', timeZone: 'UTC' }
    expect(formatDate(date, options, 'en-US')).toBe('01')
    expect(formatDate(date, { timeZone: 'UTC', hourCycle: 'h23', hour: '2-digit' }, 'en-US')).toBe('01')
    expect(constructor).toHaveBeenCalledTimes(1)
    options.timeZone = 'Asia/Tokyo'
    expect(formatDate(date, options, 'en-US')).toBe('10')
    expect(constructor).toHaveBeenCalledTimes(2)
    expect(formatDate('invalid', options)).toBe('')
    expect(constructor).toHaveBeenCalledTimes(2)
  })

  it('bounds date formatter retention', async () => {
    const { formatDate } = await import('../format')
    const constructor = vi.spyOn(Intl, 'DateTimeFormat')
    const date = '2026-11-03T01:00:00Z'
    const fields = ['year', 'month', 'day', 'hour', 'minute', 'second'] as const
    for (let mask = 0; mask < 65; mask++) {
      const options: Intl.DateTimeFormatOptions = { timeZone: 'UTC', hour12: mask >= 64 }
      fields.forEach((field, index) => { options[field] = mask & (1 << index) ? '2-digit' : 'numeric' })
      formatDate(date, options, 'en-US')
    }
    expect(constructor).toHaveBeenCalledTimes(65)
    formatDate(date, { timeZone: 'UTC', hour12: false, year: 'numeric', month: 'numeric', day: 'numeric', hour: 'numeric', minute: 'numeric', second: 'numeric' }, 'en-US')
    expect(constructor).toHaveBeenCalledTimes(66)
  })
})
