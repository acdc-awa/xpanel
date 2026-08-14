/**
 * 面板站点配置（Web Base + 站点设置）。
 * 生产环境由主控在 index.html 注入 window.__PANEL_BASE__（如 /panel）与
 * window.__PANEL_SETTINGS__（app_name/logo/stop_register 等，17 号 P0 ②）；
 * 开发环境由 vite serve，无注入 → 全部兜底默认。
 */
declare global {
  interface Window {
    __PANEL_BASE__?: string
    __PANEL_SETTINGS__?: Record<string, string>
  }
}

export const panelBase: string = window.__PANEL_BASE__ || ''

/** 站点设置（管理端「设置」页下发；开发环境为空对象）。 */
export const siteSettings: Record<string, string> = window.__PANEL_SETTINGS__ || {}

/** 系统标题（app_name 未配置时兜底）。 */
export const appName: string = siteSettings.app_name || 'Xray 面板'

/** 站点 LOGO URL（未配置时用默认图形）。 */
export const siteLogo: string = siteSettings.logo || ''

/** 注册是否关闭（stop_register=1）。 */
export const stopRegister: boolean = siteSettings.stop_register === '1'

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
