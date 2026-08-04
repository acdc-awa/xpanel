import axios, { AxiosError, type InternalAxiosRequestConfig } from 'axios'
import { ElMessage } from 'element-plus'
import { useAuthStore } from '@/stores/auth'

/** Axios 实例：统一 baseURL /api/v1（dev 由 vite proxy 转发到主控 18080）。 */
export const http = axios.create({
  baseURL: import.meta.env.VITE_API_BASE ?? '/api/v1',
  timeout: 10000,
})

http.interceptors.request.use((config) => {
  const token = localStorage.getItem('access_token')
  if (token) config.headers.Authorization = `Bearer ${token}`
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

    if (status === 401 && cfg && !cfg._retried) {
      cfg._retried = true
      const refreshToken = localStorage.getItem('refresh_token')
      if (refreshToken) {
        try {
          const { data } = await axios.post<{ code: number; data: { access_token: string } }>(
            `${import.meta.env.VITE_API_BASE ?? '/api/v1'}/auth/refresh`,
            { refresh_token: refreshToken },
          )
          if (data.code === 0 && data.data?.access_token) {
            localStorage.setItem('access_token', data.data.access_token)
            cfg.headers.Authorization = `Bearer ${data.data.access_token}`
            return http(cfg)
          }
        } catch {
          /* 刷新失败走登出 */
        }
      }
      const auth = useAuthStore()
      auth.logout()
      if (window.location.pathname !== '/login') {
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