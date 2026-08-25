import { http } from './http'
import type { ApiResp } from './types'

// PublicConfig 公开配置（站点品牌、人机验证等，无鉴权）。
// 注意：订阅端点信息（subscribe_url/subscribe_path）属内部信息，不在公开面下发，
// 登录后由 /user/me 提供（见 UserInfo.subscribe_url / subscribe_path）。
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
}

export function getPublicConfig() {
  return http.get<ApiResp<PublicConfig>>('/config')
}
