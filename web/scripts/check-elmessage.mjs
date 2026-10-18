#!/usr/bin/env node
/**
 * 确保 `ElMessage` / `ElMessageBox` 只来自 Cloudscape 兼容壳，
 * 不会被 unplugin-auto-import 静默换回 Element Plus。
 *
 * 背景：vite 的 AutoImport + ElementPlusResolver 会把未显式 import 的这两个 API
 * 自动声明成 Element Plus 的实现。这种写法不报错、类型也对，但渲染出来的是 EP 的
 * 白色方块提示与白底灰边弹窗，与 Cloudscape 组件混在同一页面里。
 * 迁移期间 `ElMessage` 因此漏掉了 165 处，`ElMessageBox` 则是另一批漏网。
 *
 * 用法：node scripts/check-elmessage.mjs
 * 退出码：0=全部走兼容壳；1=存在隐式 Element Plus 用法。
 */
import { readFileSync, readdirSync, statSync } from 'node:fs'
import { join, relative, sep } from 'node:path'
import { fileURLToPath } from 'node:url'

const webRoot = join(fileURLToPath(new URL('.', import.meta.url)), '..')
const srcRoot = join(webRoot, 'src')
const SCAN_EXTENSIONS = ['.vue', '.ts']
const SKIP_DIRS = new Set(['node_modules', 'dist', 'vendor'])
/** 兼容壳自身按定义就是 EP 的替代实现，不参与检查 */
const SKIP_FILES = new Set([
  'components/cloudscape/message.ts',
  'components/cloudscape/toast.ts',
  'components/cloudscape/messagebox.ts',
])
const CLOUDSCAPE_IMPORT = /import\s*(?:type\s*)?\{([^}]*)\}\s*from\s*'@\/components\/cloudscape'/g
/** 两个 API 的调用形态：ElMessage.success(...) / ElMessageBox.confirm(...) */
const CALLS = {
  ElMessage: /\bElMessage\.(?:success|error|warning|info|closeAll|loading)\b/,
  ElMessageBox: /\bElMessageBox\.(?:confirm|prompt|alert|close)\b/,
}

function* walk(dir) {
  for (const name of readdirSync(dir)) {
    if (SKIP_DIRS.has(name)) continue
    const path = join(dir, name)
    if (statSync(path).isDirectory()) yield* walk(path)
    else if (SCAN_EXTENSIONS.some((ext) => name.endsWith(ext))) yield path
  }
}

/** 去掉注释（JS 块/行注释 + 模板里的 HTML 注释），避免把文档里提到的调用当真实用法 */
function stripComments(text) {
  return text
    .replace(/<!--[\s\S]*?-->/g, '')
    .replace(/\/\*[\s\S]*?\*\//g, '')
    .replace(/^\s*\/\/.*$/gm, '')
}

function importedNames(code) {
  const names = new Set()
  for (const match of code.matchAll(CLOUDSCAPE_IMPORT)) {
    for (const part of match[1].split(',')) {
      names.add(part.trim().split(' as ')[0].trim())
    }
  }
  return names
}

const problems = []
for (const file of walk(srcRoot)) {
  const rel = relative(srcRoot, file).split(sep).join('/')
  if (SKIP_FILES.has(rel)) continue
  const code = stripComments(readFileSync(file, 'utf8'))
  const names = importedNames(code)
  for (const [api, pattern] of Object.entries(CALLS)) {
    if (!pattern.test(code) || names.has(api)) continue
    code.split('\n').forEach((line, index) => {
      if (pattern.test(line)) problems.push({ file: rel, line: index + 1, api })
    })
  }
}

if (!problems.length) {
  console.log('OK：所有 ElMessage / ElMessageBox 调用都显式取自 @/components/cloudscape')
  process.exit(0)
}

console.error('发现未显式导入的调用（会被自动注入成 Element Plus 版本）：')
for (const p of problems) console.error(`  src/${p.file}:${p.line}  ${p.api}`)
console.error("\n修法：import { ElMessage, ElMessageBox } from '@/components/cloudscape'")
process.exit(1)
