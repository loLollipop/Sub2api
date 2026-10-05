import { defineComponent, h } from 'vue'
import { enableAutoUnmount, flushPromises, mount } from '@vue/test-utils'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import OverloadCooldownCard, { useOverloadCooldownCard } from '../OverloadCooldownCard.vue'
import RequestRectifierCard, { useRequestRectifierCard } from '../RequestRectifierCard.vue'
import BetaPolicyCard, { useBetaPolicyCard } from '../BetaPolicyCard.vue'

enableAutoUnmount(afterEach)
const mocks = vi.hoisted(() => ({
  getOverloadCooldownSettings: vi.fn(),
  updateOverloadCooldownSettings: vi.fn(),
  getRectifierSettings: vi.fn(),
  updateRectifierSettings: vi.fn(),
  getBetaPolicySettings: vi.fn(),
  updateBetaPolicySettings: vi.fn(),
  showSuccess: vi.fn(),
  showError: vi.fn()
}))
vi.mock('@/api', () => ({ adminAPI: { settings: mocks } }))
vi.mock('@/stores', () => ({ useAppStore: () => mocks }))
vi.mock('vue-i18n', () => ({ useI18n: () => ({ t: (key: string) => key }) }))

describe('independent settings cards', () => {
  beforeEach(() => vi.resetAllMocks())

  it('hydrates and saves cooldown separately without submitting its enclosing bulk form', async () => {
    mocks.getOverloadCooldownSettings.mockResolvedValue({ enabled: true, cooldown_minutes: 17 })
    mocks.updateOverloadCooldownSettings.mockImplementation(async (value) => value)
    const submit = vi.fn()
    const wrapper = mount(defineComponent({
      setup() {
        const controller = useOverloadCooldownCard()
        void controller.loadOverloadCooldownSettings()
        return () => h('form', { onSubmit: submit }, [h(OverloadCooldownCard, { controller })])
      }
    }))
    await flushPromises()
    await wrapper.get('input[type="number"]').setValue(23)
    const save = wrapper.findAll('button').find((button) => button.text().includes('common.save'))!
    expect(save.attributes('type')).toBe('button')
    await save.trigger('click')
    await flushPromises()
    expect(mocks.updateOverloadCooldownSettings).toHaveBeenCalledWith({ enabled: true, cooldown_minutes: 23 })
    expect(mocks.showSuccess).toHaveBeenCalledWith('admin.settings.overloadCooldown.saved')
    expect(submit).not.toHaveBeenCalled()
  })

  it('keeps cooldown editable after a failed request and releases its saving state', async () => {
    mocks.getOverloadCooldownSettings.mockRejectedValue(new Error('offline'))
    mocks.updateOverloadCooldownSettings.mockRejectedValue(new Error('offline'))
    let controller!: ReturnType<typeof useOverloadCooldownCard>
    mount(defineComponent({
      setup() {
        controller = useOverloadCooldownCard()
        return () => h(OverloadCooldownCard, { controller })
      }
    }))
    await controller.loadOverloadCooldownSettings()
    expect(controller.overloadCooldownLoading).toBe(false)
    expect(controller.overloadCooldownForm.cooldown_minutes).toBe(10)
    await controller.saveOverloadCooldownSettings()
    expect(controller.overloadCooldownSaving).toBe(false)
    expect(mocks.showError).toHaveBeenCalledOnce()
  })

  it('normalizes absent rectifier patterns and omits empty patterns from the endpoint payload', async () => {
    mocks.getRectifierSettings.mockResolvedValue({
      enabled: true, thinking_signature_enabled: true, thinking_budget_enabled: true,
      apikey_signature_enabled: true, apikey_signature_patterns: null
    })
    mocks.updateRectifierSettings.mockImplementation(async (value) => value)
    let controller!: ReturnType<typeof useRequestRectifierCard>
    mount(defineComponent({
      setup() {
        controller = useRequestRectifierCard()
        return () => h(RequestRectifierCard, { controller })
      }
    }))
    await controller.loadRectifierSettings()
    expect(controller.rectifierForm.apikey_signature_patterns).toEqual([])
    controller.rectifierForm.apikey_signature_patterns = [' ', 'signature']
    await controller.saveRectifierSettings()
    expect(mocks.updateRectifierSettings).toHaveBeenCalledWith(expect.objectContaining({
      apikey_signature_patterns: ['signature']
    }))
    expect(controller.rectifierSaving).toBe(false)
  })

  it('removes beta fallback fields when no target models remain', async () => {
    mocks.getBetaPolicySettings.mockResolvedValue({ rules: [{
      beta_token: 'test-beta', action: 'pass', scope: 'all', model_whitelist: [' '],
      fallback_action: 'block', fallback_error_message: 'blocked'
    }] })
    mocks.updateBetaPolicySettings.mockImplementation(async (value) => value)
    let controller!: ReturnType<typeof useBetaPolicyCard>
    mount(defineComponent({
      setup() {
        controller = useBetaPolicyCard()
        return () => h(BetaPolicyCard, { controller })
      }
    }))
    await controller.loadBetaPolicySettings()
    await controller.saveBetaPolicySettings()
    expect(mocks.updateBetaPolicySettings).toHaveBeenCalledWith({ rules: [{
      beta_token: 'test-beta', action: 'pass', scope: 'all', error_message: undefined,
      model_whitelist: undefined, fallback_action: undefined, fallback_error_message: undefined
    }] })
    expect(controller.betaPolicySaving).toBe(false)
  })
})
