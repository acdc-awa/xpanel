import { http } from './http'
import type { ApiResp, Order, Plan } from './types'

export function getPublicPlans() {
  return http.get<ApiResp<{ items: Plan[] }>>('/plans')
}

export function payOrderByBalance(planId: number) {
  return http.post<ApiResp<{ order: Order }>>('/user/orders/pay-balance', { plan_id: planId })
}

export function getMyOrders() {
  return http.get<ApiResp<{ items: Order[] }>>('/user/orders')
}

export function createOrder(planId: number, replacePending = false) {
  return http.post<ApiResp<{ order: Order }>>('/user/orders', {
    plan_id: planId,
    replace_pending: replacePending,
  })
}

export function getOrder(orderNo: string) {
  return http.get<
    ApiResp<{ order: Order; balance_cents: number; available_methods: string[] }>
  >(`/user/orders/${encodeURIComponent(orderNo)}`)
}

export function payOrder(orderNo: string, method: 'balance' | 'gift_card', code?: string) {
  return http.post<ApiResp<{ order: Order; paid: boolean; balance_cents?: number }>>(
    `/user/orders/${encodeURIComponent(orderNo)}/pay`,
    { method, code },
  )
}

export function cancelOrder(orderNo: string) {
  return http.post<ApiResp<{ order: Order }>>(
    `/user/orders/${encodeURIComponent(orderNo)}/cancel`,
  )
}