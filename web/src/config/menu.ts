import type { Component } from 'vue'
import {
  Odometer,
  Monitor,
  Share,
  Box,
  User,
  HomeFilled,
  Promotion,
  ShoppingCart,
  UserFilled,
  Tickets,
  Key,
  Lock,
  DataAnalysis,
  Setting,
  Connection,
  Bell,
  Opportunity,
} from '@element-plus/icons-vue'

export interface SubMenuItem {
  title: string
  path: string
  icon?: Component
}

export interface MenuGroup {
  title: string
  path?: string // 单项菜单直接绑定路由
  icon: Component
  children?: SubMenuItem[] // 分组子项
}

/**
 * 分组菜单配置（Xboard 架构风格）：按业务域分为仪表盘、节点管理、订阅财务、用户运营、系统管理
 */
export const adminMenuGroups: MenuGroup[] = [
  {
    title: '仪表盘',
    path: '/admin/dashboard',
    icon: Odometer,
  },
  {
    title: '节点管理',
    icon: Connection,
    children: [
      { title: '服务器管理', path: '/admin/servers', icon: Monitor },
      { title: '节点接入点', path: '/admin/nodes', icon: Share },
      { title: '权限组管理', path: '/admin/permission-groups', icon: UserFilled },
      { title: '路由管理', path: '/admin/routing', icon: Connection },
      { title: '证书管理', path: '/admin/certs', icon: Lock },
    ],
  },
  {
    title: '订阅与财务',
    icon: Box,
    children: [
      { title: '套餐管理', path: '/admin/plans', icon: Box },
      { title: '订单记录', path: '/admin/orders', icon: Tickets },
      { title: '礼品卡管理', path: '/admin/gift-cards', icon: Tickets },
      { title: '邀请码管理', path: '/admin/invitations', icon: Key },
    ],
  },
  {
    title: '用户与运营',
    icon: User,
    children: [
      { title: '用户管理', path: '/admin/users', icon: User },
      { title: '公告管理', path: '/admin/notices', icon: Bell },
      { title: '审计日志', path: '/admin/audit', icon: DataAnalysis },
    ],
  },
  {
    title: '系统管理',
    icon: Setting,
    children: [
      { title: '系统配置', path: '/admin/settings', icon: Setting },
      { title: '设计规范 Demo', path: '/admin/design-demo', icon: Opportunity },
    ],
  },
]

export interface MenuItem {
  title: string
  path: string // 与路由 path 一一对应（唯一关联键）
  icon: Component
  roles?: ('admin' | 'user')[]
}

export const clientMenus: MenuItem[] = [
  { title: '首页', path: '/dashboard', icon: HomeFilled },
  { title: '订阅中心', path: '/subscribe', icon: Promotion },
  { title: '商店', path: '/shop', icon: ShoppingCart },
  { title: '我的', path: '/account', icon: UserFilled },
]