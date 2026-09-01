/**
 * 面板站点配置（站点设置 + Web Base）。
 * 2026-08-24 四端口拆分：web_base 退役（域名分流由反代承担），程序内全部根路径语义；
 * 生产环境由主控在 index.html 注入 window.__PANEL_SETTINGS__（app_name/logo/stop_register 等），
 * 开发环境由 vite serve，无注入 → 全部兜底默认。
 */
declare global {
  interface Window {
    __PANEL_SETTINGS__?: Record<string, string>
  }
}

/** 站点设置（管理端「设置」页下发；开发环境为空对象）。 */
export const siteSettings: Record<string, string> = window.__PANEL_SETTINGS__ || {}

/** 系统标题（app_name 未配置时兜底）。 */
export const appName: string = siteSettings.app_name || 'Xray 面板'

/** 注册是否关闭（stop_register=1）。 */
export const stopRegister: boolean = siteSettings.stop_register === '1'

/** API 基础路径（面板/API 同一域名，由反代按 /api/* 分流到后端端口）。 */
export const apiBase = '/api/v1'

/** 拼接完整订阅链接（智能兼容自定义对外订阅 URL、路径前缀与面板默认路径）。 */
export function buildSubscribeUrl(token: string, customUrl?: string, customPath?: string): string {
  if (!token) return ''
  let baseOrigin = (customUrl || window.__PANEL_SETTINGS__?.subscribe_url || '').trim()
  if (!baseOrigin) {
    baseOrigin = location.origin
  }
  baseOrigin = baseOrigin.replace(/\/+$/, '')

  let pathPrefix = (customPath || window.__PANEL_SETTINGS__?.subscribe_path || '').trim()
  if (!pathPrefix) {
    pathPrefix = '/sub'
  }
  if (!pathPrefix.startsWith('/')) {
    pathPrefix = `/${pathPrefix}`
  }
  pathPrefix = pathPrefix.replace(/\/+$/, '')

  return `${baseOrigin}${pathPrefix}/${token}`
}
