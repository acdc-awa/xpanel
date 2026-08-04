import { http } from './http'
import type { ApiResp, LoginResult } from './types'

export interface RegisterPayload {
  username: string
  email: string
  password: string
  invite_code: string
}

export function login(username: string, password: string) {
  return http.post<ApiResp<LoginResult>>('/auth/login', { username, password })
}

export function register(payload: RegisterPayload) {
  return http.post<ApiResp<LoginResult>>('/auth/register', payload)
}

export function logout() {
  return http.post<ApiResp<null>>('/auth/logout')
}