import { defineStore } from 'pinia'
import { login as apiLogin, login2fa as apiLogin2fa, register as apiRegister, logout as apiLogout, type RegisterPayload } from '@/api/auth'
import { getMe } from '@/api/user'
import { useSiteStore } from '@/stores/site'
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
      // 订阅端点信息（subscribe_url/path）仅登录态下发，同步给 site store 供订阅页拼链接
      useSiteStore().applyMeInfo(user)
    },

    /** 登录：返回 'ok'（已签发）或 '2fa'（需二次验证）。 */
    async login(username: string, password: string): Promise<'ok' | '2fa'> {
      const { data } = await apiLogin(username, password)
      if (data.code !== 0) throw new Error(data.message || '登录失败')
      if (data.data.twofa_required) return '2fa'
      this.applyUser(data.data.user!)
      return 'ok'
    },

    /** 2FA 二次验证：验证通过后签发完整令牌并写入用户信息。 */
    async verify2fa(code: string) {
      const { data } = await apiLogin2fa(code)
      if (data.code !== 0) throw new Error(data.message || '验证失败')
      this.applyUser(data.data.user!)
    },

    async register(payload: RegisterPayload) {
      const { data } = await apiRegister(payload)
      if (data.code !== 0) throw new Error(data.message || '注册失败')
      this.applyUser(data.data.user!)
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
      this.user = null
      this.isInitialized = true
      try {
        await apiLogout()
      } catch {
        // ignore
      }
    },

    /** 按角色返回登录后首页。 */
    homePath(): string {
      return this.role === 'admin' ? '/admin/dashboard' : '/dashboard'
    },
  },
})
