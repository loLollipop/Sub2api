import { afterEach, describe, expect, it, vi } from 'vitest'
import { mount } from '@vue/test-utils'
import { nextTick } from 'vue'
import HelpTooltip from '@/components/common/HelpTooltip.vue'
import BaseDialog from '@/components/common/BaseDialog.vue'
import { h } from 'vue'

function getTooltipElement(): HTMLDivElement {
  const tooltip = document.body.querySelector('[role="tooltip"]')
  if (!(tooltip instanceof HTMLDivElement)) {
    throw new Error('tooltip element not found')
  }
  return tooltip
}

describe('HelpTooltip', () => {
  afterEach(() => {
    vi.restoreAllMocks()
    document.body.innerHTML = ''
  })

  it('uses viewport coordinates after page scrolling and flips below a trigger near the top', async () => {
    const wrapper = mount(HelpTooltip, { attachTo: document.body, props: { content: 'Details' } })
    const trigger = wrapper.get('.group')
    const tooltip = getTooltipElement()
    vi.spyOn(window, 'scrollX', 'get').mockReturnValue(400)
    vi.spyOn(window, 'scrollY', 'get').mockReturnValue(900)
    vi.spyOn(trigger.element, 'getBoundingClientRect').mockReturnValue(new DOMRect(100, 20, 16, 16))
    vi.spyOn(tooltip, 'getBoundingClientRect').mockReturnValue(new DOMRect(0, 0, 200, 80))
    await trigger.trigger('mouseenter')
    await nextTick()
    expect(tooltip.style.top).toBe('44px')
    expect(tooltip.style.left).toBe('8px')
    expect(tooltip.classList.contains('before:bottom-full')).toBe(true)
    vi.spyOn(trigger.element, 'getBoundingClientRect').mockReturnValue(new DOMRect(500, 300, 16, 16))
    window.dispatchEvent(new Event('scroll'))
    await nextTick()
    expect(tooltip.style.top).toBe('212px')
    expect(tooltip.style.left).toBe('408px')
    wrapper.unmount()
  })

  it('clamps wide content to both viewport edges and scrolls tall content on the larger side', async () => {
    vi.spyOn(window, 'innerWidth', 'get').mockReturnValue(320)
    vi.spyOn(window, 'innerHeight', 'get').mockReturnValue(180)
    const wrapper = mount(HelpTooltip, { attachTo: document.body, props: { content: 'Long details' } })
    const trigger = wrapper.get('.group')
    const tooltip = getTooltipElement()
    vi.spyOn(trigger.element, 'getBoundingClientRect').mockReturnValue(new DOMRect(300, 80, 16, 16))
    vi.spyOn(tooltip, 'getBoundingClientRect').mockReturnValue(new DOMRect(0, 0, 600, 500))
    await trigger.trigger('mouseenter')
    await nextTick()
    expect(tooltip.style.left).toBe('8px')
    expect(tooltip.style.maxWidth).toBe('304px')
    expect(tooltip.style.top).toBe('104px')
    const content = tooltip.firstElementChild as HTMLElement
    expect(content.style.maxHeight).toBe('68px')
    expect(content.classList.contains('overflow-y-auto')).toBe(true)
    wrapper.unmount()
  })

  it('opens on keyboard focus, dismisses with Escape, and toggles click mode with Space', async () => {
    const wrapper = mount(HelpTooltip, { attachTo: document.body, props: { content: 'Keyboard help' } })
    const trigger = wrapper.get('.group')
    const tooltip = getTooltipElement()
    expect(trigger.attributes('tabindex')).toBe('0')
    ;(trigger.element as HTMLElement).focus()
    await nextTick()
    expect(tooltip.style.display).not.toBe('none')
    expect(trigger.attributes('aria-describedby')).toBe(tooltip.id)
    await trigger.trigger('keydown', { key: 'Escape' })
    expect(tooltip.style.display).toBe('none')
    await wrapper.setProps({ trigger: 'click' })
    await trigger.trigger('keydown', { key: ' ' })
    expect(tooltip.style.display).not.toBe('none')
    const close = tooltip.querySelector('button')!
    close.focus()
    close.dispatchEvent(new KeyboardEvent('keydown', { key: 'Escape', bubbles: true }))
    await nextTick()
    expect(tooltip.style.display).toBe('none')
    expect(document.activeElement).toBe(trigger.element)
    wrapper.unmount()
  })

  it('preserves a custom button trigger without adding a second Tab stop', async () => {
    const wrapper = mount(HelpTooltip, {
      attachTo: document.body,
      props: { trigger: 'click', content: 'Help' },
      slots: { trigger: '<button type="button">Custom help</button>' },
    })
    await nextTick()
    expect(wrapper.get('.group').attributes('tabindex')).toBeUndefined()
    await wrapper.get('button').trigger('click')
    expect(getTooltipElement().style.display).not.toBe('none')
    expect(wrapper.get('button').attributes('aria-describedby')).toBe(getTooltipElement().id)
    await wrapper.get('button').trigger('click')
    expect(wrapper.get('button').attributes('aria-describedby')).toBeUndefined()
    wrapper.unmount()
  })

  it('consumes a hover tooltip Escape before the containing dialog and window handlers', async () => {
    const escapeWindow = vi.fn()
    window.addEventListener('keydown', escapeWindow)
    const wrapper = mount(BaseDialog, {
      attachTo: document.body,
      props: { show: true, title: 'Details' },
      slots: { default: () => h(HelpTooltip, { content: 'Help' }) },
    })
    await nextTick()
    await wrapper.findComponent(HelpTooltip).get('.group').trigger('mouseenter')
    await nextTick()
    document.activeElement?.dispatchEvent(new KeyboardEvent('keydown', { key: 'Escape', bubbles: true, cancelable: true }))
    await nextTick()
    expect(getTooltipElement().style.display).toBe('none')
    expect(wrapper.emitted('close')).toBeUndefined()
    expect(escapeWindow).not.toHaveBeenCalled()
    document.activeElement?.dispatchEvent(new KeyboardEvent('keydown', { key: 'Escape', bubbles: true, cancelable: true }))
    expect(wrapper.emitted('close')).toHaveLength(1)
    expect(escapeWindow).not.toHaveBeenCalled()
    window.removeEventListener('keydown', escapeWindow)
    wrapper.unmount()
  })

  it('keeps the existing hover interaction by default', async () => {
    const wrapper = mount(HelpTooltip, {
      attachTo: document.body,
      props: {
        content: 'hover details',
      },
    })

    const trigger = wrapper.get('.group')
    const tooltip = getTooltipElement()

    expect(tooltip.style.display).toBe('none')

    await trigger.trigger('mouseenter')
    await nextTick()
    expect(tooltip.style.display).not.toBe('none')

    await trigger.trigger('mouseleave')
    await nextTick()
    expect(tooltip.style.display).toBe('none')

    wrapper.unmount()
  })

  it('keeps a hover tooltip open while the pointer moves between the trigger and the tooltip', async () => {
    const wrapper = mount(HelpTooltip, {
      attachTo: document.body,
      props: {
        content: 'copyable details',
      },
    })

    const trigger = wrapper.get('.group')
    const tooltip = getTooltipElement()

    await trigger.trigger('mouseenter')
    await nextTick()
    expect(tooltip.style.display).not.toBe('none')

    await trigger.trigger('mouseleave', { relatedTarget: tooltip })
    await nextTick()
    expect(tooltip.style.display).not.toBe('none')

    tooltip.dispatchEvent(new MouseEvent('mouseleave', { relatedTarget: trigger.element }))
    await nextTick()
    expect(tooltip.style.display).not.toBe('none')

    tooltip.dispatchEvent(new MouseEvent('mouseleave', { relatedTarget: null }))
    await nextTick()
    expect(tooltip.style.display).toBe('none')

    wrapper.unmount()
  })

  it('supports click-to-toggle details and closes on outside click', async () => {
    const wrapper = mount(HelpTooltip, {
      attachTo: document.body,
      props: {
        content: 'click details',
        trigger: 'click',
      },
    })

    const trigger = wrapper.get('.group')
    const tooltip = getTooltipElement()

    expect(tooltip.style.display).toBe('none')

    await trigger.trigger('click')
    await nextTick()
    expect(tooltip.style.display).not.toBe('none')
    expect(tooltip.textContent).toContain('click details')

    const closeButton = tooltip.querySelector('button[aria-label="Close"]')
    if (!(closeButton instanceof HTMLButtonElement)) {
      throw new Error('close button not found')
    }
    closeButton.click()
    await nextTick()
    expect(tooltip.style.display).toBe('none')

    await trigger.trigger('click')
    await nextTick()
    expect(tooltip.style.display).not.toBe('none')

    document.body.dispatchEvent(new MouseEvent('click', { bubbles: true }))
    await nextTick()
    expect(tooltip.style.display).toBe('none')

    wrapper.unmount()
  })
})
