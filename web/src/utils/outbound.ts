/** 系统内置核心出站：由后端 EnsureDefaultServerOutbounds 保证存在且各服务器一份。 */
export const RESERVED_OUTBOUND_TAGS = ['direct', 'blocked'] as const

/**
 * 内置出站的 Tag、协议、形态与启用状态都锁死（后端同样拒绝），
 * 只允许调整内部策略、优先级与备注。
 */
export function isReservedOutboundTag(tag?: string | null): boolean {
  return !!tag && (RESERVED_OUTBOUND_TAGS as readonly string[]).includes(tag)
}
