/**
 * xray 协议名 → 面板展示名。
 *
 * 面板自本版起统一使用 `tunnel`；这里保留 `dokodemo-door` 的映射，
 * 兼容尚未跑完启动迁移的存量行。其余协议原样返回，大小写由调用方决定。
 */
export function protocolDisplayName(protocol?: string): string {
  const p = (protocol || '').toLowerCase()
  return p === 'dokodemo-door' ? 'tunnel' : p
}
