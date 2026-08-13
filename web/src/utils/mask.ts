// 敏感信息脱敏工具。
// maskConfigUUIDs：配置预览/下发结果展示时打码 uuid（vless clients id、relay internal_uuid 等），
// 防止用户凭据在界面上泄露。仅用于展示层，不下发、不入库。
const UUID_RE = /[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}/gi

export function maskUUIDs(text: string): string {
  if (!text) return text
  return text.replace(UUID_RE, (m) => `${m.slice(0, 8)}-****-****-****-****`)
}
