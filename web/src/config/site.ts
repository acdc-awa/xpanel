/**
 * 面板站点配置（Web Base）。
 * 生产环境由主控在 index.html 注入 window.__PANEL_BASE__（如 /panel）；
 * 开发环境由 vite serve，无注入 → 根路径。
 */
declare global {
  interface Window {
    __PANEL_BASE__?: string
  }
}

export const panelBase: string = window.__PANEL_BASE__ || ''

/** API 基础路径（含 web base 前缀）。 */
export const apiBase = `${panelBase}/api/v1`

/** 给路径加 web base 前缀（如 /login → /panel/login）。 */
export function withBase(p: string): string {
  return panelBase ? `${panelBase}${p}` : p
}

/** 拼接完整订阅链接（location.origin + web base + /api/v1/sub/:token）。 */
export function buildSubscribeUrl(token: string): string {
  return `${location.origin}${panelBase}/api/v1/sub/${token}`
}
