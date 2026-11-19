import { marked } from 'marked'
import DOMPurify from 'dompurify'

// 配置 marked 解析选项（遵循标准 GFM 规范，段落自然分段）
marked.setOptions({
  gfm: true,
  breaks: false,
})

/**
 * 将 Markdown 文本渲染为可直接交给 v-html 的 HTML。
 *
 * 返回值必须是消毒过的：公告由管理员撰写，但管理员账号一旦被盗/被越权，
 * 未消毒的 HTML 会直接在所有已登录用户（含客户端）的浏览器里执行。
 * 因此出口统一过 DOMPurify，解析失败也不回退成原始 HTML。
 */
export function renderMarkdown(content?: string | null): string {
  if (!content) return ''
  try {
    return DOMPurify.sanitize(marked.parse(content) as string)
  } catch {
    return DOMPurify.sanitize(content)
  }
}

export interface PlanMarkdownVars {
  trafficGb: number
  priceCents: number
  durationDays: number
}

export const DEFAULT_PLAN_MARKDOWN = `### **{{traffic}}** / **{{price}}** / {{duration}}

- 包含 **{{traffic}}** 高速流量
- 有效期 **{{duration}}**
- 全量优质节点与中转链路授权
- 全面兼容 Mihomo / Clash 生态`

function formatPlanTraffic(gb: number): string {
  if (gb >= 1024) return `${+(gb / 1024).toFixed(2)} TB`
  return `${gb} GB`
}

/**
 * 解析套餐描述中的占位符（{{traffic}} / {{price}} / {{duration}}）并渲染为安全 HTML。
 * 描述为空时回退到默认 Markdown 模板；同时兼容历史 $TRAFFIC$ / $DURATION$ 占位符。
 */
export function renderPlanMarkdown(content: string | null | undefined, vars: PlanMarkdownVars): string {
  const raw = content && content.trim() ? content : DEFAULT_PLAN_MARKDOWN
  const trafficText = formatPlanTraffic(vars.trafficGb)
  const priceText = `¥${(vars.priceCents / 100).toFixed(2)}`
  const durationText = `${vars.durationDays} 天`

  const resolved = raw
    .replace(/\{\{\s*traffic\s*\}\}/g, trafficText)
    .replace(/\{\{\s*price\s*\}\}/g, priceText)
    .replace(/\{\{\s*duration\s*\}\}/g, durationText)
    .replace(/\$TRAFFIC\$/g, trafficText)
    .replace(/\$DURATION\$/g, String(vars.durationDays))

  return renderMarkdown(resolved)
}
