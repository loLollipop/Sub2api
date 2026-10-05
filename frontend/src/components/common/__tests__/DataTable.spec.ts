import { mount } from '@vue/test-utils'
import { beforeEach, describe, expect, it, vi } from 'vitest'
import postcss from 'postcss'
import { parse } from 'vue/compiler-sfc'

import DataTable from '../DataTable.vue'
import dataTableSource from '../DataTable.vue?raw'

vi.mock('vue-i18n', () => ({
  useI18n: () => ({
    t: (key: string) => key
  })
}))

const stubDesktopMatchMedia = () => {
  Object.defineProperty(window, 'matchMedia', {
    writable: true,
    value: vi.fn().mockImplementation((query: string) => ({
      matches: true,
      media: query,
      onchange: null,
      addEventListener: vi.fn(),
      removeEventListener: vi.fn(),
      addListener: vi.fn(),
      removeListener: vi.fn(),
      dispatchEvent: vi.fn()
    }))
  })
}

const stubMobileMatchMedia = () => {
  Object.defineProperty(window, 'matchMedia', {
    writable: true,
    value: vi.fn().mockImplementation((query: string) => ({
      matches: false,
      media: query,
      onchange: null,
      addEventListener: vi.fn(),
      removeEventListener: vi.fn(),
      addListener: vi.fn(),
      removeListener: vi.fn(),
      dispatchEvent: vi.fn()
    }))
  })
}

const { descriptor: dataTableDescriptor } = parse(dataTableSource)
const dataTableStyles = postcss.parse(
  dataTableDescriptor.styles.map((style) => style.content).join('\n')
)
const dataTableTemplate = dataTableDescriptor.template?.content ?? ''

const interactionClassPattern =
  /(?:(?:[a-z0-9_/-]+):)*(?:hover|group-hover(?:\/[a-z0-9_-]+)?):!?-?[a-z0-9_./%[\]():!-]+/gi

const getInteractionClassTokens = (templateSource: string) =>
  [...templateSource.matchAll(/(?:^|\s)(?::|v-bind:)?class\s*=\s*(["'])([\s\S]*?)\1/g)].flatMap(
    (attributeMatch) => attributeMatch[2].match(interactionClassPattern) ?? []
  )

const changesGeometryUtility = (classToken: string) => {
  const interactionVariant = classToken.match(/(?:^|:)(?:hover|group-hover(?:\/[a-z0-9_-]+)?):/i)
  const utility = interactionVariant
    ? classToken
        .slice((interactionVariant.index ?? 0) + interactionVariant[0].length)
        .replace(/^!/, '')
        .replace(/^-/, '')
    : ''

  if (
    /^(?:p[trblxy]?|m[trblxy]?|space-[xy]|translate-[xy]|scale(?:-[xy])?|rotate|skew-[xy]|w|h|size|min-w|max-w|min-h|max-h|gap(?:-[xy])?|inset(?:-[xy])?|top|right|bottom|left|basis|grid-cols|grid-rows|col-span|row-span|border-spacing(?:-[xy])?)-/.test(
      utility
    )
  ) {
    return true
  }

  if (/^border(?:-[trblxy])?(?:-(?:0|[0-9]+))?$/.test(utility)) return true

  const arbitraryBorderWidth = utility.match(/^border(?:-[trblxy])?-\[(.+)\]$/)?.[1]
  return arbitraryBorderWidth
    ? /^(?:(?:length|line-width):)?(?:-?(?:\d|\.)|calc\(|clamp\(|min\(|max\()/.test(
        arbitraryBorderWidth
      )
    : false
}

const changesLayoutGeometry = (property: string) =>
  /^(?:padding|margin)(?:-|$)|^(?:transform|translate|scale|rotate|width|height|min-width|max-width|min-height|max-height|inset|top|right|bottom|left|display|position|box-sizing|gap|row-gap|column-gap|flex|flex-basis|grid-template-columns|grid-template-rows|font-size|line-height|letter-spacing)$/.test(
    property
  )

const transitionsLayoutGeometry = (property: string, value: string) =>
  property.startsWith('transition') &&
  /(?:^|[\s,])(?:all|padding(?:-[a-z]+)?|margin(?:-[a-z]+)?|transform|translate|scale|rotate|width|height|min-width|max-width|min-height|max-height|inset|top|right|bottom|left|gap|row-gap|column-gap|flex(?:-basis)?|font-size|line-height|letter-spacing)(?:[\s,]|$)/.test(
    value
  )

const isUnsafeRowInteractionDeclaration = (
  property: string,
  value: string,
  targetsRowHover: boolean
) =>
  transitionsLayoutGeometry(property, value) ||
  (targetsRowHover && changesLayoutGeometry(property))

const targetsHoveringTableRow = (selector: string) =>
  selector
    .split(',')
    .some((part) => /\btbody\b[^,{]*\btr\b[^,{]*:hover\b/i.test(part))

describe('DataTable', () => {
  beforeEach(() => {
    stubDesktopMatchMedia()
    localStorage.clear()
  })

  it('renders paired sort arrows and highlights the active direction', async () => {
    const wrapper = mount(DataTable, {
      props: {
        columns: [
          { key: 'name', label: 'Name', sortable: true },
          { key: 'created_at', label: 'Created', sortable: true }
        ],
        data: [
          { id: 1, name: 'Beta', created_at: '2026-01-02T00:00:00Z' },
          { id: 2, name: 'Alpha', created_at: '2026-01-01T00:00:00Z' }
        ],
        defaultSortKey: 'name',
        defaultSortOrder: 'asc'
      },
      slots: {
        'header-name': '<span data-test="custom-name-header">Name</span>'
      }
    })

    await wrapper.vm.$nextTick()

    const nameHeader = wrapper.findAll('th')[0]
    expect(nameHeader.find('[data-test="custom-name-header"]').exists()).toBe(true)
    expect(nameHeader.attributes('aria-sort')).toBe('ascending')
    expect(nameHeader.findAll('svg')).toHaveLength(2)
    expect(nameHeader.findAll('svg')[0].classes()).toContain('text-primary-600')
    expect(nameHeader.findAll('svg')[1].classes()).toContain('text-gray-300')

    await nameHeader.trigger('click')
    await wrapper.vm.$nextTick()

    expect(nameHeader.attributes('aria-sort')).toBe('descending')
    expect(nameHeader.findAll('svg')[0].classes()).toContain('text-gray-300')
    expect(nameHeader.findAll('svg')[1].classes()).toContain('text-primary-600')
  })

  it('keeps row hover visual-only without changing table geometry', () => {
    const wrapper = mount(DataTable, {
      props: {
        columns: [{ key: 'name', label: 'Name' }],
        data: [{ id: 1, name: 'Stable row' }],
        stickyFirstColumn: true
      }
    })

    const dataRow = wrapper.get('tbody tr[data-index]')
    expect(dataRow.classes()).toEqual(
      expect.arrayContaining(['hover:bg-gray-50', 'dark:hover:bg-dark-800'])
    )

    const hoverBackgrounds: string[] = []
    const unsafeInteractionDeclarations: string[] = []

    dataTableStyles.walkRules((rule) => {
      const targetsDataRow = rule.selector.includes('tbody tr:not([aria-hidden])')
      const targetsRowHover = targetsHoveringTableRow(rule.selector)
      if (!targetsDataRow && !targetsRowHover) return

      rule.walkDecls((declaration) => {
        if (targetsRowHover && declaration.prop === 'background-color') {
          hoverBackgrounds.push(declaration.value)
        }
        if (
          isUnsafeRowInteractionDeclaration(
            declaration.prop,
            declaration.value,
            targetsRowHover
          )
        ) {
          unsafeInteractionDeclarations.push(`${declaration.prop}: ${declaration.value}`)
        }
      })
    })

    expect(hoverBackgrounds).toEqual(
      expect.arrayContaining(['rgb(249 250 251)', 'rgb(31 41 55)'])
    )
    expect(unsafeInteractionDeclarations).toEqual([])
  })

  it('keeps the frozen select column flush with the following frozen column', () => {
    const wrapper = mount(DataTable, {
      props: {
        columns: [
          { key: 'select', label: '' },
          { key: 'name', label: 'Name' },
          { key: 'id', label: 'ID' }
        ],
        data: [{ id: 1, name: 'Stable row' }],
        stickyFirstColumn: true
      },
      slots: {
        'cell-select': '<input type="checkbox" />'
      }
    })

    const headerCells = wrapper.findAll('thead th')
    const dataCells = wrapper.findAll('tbody tr[data-index] td')

    expect(headerCells[0].classes()).toEqual(
      expect.arrayContaining(['sticky-col-left-first', 'sticky-select-col'])
    )
    expect(dataCells[0].classes()).toEqual(
      expect.arrayContaining(['sticky-col-left-first', 'sticky-select-col'])
    )
    expect(headerCells[1].classes()).toContain('sticky-col-left-second')
    expect(dataCells[1].classes()).toContain('sticky-col-left-second')
    expect(headerCells[0].get('div').classes()).toContain('justify-center')

    const declarations = new Map<string, { value: string; important: boolean }>()
    dataTableStyles.walkRules('.sticky-select-col', (rule) => {
      rule.walkDecls((declaration) => declarations.set(declaration.prop, {
        value: declaration.value,
        important: declaration.important
      }))
    })

    expect(declarations.get('width')?.value).toBe('var(--select-col-width)')
    expect(declarations.get('min-width')?.value).toBe('var(--select-col-width)')
    expect(declarations.get('max-width')?.value).toBe('var(--select-col-width)')
    expect(declarations.get('padding-left')).toEqual({ value: '0', important: true })
    expect(declarations.get('padding-right')).toEqual({ value: '0', important: true })

    const offsets: string[] = []
    dataTableStyles.walkRules('.sticky-col-left-second', (rule) => {
      rule.walkDecls('left', (declaration) => offsets.push(declaration.value))
    })
    expect(offsets).toContain('var(--select-col-width)')
  })

  it('rejects broad transitions while allowing static base-row geometry', () => {
    expect(isUnsafeRowInteractionDeclaration('padding', '0.5rem', false)).toBe(false)
    expect(isUnsafeRowInteractionDeclaration('padding', '0.5rem', true)).toBe(true)
    expect(isUnsafeRowInteractionDeclaration('transition', 'all 140ms ease', false)).toBe(true)
    expect(isUnsafeRowInteractionDeclaration('transition-property', 'opacity, all', false)).toBe(
      true
    )
    expect(
      isUnsafeRowInteractionDeclaration('transition', 'background-color 140ms ease', false)
    ).toBe(false)
  })

  it('recognizes row hover selectors with intervening state pseudo-classes', () => {
    expect(targetsHoveringTableRow('tbody tr:hover .sticky-col')).toBe(true)
    expect(
      targetsHoveringTableRow(
        '.table-wrapper tbody tr:not([aria-hidden]):hover :is(td:first-child, .sticky-col)'
      )
    ).toBe(true)
    expect(targetsHoveringTableRow('tbody tr:not([aria-hidden])')).toBe(false)
  })

  it('keeps the DataTable scrollbar thumb locally visible and interactive', () => {
    const declarations = new Map<string, { value: string; important: boolean }>()

    dataTableStyles.walkRules('.table-wrapper::-webkit-scrollbar-thumb', (rule) => {
      rule.walkDecls((declaration) => {
        declarations.set(declaration.prop, {
          value: declaration.value,
          important: declaration.important
        })
      })
    })

    expect(declarations.get('background-color')).toEqual({
      value: 'rgba(107, 114, 128, 0.75)',
      important: true
    })
    expect(declarations.get('background-clip')).toEqual({
      value: 'padding-box',
      important: true
    })

    const hoverColors: string[] = []
    dataTableStyles.walkRules('.table-wrapper::-webkit-scrollbar-thumb:hover', (rule) => {
      rule.walkDecls('background-color', (declaration) => hoverColors.push(declaration.value))
    })
    expect(hoverColors).toContain('rgba(75, 85, 99, 0.9)')
  })

  it('keeps template hover classes visual-only', () => {
    const interactionClasses = getInteractionClassTokens(dataTableTemplate)
    const unsafeGeometryClasses = interactionClasses.filter(changesGeometryUtility)

    expect(interactionClasses).toEqual(
      expect.arrayContaining(['hover:bg-gray-50', 'dark:hover:bg-dark-800'])
    )
    expect(unsafeGeometryClasses).toEqual([])

    expect(
      [
        'hover:px-4',
        'group-hover:-translate-y-1',
        'dark:hover:scale-105',
        'hover:w-full',
        'hover:gap-2',
        'hover:border-2',
        'hover:border-[length:3px]'
      ].every(changesGeometryUtility)
    ).toBe(true)
    expect(
      [
        'hover:bg-gray-50',
        'dark:hover:text-white',
        'group-hover:shadow-lg',
        'hover:border-gray-200'
      ].some(changesGeometryUtility)
    ).toBe(false)
  })

  it('renders every row with no virtual padding spacer for small datasets (virtualization off)', async () => {
    const data = Array.from({ length: 8 }, (_, i) => ({ id: i + 1, name: `Row ${i + 1}` }))
    const wrapper = mount(DataTable, {
      props: {
        columns: [{ key: 'name', label: 'Name' }],
        data
      }
    })

    await wrapper.vm.$nextTick()

    // Virtualization is OFF for a small list…
    expect((wrapper.vm as any).shouldVirtualize).toBe(false)
    // …every row is in the DOM…
    expect(wrapper.findAll('tbody tr[data-index]')).toHaveLength(data.length)
    // …and there are no aria-hidden virtual padding spacer rows.
    expect(wrapper.findAll('tbody tr[aria-hidden="true"]')).toHaveLength(0)
  })

  it('switches to windowed rendering once row count exceeds virtualizeThreshold', async () => {
    const data = Array.from({ length: 12 }, (_, i) => ({ id: i + 1, name: `Row ${i + 1}` }))
    const wrapper = mount(DataTable, {
      props: {
        columns: [{ key: 'name', label: 'Name' }],
        data,
        virtualizeThreshold: 3
      }
    })

    await wrapper.vm.$nextTick()

    // Virtualization is ON: the mode-switch decision flipped…
    expect((wrapper.vm as any).shouldVirtualize).toBe(true)
    // …and the virtualizer drives off the full row count.
    const exposed = (wrapper.vm as any).virtualizer
    const instance = exposed?.value ?? exposed
    expect(instance.options.count).toBe(data.length)
  })

  it('keys the virtualizer size cache by row identity, not index (avoids stale heights on sort/filter)', async () => {
    const data = Array.from({ length: 12 }, (_, i) => ({ id: 100 + i, name: `Row ${i + 1}` }))
    const wrapper = mount(DataTable, {
      props: {
        columns: [{ key: 'name', label: 'Name' }],
        data,
        rowKey: 'id',
        virtualizeThreshold: 3
      }
    })

    await wrapper.vm.$nextTick()

    const exposed = (wrapper.vm as any).virtualizer
    const instance = exposed?.value ?? exposed
    // getItemKey must resolve to the row's stable key (id), not the positional index.
    expect(instance.options.getItemKey(0)).toBe(100)
    expect(instance.options.getItemKey(5)).toBe(105)
  })

  it('clears stale row and element caches when pagination replaces the row ID set', async () => {
    const firstPage = Array.from({ length: 100 }, (_, i) => ({ id: i + 1, name: `First ${i + 1}` }))
    const secondPage = Array.from({ length: 100 }, (_, i) => ({ id: i + 101, name: `Second ${i + 1}` }))
    const wrapper = mount(DataTable, {
      props: {
        columns: [{ key: 'name', label: 'Name' }],
        data: firstPage,
        rowKey: 'id',
        virtualizeThreshold: 1
      }
    })

    await wrapper.vm.$nextTick()

    const exposed = (wrapper.vm as any).virtualizer
    const instance = exposed?.value ?? exposed
    const firstPageIDs = firstPage.map(row => row.id)
    ;(instance as any).itemSizeCache = new Map(firstPageIDs.map(id => [id, 156]))
    instance.elementsCache.clear()
    for (const id of firstPageIDs) {
      instance.elementsCache.set(id, document.createElement('tr'))
    }
    const measureElementSpy = vi.spyOn(instance, 'measureElement')

    await wrapper.setProps({ data: secondPage })
    await wrapper.vm.$nextTick()

    const sizeCache = (instance as any).itemSizeCache as Map<number, number>
    expect(sizeCache.size).toBeLessThanOrEqual(secondPage.length)
    expect(instance.elementsCache.size).toBeLessThanOrEqual(secondPage.length)
    expect(firstPageIDs.some(id => sizeCache.has(id))).toBe(false)
    expect(firstPageIDs.some(id => instance.elementsCache.has(id))).toBe(false)
    expect(measureElementSpy.mock.calls.some(([node]) => node === null)).toBe(true)
  })

  it('clears stale caches when equal-length pages replace rows without stable keys', async () => {
    const firstPage = Array.from({ length: 12 }, (_, i) => ({ name: `First ${i + 1}` }))
    const secondPage = Array.from({ length: 12 }, (_, i) => ({ name: `Second ${i + 1}` }))
    const wrapper = mount(DataTable, {
      props: {
        columns: [{ key: 'name', label: 'Name' }],
        data: firstPage,
        virtualizeThreshold: 1
      }
    })

    await wrapper.vm.$nextTick()

    const exposed = (wrapper.vm as any).virtualizer
    const instance = exposed?.value ?? exposed
    const measureElementSpy = vi.spyOn(instance, 'measureElement')

    await wrapper.setProps({ data: secondPage })
    await wrapper.vm.$nextTick()

    expect(measureElementSpy.mock.calls.some(([node]) => node === null)).toBe(true)
  })

  it('conservatively clears caches when duplicate row-key multiplicity changes', async () => {
    const firstPage = [
      { id: 1, name: 'First A' },
      { id: 1, name: 'First B' },
      { id: 2, name: 'First C' }
    ]
    const secondPage = [
      { id: 1, name: 'Second A' },
      { id: 2, name: 'Second B' },
      { id: 2, name: 'Second C' }
    ]
    const wrapper = mount(DataTable, {
      props: {
        columns: [{ key: 'name', label: 'Name' }],
        data: firstPage,
        rowKey: 'id',
        virtualizeThreshold: 1
      }
    })

    await wrapper.vm.$nextTick()

    const exposed = (wrapper.vm as any).virtualizer
    const instance = exposed?.value ?? exposed
    const measureElementSpy = vi.spyOn(instance, 'measureElement')

    await wrapper.setProps({ data: secondPage })
    await wrapper.vm.$nextTick()

    expect(measureElementSpy.mock.calls.some(([node]) => node === null)).toBe(true)
  })

  it('preserves cache when rows without stable keys only reorder the same objects', async () => {
    const data = Array.from({ length: 12 }, (_, i) => ({ name: `Row ${i + 1}` }))
    const wrapper = mount(DataTable, {
      props: {
        columns: [{ key: 'name', label: 'Name' }],
        data,
        virtualizeThreshold: 1
      }
    })

    await wrapper.vm.$nextTick()

    const exposed = (wrapper.vm as any).virtualizer
    const instance = exposed?.value ?? exposed
    const measureSpy = vi.spyOn(instance, 'measure')

    await wrapper.setProps({ data: [...data].reverse() })
    await wrapper.vm.$nextTick()

    expect(measureSpy).not.toHaveBeenCalled()
  })

  it('preserves stable row height cache when the same row IDs are only reordered', async () => {
    const data = Array.from({ length: 100 }, (_, i) => ({ id: i + 1, name: `Row ${i + 1}` }))
    const wrapper = mount(DataTable, {
      props: {
        columns: [{ key: 'name', label: 'Name' }],
        data,
        rowKey: 'id',
        virtualizeThreshold: 1
      }
    })

    await wrapper.vm.$nextTick()

    const exposed = (wrapper.vm as any).virtualizer
    const instance = exposed?.value ?? exposed
    ;(instance as any).itemSizeCache = new Map(data.map(row => [row.id, 156]))
    const measureSpy = vi.spyOn(instance, 'measure')

    await wrapper.setProps({ data: [...data].reverse() })
    await wrapper.vm.$nextTick()

    const sizeCache = (instance as any).itemSizeCache as Map<number, number>
    expect(measureSpy).not.toHaveBeenCalled()
    expect(sizeCache.size).toBe(100)
  })

  it('emits controlled current-page selection while preserving off-page keys', async () => {
    const wrapper = mount(DataTable, {
      props: {
        columns: [{ key: 'name', label: 'Name' }],
        data: [
          { id: 1, name: 'One' },
          { id: 2, name: 'Two' }
        ],
        rowKey: 'id',
        selectable: true,
        selectedKeys: [99]
      }
    })

    await wrapper.get('[data-test="select-all"]').setValue(true)

    const selectedAll = wrapper.emitted('update:selectedKeys')?.at(-1)?.[0]
    expect(selectedAll).toEqual([99, 1, 2])

    await wrapper.setProps({ selectedKeys: selectedAll as number[] })
    const rowCheckboxes = wrapper.findAll<HTMLInputElement>('[data-test="select-row"]')
    expect(rowCheckboxes.every((checkbox) => checkbox.element.checked)).toBe(true)

    await rowCheckboxes[0].setValue(false)

    expect(wrapper.emitted('update:selectedKeys')?.at(-1)?.[0]).toEqual([99, 2])
    expect(wrapper.emitted('selectionChange')?.at(-1)?.[0]).toEqual([99, 2])
  })

  it('keeps the single usage field shrinkable in a 320px mobile card', () => {
    stubMobileMatchMedia()
    const viewport = document.createElement('div')
    viewport.style.width = '320px'
    document.body.appendChild(viewport)
    const wrapper = mount(DataTable, {
      attachTo: viewport,
      props: {
        columns: [{ key: 'usage', label: 'Usage' }],
        data: [{ id: 1, usage: 'snapshot' }],
        rowKey: 'id'
      },
      slots: {
        'cell-usage': '<div data-test="usage-cell">snapshot</div>'
      }
    })

    expect(viewport.style.width).toBe('320px')
    expect(wrapper.findAll('[data-field="usage"]')).toHaveLength(1)
    expect(wrapper.find('[data-field="ollama_cloud_usage"]').exists()).toBe(false)
    const field = wrapper.get('[data-field="usage"]')
    expect(field.classes()).toContain('min-w-0')
    expect(field.get('div').classes()).toEqual(expect.arrayContaining(['min-w-0', 'max-w-full']))
    expect(wrapper.findAll('[data-test="usage-cell"]')).toHaveLength(1)

    wrapper.unmount()
    viewport.remove()
  })

  it('offers current-page select all in the mobile card layout', async () => {
    stubMobileMatchMedia()
    const wrapper = mount(DataTable, {
      props: {
        columns: [{ key: 'name', label: 'Name' }],
        data: [
          { id: 1, name: 'One' },
          { id: 2, name: 'Two' }
        ],
        rowKey: 'id',
        selectable: true,
        selectedKeys: [99]
      }
    })

    await wrapper.get('[data-test="select-all-mobile"]').setValue(true)

    expect(wrapper.emitted('update:selectedKeys')?.at(-1)?.[0]).toEqual([99, 1, 2])
  })
})
