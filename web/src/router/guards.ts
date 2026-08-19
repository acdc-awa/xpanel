import type { Router } from 'vue-router'
import { useAuthStore } from '@/stores/auth'
import { useSiteStore } from '@/stores/site'

/**
 * 全局路由守卫：
 * - 有 token 无用户信息 → 拉取 /user/me 恢复登录态
 * - 游客页（login/register）已登录 → 跳角色首页
 * - requiresAuth 未登录 → /login?redirect=...
 * - meta.roles 与当前角色不符 → 跳角色首页
 * - 路由切换后动态同步 document.title
 */
export function setupRouterGuards(router: Router) {
  router.beforeEach(async (to) => {
    const auth = useAuthStore()
    const site = useSiteStore()

    if (!site.isLoaded) {
      await site.fetchConfig()
    }

    if (!auth.isInitialized) {
      await auth.fetchMe()
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

  router.afterEach((to) => {
    const site = useSiteStore()
    const title = to.meta.title as string | undefined
    document.title = title ? `${title} - ${site.appName}` : site.appName
  })
}