#!/usr/bin/env node
/**
 * 扫描源码里引用了、但 Cloudscape 官方主题中并不存在的 --color-* 变量。
 *
 * 背景：官方 CSS 的变量名都带哈希后缀（-66ygbb 之类）。写成无后缀的猜名
 * （如 --color-text-body-secondary）不会报错——var() 会静默回退到第二个参数
 * 写死的亮色值，暗色模式下这些位置永远是亮的。这个脚本把这类问题一次性查出来。
 *
 * 用法：node scripts/check-cloudscape-tokens.mjs
 * 退出码：0=没有未定义变量；1=存在未定义变量。
 */
import { readFileSync, readdirSync, statSync } from 'node:fs'
import { join, relative, sep } from 'node:path'
import { fileURLToPath } from 'node:url'

const webRoot = join(fileURLToPath(new URL('.', import.meta.url)), '..')
const srcRoot = join(webRoot, 'src')
const themeFile = join(srcRoot, 'components/cloudscape/vendor/base-theme.css')
const SCAN_EXTENSIONS = ['.vue', '.scss', '.css', '.ts']
const SKIP_DIRS = new Set(['node_modules', 'dist', 'vendor'])

const theme = readFileSync(themeFile, 'utf8')
const defined = new Set([...theme.matchAll(/(--[a-z0-9-]+)\s*:/g)].map((m) => m[1]))

function* walk(dir) {
  for (const name of readdirSync(dir)) {
    if (SKIP_DIRS.has(name)) continue
    const path = join(dir, name)
    if (statSync(path).isDirectory()) yield* walk(path)
    else if (SCAN_EXTENSIONS.some((ext) => name.endsWith(ext))) yield path
  }
}

const problems = []
for (const file of walk(srcRoot)) {
  const text = readFileSync(file, 'utf8')
  for (const match of text.matchAll(/var\((--color-[a-z0-9-]+)/g)) {
    if (defined.has(match[1])) continue
    const line = text.slice(0, match.index).split('\n').length
    problems.push({ file: relative(webRoot, file).split(sep).join('/'), line, name: match[1] })
  }
}

if (!problems.length) {
  console.log(`OK：未发现未定义的颜色变量（主题共 ${defined.size} 个 token）`)
  process.exit(0)
}

console.error(`发现 ${problems.length} 处未定义的颜色变量（var() 会静默回退到亮色 fallback）：`)
for (const p of problems) {
  console.error(`  ${p.file}:${p.line}  ${p.name}`)
}
console.error('\n提示：官方主题里同一语义的变量带哈希后缀，例如 --color-text-body-secondary-66ygbb。')
console.error('用 grep "变量名" src/components/cloudscape/vendor/base-theme.css 找正确写法。')
process.exit(1)
