import type { RouteRecordRaw } from 'vue-router'
import AdminLayout from '@/layouts/AdminLayout.vue'

/**
 * 管理端路由：新增栏目 → 在此数组追加一条记录即可（页面放 views/admin/ 下）。
 * meta.title 用于顶栏标题；菜单渲染见 src/config/menu.ts（与 path 关联）。
 */
export const adminRoutes: RouteRecordRaw[] = [
  {
    path: '/admin',
    component: AdminLayout,
    meta: { requiresAuth: true, roles: ['admin'] },
    children: [
      { path: '', redirect: '/admin/dashboard' },
      {
        path: 'dashboard',
        name: 'admin.dashboard',
        component: () => import('@/views/admin/dashboard.vue'),
        meta: { title: '仪表盘' },
      },
      {
        path: 'servers',
        name: 'admin.servers',
        component: () => import('@/views/admin/servers.vue'),
        meta: { title: '服务器' },
      },
      {
        path: 'nodes',
        name: 'admin.nodes',
        component: () => import('@/views/admin/nodes.vue'),
        meta: { title: '节点（接入点）' },
      },
      {
        path: 'routing',
        name: 'admin.routing',
        component: () => import('@/views/admin/routing.vue'),
        meta: { title: '路由管理' },
      },
      {
        path: 'plans',
        name: 'admin.plans',
        component: () => import('@/views/admin/plans.vue'),
        meta: { title: '套餐' },
      },
      {
        path: 'certs',
        name: 'admin.certs',
        component: () => import('@/views/admin/certs.vue'),
        meta: { title: '证书管理' },
      },
      {
        path: 'permission-groups',
        name: 'admin.permission-groups',
        component: () => import('@/views/admin/permission-groups.vue'),
        meta: { title: '权限组' },
      },
      {
        path: 'users',
        name: 'admin.users',
        component: () => import('@/views/admin/users.vue'),
        meta: { title: '用户管理' },
      },
      {
        path: 'orders',
        name: 'admin.orders',
        component: () => import('@/views/admin/orders.vue'),
        meta: { title: '订单确认' },
      },
      {
        path: 'invitations',
        name: 'admin.invitations',
        component: () => import('@/views/admin/invitations.vue'),
        meta: { title: '邀请码' },
      },
      {
        path: 'audit',
        name: 'admin.audit',
        component: () => import('@/views/admin/audit.vue'),
        meta: { title: '审计日志' },
      },
      {
        path: 'settings',
        name: 'admin.settings',
        component: () => import('@/views/admin/settings.vue'),
        meta: { title: '设置' },
      },
    ],
  },
]