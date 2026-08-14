import { http } from './http'
import type { ApiResp, BalanceLog, GiftCard } from './types'

// 用户端：卡密兑换
export function redeemGiftCard(code: string) {
  return http.post<ApiResp<{ face_value_cents: number; balance_cents: number; card_name: string }>>(
    '/user/gift-cards/redeem',
    { code },
  )
}

// 用户端：查询余额流水
export function getMyBalanceLogs(page = 1, size = 20) {
  return http.get<ApiResp<{ items: BalanceLog[]; total: number; page: number; size: number }>>(
    `/user/balance-logs?page=${page}&size=${size}`,
  )
}

// 管理端：礼品卡列表
export function getAdminGiftCards(params: { page?: number; size?: number; status?: string; search?: string }) {
  const q = new URLSearchParams()
  if (params.page) q.set('page', String(params.page))
  if (params.size) q.set('size', String(params.size))
  if (params.status) q.set('status', params.status)
  if (params.search) q.set('search', params.search)
  return http.get<ApiResp<{ items: GiftCard[]; total: number; page: number; size: number }>>(
    `/admin/gift-cards?${q.toString()}`,
  )
}

// 管理端：批量生成礼品卡
export function batchCreateGiftCards(payload: {
  count: number
  name?: string
  face_value_cents: number
  expires_at?: string
}) {
  return http.post<ApiResp<{ items: GiftCard[]; count: number }>>('/admin/gift-cards', payload)
}

// 管理端：删除/作废礼品卡
export function deleteGiftCard(id: number) {
  return http.delete<ApiResp<{ deleted: number }>>(`/admin/gift-cards/${id}`)
}

// 管理端：调整用户余额
export function adjustUserBalance(userId: number, payload: { amount_cents: number; remark?: string }) {
  return http.post<ApiResp<{ user_id: number; new_balance: number; balance_cents: number }>>(
    `/admin/users/${userId}/balance`,
    payload,
  )
}
