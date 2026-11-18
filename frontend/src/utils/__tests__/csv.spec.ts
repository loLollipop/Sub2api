import { describe, expect, it } from 'vitest'
import { escapeCsvText } from '../csv'

describe('CSV text exported through the real spreadsheet writer', () => {
  it('escapes formulas and preserves CSV quoting and numeric costs', async () => {
    const xlsx = await import('xlsx')
    const values = ['=1+1', '+SUM(A1)', '-1+2', '@SUM(A1)', ' \t=1+1', '\t=1+1', 'comma,name', 'say "hello"', 'line1\nline2', '普通名称']
    const sheet = xlsx.utils.aoa_to_sheet(values.map(value => [escapeCsvText(value), -0.15]))
    expect(xlsx.utils.sheet_to_csv(sheet)).toBe([
      "'=1+1,-0.15", "'+SUM(A1),-0.15", "'-1+2,-0.15", "'@SUM(A1),-0.15",
      "' \t=1+1,-0.15", "'\t=1+1,-0.15", '"comma,name",-0.15', '"say ""hello""",-0.15', '"line1\nline2",-0.15', '普通名称,-0.15',
    ].join('\n'))
    expect(sheet.A1).toMatchObject({ t: 's', v: "'=1+1" })
  })
})
