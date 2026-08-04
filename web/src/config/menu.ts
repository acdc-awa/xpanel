import type { Component } from 'vue'
import {
  Odometer,
  Monitor,
  Share,
  Box,
  User,
  HomeFilled,
  ShoppingCart,
  UserFilled,
  Tickets,
  Key,
  DataAnalysis,
} from '@element-plus/icons-vue'

export interface MenuItem {
  title: string
  path: string // 与路由 path 一一对应（唯一关联键）
  icon: Component
  roles?: ('admin' | 'user')[]
}

/**
 * 菜单集中注册：Layout 用 v-for 渲染，新增栏目 = 数组加一项。
 * 菜单顺序即数组顺序；roles 为空表示登录即可见。
 */
export const adminMenus: MenuItem[] = [
  { title: '仪表盘', path: '/admin/dashboard', icon: Odometer },
  { title: '服务器', path: '/admin/servers', icon: Monitor },
  { title: '节点（接入点）', path: '/admin/nodes', icon: Share },
  { title: '套餐 / 订阅', path: '/admin/plans', icon: Box },
  { title: '用户管理', path: '/admin/users', icon: User },
  { title: '订单确认', path: '/admin/orders', icon: Tickets },
  { title: '邀请码', path: '/admin/invitations', icon: Key },
  { title: '审计日志', path: '/admin/audit', icon: DataAnalysis },
]

export const clientMenus: MenuItem[] = [
  { title: '首页', path: '/dashboard', icon: HomeFilled },
  { title: '商店', path: '/shop', icon: ShoppingCart },
  { title: '我的', path: '/account', icon: UserFilled },
]