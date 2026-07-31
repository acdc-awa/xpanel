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
        path: 'plans',
        name: 'admin.plans',
        component: () => import('@/views/admin/plans.vue'),
        meta: { title: '套餐 / 订阅' },
      },
      {
        path: 'users',
        name: 'admin.users',
        component: () => import('@/views/admin/users.vue'),
        meta: { title: '用户管理' },
      },
    ],
  },
]
