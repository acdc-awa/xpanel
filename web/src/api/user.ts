import { http } from './http'
import type { ApiResp, UserInfo, NoticeItem } from './types'

export function getMe() {
  return http.get<ApiResp<UserInfo>>('/user/me')
}

export function changePassword(oldPassword: string, newPassword: string) {
  return http.post<ApiResp<{ ok: boolean }>>('/user/password', {
    old_password: oldPassword,
    new_password: newPassword,
  })
}

export function updateProfile(email: string) {
  return http.put<ApiResp<{ email: string }>>('/user/profile', { email })
}

// ---- TOTP 2FA（2026-08-14 方向③）----

export interface TwoFASetupResult {
  secret: string
  otpauth_url: string
}

export function setup2FA() {
  return http.post<ApiResp<TwoFASetupResult>>('/user/2fa/setup')
}

export function confirm2FA(secret: string, code: string) {
  return http.post<ApiResp<{ backup_codes: string[] }>>('/user/2fa/confirm', { secret, code })
}

export function disable2FA(payload: { code?: string; password?: string }) {
  return http.post<ApiResp<{ ok: boolean }>>('/user/2fa/disable', payload)
}

export function resetSubscribeToken() {
  return http.post<ApiResp<{ subscribe_token: string }>>('/user/subscribe/reset')
}

// ---- 用户端节点可用性（J15，替换 mock）----

export interface MyServerItem {
  id: number
  name: string
  host: string
  location: string
  online: boolean
  last_seen_at: string | null
}

export function getMyServers() {
  return http.get<ApiResp<{ items: MyServerItem[] }>>('/user/servers')
}

// ---- 用户端公告（方向 2）----

export function getUserNotices() {
  return http.get<ApiResp<NoticeItem[]>>('/user/notices')
}

export function getUserNotice(id: number) {
  return http.get<ApiResp<NoticeItem>>(`/user/notices/${id}`)
}