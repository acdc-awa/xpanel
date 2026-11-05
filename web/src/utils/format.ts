import { formatDateTime } from './timezone'

/** 金额数字部分（分 → `1,460.00`），不带货币符号。
 *  用于符号与数字分开排版的地方（如 KPI 卡把 ¥ 渲染成小号上标）。 */
export function formatMoneyNumber(cents: number): string {
  const n = (Number(cents) || 0) / 100
  return n.toLocaleString('zh-CN', { minimumFractionDigits: 2, maximumFractionDigits: 2 })
}

/** 金额（分 → `¥1,460.00`）。金额格式的唯一入口：
 *  符号与数字之间不留空格，固定两位小数，整数部分带千分位。
 *  见 AGENTS.md「文案规范 · 标点与格式」。 */
export function formatMoney(cents: number): string {
  return `¥${formatMoneyNumber(cents)}`
}

/** 带正负号的金额，用于流水/审计（`+¥12.00` / `-¥12.00`）。 */
export function formatSignedMoney(cents: number): string {
  const n = Number(cents) || 0
  return `${n >= 0 ? '+' : ''}${formatMoney(n)}`
}

/** 字节数格式化为可读大小（B/KB/MB/GB/TB）。 */
export function formatBytes(bytes: number): string {
  if (!bytes || bytes <= 0) return '0 B'
  const units = ['B', 'KB', 'MB', 'GB', 'TB']
  let i = 0
  let v = bytes
  while (v >= 1024 && i < units.length - 1) {
    v /= 1024
    i++
  }
  return `${v.toFixed(i === 0 || v >= 100 ? 0 : 1)} ${units[i]}`
}

/** 格式化 ISO 日期时间字符串（如 2026-08-18 10:00:00），按设置里的展示时区渲染。 */
export function formatDate(isoStr?: string | null): string {
  return formatDateTime(isoStr, '-')
}