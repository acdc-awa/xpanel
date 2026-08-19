import { http } from './http'
import type { ApiResp } from './types'

// PublicConfig 公开配置（站点品牌、人机验证等，无鉴权）。
export interface PublicConfig {
  captcha_enable: boolean
  captcha_type: string
  turnstile_site_key: string
  app_name?: string
  app_description?: string
  logo?: string
  favicon?: string
  stop_register?: boolean
  tos_url?: string
  currency?: string
  currency_symbol?: string
  subscribe_url?: string
  subscribe_path?: string
  subscribe_port?: string
}

export function getPublicConfig() {
  return http.get<ApiResp<PublicConfig>>('/config')
}
