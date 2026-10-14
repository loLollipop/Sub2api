import { describe, expect, it, vi } from 'vitest'
import { mount } from '@vue/test-utils'
import MixedProviderRates from '../MixedProviderRates.vue'
import MixedProviderRateFields from '@/components/admin/group/MixedProviderRateFields.vue'
import { initializeMixedProviderRates, mixedModelVendors } from '@/utils/mixed-provider-rates'
import en from '@/i18n/locales/en/common'

vi.mock('vue-i18n', () => ({ useI18n: () => ({ t: (key: string) => en.common[key.replace('common.', '') as keyof typeof en.common] ?? key }) }))
const global = {}

describe('Mixed vendor rate controls', () => {
  it('shows each confirmed vendor with its absolute rate and personal precedence', () => {
    const wrapper = mount(MixedProviderRates, { global, props: { rates: [{ vendor: 'deepseek', rate: 0.4 }, { vendor: 'kimi', rate: 0 }], userOverride: true } })
    expect(wrapper.text()).toContain('DeepSeek 0.4x')
    expect(wrapper.text()).toContain('Kimi 0x')
    expect(wrapper.text()).toContain('takes priority')
    expect(wrapper.text()).not.toContain('Gemini')
  })
  it('does not invent vendors without a catalogue', () => {
    expect(mount(MixedProviderRates, { global, props: { rates: [] } }).text()).toBe('No available models confirmed')
  })
  it('offers independent vendor fields, emits zero, and marks blank input invalid', async () => {
    const wrapper = mount(MixedProviderRateFields, { global, props: { modelValue: initializeMixedProviderRates({ kimi: 2 }) } })
    expect(wrapper.findAll('input')).toHaveLength(mixedModelVendors.length)
    const input = wrapper.get('input[aria-label="Kimi"]')
    await input.setValue('0')
    expect(wrapper.emitted('update:modelValue')?.[0]?.[0]).toMatchObject({ kimi: 0, deepseek: 1 })
    await input.setValue('')
    expect((input.element as HTMLInputElement).checkValidity()).toBe(false)
  })
})
