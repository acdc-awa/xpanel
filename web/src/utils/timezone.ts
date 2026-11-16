import { ref } from 'vue'

/**
 * 展示时区（设置页「时区」组的 display_timezone）。
 * - 'browser'：跟随浏览器本地时区（默认）
 * - 其它：IANA 时区名（如 Asia/Shanghai）
 * 缓存在 localStorage，使管理页在设置加载前也能按上次选择渲染。
 */
const STORAGE_KEY = 'panel.display_timezone'

export const displayTimezone = ref<string>(
  (typeof localStorage !== 'undefined' && localStorage.getItem(STORAGE_KEY)) || 'browser',
)

export function setDisplayTimezone(tz?: string | null) {
  displayTimezone.value = (tz || 'browser').trim() || 'browser'
  try {
    localStorage.setItem(STORAGE_KEY, displayTimezone.value)
  } catch {
    /* 隐私模式等场景忽略 */
  }
}

function tzOption(): string | undefined {
  return displayTimezone.value === 'browser' ? undefined : displayTimezone.value
}

function toDate(v: string | number | Date): Date | null {
  const d = v instanceof Date ? v : new Date(v)
  return isNaN(d.getTime()) ? null : d
}

/** 完整日期时间，按展示时区渲染（如 2026-09-11 15:04:05）。 */
export function formatDateTime(v?: string | number | Date | null, fallback = '-'): string {
  if (v === undefined || v === null || v === '') return fallback
  const d = toDate(v)
  if (!d) return String(v)
  return new Intl.DateTimeFormat('zh-CN', {
    timeZone: tzOption(),
    year: 'numeric',
    month: '2-digit',
    day: '2-digit',
    hour: '2-digit',
    minute: '2-digit',
    second: '2-digit',
    hour12: false,
  })
    .format(d)
    .replace(/\//g, '-')
}

/** 到分钟粒度的日期时间，按展示时区渲染（如 2026-09-11 15:04）。
 *  表格/卡片列用：秒数在列表里只占宽、不给信息。 */
export function formatDateTimeMinute(v?: string | number | Date | null, fallback = '-'): string {
  if (v === undefined || v === null || v === '') return fallback
  const d = toDate(v)
  if (!d) return String(v)
  return new Intl.DateTimeFormat('zh-CN', {
    timeZone: tzOption(),
    year: 'numeric',
    month: '2-digit',
    day: '2-digit',
    hour: '2-digit',
    minute: '2-digit',
    hour12: false,
  })
    .format(d)
    .replace(/\//g, '-')
}

/** 仅日期，按展示时区渲染（如 2026-09-11）。 */
export function formatDateOnly(v?: string | number | Date | null, fallback = '-'): string {
  if (v === undefined || v === null || v === '') return fallback
  const d = toDate(v)
  if (!d) return String(v)
  return new Intl.DateTimeFormat('zh-CN', {
    timeZone: tzOption(),
    year: 'numeric',
    month: '2-digit',
    day: '2-digit',
  })
    .format(d)
    .replace(/\//g, '-')
}

/**
 * 目标日历日距今天的天数（今天 = 0，过去为负）。
 * 只有「今天算哪一天」按展示时区取，之后用 UTC 做中性日历减法，不把日期当时刻换算。
 * 日期非法返回 null。
 */
export function daysUntilDate(dateStr?: string | null): number | null {
  const m = /^(\d{4})-(\d{2})-(\d{2})/.exec((dateStr || '').trim())
  if (!m) return null
  const today = formatDateOnly(new Date()).split('-').map(Number)
  const target = Date.UTC(Number(m[1]), Number(m[2]) - 1, Number(m[3]))
  const base = Date.UTC(today[0], today[1] - 1, today[2])
  return Math.round((target - base) / 86400000)
}

/** 图表坐标轴时间：>48h 显示 月-日 时:分，否则只显示 时:分（按展示时区）。 */
export function formatAxisTime(iso: string, spanHours: number): string {
  const d = toDate(iso)
  if (!d) return iso
  const opts: Intl.DateTimeFormatOptions =
    spanHours > 48
      ? { timeZone: tzOption(), month: '2-digit', day: '2-digit', hour: '2-digit', minute: '2-digit', hour12: false }
      : { timeZone: tzOption(), hour: '2-digit', minute: '2-digit', hour12: false }
  return new Intl.DateTimeFormat('zh-CN', opts).format(d).replace(/\//g, '-')
}

/** 展示时区的人类可读名（用于设置页提示）。 */
export function effectiveTimezoneLabel(): string {
  if (displayTimezone.value === 'browser') {
    try {
      return `跟随浏览器（${Intl.DateTimeFormat().resolvedOptions().timeZone}）`
    } catch {
      return '跟随浏览器'
    }
  }
  return displayTimezone.value
}
