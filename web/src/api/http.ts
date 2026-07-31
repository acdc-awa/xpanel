import axios from 'axios'

/**
 * Axios 实例：P0 后端就绪后接入 /api/v1。
 * 初版页面数据来自 src/mock，本文件仅预留统一封装。
 */
export const http = axios.create({
  baseURL: import.meta.env.VITE_API_BASE ?? '/api/v1',
  timeout: 10000,
})

http.interceptors.request.use((config) => {
  const token = localStorage.getItem('access_token')
  if (token) config.headers.Authorization = `Bearer ${token}`
  return config
})

http.interceptors.response.use(
  (res) => res,
  (err) => Promise.reject(err),
)
