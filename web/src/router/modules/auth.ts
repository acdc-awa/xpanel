import type { RouteRecordRaw } from 'vue-router'

/** 认证页（独立全屏，不套 Layout）；guestOnly 由全局守卫处理已登录跳转。 */
export const authRoutes: RouteRecordRaw[] = [
  {
    path: '/login',
    name: 'login',
    component: () => import('@/views/auth/login.vue'),
    meta: { title: '登录', guestOnly: true },
  },
  {
    path: '/register',
    name: 'register',
    component: () => import('@/views/auth/register.vue'),
    meta: { title: '注册', guestOnly: true },
  },
  {
    path: '/forgot',
    name: 'forgot',
    component: () => import('@/views/auth/forgot.vue'),
    meta: { title: '重置密码', guestOnly: true },
  },
]