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
