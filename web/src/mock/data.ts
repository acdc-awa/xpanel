// ===== 与后端表结构对齐的 mock 数据（P0 接入真实 API 后删除本文件） =====

export interface Inbound {
  id: number
  serverId: number
  serverName: string
  tag: string
  protocol: 'vless' | 'vmess' | 'trojan' | 'ss'
  port: number
  account: string
  network: string
  tls: string
  ratio: number
  enabled: boolean
}

export interface Plan {
  id: number
  name: string
  price: number
  trafficGb: number
  durationDays: number
  speedLimit?: string
  enabled: boolean
}

export interface PanelUser {
  id: number
  username: string
  email: string
  plan: string
  usedGb: number
  totalGb: number
  expireAt: string
  status: 'normal' | 'banned'
}

export type OrderStatus = 'pending' | 'paid' | 'cancelled'

export interface Order {
  id: number
  orderNo: string
  username: string
  planName: string
  amount: number
  status: OrderStatus
  createdAt: string
}

export interface Notice {
  id: number
  title: string
  date: string
}

export interface TrafficDay {
  day: string
  value: number
}


// 公告 mock（J15：待方向 2 公告系统替换，仅 dashboard 公告栏使用）
export const mockNotices = [
  { id: 1, title: '欢迎使用 Xray 面板', date: '2026-08-01' },
  { id: 2, title: '推荐使用 Clash Verge Rev 客户端', date: '2026-08-10' },
]
