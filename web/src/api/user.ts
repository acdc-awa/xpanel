import { http } from './http'
import type { ApiResp, UserInfo } from './types'

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