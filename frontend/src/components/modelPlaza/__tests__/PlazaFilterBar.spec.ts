import { describe, expect, it, vi } from 'vitest'
import { mount } from '@vue/test-utils'
import PlazaFilterBar from '../PlazaFilterBar.vue'

vi.mock('vue-i18n', async () => ({
  ...await vi.importActual<typeof import('vue-i18n')>('vue-i18n'),
  useI18n: () => ({ t: (key: string) => key })
}))

const groups = [
  { id: 1, name: 'Claude', platforms: ['anthropic'], rate: 1 },
  { id: 2, name: 'GPT', platforms: ['openai'], rate: 0.07 },
  { id: 3, name: 'Grok', platforms: ['grok'], rate: 1 }
]

function mountBar(props: Record<string, unknown> = {}) {
  return mount(PlazaFilterBar, {
    props: {
      platforms: ['anthropic', 'openai', 'grok'], groups,
      platform: 'all', groupId: 'all', search: '', ...props
    },
    global: {
      stubs: {
        Icon: true,
        PlazaProviderIcon: true,
        Select: {
          props: ['modelValue', 'options', 'ariaLabel'],
          emits: ['update:modelValue'],
          template: `
            <div :aria-label="ariaLabel">
              <button v-for="option in options" :key="String(option.value)" type="button"
                :disabled="option.disabled" :data-option="String(option.value)"
                @click="$emit('update:modelValue', option.value)">{{ option.label }}</button>
            </div>`
        }
      }
    }
  })
}

describe('PlazaFilterBar group and brand filters', () => {
  it('keeps one available tab stop for stale group and brand selections', () => {
    const wrapper = mountBar({ sidebar: true, groupId: 999, platform: 'anthropic' })
    for (const selector of ['.plaza-group-options', '.plaza-seg']) {
      const tabStops = wrapper.findAll(`${selector} [tabindex="0"]`)
      expect(tabStops).toHaveLength(1)
      expect(tabStops[0].attributes('disabled')).toBeUndefined()
      expect(tabStops[0].text()).toContain('modelPlaza.filters.all')
    }
  })

  it('offers one reset action only while a group, brand or search is selected', async () => {
    const wrapper = mountBar()
    expect(wrapper.find('[data-testid="plaza-clear-filters"]').exists()).toBe(false)
    await wrapper.setProps({ groupId: 2 })
    await wrapper.get('[data-testid="plaza-clear-filters"]').trigger('click')
    expect(wrapper.emitted('reset')).toEqual([[]])
    await wrapper.setProps({ groupId: 'all', search: 'gpt' })
    expect(wrapper.find('[data-testid="plaza-clear-filters"]').exists()).toBe(true)
    await wrapper.setProps({ search: '   ' })
    expect(wrapper.find('[data-testid="plaza-clear-filters"]').exists()).toBe(false)
    await wrapper.setProps({ platform: 'openai' })
    expect(wrapper.find('[data-testid="plaza-clear-filters"]').exists()).toBe(true)
  })

  it('hides brands outside the selected group and restores all brands for all groups', async () => {
    const wrapper = mountBar({ groupId: 2 })
    const labels = () => wrapper.findAll('.plaza-seg-item').map(button => button.text())
    expect(labels()).toEqual(['modelPlaza.filters.all', 'OpenAI'])
    expect(wrapper.findAll('.plaza-seg-item').every(button => button.attributes('disabled') === undefined)).toBe(true)
    await wrapper.setProps({ groupId: 1 })
    expect(labels()).toEqual(['modelPlaza.filters.all', 'Anthropic'])
    await wrapper.setProps({ groupId: 'all' })
    expect(labels()).toEqual(['modelPlaza.filters.all', 'Anthropic', 'OpenAI', 'Grok'])
  })

  it('keeps every group selectable under a different selected brand and displays rates in the toolbar', async () => {
    const wrapper = mountBar({ platform: 'openai' })
    const options = wrapper.get('[aria-label="modelPlaza.filters.groupLabel"]').findAll('button')
    expect(options.map(button => button.text())).toEqual(['modelPlaza.filters.groupLabel', 'Claude 1x', 'GPT 0.07x', 'Grok 1x'])
    expect(options.every(button => button.attributes('disabled') === undefined)).toBe(true)
    await options[1].trigger('click')
    expect(wrapper.emitted('update:groupId')?.at(-1)).toEqual([1])
    expect(wrapper.find('[aria-label="modelPlaza.filters.rateLabel"]').exists()).toBe(false)
  })

  it('shows an effective zero rate next to the sidebar group name without creating a rate selector', () => {
    const wrapper = mountBar({ sidebar: true, groups: [{ ...groups[1], rate: 0 }] })
    expect(wrapper.get('.plaza-group-rate').text()).toBe('0x')
    expect(wrapper.get('.plaza-group-rate').element.parentElement?.textContent).toContain('GPT')
    expect(wrapper.find('[aria-label="modelPlaza.filters.rateLabel"]').exists()).toBe(false)
    expect(wrapper.findAllComponents({ name: 'Select' })).toHaveLength(0)
  })

  it('uses the actual brands in composite groups and labels unknown brands independently of protocol', () => {
    const wrapper = mountBar({
      groups: [{ id: 9, name: 'mixed', platforms: ['openai', 'anthropic', 'other'], rate: 0.5 }],
      platforms: ['openai', 'anthropic', 'other', 'grok'], platform: 'anthropic', groupId: 9
    })
    expect(wrapper.findAll('.plaza-seg-item').map(button => button.text())).toEqual([
      'modelPlaza.filters.all', 'OpenAI', 'Anthropic', 'modelPlaza.filters.otherProviders'
    ])
    expect(wrapper.get('[data-option="9"]').text()).toBe('mixed 0.5x')
  })

  it('emits toolbar search, clear, group and brand changes without a rate event', async () => {
    const wrapper = mountBar()
    await wrapper.get('.plaza-search-input').setValue('gpt-5')
    expect(wrapper.emitted('update:search')?.at(-1)).toEqual(['gpt-5'])
    await wrapper.setProps({ search: 'gpt-5' })
    await wrapper.get('.plaza-search-clear').trigger('click')
    expect(wrapper.emitted('update:search')?.at(-1)).toEqual([''])
    await wrapper.get('[data-option="2"]').trigger('click')
    expect(wrapper.emitted('update:groupId')?.at(-1)).toEqual([2])
    await wrapper.get('.plaza-seg-item[aria-checked="true"]').trigger('click')
    expect(wrapper.emitted('update:platform')?.at(-1)).toEqual(['all'])
    expect(wrapper.emitted('update:rate')).toBeUndefined()
  })

  it('supports directional, Home and End navigation with one tab stop', async () => {
    const wrapper = mountBar()
    document.body.append(wrapper.element)
    try {
      const radios = wrapper.findAll('[role="radio"]')
      expect(radios.map(radio => radio.attributes('tabindex'))).toEqual(['0', '-1', '-1', '-1'])
      for (const [from, key, target, platform] of [
        [0, 'ArrowDown', 1, 'anthropic'], [1, 'ArrowRight', 2, 'openai'],
        [2, 'ArrowUp', 1, 'anthropic'], [1, 'ArrowLeft', 0, 'all'],
        [0, 'ArrowLeft', 3, 'grok'], [3, 'Home', 0, 'all'], [0, 'End', 3, 'grok']
      ] as const) {
        await radios[from].trigger('keydown', { key })
        expect(wrapper.emitted('update:platform')?.at(-1)).toEqual([platform])
        expect(document.activeElement).toBe(radios[target].element)
        await wrapper.setProps({ platform })
        expect(radios.filter(radio => radio.attributes('tabindex') === '0')).toHaveLength(1)
        expect(radios[target].attributes('aria-checked')).toBe('true')
      }
    } finally {
      wrapper.unmount()
    }
  })

  it('navigates only visible brands and leaves IME composition and ordinary keys alone', async () => {
    const wrapper = mountBar({ groupId: 2 })
    const radios = wrapper.findAll('[role="radio"]')
    expect(radios).toHaveLength(2)
    await radios[0].trigger('keydown', { key: 'ArrowDown', isComposing: true })
    await radios[0].trigger('keydown', { key: 'a' })
    expect(wrapper.emitted('update:platform')).toBeUndefined()
    await radios[0].trigger('keydown', { key: 'ArrowDown' })
    expect(wrapper.emitted('update:platform')?.at(-1)).toEqual(['openai'])
    await wrapper.setProps({ platform: 'openai' })
    await radios[1].trigger('keydown', { key: 'ArrowDown' })
    expect(wrapper.emitted('update:platform')?.at(-1)).toEqual(['all'])
  })

  it('puts counted groups first and reaches incompatible groups with the keyboard', async () => {
    const wrapper = mountBar({ sidebar: true, platform: 'openai', groups: groups.map((group, index) => ({ ...group, modelCount: index + 1 })) })
    document.body.append(wrapper.element)
    try {
      const radios = wrapper.get('.plaza-group-options').findAll('[role="radio"]')
      expect(wrapper.findAll('[role="radiogroup"]')[0].attributes('aria-label')).toBe('modelPlaza.filters.groupLabel')
      expect(radios.map(radio => radio.get('.plaza-group-count').text())).toEqual(['6', '1', '2', '3'])
      expect(radios.map(radio => radio.attributes('tabindex'))).toEqual(['0', '-1', '-1', '-1'])
      for (const [from, key, target, groupId] of [
        [0, 'ArrowDown', 1, 1], [1, 'End', 3, 3], [3, 'ArrowRight', 0, 'all'],
        [0, 'ArrowUp', 3, 3], [3, 'Home', 0, 'all']
      ] as const) {
        await radios[from].trigger('keydown', { key })
        expect(wrapper.emitted('update:groupId')?.at(-1)).toEqual([groupId])
        expect(document.activeElement).toBe(radios[target].element)
        await wrapper.setProps({ groupId })
        expect(radios.filter(radio => radio.attributes('tabindex') === '0')).toHaveLength(1)
      }
    } finally {
      wrapper.unmount()
    }
  })
})
