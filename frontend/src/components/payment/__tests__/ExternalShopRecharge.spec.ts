import { afterEach, describe, expect, it, vi } from 'vitest'
import { mount } from '@vue/test-utils'
import ExternalShopRecharge from '../ExternalShopRecharge.vue'

vi.mock('vue-i18n', () => ({ useI18n: () => ({ t: (key: string) => key }) }))

const tiers = [10, 20, 30, 50, 100]
const products = Object.fromEntries(tiers.map(tier => [String(tier), `https://shop.example.test/item/${tier}?source=site#details`]))
function mountShop(config: Record<string, string> = products) {
  return mount(ExternalShopRecharge, { props: { products: config, username: 'buyer', balance: 12.34 },
    global: { stubs: { Icon: true, RouterLink: { template: '<a :href="to"><slot /></a>', props: ['to'] } } } })
}

afterEach(() => vi.restoreAllMocks())

describe('ExternalShopRecharge', () => {
  it('requires a fixed tier, opens each original product URL synchronously and offers redemption', async () => {
    const open = vi.spyOn(window, 'open').mockReturnValue(null)
    const wrapper = mountShop()
    const purchase = wrapper.get('.shop-summary button')
    expect(purchase.attributes('disabled')).toBeDefined()
    expect(wrapper.find('input').exists()).toBe(false)
    expect(wrapper.findAll('.shop-tier').map(button => button.text())).toEqual(['¥10', '¥20', '¥30', '¥50', '¥100'])
    await purchase.trigger('click')
    expect(open).not.toHaveBeenCalled()
    for (const [index, tier] of tiers.entries()) {
      await wrapper.findAll('.shop-tier')[index].trigger('click')
      expect(wrapper.findAll('.shop-tier')[index].attributes('aria-pressed')).toBe('true')
      expect(wrapper.get('.shop-reference').text()).toContain(`¥${tier}.00`)
      expect(purchase.attributes('disabled')).toBeUndefined()
      purchase.element.dispatchEvent(new MouseEvent('click', { bubbles: true }))
      expect(open).toHaveBeenLastCalledWith(products[String(tier)], '_blank', 'noopener,noreferrer')
    }
    expect(open).toHaveBeenCalledTimes(5)
    expect(wrapper.get('[href="/redeem"]').exists()).toBe(true)
    expect(wrapper.text()).toContain('$12.34')
    expect(wrapper.props('balance')).toBe(12.34)
    expect(wrapper.emitted('update:balance')).toBeUndefined()
    wrapper.unmount()
  })

  it('disables unconfigured tiers while keeping configured products selectable', async () => {
    const wrapper = mountShop({ '20': products['20'] || '' })
    expect(wrapper.findAll('.shop-tier').map(button => button.attributes('disabled') !== undefined)).toEqual([true, false, true, true, true])
    expect(wrapper.get('.shop-tier').text()).toContain('purchase.tierNotConfigured')
    expect(wrapper.find('[role="status"]').exists()).toBe(false)
    await wrapper.findAll('.shop-tier')[1].trigger('click')
    expect(wrapper.get('.shop-summary button').attributes('disabled')).toBeUndefined()
    wrapper.unmount()
  })

  it.each(['', 'javascript:alert(1)', 'data:text/html,test', '/item/fixed', '//shop.example.test/item', 'https://user:secret@shop.example.test', 'https://shop.example.test:99999/item', 'https://shop.example.test:0/item', 'https://shop.example.test:/item', 'https://bad..host/item', 'https://127.1/item', 'https://010.0.0.1/item', 'https://shop.example.test/\\evil', 'https://shop.example.test/\nitem', 'not a url'])('disables unsafe or missing product URL %s', async url => {
    const open = vi.spyOn(window, 'open').mockReturnValue(null)
    const wrapper = mountShop({ '10': url })
    expect(wrapper.get('.shop-tier').attributes('disabled')).toBeDefined()
    await wrapper.get('.shop-tier').trigger('click')
    expect(wrapper.get('.shop-summary button').attributes('disabled')).toBeDefined()
    expect(wrapper.get('[role="status"]').text()).toBe('purchase.invalidShopUrl')
    await wrapper.get('.shop-summary button').trigger('click')
    expect(open).not.toHaveBeenCalled()
    expect(wrapper.get('[href="/redeem"]').exists()).toBe(true)
    wrapper.unmount()
  })

  it.each([{}, { '10': 'javascript:alert(1)' }, { '10': 'https://shop.example.test/replaced' }])('requires a fresh selection after a selected product changes', async changed => {
    const open = vi.spyOn(window, 'open').mockReturnValue(null)
    const wrapper = mountShop()
    await wrapper.get('.shop-tier').trigger('click')
    await wrapper.setProps({ products: changed })
    expect(wrapper.get('.shop-summary button').attributes('disabled')).toBeDefined()
    await wrapper.get('.shop-summary button').trigger('click')
    expect(open).not.toHaveBeenCalled()
    wrapper.unmount()
  })

  it('supports a configured http URL without adding identity or amount', async () => {
    const open = vi.spyOn(window, 'open').mockReturnValue(null)
    const wrapper = mountShop({ '10': 'http://shop.example.test/product' })
    await wrapper.get('.shop-tier').trigger('click')
    await wrapper.get('.shop-summary button').trigger('click')
    expect(open).toHaveBeenCalledWith('http://shop.example.test/product', '_blank', 'noopener,noreferrer')
    wrapper.unmount()
  })
})
