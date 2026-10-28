import type { BadgeColor, StatusType } from '@/components/cloudscape'

// 服务器状态芯片的唯一实现：服务器列表卡片与服务器设置弹窗共用。
// 状态本身由后端推导（push_state / xray_state），这里只负责文案、配色与提示，
// 避免同一状态在两处出现不同措辞（历史上前者写「已同步」、后者写「SYNCED」）。

/** 状态芯片的数据源。字段全部可选，以兼容未上报这些列的旧 Agent。 */
export interface ServerStatusSource {
  status?: number
  push_state?: string
  config_status?: string
  xray_disk_hash?: string
  push_error?: string
  push_attempts?: number
  xray_state?: string
  xray_running?: boolean
  xray_last_error?: string
}

export interface StatusChip {
  color: BadgeColor
  text: string
  /** 悬停提示；空串表示无需解释。 */
  tip: string
}

export interface AlertChip extends StatusChip {
  /** 只有真正异常时才为 true，列表里的异常徽标据此决定显不显示。 */
  show: boolean
}

function withNote(main: string, note?: string) {
  return note ? `${main}（${note}）` : main
}

function attemptNote(attempts?: number) {
  return attempts ? `已尝试 ${attempts} 次` : undefined
}

function staleNote(src: ServerStatusSource) {
  return src.status !== 1 ? '服务器离线，此为离线前最后一次上报' : undefined
}

/** 配置下发状态：未投递 / 待推送 / 推送被拒 / 磁盘偏离 / 已同步。 */
export function pushStateChip(src: ServerStatusSource): StatusChip {
  const st =
    src.push_state ||
    (src.config_status === 'pushed' ? 'synced' : src.config_status === 'pending' ? 'pending' : 'none')
  const attempts = attemptNote(src.push_attempts)
  switch (st) {
    case 'drift':
      return {
        color: 'red',
        text: '磁盘偏离',
        tip: `节点磁盘上的配置与主控记录不一致（节点哈希 ${(src.xray_disk_hash || '').slice(0, 12) || '未上报'}…）。可能被手工改过、落盘失败或节点回退过配置，重新生成并推送一次即可对齐。`,
      }
    case 'rejected':
      return {
        color: 'red',
        text: '推送被拒',
        tip: withNote(`节点拒绝该配置：${src.push_error || '原因未知'}`, attempts),
      }
    case 'synced':
      return { color: 'green', text: '已同步', tip: '节点磁盘配置与主控记录一致' }
    case 'pending':
      return {
        color: 'severity-medium',
        text: '待推送',
        tip: src.push_error
          ? withNote(`尚未应用：${src.push_error}`, attempts)
          : '配置已生成，等待节点上线后自动推送',
      }
    default:
      return { color: 'grey', text: '未投递', tip: '尚无配置下发记录' }
  }
}

/** 配置同步状态的一句话说明，供状态带副标题使用（比 tooltip 短）。 */
export function pushStateSummary(src: ServerStatusSource): string {
  const st =
    src.push_state ||
    (src.config_status === 'pushed' ? 'synced' : src.config_status === 'pending' ? 'pending' : 'none')
  switch (st) {
    case 'drift':
      return '磁盘配置被改动'
    case 'rejected':
      return '节点拒绝，需处理'
    case 'synced':
      return '与主控记录一致'
    case 'pending':
      if (!src.push_error) return '等待推送'
      return src.push_attempts ? `第 ${src.push_attempts} 次尝试失败` : '上次尝试失败'
    default:
      return '尚未生成推送'
  }
}

/** Xray 进程状态：运行中 / 未运行 / 重试中 / 启动失败 / 未启动。 */
export function xrayChip(src: ServerStatusSource): StatusChip {
  const stale = staleNote(src)
  const reason = src.xray_last_error || '原因未知'
  switch (src.xray_state) {
    case 'failed':
      return {
        color: 'red',
        text: 'Xray 启动失败',
        tip: withNote(`连续启动失败，已停止自动拉起：${reason}`, stale),
      }
    case 'restarting':
      return {
        color: 'severity-medium',
        text: 'Xray 重试中',
        tip: withNote(`启动失败，自动重试中：${reason}`, stale),
      }
    case 'stopped':
      return { color: 'grey', text: 'Xray 未启动', tip: withNote('xray 进程当前未运行', stale) }
    default:
      if (src.xray_running) {
        return { color: 'green', text: 'Xray 运行中', tip: withNote('xray 进程正常运行', stale) }
      }
      return {
        color: 'red',
        text: 'Xray 未运行',
        tip: withNote('节点未上报 xray 运行状态，或进程已退出', stale),
      }
  }
}

/**
 * 列表卡片上的 Xray 异常徽标：只在 failed / restarting 时显示，正常态不加噪音。
 * 节点离线时状态是「离线前最后一次上报」，文案显式加「上次」前缀。
 */
export function xrayAlertChip(src: ServerStatusSource): AlertChip {
  const s = src.xray_state
  if (s !== 'failed' && s !== 'restarting') {
    return { show: false, color: 'grey', text: '', tip: '' }
  }
  const chip = xrayChip(src)
  return { ...chip, show: true, text: src.status === 1 ? chip.text : `上次 ${chip.text}` }
}

/**
 * 把 chip 的 Badge 配色映射成 CsStatusIndicator 的 type。
 * 卡片状态条用「圆点 + 文案」而不是实心徽标，正常态才不会三块绿色堆在一起；
 * 配色语义仍以 chip 为唯一来源（red=异常、severity-medium=待处理、grey=未启用）。
 */
export function indicatorTypeOf(color: BadgeColor): StatusType {
  switch (color) {
    case 'red':
    case 'severity-critical':
    case 'severity-high':
      return 'error'
    case 'severity-medium':
    case 'severity-low':
      return 'warning'
    case 'green':
      return 'success'
    case 'blue':
      return 'info'
    default:
      return 'stopped'
  }
}
