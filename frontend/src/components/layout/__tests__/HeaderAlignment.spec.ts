import { readFileSync } from 'node:fs'
import { resolve } from 'node:path'
import { describe, expect, it } from 'vitest'
import postcss from 'postcss'

const stylesheets = ['console-signal.css', 'console-studio.css']
  .map((filename) => readFileSync(resolve(process.cwd(), 'src/styles', filename), 'utf8'))
  .join('\n')
const root = postcss.parse(stylesheets)

function declarationValues(selector: string, property: string) {
  const values: string[] = []

  root.walkRules(selector, (rule) => {
    rule.walkDecls(property, (declaration) => values.push(declaration.value))
  })

  return values
}

describe('application shell header alignment', () => {
  it('uses one shared height for the sidebar brand and main header', () => {
    const heightVariable = '--signal-shell-header-height'
    const sharedHeight = `var(${heightVariable})`

    expect(declarationValues('.console-signal', heightVariable)).toEqual(['64px'])
    expect(declarationValues('.console-signal .sidebar-header', 'height')).toEqual([sharedHeight])
    expect(declarationValues('.console-signal .signal-header', 'height')).toEqual([sharedHeight])
    expect(declarationValues('.console-signal .header-inner', 'height')).toEqual(['100%'])
  })
})
