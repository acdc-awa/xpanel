import { formatDateOnly } from '@/utils/timezone'

export interface ExpiryStatus {
  hasExpiry: boolean
  isExpired: boolean
  isUrgent: boolean // 已过期或 <= 7 天进入紧急待续费
  cls: 'red' | 'deep-orange' | 'yellow' | 'orange' | 'gray'
  color: string // 强调文字色
  text: string
  tip: string
  remainingDays: number
}

// 到期日是纯日历日（YYYY-MM-DD，后端也只存这个形态）。续费日是"哪一天"而不是某一瞬，
// 所以除了"今天算哪天"这一步，全程不做时区换算——一旦把日期当时刻换算，它就会漂成前后一天。
const DATE_RE = /^(\d{4})-(\d{2})-(\d{2})/

/** 取日期的年月日；非日期返回 null。容忍历史/异常值里的时间后缀（只取日期部分）。 */
function dateParts(v?: string | null): [number, number, number] | null {
  if (!v) return null
  const m = DATE_RE.exec(v.trim())
  if (!m) return null
  const y = Number(m[1])
  const mo = Number(m[2])
  const d = Number(m[3])
  if (mo < 1 || mo > 12 || d < 1 || d > 31) return null
  return [y, mo, d]
}

/** 今天（面板展示时区的日历日）。"今天算哪一天"是唯一该带时区的一步。 */
function todayParts(): [number, number, number] {
  const [y, m, d] = formatDateOnly(new Date()).split('-').map(Number)
  return [y, m, d]
}

/** 两个日历日的整天差（借 UTC 做中性日历减法，不含时区语义）。 */
function dayDiff(target: [number, number, number], today: [number, number, number]): number {
  const t = Date.UTC(target[0], target[1] - 1, target[2])
  const n = Date.UTC(today[0], today[1] - 1, today[2])
  return Math.round((t - n) / (24 * 3600 * 1000))
}

/** 到期日展示文本：原样输出日期，不做时区换算。 */
export function formatExpireDate(v?: string | null): string {
  const p = dateParts(v)
  if (!p) return '—'
  return `${p[0]}-${String(p[1]).padStart(2, '0')}-${String(p[2]).padStart(2, '0')}`
}

/**
 * 计算 VPS 到期状态（纯日历日比较，不做时区换算）。
 * 规则：
 * - 已过期（diffDays < 0）：红色（red）
 * - 一天内（diffDays === 0 或 1）：深橙色（deep-orange）
 * - 3 天内（diffDays === 2 或 3）：黄色（yellow）
 * - 过期前 7 天（4 <= diffDays <= 7）：常规预警橙色（orange）
 * - 超过 7 天（diffDays > 7）：灰色（gray）
 */
export function getExpiryStatus(expireDate?: string | null): ExpiryStatus | null {
  const target = dateParts(expireDate)
  if (!target) return null
  const diffDays = dayDiff(target, todayParts())
  const dateStr = formatExpireDate(expireDate)

  // 1. 已过期
  if (diffDays < 0) {
    const overdue = -diffDays
    return {
      hasExpiry: true,
      isExpired: true,
      isUrgent: true,
      cls: 'red',
      color: 'var(--el-color-danger, #ef4444)',
      text: `已过期 ${overdue} 天`,
      tip: `已于 ${dateStr} 到期（逾期 ${overdue} 天），请尽快续费或确认是否保留`,
      remainingDays: diffDays,
    }
  }

  // 2. 今日到期
  if (diffDays === 0) {
    return {
      hasExpiry: true,
      isExpired: false,
      isUrgent: true,
      cls: 'deep-orange',
      color: '#ea580c',
      text: '今日到期',
      tip: `将于今日（${dateStr}）到期，请尽快续费`,
      remainingDays: 0,
    }
  }

  // 3. 明日到期
  if (diffDays === 1) {
    return {
      hasExpiry: true,
      isExpired: false,
      isUrgent: true,
      cls: 'deep-orange',
      color: '#ea580c',
      text: '1 天后到期',
      tip: `将于明日（${dateStr}）到期（剩余 1 天），请及时续费`,
      remainingDays: 1,
    }
  }

  // 4. 3 天内（2~3 天）——黄色
  if (diffDays <= 3) {
    return {
      hasExpiry: true,
      isExpired: false,
      isUrgent: true,
      cls: 'yellow',
      color: '#d97706',
      text: `${diffDays} 天后到期`,
      tip: `将于 ${dateStr} 到期（剩余 ${diffDays} 天），请注意续费`,
      remainingDays: diffDays,
    }
  }

  // 5. 过期前 7 天（4~7 天）——橙色预警
  if (diffDays <= 7) {
    return {
      hasExpiry: true,
      isExpired: false,
      isUrgent: true,
      cls: 'orange',
      color: 'var(--x-warning, #f59e0b)',
      text: `${diffDays} 天后到期`,
      tip: `将于 ${dateStr} 到期（剩余 ${diffDays} 天），请及时续费`,
      remainingDays: diffDays,
    }
  }

  // 6. > 7 天——灰色正常
  return {
    hasExpiry: true,
    isExpired: false,
    isUrgent: false,
    cls: 'gray',
    color: 'var(--x-text-2, #64748b)',
    text: `剩余 ${diffDays} 天`,
    tip: `到期日：${dateStr}（剩余 ${diffDays} 天）`,
    remainingDays: diffDays,
  }
}

/** 规范化网址（自动补齐 https:// 前缀） */
export function normalizeUrl(url?: string): string {
  if (!url) return ''
  const trimmed = url.trim()
  if (!trimmed) return ''
  if (/^https?:\/\//i.test(trimmed)) {
    return trimmed
  }
  return `https://${trimmed}`
}

/** 判断字符串是否为网址/域名 */
export function isUrl(text?: string): boolean {
  if (!text) return false
  const t = text.trim()
  return /^https?:\/\//i.test(t) || /^[a-zA-Z0-9.-]+\.[a-zA-Z]{2,}(\/.*)?$/i.test(t)
}
