// ===== 与后端表结构对齐的 mock 数据（P0 接入真实 API 后删除本文件） =====

export interface Server {
  id: number
  name: string
  host: string
  ip: string
  location: string
  status: 'online' | 'connecting' | 'offline'
  lastSeenAt: string
  remark?: string
}

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

export const mockServers: Server[] = [
  { id: 1, name: 'Tokyo-01', host: 'tokyo01.example.com', ip: '45.77.10.12', location: '🇯🇵 日本', status: 'online', lastSeenAt: '10 秒前', remark: '主力线路' },
  { id: 2, name: 'LosAngeles-01', host: 'la01.example.com', ip: '66.42.98.33', location: '🇺🇸 美国', status: 'online', lastSeenAt: '22 秒前' },
  { id: 3, name: 'Frankfurt-01', host: 'fra01.example.com', ip: '5.78.101.66', location: '🇩🇪 德国', status: 'online', lastSeenAt: '45 秒前' },
  { id: 4, name: 'Singapore-01', host: 'sg01.example.com', ip: '139.162.55.8', location: '🇸🇬 新加坡', status: 'connecting', lastSeenAt: '3 分钟前' },
  { id: 5, name: 'HongKong-01', host: 'hk01.example.com', ip: '47.242.33.90', location: '🇭🇰 香港', status: 'offline', lastSeenAt: '1 小时前', remark: '维护中' },
]

export const mockInbounds: Inbound[] = [
  { id: 1, serverId: 1, serverName: 'Tokyo-01', tag: 'Tokyo-VLESS', protocol: 'vless', port: 443, account: 'a1b2…9f0e', network: 'tcp', tls: 'reality', ratio: 1.0, enabled: true },
  { id: 2, serverId: 1, serverName: 'Tokyo-01', tag: 'Tokyo-Vmess-WS', protocol: 'vmess', port: 8443, account: '3f2e…77ab', network: 'ws', tls: 'tls', ratio: 0.8, enabled: true },
  { id: 3, serverId: 2, serverName: 'LosAngeles-01', tag: 'LA-Trojan', protocol: 'trojan', port: 443, account: '••••••••', network: 'tcp', tls: 'tls', ratio: 1.0, enabled: true },
  { id: 4, serverId: 3, serverName: 'Frankfurt-01', tag: 'FRA-SS', protocol: 'ss', port: 8388, account: '••••••••', network: 'tcp', tls: 'none', ratio: 1.5, enabled: false },
]

export const mockPlans: Plan[] = [
  { id: 1, name: '月付 200G', price: 25, trafficGb: 200, durationDays: 30, enabled: true },
  { id: 2, name: '季付 600G', price: 68, trafficGb: 600, durationDays: 90, enabled: true },
  { id: 3, name: '年付 1T', price: 188, trafficGb: 1024, durationDays: 365, enabled: true },
  { id: 4, name: '体验 5G', price: 1, trafficGb: 5, durationDays: 3, speedLimit: '5 Mbps', enabled: false },
]

export const mockUsers: PanelUser[] = [
  { id: 1001, username: 'alice', email: 'alice@example.com', plan: '月付 200G', usedGb: 76.6, totalGb: 200, expireAt: '2026-08-30', status: 'normal' },
  { id: 1002, username: 'bob', email: 'bob@example.com', plan: '年付 1T', usedGb: 212, totalGb: 1024, expireAt: '2027-07-31', status: 'normal' },
  { id: 1003, username: 'carol', email: 'carol@example.com', plan: '无', usedGb: 0, totalGb: 0, expireAt: '—', status: 'banned' },
  { id: 1004, username: 'dave', email: 'dave@example.com', plan: '体验 5G', usedGb: 4.7, totalGb: 5, expireAt: '2026-08-03', status: 'normal' },
]

export const mockOrders: Order[] = [
  { id: 1, orderNo: '20260801001', username: 'alice', planName: '月付 200G', amount: 25, status: 'pending', createdAt: '2026-08-01 10:24' },
  { id: 2, orderNo: '20260801002', username: 'bob', planName: '年付 1T', amount: 188, status: 'paid', createdAt: '2026-08-01 09:12' },
  { id: 3, orderNo: '20260731098', username: 'carol', planName: '季付 600G', amount: 68, status: 'cancelled', createdAt: '2026-07-31 18:02' },
]

export const mockNotices: Notice[] = [
  { id: 1, title: '8 月 1 日机房维护通知', date: '2026-08-01 09:00' },
  { id: 2, title: '新节点上线：德国 Frankfurt-01', date: '2026-07-28 18:30' },
]

export const mockTraffic: TrafficDay[] = [
  { day: '一', value: 38 },
  { day: '二', value: 55 },
  { day: '三', value: 42 },
  { day: '四', value: 70 },
  { day: '五', value: 58 },
  { day: '六', value: 84 },
  { day: '日', value: 100 },
]

/** 当前登录用户（客户端）mock */
export const mockClient = {
  username: 'alice',
  planName: '月付 200G',
  usedGb: 76.6,
  totalGb: 200,
  expireAt: '2026-08-30',
  balance: 50,
  subscribeUrl: 'https://panel.example.com/api/v1/client/subscribe?token=sk_ab12…',
  subscribeToken: 'sk_••••••••9f0e',
  onlineNodes: 3,
  totalNodes: 5,
}
