import { http } from './http'
import type { ApiResp, LoginResult } from './types'

// RegisterPayload 注册请求（邮箱即账号，必填；邀请码必填；人机验证见 turnstile_token）。
export interface RegisterPayload {
  email: string
  password: string
  invite_code: string
  turnstile_token?: string
}

export function login(email: string, password: string, turnstile_token?: string) {
  return http.post<ApiResp<LoginResult>>('/auth/login', { email, password, turnstile_token })
}

export function register(payload: RegisterPayload) {
  return http.post<ApiResp<LoginResult>>('/auth/register', payload)
}

export function login2fa(code: string) {
  return http.post<ApiResp<LoginResult>>('/auth/2fa/verify', { code })
}

export function forgotPassword(email: string, turnstile_token?: string) {
  return http.post<ApiResp<{ message: string }>>('/auth/forgot', { email, turnstile_token })
}

export function resetPassword(email: string, code: string, password: string, turnstile_token?: string) {
  return http.post<ApiResp<{ ok: boolean; message: string }>>('/auth/reset', { email, code, password, turnstile_token })
}

export function logout() {
  return http.post<ApiResp<null>>('/auth/logout')
}