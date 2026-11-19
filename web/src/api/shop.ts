import { http } from './http'
import type { ApiResp, Order, PaymentChannelInfo, Plan } from './types'

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
    ApiResp<{
      order: Order
      balance_cents: number
      available_methods: string[]
      channels?: PaymentChannelInfo[]
    }>
  >(`/user/orders/${encodeURIComponent(orderNo)}`)
}

export function payOrder(orderNo: string, method: string, code?: string) {
  return http.post<
    ApiResp<{
      order: Order
      paid: boolean
      balance_cents?: number
      flow?: string
      pay_url?: string
      qr_code?: string
      html?: string
    }>
  >(`/user/orders/${encodeURIComponent(orderNo)}/pay`, { method, code })
}

export function getPaymentChannels(purpose?: 'recharge' | 'plan') {
  return http.get<ApiResp<{ items: PaymentChannelInfo[] }>>('/user/payment-channels', {
    params: purpose ? { purpose } : undefined,
  })
}

export function createRecharge(payload: {
  channel: string
  amount_cents?: number
  code?: string
  replace_pending?: boolean
}) {
  return http.post<
    ApiResp<{
      order: Order
      paid: boolean
      face_value_cents?: number
      balance_cents?: number
    }>
  >('/user/recharges', payload)
}

export function cancelOrder(orderNo: string) {
  return http.post<ApiResp<{ order: Order }>>(
    `/user/orders/${encodeURIComponent(orderNo)}/cancel`,
  )
}