import { readFileSync } from 'node:fs'
import { resolve } from 'node:path'
import postcss, { type Rule } from 'postcss'
import { compileStyle, parse } from 'vue/compiler-sfc'
import { describe, expect, it } from 'vitest'

const component = readFileSync(resolve(process.cwd(), 'src/components/admin/account/AccountModelTestPanel.vue'), 'utf8')
const { descriptor } = parse(component)
const compiled = compileStyle({ source: descriptor.styles[0].content, id: 'data-v-search-focus', scoped: true })
const stylesheet = postcss.parse(compiled.code)

function findRule(selector: string): Rule {
  let found: Rule | undefined
  stylesheet.walkRules(rule => {
    if (rule.selector.replace(/\[data-v-search-focus\]/g, '') === selector) found = rule
  })
  expect(found, `Missing scoped focus rule: ${selector}`).toBeDefined()
  return found!
}

function declaration(rule: Rule, property: string) {
  let value: string | undefined
  rule.walkDecls(property, item => { value = item.value })
  return value
}

describe('model search focus styles', () => {
  it('uses a scoped reset that outranks both console and panel generic input focus rules', () => {
    expect(compiled.errors).toEqual([])
    const rule = findRule('.model-test .model-test-search-wrap .model-test-search:focus-visible')
    expect(declaration(rule, 'outline')).toBe('none')
  })

  it('keeps the rounded wrapper as the search keyboard focus indicator', () => {
    const rule = findRule('.model-test-search-wrap:focus-within')
    expect(declaration(rule, 'border-color')).toBe('var(--test-accent)')
    expect(declaration(rule, 'box-shadow')).toContain('0 0 0 3px')
  })

  it('keeps visible keyboard outlines for the other controls', () => {
    const rule = findRule('.model-test :is(button, input):focus-visible')
    expect(declaration(rule, 'outline')).toBe('2px solid var(--test-accent)')
  })
})
