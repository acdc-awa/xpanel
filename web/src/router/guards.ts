import type { Router } from 'vue-router'
import { useAuthStore } from '@/stores/auth'

/** 全局路由守卫：登录态 / 角色校验统一在这里（P0 接入真实 JWT 后完善）。 */
export function setupRouterGuards(router: Router) {
  router.beforeEach((to) => {
    const auth = useAuthStore()

    if (to.meta.requiresAuth && !auth.isLoggedIn) {
      // TODO: 真实登录页接入后改为 { path: '/login' }
      return { path: '/dashboard' }
    }

    // TODO: 按 to.meta.roles 校验角色（与 menu.ts 的 roles 共用同一权限逻辑）
    return true
  })
}
