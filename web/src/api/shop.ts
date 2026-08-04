import { http } from './http'
import type { ApiResp, Order, Plan } from './types'

export function getPublicPlans() {
  return http.get<ApiResp<{ items: Plan[] }>>('/plans')
}

export function createOrder(planId: number) {
  return http.post<ApiResp<{ order: Order }>>('/user/orders', { plan_id: planId })
}

export function getMyOrders() {
  return http.get<ApiResp<{ items: Order[] }>>('/user/orders')
}