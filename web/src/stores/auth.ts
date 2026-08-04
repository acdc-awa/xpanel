import { defineStore } from 'pinia'
import { login as apiLogin, register as apiRegister, logout as apiLogout, type RegisterPayload } from '@/api/auth'
import { getMe } from '@/api/user'
import type { Role, UserInfo } from '@/api/types'

export const useAuthStore = defineStore('auth', {
  state: () => ({
    user: null as UserInfo | null,
    isInitialized: false,
  }),

  getters: {
    isLoggedIn: (s) => !!s.user,
    role: (s): Role | null => s.user?.role ?? null,
    username: (s) => s.user?.username ?? '',
    displayName: (s) => s.user?.username ?? '访客',
    avatarText: (s) => (s.user?.username?.[0] ?? '?').toUpperCase(),
  },

  actions: {
    applyUser(user: UserInfo) {
      this.user = user
      this.isInitialized = true
    },

    async login(username: string, password: string) {
      const { data } = await apiLogin(username, password)
      if (data.code !== 0) throw new Error(data.message || '登录失败')
      this.applyUser(data.data.user)
    },

    async register(payload: RegisterPayload) {
      const { data } = await apiRegister(payload)
      if (data.code !== 0) throw new Error(data.message || '注册失败')
      this.applyUser(data.data.user)
    },

    /** 拉取当前用户资料（刷新页面后恢复 user 信息）。 */
    async fetchMe() {
      try {
        const { data } = await getMe()
        if (data.code === 0) {
          this.user = data.data
        } else {
          this.user = null
        }
      } catch {
        this.user = null
      } finally {
        this.isInitialized = true
      }
    },

    async logout() {
      try {
        await apiLogout()
      } catch {
        // ignore
      }
      this.user = null
    },

    /** 按角色返回登录后首页。 */
    homePath(): string {
      return this.role === 'admin' ? '/admin/dashboard' : '/dashboard'
    },
  },
})
