import { defineStore } from 'pinia'

export const useAuthStore = defineStore('auth', {
  state: () => ({
    // TODO: P0 接入真实 JWT 后由登录接口写入（access + refresh）
    token: 'mock-token',
    isLoggedIn: true,
    role: 'admin' as 'admin' | 'user',
    username: 'admin',
  }),
  actions: {
    logout() {
      this.token = ''
      this.isLoggedIn = false
    },
  },
})
