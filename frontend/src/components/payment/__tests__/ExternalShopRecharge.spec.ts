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
  it('keeps optional help in collapsed native disclosures while purchase and redemption remain exposed', () => {
    const wrapper = mountShop()
    for (const [selector, title] of [['.shop-faq', 'purchase.faqTitle'], ['.shop-reminders', 'purchase.remindersTitle']] as const) {
      const disclosure = wrapper.get(selector)
      expect(disclosure.element.tagName).toBe('DETAILS')
      expect(disclosure.attributes('open')).toBeUndefined()
      expect(disclosure.get('summary').text()).toBe(title)
      expect(disclosure.get('summary').attributes('tabindex')).not.toBe('-1')
    }
    expect(wrapper.get('.shop-summary button').exists()).toBe(true)
    expect(wrapper.get('[href="/redeem"]').exists()).toBe(true)
    wrapper.unmount()
  })

  it('shows an empty purchase summary, separate USD balance and the complete redemption journey', () => {
    const wrapper = mountShop()
    expect(wrapper.get('h1').text()).toBe('purchase.title')
    expect(wrapper.find('.shop-channel').exists()).toBe(false)
    expect(wrapper.find('.shop-heading a').exists()).toBe(false)
    expect(wrapper.get('.shop-reference').text()).toContain('purchase.chooseAmount')
    expect(wrapper.get('.shop-reference').text()).not.toContain('¥0.00')
    expect(wrapper.get('.shop-reference').attributes('aria-live')).toBe('polite')
    expect(wrapper.get('.shop-balance').text()).toContain('purchase.balanceUsd')
    expect(wrapper.get('.shop-balance').text()).toContain('$12.34')
    expect(wrapper.get('.shop-account__name strong').text()).toBe('buyer')
    expect(wrapper.get('.shop-tiers').attributes('aria-label')).toBe('purchase.referenceAmount')
    expect(wrapper.findAll('.shop-tier').every(button => button.attributes('aria-pressed') === 'false')).toBe(true)
    expect(wrapper.findAll('.shop-guide li strong').map(step => step.text())).toEqual([
      'purchase.stepSelectTitle', 'purchase.stepBuyTitle', 'purchase.stepRedeemTitle',
    ])
    expect(wrapper.get('.shop-value-hint').text()).toBe('purchase.codeValueHint')
    expect(wrapper.get('.shop-purchase-hint').text()).toBe('purchase.redemptionRequired')
    expect(wrapper.findAll('.shop-order-details dt').map(label => label.text())).toEqual([
      'purchase.productType', 'purchase.creditMethod',
    ])
    expect(wrapper.findAll('.shop-order-details dd').map(value => value.text())).toEqual([
      'purchase.codeProduct', 'purchase.manualRedemption',
    ])
    expect(wrapper.findAll('.shop-faq dt').map(question => question.text())).toEqual([
      'purchase.faqCreditQuestion', 'purchase.faqValueQuestion',
    ])
    expect(wrapper.findAll('.shop-reminders li').map(reminder => reminder.text())).toEqual([
      'purchase.reminderProduct', 'purchase.reminderAccount', 'purchase.reminderCode',
    ])
    expect(wrapper.findAll('.shop-summary button')).toHaveLength(1)
    expect(wrapper.get('[href="/redeem"]').text()).toContain('purchase.redeemCode')
    wrapper.unmount()
  })

  it('exposes exactly one selected tier and an aria-hidden selection indicator', async () => {
    const wrapper = mountShop()
    await wrapper.findAll('.shop-tier')[0].trigger('click')
    await wrapper.findAll('.shop-tier')[4].trigger('click')
    expect(wrapper.findAll('.shop-tier').map(button => button.attributes('aria-pressed'))).toEqual(['false', 'false', 'false', 'false', 'true'])
    expect(wrapper.findAll('.shop-tier--selected')).toHaveLength(1)
    expect(wrapper.get('.shop-tier__check').attributes('aria-hidden')).toBe('true')
    expect(wrapper.get('.shop-reference').text()).toContain('¥100.00')
    expect(wrapper.find('.shop-reference--empty').exists()).toBe(false)
    wrapper.unmount()
  })

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

  it.each([{}, { '10': 'javascript:alert(1)' }, { '10': 'https://shop.example.test/replaced' }])('requires a fresh selection after a selected product changes', async (changed: Record<string, string>) => {
    const open = vi.spyOn(window, 'open').mockReturnValue(null)
    const wrapper = mountShop()
    await wrapper.get('.shop-tier').trigger('click')
    await wrapper.setProps({ products: changed })
    expect(wrapper.get('.shop-reference--empty').text()).toBe('purchase.chooseAmount')
    expect(wrapper.findAll('.shop-tier').every(button => button.attributes('aria-pressed') === 'false')).toBe(true)
    expect(wrapper.get('.shop-summary button').attributes('disabled')).toBeDefined()
    await wrapper.get('.shop-summary button').trigger('click')
    expect(open).not.toHaveBeenCalled()
    if (changed['10']?.startsWith('https://')) {
      await wrapper.get('.shop-tier').trigger('click')
      await wrapper.get('.shop-summary button').trigger('click')
      expect(open).toHaveBeenCalledWith(changed['10'], '_blank', 'noopener,noreferrer')
    }
    wrapper.unmount()
  })

  it('keeps the selected product when only an unrelated tier is reconfigured', async () => {
    const wrapper = mountShop()
    await wrapper.get('.shop-tier').trigger('click')
    await wrapper.setProps({ products: { ...products, '20': 'https://shop.example.test/another-product' } })
    expect(wrapper.get('.shop-tier').attributes('aria-pressed')).toBe('true')
    expect(wrapper.get('.shop-summary button').attributes('disabled')).toBeUndefined()
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
