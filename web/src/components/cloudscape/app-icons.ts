// 面板自用图标：补充官方 @cloudscape-design/components 未收录的图标（如 sun/moon/home/shopping-bag）。
// 与 icons.ts 同一套绘制约定：viewBox 16、只写几何路径，描边/填充由 Icon 组件的 CSS 决定。
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
  home: {
    viewBox: '0 0 16 16',
    inner:
      '<path d="M1 7.5 8 1.5l7 6" class="stroke-linejoin-round stroke-linecap-round"></path>' +
      '<path d="M3 6v8.5h10V6" class="stroke-linejoin-round"></path>',
  },
  'shopping-bag': {
    viewBox: '0 0 16 16',
    inner:
      '<path d="M2.5 5.5h11l1 9h-13l1-9Z" class="stroke-linejoin-round"></path>' +
      '<path d="M5.5 5.5V4a2.5 2.5 0 0 1 5 0v1.5" class="stroke-linejoin-round"></path>',
  },
  'send-ne': {
    viewBox: '0 0 16 16',
    inner:
      '<path d="M14.5 1.5 1.5 6.5l5 3 3 5 5-13ZM6.5 9.5l8-8" class="stroke-linejoin-round"></path>',
  },
}
