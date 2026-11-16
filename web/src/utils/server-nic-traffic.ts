import type { NicQuotaDirection, ServerItem, ServerNicTraffic } from '@/api/admin'
import { formatBytes } from './format'
import { daysUntilDate, formatDateOnly } from './timezone'

// 服务器网卡周期流量的展示口径（单一来源，卡片与概览共用）。
//
// 数据源是节点上报的累计物理网卡 Rx/Tx（整机口径，含 SSH/系统更新等全部流量），
// 不是 xray 用户流量；周期与额度都由后端解析，前端只负责格式化。

/** 取服务器附带的网卡周期数据（列表接口未附带时为 undefined）。 */
export function nicTrafficOf(server: ServerItem): ServerNicTraffic | undefined {
  return server.nic_traffic
}

/** 日表里是否有过数据（tracked_since 为空 = 节点从未上报过可用的网卡计数）。 */
export function hasNicTrafficData(t: ServerNicTraffic | undefined | null): boolean {
  return Boolean(t?.tracked_since)
}

/** 额度口径的展示名。 */
export function quotaDirectionLabel(dir: NicQuotaDirection | undefined): string {
  switch (dir) {
    case 'rx':
      return '仅下行'
    case 'tx':
      return '仅上行'
    case 'max':
      return '取较大方向'
    default:
      return '双向合计'
  }
}

/** 周期区间（结束日按「含」显示：后端存的是左闭右开的下一周期起点）。 */
export function nicCycleRangeText(t: ServerNicTraffic): string {
  const [y, m, d] = t.cycle_end.split('-').map(Number)
  const end = new Date(Date.UTC(y, (m || 1) - 1, d || 1))
  end.setUTCDate(end.getUTCDate() - 1)
  const endInclusive = end.toISOString().slice(0, 10)
  return `${t.cycle_start} 至 ${endInclusive}`
}

/** 本周期距重置的天数（cycle_end 是左闭右开的下一周期起点；已过或当天都算 0）。 */
export function nicDaysToReset(t: ServerNicTraffic): number | null {
  const days = daysUntilDate(t.cycle_end)
  return days === null ? null : Math.max(days, 0)
}

/**
 * 卡片上的重置提示：`3 天后重置 · 11-19`（跨年才带年份），当天重置只说「今天重置」。
 * 完整区间与数据完整性不在这里展开——由 tooltip 承担，卡片只留「还剩几天、什么时候重置」。
 */
export function nicResetText(t: ServerNicTraffic): string {
  const days = nicDaysToReset(t)
  if (days === null) return ''
  if (days === 0) return '今天重置'
  const [y, m, d] = t.cycle_end.split('-')
  const date = y === formatDateOnly(new Date()).slice(0, 4) ? `${m}-${d}` : t.cycle_end
  return `${days} 天后重置 · ${date}`
}

/** 本周期用量（按额度口径取数；无额度时显示双向合计）。 */
export function nicUsageHeadline(t: ServerNicTraffic): string {
  if (t.quota_bytes > 0) {
    return `${formatBytes(t.quota_used_bytes)} / ${formatBytes(t.quota_bytes)}`
  }
  return formatBytes(t.total_bytes)
}

/** 用量百分比文本（整数不带小数，非整数保留 1 位）。 */
export function nicQuotaPercentText(t: ServerNicTraffic): string {
  const p = Number(t.quota_percent) || 0
  const text = Number.isInteger(p) ? String(p) : p.toFixed(1)
  return `${text}%`
}

/** 进度条/数值的状态色（ok 用默认主色，warning 橙色，exceeded 红色）。 */
export function nicQuotaColor(t: ServerNicTraffic | undefined): string | undefined {
  if (!t || t.quota_bytes <= 0) return undefined
  if (t.quota_state === 'exceeded') return 'var(--color-text-status-error-ri38gt, #db0000)'
  if (t.quota_state === 'warning') return 'var(--color-text-status-warning-fabq6h, #8d6605)'
  return undefined
}

/** 额度状态文案（不超额时为空）。 */
export function nicQuotaStateText(t: ServerNicTraffic | undefined): string {
  if (!t || t.quota_bytes <= 0) return ''
  if (t.quota_state === 'exceeded') return '已超额'
  if (t.quota_state === 'warning') return '接近额度'
  return ''
}

/** 数据不完整时的说明（partial=false 返回空串）。 */
export function nicPartialText(t: ServerNicTraffic | undefined): string {
  if (!t?.partial || !t.tracked_since) return ''
  return `本周期数据自 ${t.tracked_since} 起`
}

/** 卡片/概览的悬停说明：口径 + 区间 + 双向明细。 */
export function nicTrafficTooltip(t: ServerNicTraffic | undefined): string {
  if (!t) return '该服务器尚未上报网卡流量'
  if (!hasNicTrafficData(t)) return '节点尚未上报可用的网卡计数（需要 agent 升级或节点在线）'
  const parts = [
    '网卡整机口径，含 SSH、系统更新等全部流量',
    `本周期 ${nicCycleRangeText(t)}（${t.cycle_label}）`,
    `下行 ${formatBytes(t.rx_bytes)}，上行 ${formatBytes(t.tx_bytes)}，合计 ${formatBytes(t.total_bytes)}`,
  ]
  if (t.quota_bytes > 0) {
    parts.push(`额度按${quotaDirectionLabel(t.quota_direction)}计算，已用 ${nicQuotaPercentText(t)}`)
  }
  const partial = nicPartialText(t)
  if (partial) parts.push(partial)
  return parts.join('；')
}
