import { afterEach, describe, expect, it } from 'vitest'
import { enableAutoUnmount, mount } from '@vue/test-utils'
import BaseDialog from '../BaseDialog.vue'

enableAutoUnmount(afterEach)
const pressEscape = () => document.dispatchEvent(new KeyboardEvent('keydown', { key: 'Escape' }))

describe('stacked dialog Escape handling', () => {
  it('keeps the body locked until the last open dialog closes or unmounts', async () => {
    const parent = mount(BaseDialog, { props: { show: true, title: 'Parent' } })
    const child = mount(BaseDialog, { props: { show: true, title: 'Child' } })
    const hidden = mount(BaseDialog, { props: { show: false, title: 'Hidden' } })
    expect(document.body.classList.contains('modal-open')).toBe(true)
    hidden.unmount()
    child.unmount()
    expect(document.body.classList.contains('modal-open')).toBe(true)
    await parent.setProps({ show: false })
    expect(document.body.classList.contains('modal-open')).toBe(false)
  })
  it('closes only the most recently opened dialog, then allows the parent to close', async () => {
    const parent = mount(BaseDialog, { props: { show: true, title: 'Parent' } })
    const child = mount(BaseDialog, { props: { show: true, title: 'Child' } })
    pressEscape()
    expect(child.emitted('close')).toHaveLength(1)
    expect(parent.emitted('close')).toBeUndefined()
    await child.setProps({ show: false })
    pressEscape()
    expect(parent.emitted('close')).toHaveLength(1)
  })

  it('does not dismiss a parent behind a child that disallows Escape', () => {
    const parent = mount(BaseDialog, { props: { show: true, title: 'Parent' } })
    const child = mount(BaseDialog, { props: { show: true, title: 'Child', closeOnEscape: false } })
    pressEscape()
    expect(child.emitted('close')).toBeUndefined()
    expect(parent.emitted('close')).toBeUndefined()
    child.unmount()
    pressEscape()
    expect(parent.emitted('close')).toHaveLength(1)
  })

  it('uses z-index before open order and follows changes to the rendered z-index', async () => {
    const higher = mount(BaseDialog, { props: { show: true, title: 'Higher', zIndex: 200 } })
    const later = mount(BaseDialog, { props: { show: true, title: 'Later', zIndex: 50 } })
    pressEscape()
    expect(higher.emitted('close')).toHaveLength(1)
    expect(later.emitted('close')).toBeUndefined()
    await later.setProps({ zIndex: 210 })
    pressEscape()
    expect(later.emitted('close')).toHaveLength(1)
    expect(higher.emitted('close')).toHaveLength(1)
    later.unmount()
    pressEscape()
    expect(higher.emitted('close')).toHaveLength(2)
  })
})
