#!/usr/bin/env node
/**
 * 全站 Element Plus 彻底退化门禁（取代早期的兼容壳检查）。
 *
 * 扫描 src/ 下的 .vue / .ts / .scss / .css（跳过 vendor），禁止再出现任何：
 *   1. element-plus / @element-plus/* 导入或样式引入
 *   2. <el-*> 模板标签
 *   3. ElMessage / ElMessageBox 残留引用
 *   4. --el-* CSS 变量
 *
 * 用法：node scripts/check-elmessage.mjs
 * 退出码：0=全站已彻底清除；1=存在残留。
 */
import { readFileSync, readdirSync, statSync } from 'node:fs'
import { join, relative, sep } from 'node:path'
import { fileURLToPath } from 'node:url'

const webRoot = join(fileURLToPath(new URL('.', import.meta.url)), '..')
const srcRoot = join(webRoot, 'src')
const SCAN_EXTENSIONS = ['.vue', '.ts', '.scss', '.css']
const SKIP_DIRS = new Set(['node_modules', 'dist', 'vendor'])

const CHECKS = [
  { name: 'element-plus 导入', pattern: /(?:from|import)\s*['"](?:@?element-plus(?:\/[^'"]*)?)['"]/ },
  { name: '<el-*> 组件标签', pattern: /<el-[a-z-]+/ },
  { name: 'ElMessage 残留', pattern: /\bElMessage\b/ },
  { name: 'ElMessageBox 残留', pattern: /\bElMessageBox\b/ },
  { name: '--el-* CSS 变量', pattern: /--el-[a-z0-9-]+/ },
]

function* walk(dir) {
  for (const name of readdirSync(dir)) {
    if (SKIP_DIRS.has(name)) continue
    const path = join(dir, name)
    if (statSync(path).isDirectory()) yield* walk(path)
    else if (SCAN_EXTENSIONS.some((ext) => name.endsWith(ext))) yield path
  }
}

/** 去掉注释（保留换行符以保持行号准确） */
function stripComments(text) {
  const blank = (m) => m.replace(/[^\n]/g, '')
  return text
    .replace(/<!--[\s\S]*?-->/g, blank)
    .replace(/\/\*[\s\S]*?\*\//g, blank)
    .replace(/^\s*\/\/.*$/gm, '')
}

const problems = []
for (const file of walk(srcRoot)) {
  const rel = relative(srcRoot, file).split(sep).join('/')
  const code = stripComments(readFileSync(file, 'utf8'))
  const lines = code.split(/\r?\n/)
  lines.forEach((line, index) => {
    for (const check of CHECKS) {
      if (check.pattern.test(line)) {
        problems.push({ file: rel, line: index + 1, rule: check.name, text: line.trim().slice(0, 100) })
      }
    }
  })
}

if (!problems.length) {
  console.log('OK：全站已彻底清除 Element Plus 依赖与 ElMessage/ElMessageBox 残留')
  process.exit(0)
}

console.error(`发现 ${problems.length} 处 Element Plus / ElMessage 残留：`)
for (const p of problems) {
  console.error(`  src/${p.file}:${p.line}  [${p.rule}]  ${p.text}`)
}
process.exit(1)
