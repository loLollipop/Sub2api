import { computed } from 'vue'
import { useI18n } from 'vue-i18n'
import { formatScaled, resolveIntervalPrices } from '@/utils/pricing'
import { BILLING_MODE_TOKEN, BILLING_MODE_IMAGE, BILLING_MODE_VIDEO, type BillingMode } from '@/constants/channel'
import type { PlazaModel, PlazaTimePricingPeriod } from '@/api/modelPlaza'
import type { UserPricingInterval } from '@/api/channels'

export interface ModelPlazaPricingProps {
  models: PlazaModel[]
  /** 分组平台;实付分区底色随平台着色,未知平台回退品牌青。 */
  platform?: string
  /** 分组默认倍率。 */
  rateMultiplier: number
  /** 用户专属倍率;覆盖每行默认倍率，与该行不同则划线展示原倍率。 */
  userRateMultiplier?: number | null
  /** 生图独立倍率:true 时图片计费模型的实付倍率取 imageRateMultiplier,不取分组/专属倍率。 */
  imageRateIndependent?: boolean
  imageRateMultiplier?: number | null
  /**
   * 高峰窗口描述(含倍率与服务器时区标注),空串/缺省 = 分组未启用高峰。
   * 表格所有价格均为不含高峰因子的口径,该窗口用于分时标签的 tooltip 披露:
   * 与高峰重叠的部分实付还会再乘高峰倍率。
   */
  peakWindow?: string
  peakRateMultiplier?: number | null
}

/** Shared display calculations for the overview and full pricing breakdown. */
export function useModelPlazaPricing(props: ModelPlazaPricingProps) {
  const { t } = useI18n()
  const PER_MILLION = 1_000_000

  /**
   * 展示顺序:
   * 1. token 计费的排在前,按图/按次计费的沉到末尾——它们的参考 token 价与实付的按张/按次价不同量纲,混排无意义;
   * 2. 组内按参考输出价从高到低,无参考价的排最后;
   * 3. 同价按名称降序(新版本号在前,如 gpt-5.6 先于 gpt-5.5)。
   */
  const sortedModels = computed(() => {
    return [...props.models].sort((a, b) => {
      const ta = billingMode(a) === BILLING_MODE_TOKEN
      const tb = billingMode(b) === BILLING_MODE_TOKEN
      if (ta !== tb) return ta ? -1 : 1
      const pa = a.official_pricing?.output_price ?? null
      const pb = b.official_pricing?.output_price ?? null
      if (pa != null && pb != null && pa !== pb) return pb - pa
      if (pa != null && pb == null) return -1
      if (pa == null && pb != null) return 1
      return b.name.localeCompare(a.name)
    })
  })

  const effectiveRate = computed(() => props.userRateMultiplier ?? props.rateMultiplier)
  const hasCustomRate = computed(
    () => props.userRateMultiplier != null && props.userRateMultiplier !== props.rateMultiplier
  )

  function billingMode(m: PlazaModel): BillingMode {
    return (m.pricing?.billing_mode || BILLING_MODE_TOKEN) as BillingMode
  }

  function billingModeLabel(m: PlazaModel): string {
    if (billingMode(m) === BILLING_MODE_VIDEO) return t('modelPlaza.list.videoBilling')
    return billingMode(m) === BILLING_MODE_IMAGE
      ? t('modelPlaza.table.perImage')
      : t('modelPlaza.table.perRequest')
  }

  /** 价格统一保底 2 位小数,更长的有效小数原样保留。 */
  const MIN_DECIMALS = 2

  interface TimeGroup {
    multiplier: number
    rate: number
    windows: string[]
  }

  /** Same-rate windows share one label; preserve small nonzero multipliers. */
  function timeGroups(m: PlazaModel): TimeGroup[] {
    const groups = new Map<number, TimeGroup>()
    for (const period of timePeriods(m)) {
      let group = groups.get(period.multiplier)
      if (!group) {
        group = { multiplier: period.multiplier, rate: Number((requestRate(m) * period.multiplier).toPrecision(12)), windows: [] }
        groups.set(period.multiplier, group)
      }
      group.windows.push(formatTimeWindow(period))
    }
    return [...groups.values()]
  }

  /** 实付价 = 渠道单价 × 生效倍率(分时倍率在标签中展示),按 $/1M token 展示。 */
  function paidPerMillion(_model: PlazaModel, value: number | null | undefined): string {
    if (value == null) return '-'
    const rate = effectiveRate.value
    return formatScaled(value * rate, PER_MILLION, MIN_DECIMALS)
  }

  /** 图片计费模型且分组开启生图独立倍率:实付倍率取独立倍率,与计费口径一致。 */
  function usesIndependentImageRate(m: PlazaModel): boolean {
    return billingMode(m) === BILLING_MODE_IMAGE && props.imageRateIndependent === true
  }

  /** 按次/按图片行的生效倍率。 */
  function requestRate(m: PlazaModel): number {
    return usesIndependentImageRate(m) ? (props.imageRateMultiplier ?? 1) : effectiveRate.value
  }

  /** 按次 / 按图片单价(乘该行生效倍率,不换算 1M)。 */
  function paidRequestPrice(m: PlazaModel, value: number | null | undefined): string {
    // The plaza API does not expose the resolved per-model video prices and
    // independent video rate. Never label a channel fallback as an actual quote.
    if (billingMode(m) === BILLING_MODE_VIDEO) return '-'
    if (value == null) return '-'
    return formatScaled(value * requestRate(m), 1, MIN_DECIMALS)
  }

  /** 参考价不乘倍率。 */
  function official(value: number | null | undefined): string {
    if (value == null) return '-'
    return formatScaled(value, PER_MILLION, MIN_DECIMALS)
  }

  /** 非 token 计费的单位后缀:按图片 → “/ 张”,按次 → “/ 次”。 */
  function perUnitSuffix(m: PlazaModel): string {
    if (billingMode(m) === BILLING_MODE_VIDEO) return t('modelPlaza.list.perSecond')
    return billingMode(m) === BILLING_MODE_IMAGE
      ? t('modelPlaza.table.perUnitImage')
      : t('modelPlaza.table.perUnitRequest')
  }

  function hasCachePricing(m: PlazaModel): boolean {
    return m.pricing?.cache_write_price != null || m.pricing?.cache_write_1h_price != null || m.pricing?.cache_read_price != null
  }

  function hasOfficialCache(o: NonNullable<PlazaModel['official_pricing']>): boolean {
    return o.cache_write_price != null || o.cache_read_price != null || o.cache_write_1h_price != null
  }

  /** 分时倍率时段(后端只给出倍率 ≠ 1 的时段,已升序)。 */
  function timePeriods(m: PlazaModel): PlazaTimePricingPeriod[] {
    return m.time_pricing?.periods ?? []
  }

  /**
   * 分时标签 tooltip:仅工作日生效的配置换用带周末回落说明的文案;
   * 分组启用高峰倍率时追加披露——标签倍率不含高峰因子,与高峰窗口重叠的部分实付再乘高峰倍率。
   */
  function timePricingHint(m: PlazaModel, group: TimeGroup): string {
    const key = m.time_pricing?.weekdays_only
      ? 'modelPlaza.table.timePricingHintWeekdays'
      : 'modelPlaza.table.timePricingHint'
    let hint = t(key, { timezone: m.time_pricing?.timezone, rate: group.rate, base: requestRate(m), multiplier: group.multiplier })
    if (props.peakWindow) {
      hint += t('modelPlaza.table.timePricingHintPeak', {
        window: props.peakWindow,
        multiplier: props.peakRateMultiplier ?? 1
      })
    }
    return hint
  }

  /** “00:30–08:30”;整分钟的 HH:mm:ss 省略秒。 */
  function formatTimeWindow(p: PlazaTimePricingPeriod): string {
    const clock = (v: string) => v.replace(/^(\d{2}:\d{2}):00$/, '$1')
    return `${clock(p.start_time)}–${clock(p.end_time)}`
  }

  /** 上下文档位按下限升序展示(后端已升序,此处兜底)。 */
  function sortByContext(intervals: UserPricingInterval[]): UserPricingInterval[] {
    return [...intervals].sort((a, b) => a.min_tokens - b.min_tokens)
  }

  /** token 模式的阶梯定价(内联进输入/输出/缓存列)。 */
  function tokenIntervals(m: PlazaModel): UserPricingInterval[] {
    return sortByContext(m.pricing?.intervals ?? []).map(iv => resolveIntervalPrices(iv, m.pricing!))
  }

  /** 参考价阶梯：配置参考价跟随实际计费阶梯，纯目录参考价保留目录阶梯。 */
  function officialIntervals(m: PlazaModel): UserPricingInterval[] {
    return sortByContext(m.official_pricing?.intervals ?? [])
  }

  /** 任一档带缓存价才按档渲染缓存列;否则沿用平价的写入/读取两行。 */
  function hasTierCachePricing(intervals: UserPricingInterval[]): boolean {
    return intervals.some((iv) =>
      iv.cache_write_price != null || iv.cache_write_1h_price != null || iv.cache_read_price != null ||
      iv.cache_write_multiplier != null || iv.cache_read_multiplier != null
    )
  }

  /** 档位说明:整单按档计价,或(平台旧规则)仅超出部分按档计价。 */
  function tierHint(m: PlazaModel): string {
    return m.long_context_basis === 'marginal'
      ? t('modelPlaza.table.tierHintMarginal')
      : t('modelPlaza.table.tierHint')
  }


  /** 按次/按图模式的阶梯定价(仅保留配了按次价的档位)。 */
  function requestIntervals(m: PlazaModel): UserPricingInterval[] {
    return (m.pricing?.intervals ?? []).filter((iv) => iv.per_request_price != null)
  }

  /**
   * 档位标签:优先后端/管理员给出的 tier_label,否则按区间生成统一形态——
   * 有上限为「≤上限」,末档为「>下限」;档位升序排列,相邻的 ≤100K / ≤200K 即表示 (100K,200K]。
   */
  function tierLabel(iv: UserPricingInterval): string {
    if (iv.tier_label) return iv.tier_label
    const { min_tokens: min, max_tokens: max } = iv
    return max == null ? `>${formatTokenCount(min)}` : `≤${formatTokenCount(max)}`
  }

  function formatTokenCount(n: number): string {
    if (n >= 1_000_000) return `${trimZero(n / 1_000_000)}M`
    if (n >= 1_000) return `${trimZero(n / 1_000)}K`
    return String(n)
  }

  function trimZero(n: number): string {
    return String(Math.round(n * 100) / 100)
  }

  return { sortedModels, effectiveRate, hasCustomRate, billingMode, billingModeLabel, timeGroups, paidPerMillion, usesIndependentImageRate, requestRate, paidRequestPrice, official, perUnitSuffix, hasCachePricing, hasOfficialCache, timePeriods, timePricingHint, formatTimeWindow, tokenIntervals, officialIntervals, hasTierCachePricing, tierHint, requestIntervals, tierLabel }
}
