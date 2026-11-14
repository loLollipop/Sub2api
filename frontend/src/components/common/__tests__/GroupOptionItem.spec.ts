import { mount } from '@vue/test-utils'
import { describe, expect, it, vi } from 'vitest'

import GroupOptionItem from '../GroupOptionItem.vue'

vi.mock('vue-i18n', async () => {
  const actual = await vi.importActual<typeof import('vue-i18n')>('vue-i18n')
  return {
    ...actual,
    useI18n: () => ({ t: (key: string) => key }),
  }
})

vi.mock('@/stores/app', () => ({
  useAppStore: () => ({ cachedPublicSettings: null }),
}))

describe('GroupOptionItem description layout', () => {
  it('applies multiline and overflow-safe text styles', () => {
    const description = 'First section\nvery-long-unbroken-description-value-that-must-not-overflow'
    const wrapper = mount(GroupOptionItem, {
      props: {
        name: 'Example group',
        platform: 'openai',
        description,
      },
      global: {
        stubs: {
          GroupBadge: true,
        },
      },
    })

    const descriptionElement = wrapper
      .findAll('span')
      .find((element) => element.text() === description)

    expect(descriptionElement).toBeDefined()
    expect(descriptionElement?.classes()).toContain('whitespace-pre-line')
    expect(descriptionElement?.classes()).toContain('[overflow-wrap:anywhere]')
    expect(descriptionElement?.classes()).toContain('line-clamp-3')
    expect(wrapper.find('[title]').attributes('title')).toBe(description)
  })
})

describe('GroupOptionItem status dot', () => {
  const mountOption = (statusClass: string | null = null) => mount(GroupOptionItem, {
    props: {
      name: 'OpenAI', platform: 'openai', rateMultiplier: 1, userRateMultiplier: 0.5,
      statusClass, statusTitle: 'Availability 98.0%',
    },
    global: { stubs: { GroupBadge: true, Icon: true } },
  })

  it('places a labelled dot after the rate inside the pill', () => {
    const wrapper = mountOption('bg-emerald-600 dark:bg-emerald-400')
    const dot = wrapper.get('[data-testid="group-option-status-dot"]')
    expect(dot.classes()).toContain('bg-emerald-600')
    expect(dot.attributes('title')).toBe('Availability 98.0%')
    expect(dot.attributes('aria-label')).toBe('Availability 98.0%')
    expect(dot.attributes('role')).toBe('img')
    expect(dot.element.parentElement?.textContent).toContain('0.5x')
    expect(dot.element.parentElement?.lastElementChild).toBe(dot.element)
    wrapper.unmount()
  })

  it('hides the dot entirely without monitor buckets', () => {
    const wrapper = mountOption()
    expect(wrapper.find('[data-testid="group-option-status-dot"]').exists()).toBe(false)
    expect(wrapper.text()).toContain('0.5x')
    wrapper.unmount()
  })
})
