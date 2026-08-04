import type { Router } from 'vue-router'
import { useAuthStore } from '@/stores/auth'

/**
 * 全局路由守卫：
 * - 有 token 无用户信息 → 拉取 /user/me 恢复登录态
 * - 游客页（login/register）已登录 → 跳角色首页
 * - requiresAuth 未登录 → /login?redirect=...
 * - meta.roles 与当前角色不符 → 跳角色首页
 */
export function setupRouterGuards(router: Router) {
  router.beforeEach(async (to) => {
    const auth = useAuthStore()

    if (auth.isLoggedIn && !auth.user) {
      try {
        await auth.fetchMe()
      } catch {
        auth.logout()
      }
    }

    if (to.meta.guestOnly && auth.isLoggedIn) {
      return auth.homePath()
    }

    if (to.meta.requiresAuth && !auth.isLoggedIn) {
      return { path: '/login', query: { redirect: to.fullPath } }
    }

    const roles = to.meta.roles as string[] | undefined
    if (roles && auth.role && !roles.includes(auth.role)) {
      return auth.homePath()
    }

    return true
  })
}