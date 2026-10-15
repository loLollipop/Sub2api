import { describe, expect, it } from 'vitest'

describe('pinned spreadsheet export writer', () => {
  it('writes usage rows, Unicode and formula-like text without a parsing API', async () => {
    const sheetjs = await import('xlsx')
    expect(sheetjs.version).toBe('0.20.3')
    const sheet = sheetjs.utils.aoa_to_sheet([['用户', '金额', '备注']])
    sheetjs.utils.sheet_add_aoa(sheet, [['客户一', 0.15, '=1+1'], ['客户二', 0, '<tag>&value']], { origin: -1 })
    expect(sheet.A2.v).toBe('客户一')
    expect(sheet.B2.v).toBe(0.15)
    expect(sheet.C2).toMatchObject({ t: 's', v: '=1+1' })
    expect(sheet.C2.f).toBeUndefined()
    expect(sheet['!ref']).toBe('A1:C3')

    const workbook = sheetjs.utils.book_new()
    sheetjs.utils.book_append_sheet(workbook, sheet, 'Usage')
    const exported = sheetjs.write(workbook, { bookType: 'xlsx', type: 'array', compression: false })
    const bytes = Buffer.from(exported)
    expect(bytes.subarray(0, 4)).toEqual(Buffer.from([0x50, 0x4b, 0x03, 0x04]))
    // Uncompressed XML is inspected without introducing workbook parsing into
    // the application or changing its lazy, write-only export contract.
    const xml = bytes.toString('utf8')
    expect(xml).toContain('xl/worksheets/sheet1.xml')
    expect(xml).toContain('客户一')
    expect(xml).toContain('&lt;tag&gt;&amp;value')
    expect(xml).toContain('=1+1')
    expect(xml).not.toContain('<f>')
  })
})
