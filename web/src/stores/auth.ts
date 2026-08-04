import { defineStore } from 'pinia'
import { login as apiLogin, register as apiRegister, type RegisterPayload } from '@/api/auth'
import { getMe } from '@/api/user'
import type { Role, UserInfo } from '@/api/types'

const ACCESS_KEY = 'access_token'
const REFRESH_KEY = 'refresh_token'

/** 认证状态：JWT 存 localStorage，页面刷新后由 token 恢复登录态。 */
export const useAuthStore = defineStore('auth', {
  state: () => ({
    token: localStorage.getItem(ACCESS_KEY) ?? '',
    refreshToken: localStorage.getItem(REFRESH_KEY) ?? '',
    user: null as UserInfo | null,
  }),

  getters: {
    isLoggedIn: (s) => !!s.token,
    role: (s): Role | null => s.user?.role ?? null,
    username: (s) => s.user?.username ?? '',
    displayName: (s) => s.user?.username ?? '访客',
    avatarText: (s) => (s.user?.username?.[0] ?? '?').toUpperCase(),
  },

  actions: {
    applyTokens(access: string, refresh: string, user: UserInfo) {
      this.token = access
      this.refreshToken = refresh
      this.user = user
      localStorage.setItem(ACCESS_KEY, access)
      localStorage.setItem(REFRESH_KEY, refresh)
    },

    async login(username: string, password: string) {
      const { data } = await apiLogin(username, password)
      if (data.code !== 0) throw new Error(data.message || '登录失败')
      this.applyTokens(data.data.access_token, data.data.refresh_token, data.data.user)
    },

    async register(payload: RegisterPayload) {
      const { data } = await apiRegister(payload)
      if (data.code !== 0) throw new Error(data.message || '注册失败')
      this.applyTokens(data.data.access_token, data.data.refresh_token, data.data.user)
    },

    /** 拉取当前用户资料（刷新页面后恢复 user 信息）。 */
    async fetchMe() {
      if (!this.token) return
      const { data } = await getMe()
      if (data.code === 0) {
        this.user = data.data
      } else {
        throw new Error(data.message)
      }
    },

    logout() {
      this.token = ''
      this.refreshToken = ''
      this.user = null
      localStorage.removeItem(ACCESS_KEY)
      localStorage.removeItem(REFRESH_KEY)
    },

    /** 按角色返回登录后首页。 */
    homePath(): string {
      return this.role === 'admin' ? '/admin/dashboard' : '/dashboard'
    },
  },
})
