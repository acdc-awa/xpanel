import { defineStore } from 'pinia'
import { getPublicConfig, type PublicConfig } from '@/api/config'

export const useSiteStore = defineStore('site', {
  state: () => ({
    appName: window.__PANEL_SETTINGS__?.app_name || 'XrayPanel',
    appDescription: window.__PANEL_SETTINGS__?.app_description || '主控 · 节点 · 用户 一体化代理分发系统',
    logo: window.__PANEL_SETTINGS__?.logo || '',
    favicon: window.__PANEL_SETTINGS__?.favicon || '',
    stopRegister: window.__PANEL_SETTINGS__?.stop_register === '1',
    captchaEnable: false,
    captchaType: 'turnstile',
    turnstileSiteKey: '',
    tosUrl: '',
    currency: 'CNY',
    currencySymbol: '¥',
    subscribeUrl: window.__PANEL_SETTINGS__?.subscribe_url || '',
    subscribePath: window.__PANEL_SETTINGS__?.subscribe_path || '',
    subscribePort: window.__PANEL_SETTINGS__?.subscribe_port || '',
    isLoaded: false,
  }),

  actions: {
    async fetchConfig() {
      try {
        const { data } = await getPublicConfig()
        if (data.code === 0) {
          this.applyConfig(data.data)
        }
      } catch {
        // ignore
      } finally {
        this.isLoaded = true
      }
    },

    applyConfig(cfg: Partial<PublicConfig>) {
      if (cfg.app_name !== undefined) this.appName = cfg.app_name || 'XrayPanel'
      if (cfg.app_description !== undefined) this.appDescription = cfg.app_description || '主控 · 节点 · 用户 一体化代理分发系统'
      if (cfg.logo !== undefined) this.logo = cfg.logo || ''
      if (cfg.favicon !== undefined) this.favicon = cfg.favicon || ''
      if (cfg.stop_register !== undefined) this.stopRegister = !!cfg.stop_register
      if (cfg.captcha_enable !== undefined) this.captchaEnable = !!cfg.captcha_enable
      if (cfg.captcha_type !== undefined) this.captchaType = cfg.captcha_type || 'turnstile'
      if (cfg.turnstile_site_key !== undefined) this.turnstileSiteKey = cfg.turnstile_site_key || ''
      if (cfg.tos_url !== undefined) this.tosUrl = cfg.tos_url || ''
      if (cfg.currency !== undefined) this.currency = cfg.currency || 'CNY'
      if (cfg.currency_symbol !== undefined) this.currencySymbol = cfg.currency_symbol || '¥'
      if (cfg.subscribe_url !== undefined) this.subscribeUrl = cfg.subscribe_url || ''
      if (cfg.subscribe_path !== undefined) this.subscribePath = cfg.subscribe_path || ''
      if (cfg.subscribe_port !== undefined) this.subscribePort = cfg.subscribe_port || ''

      // 同步更新网页 Favicon
      if (this.favicon) {
        let link = document.querySelector("link[rel~='icon']") as HTMLLinkElement | null
        if (!link) {
          link = document.createElement('link')
          link.rel = 'icon'
          document.head.appendChild(link)
        }
        link.href = this.favicon
      }
    },
  },
})
