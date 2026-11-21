export interface SubMenuItem {
  title: string
  path: string
  /** Cloudscape 图标名。 */
  iconName: string
}

export interface MenuGroup {
  title: string
  path?: string // 单项菜单直接绑定路由
  iconName: string
  children?: SubMenuItem[] // 分组子项
}

/**
 * 分组菜单配置（Xboard 架构风格）：按业务域分为仪表盘、节点管理、订阅财务、用户运营、系统管理
 */
export const adminMenuGroups: MenuGroup[] = [
  {
    title: '仪表盘',
    path: '/admin/dashboard',
    iconName: 'view-horizontal',
  },
  {
    title: '服务器与接入点',
    iconName: 'share',
    children: [
      { title: '服务器管理', path: '/admin/servers', iconName: 'multiscreen' },
      { title: '接入点', path: '/admin/nodes', iconName: 'share' },
      { title: '出站管理', path: '/admin/outbounds', iconName: 'send' },
      { title: '路由管理', path: '/admin/routing', iconName: 'map' },
      { title: '证书管理', path: '/admin/certs', iconName: 'lock-private' },
    ],
  },
  {
    // 套餐 / 权限组 / 订阅模板 同属「用户能买到什么、看到什么配置」的一条链，放在一组更符合心智。
    title: '订阅与财务',
    iconName: 'ticket',
    children: [
      { title: '套餐管理', path: '/admin/plans', iconName: 'grid-view' },
      { title: '权限组管理', path: '/admin/permission-groups', iconName: 'group' },
      { title: '订阅模板', path: '/admin/sub-templates', iconName: 'file' },
      { title: '订单记录', path: '/admin/orders', iconName: 'ticket' },
      { title: '礼品卡管理', path: '/admin/gift-cards', iconName: 'ticket' },
      { title: '邀请码管理', path: '/admin/invitations', iconName: 'key' },
    ],
  },
  {
    title: '用户与运营',
    iconName: 'user-profile',
    children: [
      { title: '用户管理', path: '/admin/users', iconName: 'user-profile' },
      { title: '公告管理', path: '/admin/notices', iconName: 'notification' },
      { title: '审计日志', path: '/admin/audit', iconName: 'list-view' },
    ],
  },
  {
    title: '系统管理',
    iconName: 'settings',
    children: [
      { title: '系统配置', path: '/admin/settings', iconName: 'settings' },
      { title: '数据管理', path: '/admin/data-management', iconName: 'upload-download' },
    ],
  },
]

export interface MenuItem {
  title: string
  path: string // 与路由 path 一一对应（唯一关联键）
  iconName: string
  roles?: ('admin' | 'user')[]
}

export const clientMenus: MenuItem[] = [
  { title: '首页', path: '/dashboard', iconName: 'home' },
  { title: '订阅中心', path: '/subscribe', iconName: 'send-ne' },
  { title: '商店', path: '/shop', iconName: 'shopping-bag' },
  { title: '我的', path: '/account', iconName: 'user-profile' },
]
