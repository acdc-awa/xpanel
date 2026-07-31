import type { RouteRecordRaw } from 'vue-router'
import ClientLayout from '@/layouts/ClientLayout.vue'

/** 客户端路由：新增页面 → 在此数组追加一条记录（页面放 views/client/ 下）。 */
export const clientRoutes: RouteRecordRaw[] = [
  {
    path: '/',
    component: ClientLayout,
    meta: { requiresAuth: true },
    children: [
      { path: '', redirect: '/dashboard' },
      {
        path: 'dashboard',
        name: 'client.dashboard',
        component: () => import('@/views/client/dashboard.vue'),
        meta: { title: '首页' },
      },
      {
        path: 'shop',
        name: 'client.shop',
        component: () => import('@/views/client/shop.vue'),
        meta: { title: '商店' },
      },
      {
        path: 'account',
        name: 'client.account',
        component: () => import('@/views/client/account.vue'),
        meta: { title: '我的' },
      },
    ],
  },
  { path: '/:pathMatch(.*)*', name: 'not-found', redirect: '/dashboard' },
]
