// 面板自用图标。官方 @cloudscape-design/components 只有 light-dark 半边圆，
// 没有太阳/月亮，主题切换按钮需要随当前模式换图形，故在此单独维护。
// 与 icons.ts 同一套绘制约定：viewBox 16、只写 d，描边/填充由 Icon 组件的 CSS 决定。
import type { CloudscapeIconDefinition } from './icons'

export const APP_ICON_DEFINITIONS: Record<string, CloudscapeIconDefinition> = {
  sun: {
    viewBox: '0 0 16 16',
    inner:
      '<circle cx="8" cy="8" r="3.2"></circle>' +
      '<path d="M8 1v1.6M8 13.4V15M1 8h1.6M13.4 8H15M3.05 3.05l1.13 1.13M11.82 11.82l1.13 1.13M12.95 3.05l-1.13 1.13M3.05 12.95l1.13-1.13" class="stroke-linecap-round"></path>',
  },
  moon: {
    viewBox: '0 0 16 16',
    inner:
      '<path d="M13.2 10.4A5.6 5.6 0 0 1 5.6 2.8 6.2 6.2 0 1 0 13.2 10.4Z" class="stroke-linejoin-round"></path>',
  },
}
