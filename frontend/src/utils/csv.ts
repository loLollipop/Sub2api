// CSV has no string cell type. Keep user text from becoming a spreadsheet formula.
export function escapeCsvText(value: string): string {
  // eslint-disable-next-line no-control-regex -- Spreadsheet readers may ignore control characters before formula prefixes.
  return /^[\s\u0000-\u001f]*[=+\-@]/u.test(value) || /^[\t\r\n]/u.test(value)
    ? `'${value}`
    : value
}
