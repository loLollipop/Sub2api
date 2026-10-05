import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { flushPromises, shallowMount } from '@vue/test-utils'
import postcss from 'postcss'
import { parse } from 'vue/compiler-sfc'
import ProxiesView from '../ProxiesView.vue'
import proxiesSource from '../ProxiesView.vue?raw'

const { listProxies, getAllWithCount } = vi.hoisted(() => ({
  listProxies: vi.fn(),
  getAllWithCount: vi.fn()
}))

vi.mock('@/api/admin', () => ({
  adminAPI: { proxies: { list: listProxies, getAllWithCount } }
}))
vi.mock('@/stores/app', () => ({ useAppStore: () => ({ showError: vi.fn() }) }))
vi.mock('vue-i18n', async () => ({
  ...await vi.importActual<typeof import('vue-i18n')>('vue-i18n'),
  useI18n: () => ({ t: (key: string) => key })
}))

const mountView = () => shallowMount(ProxiesView, {
  global: {
    stubs: {
      AppLayout: { template: '<div><slot /></div>' },
      TablePageLayout: {
        template: '<div><slot name="filters" /><slot name="table" /><slot name="pagination" /></div>'
      },
      DataTable: {
        props: ['data'],
        template: '<div data-test="rows">{{ data.map(row => row.name).join(",") }}</div>'
      },
      Pagination: {
        props: ['page'],
        emits: ['update:page'],
        template: '<button data-test="page" @click="$emit(\'update:page\', 2)">{{ page }}</button>'
      },
      Select: {
        props: ['modelValue', 'options', 'placeholder'],
        emits: ['update:modelValue', 'change'],
        template: `<select :data-filter="placeholder" :value="modelValue"
          @change="$emit('update:modelValue', $event.target.value); $emit('change', $event.target.value)">
          <option v-for="option in options" :key="option.value" :value="option.value">{{ option.label }}</option>
        </select>`
      }
    }
  }
})

let wrapper: ReturnType<typeof mountView>

beforeEach(() => {
  vi.clearAllMocks()
  getAllWithCount.mockResolvedValue([])
  listProxies.mockResolvedValue({ items: [], total: 100, pages: 5 })
})

afterEach(() => wrapper?.unmount())

describe('proxy list filter pagination', () => {
  it('groups the filters and keeps all seven compact actions available', () => {
    wrapper = mountView()
    const filters = wrapper.get('.proxy-toolbar-filters')
    expect(filters.findAll('input')).toHaveLength(1)
    expect(filters.findAll('select')).toHaveLength(2)
    const actions = wrapper.get('.proxy-toolbar-actions')
    expect(actions.findAll('button')).toHaveLength(7)
    expect(actions.find('.mr-2').exists()).toBe(false)
    expect(actions.get('button').attributes('aria-label')).toBe('common.refresh')
  })

  it('allows toolbar wrapping and gives mobile search and actions their own rows', () => {
    const { descriptor } = parse(proxiesSource)
    const styles = postcss.parse(descriptor.styles.map((style) => style.content).join('\n'))
    const declaration = (selector: string, property: string, mobile = false) => {
      let value: string | undefined
      styles.walkRules(selector, (rule) => {
        const isMobile = rule.parent?.type === 'atrule' && rule.parent.params === '(max-width: 639px)'
        if (isMobile !== mobile) return
        rule.walkDecls(property, (decl) => { value = decl.value })
      })
      return value
    }
    expect(declaration('.proxy-toolbar', 'flex-wrap')).toBe('wrap')
    expect(declaration('.proxy-toolbar-actions', 'flex-wrap')).toBe('wrap')
    expect(declaration('.proxy-toolbar-actions', 'max-width')).toBe('100%')
    expect(declaration('.proxy-toolbar-actions > .btn', 'white-space')).toBe('nowrap')
    expect(declaration('.proxy-toolbar-search', 'flex-basis', true)).toBe('100%')
    expect(declaration('.proxy-toolbar-actions', 'width', true)).toBe('100%')
    expect(declaration('.proxy-toolbar-actions', 'justify-content', true)).toBe('flex-start')
  })

  it.each([
    ['protocol', 'admin.proxies.allProtocols', 'socks5', ''],
    ['protocol', 'admin.proxies.allProtocols', '', 'http'],
    ['status', 'admin.proxies.allStatus', 'expired', ''],
    ['status', 'admin.proxies.allStatus', '', 'active']
  ])('starts at page one when changing %s through %s to "%s"', async (field, placeholder, value, initial) => {
    wrapper = mountView()
    await flushPromises()
    const filter = wrapper.get(`select[data-filter="${placeholder}"]`)
    if (initial) {
      await filter.setValue(initial)
      await flushPromises()
    }
    await wrapper.get('[data-test="page"]').trigger('click')
    await flushPromises()
    expect(listProxies.mock.lastCall?.[0]).toBe(2)
    const [, pageSize, previousFilters] = listProxies.mock.lastCall!
    listProxies.mockImplementation(async (page) => ({
      items: page === 1 ? [{ id: 1, name: 'matching-proxy' }] : [],
      total: 1,
      pages: 1
    }))

    await filter.setValue(value)
    await flushPromises()

    expect(listProxies).toHaveBeenLastCalledWith(
      1, pageSize, { ...previousFilters, [field]: value || undefined },
      { signal: expect.any(AbortSignal) }
    )
    expect(wrapper.get('[data-test="page"]').text()).toBe('1')
    expect(wrapper.get('[data-test="rows"]').text()).toBe('matching-proxy')
  })

  it('keeps the current page when refreshing the same filter', async () => {
    wrapper = mountView()
    await flushPromises()
    await wrapper.get('[data-test="page"]').trigger('click')
    await flushPromises()

    await wrapper.get('button[title="common.refresh"]').trigger('click')
    await flushPromises()

    expect(listProxies.mock.lastCall?.[0]).toBe(2)
    expect(wrapper.get('[data-test="page"]').text()).toBe('2')
  })
})
