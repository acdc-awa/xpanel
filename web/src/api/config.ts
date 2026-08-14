import { http } from './http'
import type { ApiResp } from './types'

// PublicConfig 公开配置（人机验证 site key 等，无鉴权）。
export interface PublicConfig {
  captcha_enable: boolean
  captcha_type: string
  turnstile_site_key: string
}

export function getPublicConfig() {
  return http.get<ApiResp<PublicConfig>>('/config')
}
