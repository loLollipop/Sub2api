import { readFileSync } from 'node:fs'
import { resolve } from 'node:path'

import postcss, { type AtRule, type Rule } from 'postcss'
import { describe, expect, it } from 'vitest'

const stylesheet = postcss.parse(readFileSync(resolve(process.cwd(), 'src/style.css'), 'utf8'))
const ticketsStylesheet = postcss.parse(
  readFileSync(resolve(process.cwd(), 'src/styles/tickets.css'), 'utf8')
)

const normalizeSelector = (selector: string) =>
  selector
    .split(',')
    .map((part) => part.trim().replace(/\s+/g, ' '))
    .join(',')

const findRuleIn = (
  root: ReturnType<typeof postcss.parse>,
  selector: string,
  parent?: AtRule
) => {
  let matchingRule: Rule | undefined

  root.walkRules((rule) => {
    if (
      !matchingRule &&
      normalizeSelector(rule.selector) === normalizeSelector(selector) &&
      (!parent || rule.parent === parent)
    ) {
      matchingRule = rule
    }
  })

  expect(matchingRule, `Missing CSS rule for ${selector}`).toBeDefined()
  return matchingRule as Rule
}

const findRule = (selector: string, parent?: AtRule) => findRuleIn(stylesheet, selector, parent)

const declarationValue = (rule: Rule, property: string) => {
  let value: string | undefined
  rule.walkDecls(property, (declaration) => {
    value = declaration.value
  })
  return value
}

const appliedUtilities = (rule: Rule) => {
  const utilities: string[] = []
  rule.walkAtRules('apply', (atRule) => utilities.push(...atRule.params.split(/\s+/)))
  return utilities
}

describe('global scrollbar visibility', () => {
  it('keeps Firefox thumbs visible by default and strengthens them on interaction', () => {
    let firefoxSupport: AtRule | undefined
    stylesheet.walkAtRules('supports', (atRule) => {
      if (atRule.params === '(-moz-appearance:none)') firefoxSupport = atRule
    })

    expect(firefoxSupport).toBeDefined()
    expect(declarationValue(findRule('*', firefoxSupport), 'scrollbar-color')).toBe(
      'rgba(209, 213, 219, 0.6) transparent'
    )
    expect(
      declarationValue(findRule('.dark, .dark *', firefoxSupport), 'scrollbar-color')
    ).toBe('rgba(71, 85, 105, 0.6) transparent')
    expect(
      declarationValue(findRule('*:hover, *:focus-within', firefoxSupport), 'scrollbar-color')
    ).toBe('rgba(156, 163, 175, 0.75) transparent')
    expect(
      declarationValue(
        findRule(
          '.dark:hover, .dark:focus-within, .dark *:hover, .dark *:focus-within',
          firefoxSupport
        ),
        'scrollbar-color'
      )
    ).toBe('rgba(100, 116, 139, 0.75) transparent')
  })

  it('keeps WebKit thumbs visible and increases contrast for focus and hover', () => {
    expect(appliedUtilities(findRule('::-webkit-scrollbar'))).toEqual(
      expect.arrayContaining(['h-2', 'w-2'])
    )
    expect(appliedUtilities(findRule('::-webkit-scrollbar-thumb'))).toEqual(
      expect.arrayContaining(['bg-gray-300/60'])
    )
    expect(
      appliedUtilities(
        findRule('.dark::-webkit-scrollbar-thumb, .dark *::-webkit-scrollbar-thumb')
      )
    ).toEqual(expect.arrayContaining(['bg-dark-600/60']))
    expect(
      appliedUtilities(
        findRule('*:hover::-webkit-scrollbar-thumb, *:focus-within::-webkit-scrollbar-thumb')
      )
    ).toEqual(expect.arrayContaining(['bg-gray-400/75']))
    expect(
      appliedUtilities(
        findRule(
          '.dark:hover::-webkit-scrollbar-thumb, .dark:focus-within::-webkit-scrollbar-thumb, .dark *:hover::-webkit-scrollbar-thumb, .dark *:focus-within::-webkit-scrollbar-thumb'
        )
      )
    ).toEqual(expect.arrayContaining(['bg-dark-500/75']))
    expect(appliedUtilities(findRule('::-webkit-scrollbar-thumb:hover'))).toEqual(
      expect.arrayContaining(['bg-gray-500/90'])
    )
    expect(
      appliedUtilities(
        findRule(
          '.dark::-webkit-scrollbar-thumb:hover, .dark *::-webkit-scrollbar-thumb:hover'
        )
      )
    ).toEqual(expect.arrayContaining(['bg-dark-400/90']))
  })

  it('does not rely on dark variants for WebKit pseudo-elements', () => {
    const scrollbarUtilities: string[] = []
    stylesheet.walkRules((rule) => {
      if (!rule.selector.includes('::-webkit-scrollbar')) return
      scrollbarUtilities.push(...appliedUtilities(rule))
    })

    expect(scrollbarUtilities.some((utility) => utility.startsWith('dark:'))).toBe(false)
  })

  it('hides explicitly opted-out and ticket tab scrollbars in both engines', () => {
    expect(declarationValue(findRule('.scrollbar-hide'), 'scrollbar-width')).toBe('none')
    expect(declarationValue(findRule('.scrollbar-hide::-webkit-scrollbar'), 'display')).toBe('none')
    expect(
      declarationValue(findRuleIn(ticketsStylesheet, '.tickets-status-tabs'), 'scrollbar-width')
    ).toBe('none')
    expect(
      declarationValue(
        findRuleIn(ticketsStylesheet, '.tickets-status-tabs::-webkit-scrollbar'),
        'display'
      )
    ).toBe('none')
  })

  it('leaves global scrollbar dimensions unchanged', () => {
    expect(appliedUtilities(findRule('::-webkit-scrollbar'))).toEqual(
      expect.arrayContaining(['h-2', 'w-2'])
    )
  })
})
