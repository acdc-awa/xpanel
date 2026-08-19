import { marked } from 'marked'

// 配置 marked 解析选项（遵循标准 GFM 规范，段落自然分段）
marked.setOptions({
  gfm: true,
  breaks: false,
})

/**
 * 将 Markdown 文本安全渲染为 HTML
 */
export function renderMarkdown(content?: string | null): string {
  if (!content) return ''
  try {
    return marked.parse(content) as string
  } catch {
    return content
  }
}
