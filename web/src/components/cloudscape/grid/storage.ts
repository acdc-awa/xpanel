import type { WidgetDefinition, WidgetLayoutItem } from './types'

export const WIDGET_DEFINITIONS: Record<string, WidgetDefinition> = {
  'kpi-summary': {
    id: 'kpi-summary',
    title: '核心业务与运行指标 (Key Metrics)',
    description: '展示今日与月度营收、流量消耗、实时网络速率、服务器在线率及有效用户与订单',
    defaultW: 6,
    defaultH: 3,
    minW: 3,
    minH: 2,
  },
  'traffic-trend': {
    id: 'traffic-trend',
    title: '吞吐趋势 (Throughput Trend)',
    description: '每日上行与下行流量历史用量堆叠柱形图，支持 3/7/30 天颗粒度切换',
    defaultW: 3,
    defaultH: 3,
    minW: 2,
    minH: 3,
  },
  'traffic-share': {
    id: 'traffic-share',
    title: '服务器流量分布 (Traffic Share)',
    description: '统计各服务器承载流量比例环形图，支持多时段分析',
    defaultW: 3,
    defaultH: 3,
    minW: 2,
    minH: 3,
  },
  'server-matrix': {
    id: 'server-matrix',
    title: '服务器监控矩阵 (Server Matrix)',
    description: '各节点探活心跳、CPU / 内存负载与实时双向速率表格监控',
    defaultW: 4,
    defaultH: 6,
    minW: 3,
    minH: 4,
  },
  'analytics-ranking': {
    id: 'analytics-ranking',
    title: '数据洞察与业务流水 (Analytics & Streams)',
    description: '多标签页聚合：用户流量排行、节点用量排行、卡密核销记录与套餐订单流水',
    defaultW: 2,
    defaultH: 6,
    minW: 2,
    minH: 4,
  },
}

export const DEFAULT_LAYOUT: WidgetLayoutItem[] = [
  { id: 'kpi-summary', x: 0, y: 0, w: 6, h: 3, minW: 3, minH: 2 },
  { id: 'traffic-trend', x: 0, y: 3, w: 3, h: 3, minW: 2, minH: 3 },
  { id: 'traffic-share', x: 3, y: 3, w: 3, h: 3, minW: 2, minH: 3 },
  { id: 'server-matrix', x: 0, y: 6, w: 4, h: 6, minW: 3, minH: 4 },
  { id: 'analytics-ranking', x: 4, y: 6, w: 2, h: 6, minW: 2, minH: 4 },
]

export const DASHBOARD_LAYOUT_STORAGE_KEY = 'xpanel_admin_dashboard_layout_v4'

export function loadSavedLayout(): WidgetLayoutItem[] {
  try {
    const raw = localStorage.getItem(DASHBOARD_LAYOUT_STORAGE_KEY)
    if (!raw) return JSON.parse(JSON.stringify(DEFAULT_LAYOUT))
    const parsed = JSON.parse(raw)
    if (!Array.isArray(parsed) || parsed.length === 0) {
      return JSON.parse(JSON.stringify(DEFAULT_LAYOUT))
    }
    const valid = parsed.filter((item) => item && typeof item.id === 'string' && WIDGET_DEFINITIONS[item.id])
    if (valid.length === 0) {
      return JSON.parse(JSON.stringify(DEFAULT_LAYOUT))
    }
    return valid.map((item) => {
      const def = WIDGET_DEFINITIONS[item.id]
      return {
        id: item.id,
        x: Number(item.x ?? 0),
        y: Number(item.y ?? 0),
        w: Number(item.w ?? def.defaultW),
        h: Number(item.h ?? def.defaultH),
        minW: def.minW,
        minH: def.minH,
      }
    })
  } catch {
    return JSON.parse(JSON.stringify(DEFAULT_LAYOUT))
  }
}

export function saveLayout(items: WidgetLayoutItem[]) {
  try {
    const data = items.map((i) => ({
      id: i.id,
      x: i.x,
      y: i.y,
      w: i.w,
      h: i.h,
    }))
    localStorage.setItem(DASHBOARD_LAYOUT_STORAGE_KEY, JSON.stringify(data))
  } catch (e) {
    console.error('Failed to save dashboard layout', e)
  }
}

export function resetLayout(): WidgetLayoutItem[] {
  try {
    localStorage.removeItem(DASHBOARD_LAYOUT_STORAGE_KEY)
  } catch {}
  return JSON.parse(JSON.stringify(DEFAULT_LAYOUT))
}
