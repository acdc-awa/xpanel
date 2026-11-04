#!/usr/bin/env node
/**
 * 设计 token 回归护栏（管理端）。
 *
 * 这里守的都是「改了不会有编译错误、但会让界面重新长回去」的约定：
 *   1. 字号只允许 12 / 13 / 14 / 16 / 20 / 24（拓扑画布是图纸元素，单独放行）；
 *   2. 圆角只允许 4 / 8 / 16 / 999 / 50% / 0；
 *   3. 管理端不许再出现品牌靛紫（主色 = Cloudscape 蓝，见 styles/tokens.scss）；
 *   4. 不允许用「 · 」拼接两段信息，也不允许 Emoji / 变体选择符（改用结构化排版或 CsAlert）。
 *
 * 用法：npm run check:design   —— 命中即列 file:line 并以 1 退出。
 * 例外：行内写 design-allow 注释即可跳过该行。
 */
import { readdirSync, readFileSync, statSync } from 'node:fs'
import { join, relative, sep } from 'node:path'

const ROOT = new URL('../', import.meta.url).pathname.replace(/^\/([A-Za-z]:)/, '$1')

/** 扫描范围：管理端渲染路径（vendor 是第三方快照，不受本护栏约束）。 */
const SCOPE = [
  'src/views/admin',
  'src/layouts/AdminLayout.vue',
  'src/components/base',
  'src/components/cloudscape',
  'src/components/topology',
  'src/components/NotificationBell.vue',
  'src/components/ImageCropperDialog.vue',
  'src/styles',
]

/** 拓扑画布：节点卡片与坐标尺是图纸元素，字号随缩放变化，只查圆角/颜色。 */
const FONT_EXEMPT = ['components/topology/nodes/', 'components/topology/CanvasRulers.vue']

const FONT_SIZES = new Set(['12', '13', '14', '16', '20', '24'])
const RADII = new Set(['4', '8', '16', '999'])
const BANNED_COLORS = [
  '#6366f1',
  '#818cf8',
  '#a855f7',
  '#8b5cf6',
  '#4f46e5',
  '#3730a3',
  '#4338ca',
  'rgba(99, 102, 241',
  'rgba(99,102,241',
]
const EMOJI = /[\u{1F300}-\u{1FAFF}\u{2600}-\u{27BF}\u{FE0F}]/u
const MIDDLE_DOT = / · /

function walk(p, out = []) {
  const abs = join(ROOT, p)
  let st
  try {
    st = statSync(abs)
  } catch {
    return out
  }
  if (st.isFile()) {
    out.push(p)
    return out
  }
  for (const name of readdirSync(abs)) {
    if (name === 'vendor' || name === 'node_modules') continue
    walk(join(p, name).split(sep).join('/'), out)
  }
  return out
}

const files = SCOPE.flatMap((p) => walk(p)).filter((p) => /\.(vue|scss|css|ts)$/.test(p))
const problems = []

function report(file, line, rule, text) {
  problems.push({ file, line, rule, text: text.trim().slice(0, 120) })
}

for (const file of files) {
  const fontExempt = FONT_EXEMPT.some((x) => file.includes(x))
  const lines = readFileSync(join(ROOT, file), 'utf8').split(/\r?\n/)

  lines.forEach((line, i) => {
    if (line.includes('design-allow')) return
    const ln = i + 1

    if (!fontExempt) {
      for (const m of line.matchAll(/font-size:\s*([0-9]+(?:\.[0-9]+)?)px/g)) {
        if (!FONT_SIZES.has(m[1])) report(file, ln, '字号不在 6 档内', line)
      }
    }

    for (const m of line.matchAll(/border-radius:\s*([0-9]+)px/g)) {
      if (!RADII.has(m[1])) report(file, ln, '圆角不在 3 档内', line)
    }

    const lower = line.toLowerCase()
    for (const c of BANNED_COLORS) {
      if (lower.includes(c)) report(file, ln, `品牌靛紫残留 ${c}`, line)
    }

    if (file.endsWith('.vue') && MIDDLE_DOT.test(line) && !line.trim().startsWith('//') && !line.trim().startsWith('/*')) {
      report(file, ln, '用「·」拼接信息（改结构化排版）', line)
    }

    if (file.endsWith('.vue') && EMOJI.test(line) && !line.trim().startsWith('//') && !line.trim().startsWith('/*') && !line.includes('<!--')) {
      report(file, ln, 'Emoji（改用 CsAlert / CsBadge / 图标）', line)
    }
  })
}

if (problems.length === 0) {
  console.log(`check:design 通过（${files.length} 个文件）`)
  process.exit(0)
}

const byFile = new Map()
for (const p of problems) {
  const rel = relative(ROOT, join(ROOT, p.file)).split(sep).join('/')
  if (!byFile.has(rel)) byFile.set(rel, [])
  byFile.get(rel).push(p)
}

console.error(`check:design 发现 ${problems.length} 处不符合约定：\n`)
for (const [file, items] of byFile) {
  console.error(file)
  for (const it of items) console.error(`  ${it.line}  ${it.rule}  |  ${it.text}`)
  console.error('')
}
console.error('规则与例外见 web/scripts/check-design.mjs 头部注释与 master/AGENTS.md「前端」节。')
process.exit(1)
