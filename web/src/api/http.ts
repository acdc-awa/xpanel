import axios, { AxiosError, type InternalAxiosRequestConfig } from 'axios'
import { ElMessage } from '@/components/cloudscape'
import { useAuthStore } from '@/stores/auth'
import { apiBase } from '@/config/site'

/** Axios 实例：统一 baseURL /api/v1（dev 由 vite proxy 转发到主控 18080）。 */
export const http = axios.create({
  baseURL: import.meta.env.VITE_API_BASE ?? apiBase,
  timeout: 10000,
})

http.interceptors.request.use((config) => {
  // 凭证现在通过 Cookie 发送，不需要手动附加 Authorization
  return config
})

interface RetriableRequest extends InternalAxiosRequestConfig {
  _retried?: boolean
}

let refreshPromise: Promise<boolean> | null = null
let redirectingToLogin = false

function refreshSession(): Promise<boolean> {
  if (!refreshPromise) {
    refreshPromise = axios
      .post<{ code: number }>(
        `${import.meta.env.VITE_API_BASE ?? apiBase}/auth/refresh`,
        {},
        { withCredentials: true },
      )
      .then((res) => res.data.code === 0)
      .catch(() => false)
      .finally(() => {
        refreshPromise = null
      })
  }
  return refreshPromise
}

// 401 时用 single-flight refresh token 换新 access 后重放一次；失败则登出
http.interceptors.response.use(
  (res) => res,
  async (error: AxiosError) => {
    const cfg = error.config as RetriableRequest | undefined
    const status = error.response?.status

    // 访客页面（login / register / forgot）未登录探针返回 401 为正常现象，不触发刷新与强跳
    const currentPath = window.location.pathname
    const isGuestPage = ['/login', '/register', '/forgot'].some(
      (p) => currentPath === p || currentPath.startsWith(`${p}/`),
    )

    if (status === 401 && cfg && !cfg._retried && !isGuestPage) {
      cfg._retried = true
      const refreshed = await refreshSession()
      if (refreshed) {
        return http(cfg)
      }
      const auth = useAuthStore()
      // 首次加载（路由守卫尚在 fetchMe 探测登录态）：静默清态，交由守卫无刷新跳转到 /login?redirect=...
      if (!auth.isInitialized) {
        auth.user = null
        return Promise.reject(error)
      }
      await auth.logout()
      if (!isGuestPage && !redirectingToLogin) {
        redirectingToLogin = true
        ElMessage.error('登录已过期，请重新登录')
        window.location.href = '/login'
      }
    }
    return Promise.reject(error)
  },
)

/** 从统一响应中取 message（后端 400/401 等也走 data.message）。 */
export function errMsg(error: unknown, fallback = '请求失败'): string {
  const e = error as AxiosError<{ message?: string }>
  return e?.response?.data?.message ?? (error instanceof Error ? error.message : fallback)
}