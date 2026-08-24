import axios, { AxiosError, type InternalAxiosRequestConfig } from 'axios'
import { ElMessage } from 'element-plus'
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

// 401 时用 refresh token 换新 access 后重放一次；失败则登出
http.interceptors.response.use(
  (res) => res,
  async (error: AxiosError) => {
    const cfg = error.config as RetriableRequest | undefined
    const status = error.response?.status

    // 访客页面（login / register / forgot）未登录探针返回 401 为正常现象，不触发刷新与强跳
    const currentPath = window.location.pathname
    const isGuestPage = ['/login', '/register', '/forgot'].some(
      (p) => currentPath === p || currentPath.startsWith(`${p}/`)
    )

    if (status === 401 && cfg && !cfg._retried && !isGuestPage) {
      cfg._retried = true
      try {
        const { data } = await axios.post<{ code: number }>(
          `${import.meta.env.VITE_API_BASE ?? apiBase}/auth/refresh`,
          {},
          { withCredentials: true }
        )
        if (data.code === 0) {
          return http(cfg)
        }
      } catch {
        /* 刷新失败走登出 */
      }
      const auth = useAuthStore()
      await auth.logout()
      if (!isGuestPage) {
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