<script setup lang="ts">
// CodeEditor —— 带语法高亮 / 行内错误标记 / 分段折叠的代码编辑器（2026-09-17）。
// 支持两种语言：YAML（订阅模板）与 JSON（入站原始配置、路由自定义 Rule JSON）。
//
// 为什么 YAML 校验前要「占位符中性化」：订阅模板不是纯 YAML，里面允许 $PROXIES$、$ALL_PROXIES$、
// $FILTER_PROXIES(关键词)$、$PANEL_HOST$ 等占位符，由后端 subscribe.BuildClashWithTemplate 在
// 下发前做字符串替换（含旧式 {proxies} / {all_proxies} / {filter_proxies(..)} 写法）。
// 直接把模板喂给严格 YAML 解析器会误报：系统预设里 proxies: 之后独占一行的 $PROXIES$，会被当成
// 「跨行隐式键」报错，而它替换成节点列表后完全合法。故校验前先把占位符换成结构等价的空节点
// （独占一行 → "- {}"，行内 → "{}"，标量位置 → 域名），只暴露真正的语法错误。
// 替换不增删换行，因此报错行号与原模板一一对应。JSON 模式不做替换（JSON 字段不透传占位符）。
import { onBeforeUnmount, onMounted, ref, watch } from 'vue'
import { basicSetup } from 'codemirror'
import { yaml as yamlLanguage } from '@codemirror/lang-yaml'
import { json as jsonLanguage } from '@codemirror/lang-json'
import { EditorState, type Extension } from '@codemirror/state'
import { EditorView, placeholder as cmPlaceholder, ViewPlugin, Decoration, type DecorationSet, type ViewUpdate } from '@codemirror/view'
import { HighlightStyle, syntaxHighlighting, indentUnit, syntaxTree } from '@codemirror/language'
import { linter, lintGutter, type Diagnostic } from '@codemirror/lint'
import { tags as t } from '@lezer/highlight'
import { parseDocument } from 'yaml'

const props = withDefaults(
  defineProps<{
    // 可选：绑定源是可选字段（如 RoutingRulePayload.rule_json）时与 el-input 行为一致
    modelValue?: string
    language?: 'yaml' | 'json'
    height?: string
    readonly?: boolean
    placeholder?: string
    // 关闭校验：用于「预览」这类由后端生成的只读内容（默认仍开启，生成的 YAML 同样可能不合法）
    lint?: boolean
  }>(),
  { modelValue: '', language: 'yaml', height: '360px', readonly: false, placeholder: '', lint: true }
)

const emit = defineEmits<{
  (e: 'update:modelValue', v: string): void
  // 校验结果回调：父组件据此在保存前提示（错误数 > 0 时允许确认后继续保存）
  (e: 'lint', errors: number, warnings: number): void
}>()

const host = ref<HTMLDivElement>()
let view: EditorView | null = null
// 由外部 prop 驱动的文档替换不再回吐 update 事件，避免自激循环
let applyingExternal = false

// ==================== YAML：占位符中性化 ====================
// 行内替换用的空节点：{} 在流式上下文（[a, {}] / {a: {}}）与标量位置都合法
const INLINE_PLACEHOLDERS: RegExp[] = [
  /\$FILTER_PROXIES\([^)]*\)\$/g,
  /\{filter_proxies\([^)]*\)\}/g,
  /\$ALL_PROXIES\$/g,
  /\{all_proxies\}/g,
  /\$PROXIES\$/g,
  /\{proxies\}/g,
]
// 标量位置占位符（后端替换为面板域名，本身不带结构）
const SCALAR_PLACEHOLDERS: RegExp[] = [/\$PANEL_HOST\$/g, /\$PANEL_DOMAIN\$/g, /\$SUB_DOMAIN\$/g]
const ALL_PLACEHOLDERS = [...INLINE_PLACEHOLDERS, ...SCALAR_PLACEHOLDERS]

// neutralizePlaceholders 把占位符换成结构等价的 YAML，行数保持不变。
function neutralizePlaceholders(src: string): string {
  const out: string[] = []
  for (const line of src.split('\n')) {
    let stripped = line
    for (const re of ALL_PLACEHOLDERS) stripped = stripped.replace(re, '')
    if (line.trim() !== '' && stripped.trim() === '') {
      // 整行只有占位符：后端会展开成 "- '名称'" 列表项（沿用该行缩进）
      out.push((line.match(/^[ \t]*/) as RegExpMatchArray)[0] + '- {}')
      continue
    }
    let replaced = line
    for (const re of INLINE_PLACEHOLDERS) replaced = replaced.replace(re, '{}')
    for (const re of SCALAR_PLACEHOLDERS) replaced = replaced.replace(re, 'panel.local')
    out.push(replaced)
  }
  return out.join('\n')
}

// ==================== 错误文案 ====================
// 解析错误码 → 中文说明（管理端文案，保留原始 code/原文便于排查）
const YAML_ERROR_HINTS: Record<string, string> = {
  TAB_AS_INDENT: '缩进不能使用 Tab，请改用空格',
  BAD_INDENT: '缩进不一致（同级条目必须对齐，子级需比父级更深）',
  MULTILINE_IMPLICIT_KEY: '键不能跨行，冒号前的键名需写在同一行',
  BLOCK_AS_IMPLICIT_KEY: '这里的键名写法不合法，请检查冒号与缩进',
  MISSING_CHAR: '缺少闭合字符（引号或括号没有成对）',
  DUPLICATE_KEY: '同一层级出现重复的键，后面的会覆盖前面的',
  KEY_OVER_1024_CHARS: '键名过长（超过 1024 字符）',
  UNEXPECTED_TOKEN: '存在无法解析的字符或结构',
  FLOW_MAP_KEY_INDICATOR: '流式映射 {..} 里的键写法有误',
}

function describeYamlError(code: string, fallback: string): string {
  const hint = YAML_ERROR_HINTS[code]
  // 无法归类的错误退回解析器原文（含行号），避免误导
  return hint ? `${hint}（${code}）` : fallback
}

// JSON.parse 的报错文案（V8）→ 中文说明。V8 只给英文片段，这里按模式归类。
const JSON_ERROR_HINTS: [RegExp, string][] = [
  [/Unexpected end of JSON input/, 'JSON 不完整：缺少结尾的括号或引号'],
  [/Expected double-quoted property name/, '属性名必须用双引号包裹'],
  [/Expected property name or/, '这里应填写属性名（用双引号包裹）'],
  [/Expected ',' or '}' after property value/, '属性值之后缺少逗号或右花括号'],
  [/Expected ',' or ']' after array element/, '数组元素之后缺少逗号或右方括号'],
  [/Unterminated string/, '字符串缺少结尾的双引号'],
  [/Bad escaped character/, '字符串里的反斜杠转义不合法'],
  [/Bad control character/, '字符串里有未转义的控制字符（换行需写成 \\n）'],
  [/Unexpected non-whitespace character after JSON/, 'JSON 结束之后还有多余内容'],
  [/Unexpected token/, '存在无法解析的字符（常见原因是多写了一个逗号）'],
]

function describeJsonError(raw: string): string {
  for (const [re, hint] of JSON_ERROR_HINTS) if (re.test(raw)) return `${hint}（${raw}）`
  return `JSON 语法错误（${raw}）`
}

// ==================== 定位 ====================
// 偏移量 → 行号（1 起算）
function lineAt(text: string, offset: number): number {
  let line = 1
  const end = Math.min(Math.max(offset, 0), text.length)
  for (let i = 0; i < end; i++) if (text.charCodeAt(i) === 10) line++
  return line
}

// 解析器常在读到下一行（或文件末尾）时才判定前一行没写完，落点是空白行时回退到最近的非空行
function resolveLine(v: EditorView, lines: string[], line: number): number {
  let ln = line
  while (ln > 1 && (lines[ln - 1] ?? '').trim() === '') ln--
  return Math.min(Math.max(ln, 1), v.state.doc.lines)
}

// JSON.parse 的错误消息里取行号：优先 "(line L column C)"，退回 "at position N"，
// 两者都没有时（V8 对部分 token 错误只给片段）用 YAML 解析器定位——JSON 是 YAML 子集，
// 只在「JSON.parse 已判定失败」的前提下用它取位置，不会放宽判定。
function jsonErrorLine(src: string, raw: string): number {
  const lc = raw.match(/\(line (\d+) column (\d+)\)/)
  if (lc) return Number(lc[1])
  const at = raw.match(/at position (\d+)/)
  if (at) return lineAt(src, Number(at[1]))
  try {
    const doc = parseDocument(src, { prettyErrors: false })
    const pos = doc.errors[0]?.pos?.[0]
    if (typeof pos === 'number') return lineAt(src, pos)
  } catch {
    /* 取不到位置就退回首行 */
  }
  return 1
}

// ==================== 诊断构造 ====================
type Diag = { line: number; severity: 'error' | 'warning'; message: string }

function buildYamlDiags(src: string): Diag[] {
  const neutral = neutralizePlaceholders(src)
  let doc
  try {
    doc = parseDocument(neutral, { prettyErrors: false })
  } catch {
    // 解析器自身异常不应打断编辑，交给语法树的 invalid 高亮兜底
    return []
  }
  const out: Diag[] = []
  for (const err of doc.errors) {
    out.push({ line: lineAt(neutral, err.pos[0] ?? 0), severity: 'error', message: describeYamlError(err.code, err.message.split('\n')[0]) })
  }
  for (const warn of doc.warnings) {
    out.push({ line: lineAt(neutral, warn.pos[0] ?? 0), severity: 'warning', message: describeYamlError(warn.code, warn.message.split('\n')[0]) })
  }
  return out
}

function buildJsonDiags(src: string): Diag[] {
  try {
    JSON.parse(src)
    return []
  } catch (e) {
    const raw = e instanceof Error ? e.message : String(e)
    return [{ line: jsonErrorLine(src, raw), severity: 'error', message: describeJsonError(raw) }]
  }
}

function buildDiagnostics(v: EditorView): Diagnostic[] {
  const src = v.state.doc.toString()
  if (src.trim() === '') return []
  const raw = props.language === 'json' ? buildJsonDiags(src) : buildYamlDiags(src)
  const lines = src.split('\n')
  const seen = new Set<string>()
  const diags: Diagnostic[] = []
  for (const d of raw) {
    const ln = resolveLine(v, lines, d.line)
    const key = `${d.severity}:${ln}:${d.message}`
    if (seen.has(key)) continue
    seen.add(key)
    const line = v.state.doc.line(ln)
    diags.push({ from: line.from, to: line.to, severity: d.severity, message: d.message })
  }
  return diags
}

// ==================== 字面量类型着色（仅 YAML） ====================
// lezer 的 YAML 语法把普通标量统一标成 tags.content，数字/布尔/空值无法从语法树标签区分，
// 这里按可见区域补一层标记。JSON 语法本身就区分 Number/True/False/Null，无需此步。
const NUMBER_RE = /^-?(?:0|[1-9]\d*)(?:\.\d+)?(?:[eE][+-]?\d+)?$/
const BOOL_NULL_RE = /^(?:true|false|null|~)$/i
const scalarMark = Decoration.mark({ class: 'cm-lit-number' })
const boolMark = Decoration.mark({ class: 'cm-lit-bool' })

function buildScalarDecorations(v: EditorView): DecorationSet {
  const marks: { from: number; to: number; deco: Decoration }[] = []
  for (const { from, to } of v.visibleRanges) {
    syntaxTree(v.state).iterate({
      from,
      to,
      enter: (node) => {
        // Key/Literal 是键名（已由定义标签着色），跳过
        if (node.name !== 'Literal' || node.node.parent?.name === 'Key') return
        const text = v.state.doc.sliceString(node.from, node.to)
        if (NUMBER_RE.test(text)) marks.push({ from: node.from, to: node.to, deco: scalarMark })
        else if (BOOL_NULL_RE.test(text)) marks.push({ from: node.from, to: node.to, deco: boolMark })
      },
    })
  }
  return marks.length ? Decoration.set(marks.map((m) => m.deco.range(m.from, m.to))) : Decoration.none
}

const scalarTypeHighlighter = ViewPlugin.fromClass(
  class {
    decorations: DecorationSet
    constructor(v: EditorView) {
      this.decorations = buildScalarDecorations(v)
    }
    update(u: ViewUpdate) {
      if (u.docChanged || u.viewportChanged) this.decorations = buildScalarDecorations(u.view)
    }
  },
  { decorations: (plugin) => plugin.decorations }
)

// ==================== 主题 ====================
// 颜色全部取自 CSS 变量（tokens.scss），html.dark 切换后自动跟随，无需重建编辑器。
const highlight = HighlightStyle.define([
  { tag: [t.definition(t.propertyName), t.propertyName], color: 'var(--x-code-key)', fontWeight: '600' },
  { tag: [t.string, t.special(t.string)], color: 'var(--x-code-string)' },
  { tag: t.attributeValue, color: 'var(--x-code-string)' },
  { tag: [t.number, t.bool, t.null, t.atom, t.literal], color: 'var(--x-code-number)' },
  { tag: [t.lineComment, t.blockComment, t.comment], color: 'var(--x-code-comment)', fontStyle: 'italic' },
  { tag: [t.separator, t.punctuation, t.squareBracket, t.brace, t.bracket], color: 'var(--x-code-punct)' },
  { tag: [t.keyword, t.typeName, t.labelName], color: 'var(--x-code-string)' },
  // 语法树里的错误节点：波浪下划线直接标出解析失败的位置
  { tag: t.invalid, color: 'var(--x-code-invalid)', textDecoration: 'underline wavy var(--x-code-invalid)' },
])

const editorTheme = EditorView.theme({
  '&': {
    backgroundColor: 'var(--x-code-bg)',
    color: 'var(--x-code-text)',
    border: '1px solid var(--x-border)',
    borderRadius: '8px',
    fontSize: '12px',
    overflow: 'hidden',
  },
  '&.cm-focused': { outline: 'none', borderColor: 'var(--x-primary)' },
  '.cm-scroller': { fontFamily: 'var(--x-font-mono)', lineHeight: '1.55' },
  '.cm-content': { padding: '8px 0', caretColor: 'var(--x-code-text)' },
  '.cm-gutters': {
    backgroundColor: 'var(--x-code-gutter)',
    color: 'var(--x-text-3)',
    border: 'none',
    borderRight: '1px solid var(--x-border)',
  },
  '.cm-activeLine': { backgroundColor: 'var(--x-code-active-line)' },
  '.cm-activeLineGutter': { backgroundColor: 'var(--x-code-active-line)', color: 'var(--x-text-2)' },
  '&.cm-focused .cm-selectionBackground, .cm-selectionBackground, .cm-content ::selection': {
    backgroundColor: 'var(--x-code-selection)',
  },
  // 字面量着色：用两级选择器压过语法高亮规则（同为单类选择器时会受注入顺序影响）
  '.cm-content .cm-lit-number': { color: 'var(--x-code-number)' },
  '.cm-content .cm-lit-bool': { color: 'var(--x-code-number)', fontWeight: '600' },
  // 行内错误/警告底色（lint 落下的是整行范围）
  '.cm-lintRange-error': {
    backgroundImage: 'none',
    backgroundColor: 'var(--x-code-error-bg)',
    borderBottom: '1px dotted var(--x-code-invalid)',
  },
  '.cm-lintRange-warning': { backgroundImage: 'none', backgroundColor: 'var(--x-code-warn-bg)' },
  '.cm-tooltip': {
    backgroundColor: 'var(--x-card)',
    color: 'var(--x-text)',
    border: '1px solid var(--x-border)',
    borderRadius: '6px',
  },
  '.cm-tooltip-lint': { padding: '4px 8px', fontSize: '12px' },
  '.cm-diagnostic': { paddingLeft: '18px' },
  '.cm-foldPlaceholder': {
    backgroundColor: 'var(--x-fill-2)',
    border: '1px solid var(--x-border)',
    color: 'var(--x-text-2)',
  },
})

function extensions(): Extension[] {
  const isYaml = props.language === 'yaml'
  const list: Extension[] = [
    basicSetup, // 含行号、折叠（分段）、括号匹配、历史、搜索
    isYaml ? yamlLanguage() : jsonLanguage(),
    indentUnit.of('  '),
    syntaxHighlighting(highlight),
    editorTheme,
    EditorView.lineWrapping,
    EditorView.updateListener.of((u) => {
      if (!u.docChanged || applyingExternal) return
      emit('update:modelValue', u.state.doc.toString())
    }),
  ]
  if (isYaml) list.push(scalarTypeHighlighter)
  if (props.placeholder) list.push(cmPlaceholder(props.placeholder))
  if (props.readonly) list.push(EditorState.readOnly.of(true), EditorView.editable.of(false))
  if (props.lint) {
    list.push(
      lintGutter(),
      linter(
        (v) => {
          const diags = buildDiagnostics(v)
          emit('lint', diags.filter((d) => d.severity === 'error').length, diags.filter((d) => d.severity === 'warning').length)
          return diags
        },
        { delay: 400 }
      )
    )
  }
  return list
}

onMounted(() => {
  if (!host.value) return
  view = new EditorView({
    state: EditorState.create({ doc: props.modelValue || '', extensions: extensions() }),
    parent: host.value,
  })
})

onBeforeUnmount(() => {
  view?.destroy()
  view = null
})

// 外部改值（切换权限组 / 载入模板库 / 刷新预览 / 宿主按表单重生成 JSON）时同步进编辑器。
// 内容一致则不动，避免打断光标。
// 整篇替换时显式把光标钳到原偏移：默认映射会把光标推到文末，而宿主（如入站原始 JSON 视图）
// 会在每次输入后按表单重生成整篇文本，若任由光标跳到末尾，用户根本无法在中间继续输入。
watch(
  () => props.modelValue,
  (next) => {
    if (!view) return
    const cur = view.state.doc.toString()
    if (next === cur) return
    const text = next ?? ''
    const caret = view.state.selection.main.head
    applyingExternal = true
    view.dispatch({
      changes: { from: 0, to: cur.length, insert: text },
      selection: { anchor: Math.min(caret, text.length) },
    })
    applyingExternal = false
  }
)

// 光标处插入文本（占位符按钮用），替换当前选区后把光标移到插入内容之后
function insertAtCursor(text: string) {
  if (!view || props.readonly) return
  const { from, to } = view.state.selection.main
  view.dispatch({
    changes: { from, to, insert: text },
    selection: { anchor: from + text.length },
    scrollIntoView: true,
  })
  view.focus()
}

function focus() {
  view?.focus()
}

// validate 同步返回当前校验结果：保存前调用可避免「刚粘贴完、防抖未触发」的窗口
function validate(): { errors: number; warnings: number; messages: string[] } {
  if (!view || !props.lint) return { errors: 0, warnings: 0, messages: [] }
  const diags = buildDiagnostics(view)
  return {
    errors: diags.filter((d) => d.severity === 'error').length,
    warnings: diags.filter((d) => d.severity === 'warning').length,
    messages: diags.filter((d) => d.severity === 'error').map((d) => d.message),
  }
}

defineExpose({ insertAtCursor, focus, validate })
</script>

<template>
  <div ref="host" class="code-editor" :style="{ height }" />
</template>

<style scoped>
.code-editor {
  width: 100%;
}
.code-editor :deep(.cm-editor) {
  height: 100%;
}
.code-editor :deep(.cm-scroller) {
  overflow: auto;
}
</style>
