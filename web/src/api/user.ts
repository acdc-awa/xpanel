import { http } from './http'
import type { ApiResp, UserInfo } from './types'

export function getMe() {
  return http.get<ApiResp<UserInfo>>('/user/me')
}