import { http } from './http'
import type {
  AdminDashboard,
  AdminUserPage,
  ApiResp,
  CreateInvitationResult,
  Invitation,
} from './types'

export function getDashboard() {
  return http.get<ApiResp<AdminDashboard>>('/admin/dashboard')
}

export function getUsers(page = 1, size = 20) {
  return http.get<ApiResp<AdminUserPage>>('/admin/users', { params: { page, size } })
}

export function getInvitations() {
  return http.get<ApiResp<{ items: Invitation[] }>>('/admin/invitations')
}

export function createInvitations(count: number, expires?: string) {
  return http.post<ApiResp<CreateInvitationResult>>('/admin/invitations', {
    count,
    expires: expires ?? '',
  })
}