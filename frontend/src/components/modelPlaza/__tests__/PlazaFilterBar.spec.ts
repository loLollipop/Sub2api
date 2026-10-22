import { describe, expect, it, vi } from 'vitest'
import { mount } from '@vue/test-utils'
import PlazaFilterBar from '../PlazaFilterBar.vue'

vi.mock('vue-i18n', async () => {
  const actual = await vi.importActual<typeof import('vue-i18n')>('vue-i18n')
  return {
    ...actual,
    useI18n: () => ({ t: (key: string) => key }),
  }
})

const groups = [
  { id: 1, name: 'Claude', platform: 'anthropic', rate: 1 },
  { id: 2, name: 'GPT', platform: 'openai', rate: 0.07 },
  { id: 3, name: 'Grok', platform: 'grok', rate: 1 },
]

function mountBar(props: Record<string, unknown> = {}) {
  return mount(PlazaFilterBar, {
    props: {
      platforms: ['anthropic', 'openai', 'grok'],
      groups,
      rates: [0.07, 1],
      platform: 'all',
      groupId: 'all',
      rate: 'all',
      search: '',
      ...props,
    },
    global: {
      stubs: {
        Icon: true,
        PlatformIcon: true,
        Select: {
          props: ['modelValue', 'options', 'ariaLabel'],
          emits: ['update:modelValue'],
          template: `
            <div :aria-label="ariaLabel">
              <button
                v-for="option in options"
                :key="String(option.value)"
                type="button"
                :disabled="option.disabled"
                :data-option="String(option.value)"
                @click="$emit('update:modelValue', option.value)"
              >{{ option.label }}</button>
            </div>
          `,
        },
      },
    },
  })
}

describe('PlazaFilterBar faceted filters', () => {

  it('offers one reset action only while any filter is selected, including a zero rate', async () => {
    const wrapper = mountBar()
    expect(wrapper.find('[data-testid="plaza-clear-filters"]').exists()).toBe(false)
    await wrapper.setProps({ rate: 0 })
    await wrapper.get('[data-testid="plaza-clear-filters"]').trigger('click')
    expect(wrapper.emitted('reset')).toEqual([[]])
    await wrapper.setProps({ rate: 'all', search: 'gpt' })
    expect(wrapper.find('[data-testid="plaza-clear-filters"]').exists()).toBe(true)
    await wrapper.setProps({ search: '   ' })
    expect(wrapper.find('[data-testid="plaza-clear-filters"]').exists()).toBe(false)
  })

  it('keeps unmatched platforms, groups and rates disabled instead of hiding them', () => {
    const byGroup = mountBar({ groupId: 2 })
    const platforms = byGroup.findAll('.plaza-seg-item')
    expect(platforms.map((button) => button.text().replace(/\s+/g, ' ').trim())).toEqual([
      'modelPlaza.filters.all',
      'Anthropic',
      'OpenAI',
      'Grok',
    ])
    expect(platforms[1].attributes('disabled')).toBeDefined()
    expect(platforms[2].attributes('disabled')).toBeUndefined()
    expect(platforms[3].attributes('disabled')).toBeDefined()

    const byPlatform = mountBar({ platform: 'openai' })
    const groupButtons = byPlatform.get('[aria-label="modelPlaza.filters.groupLabel"]').findAll('button')
    expect(groupButtons.map((button) => button.text())).toEqual(['modelPlaza.filters.groupLabel', 'Claude', 'GPT', 'Grok'])
    expect(groupButtons[1].attributes('disabled')).toBeDefined()
    expect(groupButtons[2].attributes('disabled')).toBeUndefined()

    const rateButtons = byPlatform.get('[aria-label="modelPlaza.filters.rateLabel"]').findAll('button')
    expect(rateButtons.map((button) => button.text())).toEqual(['modelPlaza.filters.rateLabel', '0.07x', '1x'])
    expect(rateButtons[1].attributes('disabled')).toBeUndefined()
    expect(rateButtons[2].attributes('disabled')).toBeDefined()
  })

  it('supports actual vendors within composite groups in all three facets', () => {
    const composite = [{ id: 9, name: 'mixed', platform: 'composite', platforms: ['openai', 'anthropic'], rate: 0.5 }]
    const wrapper = mountBar({ groups: composite, platforms: ['openai', 'anthropic'], rates: [0.5], platform: 'anthropic', groupId: 9 })
    expect(wrapper.findAll('.plaza-seg-item').every(button => button.attributes('disabled') === undefined)).toBe(true)
    expect(wrapper.get('[aria-label="modelPlaza.filters.groupLabel"] [data-option="9"]').attributes('disabled')).toBeUndefined()
    expect(wrapper.get('[aria-label="modelPlaza.filters.rateLabel"] [data-option="0.5"]').attributes('disabled')).toBeUndefined()
  })

  it('emits compact toolbar changes without dropping the all option', async () => {
    const wrapper = mountBar()
    await wrapper.get('.plaza-search-input').setValue('gpt-5')
    expect(wrapper.emitted('update:search')?.at(-1)).toEqual(['gpt-5'])
    await wrapper.get('[aria-label="modelPlaza.filters.groupLabel"]').get('[data-option="2"]').trigger('click')
    expect(wrapper.emitted('update:groupId')?.at(-1)).toEqual([2])
    await wrapper.get('[aria-label="modelPlaza.filters.rateLabel"]').get('[data-option="0.07"]').trigger('click')
    expect(wrapper.emitted('update:rate')?.at(-1)).toEqual([0.07])
    await wrapper.get('.plaza-seg-item[aria-checked="true"]').trigger('click')
    expect(wrapper.emitted('update:platform')?.at(-1)).toEqual(['all'])
  })

  it('supports directional, Home and End navigation with one tab stop', async () => {
    const wrapper = mountBar()
    document.body.append(wrapper.element)
    try {
      const radios = wrapper.findAll('[role="radio"]')
      expect(radios.map(radio => radio.attributes('tabindex'))).toEqual(['0', '-1', '-1', '-1'])
      for (const [from, key, target, platform] of [
        [0, 'ArrowDown', 1, 'anthropic'],
        [1, 'ArrowRight', 2, 'openai'],
        [2, 'ArrowUp', 1, 'anthropic'],
        [1, 'ArrowLeft', 0, 'all'],
        [0, 'ArrowLeft', 3, 'grok'],
        [3, 'Home', 0, 'all'],
        [0, 'End', 3, 'grok']
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

  it('skips disabled providers and leaves IME composition and ordinary keys alone', async () => {
    const wrapper = mountBar({ groupId: 2 })
    const radios = wrapper.findAll('[role="radio"]')
    await radios[0].trigger('keydown', { key: 'ArrowDown', isComposing: true })
    await radios[0].trigger('keydown', { key: 'a' })
    expect(wrapper.emitted('update:platform')).toBeUndefined()
    await radios[0].trigger('keydown', { key: 'ArrowDown' })
    expect(wrapper.emitted('update:platform')?.at(-1)).toEqual(['openai'])
    await wrapper.setProps({ platform: 'openai' })
    await radios[2].trigger('keydown', { key: 'ArrowDown' })
    expect(wrapper.emitted('update:platform')?.at(-1)).toEqual(['all'])
  })
})
