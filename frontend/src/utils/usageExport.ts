interface ExportRow {
  id: number
  inflight?: boolean
}

interface ExportPage<T> {
  items: T[]
  total: number
  page?: number
  page_size?: number
}

interface ExportOptions<T> {
  pageSize?: number
  onWaiting?: (waiting: boolean) => void
  onRestart?: () => void
  onPage: (items: T[], total: number, current: number) => void
}

const RETRY_BUDGET_MS = 15 * 60_000

export class UsageExportChangedError extends Error {
  constructor() {
    super('Usage records changed during export')
    this.name = 'UsageExportChangedError'
  }
}

export function isUsageExportRateLimited(error: unknown): error is { status: 429; retryAfter?: string } {
  return typeof error === 'object' && error !== null && 'status' in error && error.status === 429
}

function retryDelay(retryAfter: unknown): number {
  if (typeof retryAfter !== 'string' || !retryAfter.trim()) return 60_000
  const raw = retryAfter.trim()
  const delay = /^\d+$/.test(raw) ? Number(raw) * 1000 : Date.parse(raw) - Date.now()
  return Number.isNaN(delay) ? 60_000 : Math.max(1000, delay)
}

function waitForRetry(ms: number, signal: AbortSignal): Promise<void> {
  signal.throwIfAborted()
  return new Promise((resolve, reject) => {
    const timer = setTimeout(() => {
      signal.removeEventListener('abort', abort)
      resolve()
    }, ms)
    function abort() {
      clearTimeout(timer)
      reject(signal.reason)
    }
    signal.addEventListener('abort', abort, { once: true })
  })
}

// Visit validated pages so large XLSX exports do not retain a second copy of every row.
export async function forEachUsageExportPage<T extends ExportRow>(
  fetchPage: (page: number, pageSize: number) => Promise<ExportPage<T>>,
  signal: AbortSignal,
  options: ExportOptions<T>,
): Promise<void> {
  let waitBudget = RETRY_BUDGET_MS
  async function fetchWithRetry(page: number, pageSize: number) {
    for (let attempt = 0; ; attempt++) {
      signal.throwIfAborted()
      try {
        const result = await fetchPage(page, pageSize)
        signal.throwIfAborted()
        return result
      } catch (error) {
        signal.throwIfAborted()
        const status = (error as { status?: number } | null)?.status
        const limited = isUsageExportRateLimited(error)
        if (attempt >= 2 || !(limited || status === 0 || status === 408 || (status != null && status >= 500 && status < 600))) throw error
        const delay = limited ? retryDelay(error.retryAfter) : 400 * (attempt + 1)
        if (delay > waitBudget) throw error
        waitBudget -= delay
        if (limited) options.onWaiting?.(true)
        try {
          await waitForRetry(delay, signal)
        } finally {
          if (limited) options.onWaiting?.(false)
        }
      }
    }
  }

  // OFFSET paging has no snapshot: restart once on observable drift, then fail without a download.
  for (let run = 0; ; run++) {
    let size = options.pageSize ?? 100
    let total = 0
    let current = 0
    const ids = new Set<number>()
    try {
      for (let page = 1; ; page++) {
        const result = await fetchWithRetry(page, size)
        if (!Array.isArray(result?.items) || !Number.isSafeInteger(result.total) || result.total < 0 ||
            (result.page != null && result.page !== page) ||
            (result.page_size != null && (!Number.isSafeInteger(result.page_size) || result.page_size < 1 || result.page_size > size))) {
          throw new Error('Invalid usage export page')
        }
        const items = result.items.filter(item => !item?.inflight)
        if (page === 1) {
          total = result.total
          size = result.page_size ?? (items.length > 0 && items.length < size ? items.length : size)
        }
        const expected = Math.max(0, Math.min(size, total - current))
        if (result.total !== total || (result.page_size != null && result.page_size !== size) || items.length !== expected) {
          throw new UsageExportChangedError()
        }
        for (const row of items) {
          if (!Number.isSafeInteger(row?.id) || row.id <= 0) throw new Error('Invalid usage export row')
          if (ids.has(row.id)) throw new UsageExportChangedError()
          ids.add(row.id)
        }
        current += items.length
        options.onPage(items, total, current)
        if (current >= total) return
      }
    } catch (error) {
      if (!(error instanceof UsageExportChangedError) || run !== 0) throw error
      signal.throwIfAborted()
      options.onRestart?.()
    }
  }
}

export async function collectUsageForExport<T extends ExportRow>(
  fetchPage: (page: number, pageSize: number) => Promise<ExportPage<T>>,
  signal: AbortSignal,
  onWaiting: (waiting: boolean) => void,
): Promise<T[]> {
  const items: T[] = []
  await forEachUsageExportPage(fetchPage, signal, {
    onWaiting,
    onRestart: () => { items.length = 0 },
    onPage: page => { items.push(...page) },
  })
  return items
}
