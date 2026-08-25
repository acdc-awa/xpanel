import { http } from './http'
import type { ApiResp, LoginResult } from './types'

// RegisterPayload 注册请求（2026-08-14 方向①：用户名=邮箱必填 + 邀请码必填；方向②：人机验证）。
export interface RegisterPayload {
  email: string
  password: string
  invite_code: string
  turnstile_token?: string
}

export function login(username: string, password: string) {
  return http.post<ApiResp<LoginResult>>('/auth/login', { username, password })
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