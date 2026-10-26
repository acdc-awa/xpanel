<script setup lang="ts">
// 订阅模板：组级模板与模板库共用一个页面（2026-09-15 独立成页，2026-10-26 支持绑定）。
//
// 分层语义：
//   - 权限组两种模式：绑定模板库条目（活引用，改模板即更新所有绑定组）或自定义（空键，读组内副本）；
//   - 在组编辑器里保存内容会把该组自动转回自定义（解除绑定）；
//   - 删除被引用的模板时，绑定组会转为自定义并保留模板当前内容（订阅输出不跳变）。
//
// 交互结构：两个标签页共用「左列表 + 右编辑器」骨架。跨层动作只保留方向明确的入口：
// 组级页的「模板来源」选择器（自定义 / 绑定模板）、「另存为模板」。选择模板即建立绑定，
// 不再提供「加载副本」这种不建绑定又会被保存成自定义的隐性入口。
//
// 全页不自动保存：切换到别的权限组/模板、新建、清空、离开页面之前都会先确认，绝不静默丢弃修改。
import { computed, onBeforeUnmount, onMounted, reactive, ref } from 'vue'
import { onBeforeRouteLeave, useRoute } from 'vue-router'
import { ElMessage, ElMessageBox } from '@/components/cloudscape'
import TipIcon from '@/components/base/TipIcon.vue'
import {
  CsBadge,
  CsBox,
  CsButton,
  CsContainer,
  CsInput,
  CsLoading,
  CsPage,
  CsSelect,
  CsTabs,
  CsToolbar,
} from '@/components/cloudscape'
import {
  bindPermissionGroupSubTemplate,
  clearPermissionGroupSubTemplate,
  createSubTemplate,
  deleteSubTemplate,
  getPermissionGroups,
  getSubTemplates,
  previewPermissionGroupTemplate,
  updatePermissionGroup,
  updateSubTemplate,
  type PermissionGroup,
  type SubTemplate,
  type TemplatePreviewResult,
} from '@/api/admin'
import { errMsg } from '@/api/http'
import { formatDateTime } from '@/utils/timezone'
import CodeEditor from '@/components/CodeEditor.vue'

const route = useRoute()

const activeTab = ref<'groups' | 'library'>('groups')
const outerTabs = [
  { id: 'groups', label: '组级订阅模板' },
  { id: 'library', label: '我的模板库' },
]
const editorTabs = [
  { id: 'edit', label: '模板代码 (YAML)' },
  { id: 'preview', label: '实时编译预览 (Preview)' },
]

// ==================== 未保存修改闸门 ====================
// 所有会覆盖编辑缓冲的动作都先过这里：不自动保存，但也不静默丢弃。
async function allowDiscard(dirty: boolean, message: string): Promise<boolean> {
  if (!dirty) return true
  try {
    await ElMessageBox.confirm(message, '有未保存的修改', {
      type: 'warning',
      confirmButtonText: '放弃修改',
      cancelButtonText: '返回编辑',
    })
    return true
  } catch {
    return false
  }
}

// ==================== 组级模板编辑器状态 ====================
const templateCode = ref('')
const templateSaving = ref(false)
const editorTab = ref('edit')
const previewLoading = ref(false)
const previewData = ref<TemplatePreviewResult | null>(null)
const groupEditorRef = ref<InstanceType<typeof CodeEditor> | null>(null)
// 编辑器实时校验结果：保存前据此提示，避免把语法错误的模板下发给客户端
const groupLint = reactive({ errors: 0, warnings: 0 })
const previewLint = reactive({ errors: 0, warnings: 0 })
// 切换权限组/刷新模板库时正在加载生效内容：此窗口内不参与脏判定，避免闪一下「未保存」。
const editorSyncing = ref(false)

function clearLint() {
  groupLint.errors = 0
  groupLint.warnings = 0
  previewLint.errors = 0
  previewLint.warnings = 0
}

// ==================== 权限组（组级模板的归属目标） ====================
const groups = ref<PermissionGroup[]>([])
const groupsLoading = ref(false)
const groupKeyword = ref('')
const selectedGroupId = ref<number | undefined>(undefined)

const selectedGroup = computed(() => groups.value.find((g) => g.id === selectedGroupId.value))

// 绑定态以服务端解析结果为准：template_mode=bound 才表示活引用；
// 悬挂绑定（模板行已不存在）服务端会报 custom，这里同步显示为自定义。
const isGroupBound = (g: PermissionGroup | undefined) => {
  if (!g) return false
  if (g.template_mode) return g.template_mode === 'bound' && !!g.sub_template_id
  return !!g.sub_template_id // 兼容旧响应：没有解析字段时按绑定键判断
}
const isBoundGroup = computed(() => isGroupBound(selectedGroup.value))
const boundTemplate = computed(() => {
  const id = selectedGroup.value?.sub_template_id
  return id ? subTemplates.value.find((t) => t.id === id) : undefined
})
// 绑定组编辑器的基线是模板库当前内容（不是 clash_template 里可能过期的快照）。
const liveTemplateContent = computed(() => {
  const g = selectedGroup.value
  if (!g) return ''
  if (isBoundGroup.value && boundTemplate.value) return boundTemplate.value.content
  return g.clash_template || ''
})
// 编辑缓冲与生效内容不一致 = 有未保存修改（加载生效内容期间不判定）
const groupDirty = computed(
  () => !editorSyncing.value && !!selectedGroup.value && templateCode.value !== liveTemplateContent.value
)
// 「模板来源」选择器的值：0 = 自定义（空键），其余 = 绑定模板 ID
const templateSourceValue = computed(() => (isBoundGroup.value ? selectedGroup.value?.sub_template_id ?? 0 : 0))
const bindingOptions = computed(() => {
  const opts: { label: string; value: number }[] = [
    { label: '自定义（不绑定）', value: 0 },
    ...subTemplates.value.map((t) => ({ label: t.name, value: t.id })),
  ]
  // 模板库尚未加载完时，用组数据里的模板名占位，避免选择器显示空白。
  const g = selectedGroup.value
  if (g?.sub_template_id && !subTemplates.value.some((t) => t.id === g.sub_template_id)) {
    opts.splice(1, 0, {
      label: g.sub_template_name || `模板 #${g.sub_template_id}`,
      value: g.sub_template_id,
    })
  }
  return opts
})
const templateSourceLabel = computed(() => {
  const g = selectedGroup.value
  if (!g) return '系统默认'
  if (isGroupBound(g)) return `绑定「${g.sub_template_name || '模板'}」`
  return g.template_source === 'custom' ? '自定义模板' : '系统默认'
})
function groupBadge(g: PermissionGroup): string {
  if (isGroupBound(g)) return `绑定「${g.sub_template_name || '模板'}」`
  return g.template_source === 'custom' ? '自定义' : '系统默认'
}
function groupBadgeColor(g: PermissionGroup): 'blue' | 'grey' {
  return isGroupBound(g) || g.template_source === 'custom' ? 'blue' : 'grey'
}

const filteredGroups = computed(() => {
  const kw = groupKeyword.value.trim().toLowerCase()
  if (!kw) return groups.value
  return groups.value.filter(
    (g) => g.name.toLowerCase().includes(kw) || (g.remark || '').toLowerCase().includes(kw)
  )
})

// 首次加载才消费 ?group= 深链（权限组卡片徽标跳转过来）；之后的刷新（绑定/保存/删模板）
// 只更新数据、保持当前选中与编辑缓冲，绝不走 selectGroup —— 否则一次数据刷新会被渲染成
// 「切换权限组」，还会拿刚过期、尚未重新同步的编辑缓冲误判成「有未保存修改」并弹确认框。
let initialSelectionDone = false

async function loadGroups() {
  groupsLoading.value = true
  try {
    const { data } = await getPermissionGroups()
    if (data.code === 0) {
      groups.value = data.data.items
      if (!initialSelectionDone) {
        initialSelectionDone = true
        const want = Number(route.query.group)
        const target = want && groups.value.some((g) => g.id === want) ? want : groups.value[0]?.id
        if (target) {
          selectedGroupId.value = target
          await syncEditorFromGroup()
        }
        return
      }
      // 刷新：当前组还在就什么都不动（需要换内容的调用方会显式 sync）；
      // 只有该组已被删除时才回落到深链/第一个组。
      if (!groups.value.some((g) => g.id === selectedGroupId.value)) {
        const want = Number(route.query.group)
        selectedGroupId.value =
          want && groups.value.some((g) => g.id === want) ? want : groups.value[0]?.id
        await syncEditorFromGroup()
      }
    } else {
      ElMessage.error(data.message)
    }
  } catch (e) {
    ElMessage.error(errMsg(e, '加载权限组失败'))
  } finally {
    groupsLoading.value = false
  }
}

// 模板库按需加载：绑定组的内容必须来自模板库当前值，不能在库未就绪时先显示快照再替换（会闪烁）。
let libraryPromise: Promise<void> | null = null
const libraryLoaded = ref(false)
async function ensureLibrary() {
  if (libraryLoaded.value) return
  if (!libraryPromise) {
    libraryPromise = loadLibrary().finally(() => {
      libraryPromise = null
    })
  }
  await libraryPromise
}

// 把选中组的「当前生效内容」读进编辑器：绑定组读模板库当前内容，自定义组读组内副本。
async function syncEditorFromGroup() {
  const g = selectedGroup.value
  if (!g) {
    templateCode.value = ''
    return
  }
  editorSyncing.value = true
  try {
    if (isGroupBound(g)) {
      await ensureLibrary()
      const tpl = subTemplates.value.find((t) => t.id === g.sub_template_id)
      if (tpl) {
        templateCode.value = tpl.content
        return
      }
    }
    templateCode.value = g.clash_template || ''
  } finally {
    editorSyncing.value = false
  }
}

// 选中权限组 = 把该组生效模板读进编辑器。再次点击当前组则从服务端重新加载（放弃修改的退路）。
async function selectGroup(id: number) {
  if (id === selectedGroupId.value && !groupDirty.value) return
  const message =
    id === selectedGroupId.value
      ? '重新加载会丢弃当前编辑器中未保存的修改，回到服务端已存的模板。'
      : '切换权限组后，当前编辑器中未保存的模板修改会丢失。'
  if (!(await allowDiscard(groupDirty.value, message))) return
  selectedGroupId.value = id
  await syncEditorFromGroup()
  editorTab.value = 'edit'
  previewData.value = null
  clearLint()
}

onMounted(() => {
  window.addEventListener('beforeunload', onBeforeUnload)
  loadGroups()
  ensureLibrary()
})

// 离开页面 / 刷新：有未保存修改时拦一道（内容不会自动保存）
onBeforeRouteLeave(async () => {
  if (!unsaved.value) return true
  try {
    await ElMessageBox.confirm(
      '本页有未保存的模板修改，离开后将丢失（模板编辑不会自动保存）。',
      '有未保存的修改',
      { type: 'warning', confirmButtonText: '放弃并离开', cancelButtonText: '留在本页' }
    )
    return true
  } catch {
    return false
  }
})

function onBeforeUnload(e: BeforeUnloadEvent) {
  if (!unsaved.value) return
  e.preventDefault()
  e.returnValue = ''
}

onBeforeUnmount(() => window.removeEventListener('beforeunload', onBeforeUnload))

// ==================== 组模板编辑动作 ====================
function insertPlaceholder(placeholder: string) {
  // 交给编辑器在光标处插入（替换选中内容），不再从 DOM 反查 textarea 选区
  groupEditorRef.value?.insertAtCursor(placeholder)
}

// 清空 = 把该组退回「未自定义」，订阅时走系统内置默认模板
async function clearTemplate() {
  if (!(await allowDiscard(groupDirty.value, '清空会丢弃当前编辑器中未保存的修改。'))) return
  templateCode.value = ''
  previewData.value = null
  editorTab.value = 'edit'
  clearLint()
  ElMessage.info(
    isBoundGroup.value
      ? '已清空编辑内容；保存后该组会转为自定义并使用系统默认模板'
      : '已清空模板（将使用系统内置默认模板），保存到权限组后生效'
  )
}

async function fetchPreview() {
  if (!selectedGroup.value) return
  previewLoading.value = true
  try {
    const { data } = await previewPermissionGroupTemplate(selectedGroup.value.id, templateCode.value)
    if (data.code === 0) previewData.value = data.data
    else ElMessage.error(data.message)
  } catch (e) {
    ElMessage.error(errMsg(e, '编译预览失败'))
  } finally {
    previewLoading.value = false
  }
}

function onTabChange(detail: { activeTabId: string }) {
  if (detail.activeTabId === 'preview') fetchPreview()
}

async function saveTemplate() {
  if (!selectedGroup.value) {
    ElMessage.warning('请先选择目标权限组')
    return
  }
  const target = selectedGroup.value
  const wasBound = isBoundGroup.value
  // 绑定组内容未修改时不允许保存：否则点一下保存就静默解绑。
  if (wasBound && !groupDirty.value) return
  // 后端下发前只做占位符字符串替换，不校验 YAML：语法错误会静默发给客户端导致其无法解析，
  // 故保存前先拦一道（确认后仍可保存，兼容「故意写非标准 YAML 由客户端容忍」的用法）。
  // 同步取一次校验结果，避免依赖防抖后的计数造成漏判。
  const lintNow = groupEditorRef.value?.validate()
  const errCount = lintNow ? lintNow.errors : groupLint.errors
  if (errCount > 0) {
    const detail = lintNow?.messages.length ? `\n首条：${lintNow.messages[0]}` : ''
    try {
      await ElMessageBox.confirm(
        `当前模板有 ${errCount} 处 YAML 语法错误（编辑器内已标红）。保存后客户端可能无法解析该订阅，确认保存？${detail}`,
        '模板存在语法错误',
        { type: 'error', confirmButtonText: '仍然保存', cancelButtonText: '返回修改' }
      )
    } catch {
      return
    }
  }
  try {
    await ElMessageBox.confirm(
      wasBound
        ? `当前权限组绑定「${target.sub_template_name || '模板'}」。保存后会自动转为自定义模板（解除绑定），此后模板库改动不再影响该组。确认保存？`
        : `将当前模板保存到权限组「${target.name}」？保存后该组用户请求订阅时直接返回此配置。`,
      wasBound ? '保存并解除绑定' : '保存订阅模板',
      { type: 'warning', confirmButtonText: wasBound ? '保存并转为自定义' : '保存' }
    )
  } catch {
    return
  }
  templateSaving.value = true
  try {
    const { data } = await updatePermissionGroup(target.id, { clash_template: templateCode.value })
    if (data.code === 0) {
      ElMessage.success(wasBound ? '已转为组内自定义模板（绑定已解除）' : '订阅模板已保存')
      await loadGroups()
      await syncEditorFromGroup()
    } else {
      ElMessage.error(data.message)
    }
  } catch (e) {
    ElMessage.error(errMsg(e, '保存模板失败'))
  } finally {
    templateSaving.value = false
  }
}

// ==================== 模板来源（绑定模板 / 自定义空键） ====================
const bindingSaving = ref(false)

// 选择器切换：0 = 转为自定义（服务端先落快照再清键），>0 = 绑定模板库条目。
async function onTemplateSourceChange(value: string | number | (string | number)[] | '') {
  const g = selectedGroup.value
  if (!g) return
  const target = Number(Array.isArray(value) ? value[0] : value)

  if (!target) {
    if (!isBoundGroup.value) return
    if (!(await allowDiscard(groupDirty.value, '转为自定义会丢弃编辑器中未保存的修改。'))) return
    try {
      await ElMessageBox.confirm(
        `将权限组「${g.name}」转为自定义（空键）？当前模板内容会保留为该组的副本，之后模板库改动不再影响它。`,
        '转为自定义模板',
        { type: 'warning', confirmButtonText: '转为自定义', cancelButtonText: '取消' }
      )
    } catch {
      return
    }
    bindingSaving.value = true
    try {
      const { data } = await clearPermissionGroupSubTemplate(g.id)
      if (data.code === 0) {
        ElMessage.success('已转为自定义模板（保留当前内容）')
        await loadGroups()
        await syncEditorFromGroup()
      } else {
        ElMessage.error(data.message)
      }
    } catch (e) {
      ElMessage.error(errMsg(e, '转为自定义失败'))
    } finally {
      bindingSaving.value = false
    }
    return
  }

  const tpl = subTemplates.value.find((t) => t.id === target)
  if (!tpl) return
  if (!(await allowDiscard(groupDirty.value, '绑定模板会覆盖编辑器内容。'))) return
  try {
    await ElMessageBox.confirm(
      `将权限组「${g.name}」绑定到模板「${tpl.name}」？绑定后该组订阅跟随模板库更新；在组编辑器里保存会自动转为自定义。`,
      '绑定订阅模板',
      { type: 'warning', confirmButtonText: '绑定', cancelButtonText: '取消' }
    )
  } catch {
    return
  }
  bindingSaving.value = true
  try {
    const { data } = await bindPermissionGroupSubTemplate(g.id, tpl.id)
    if (data.code === 0) {
      ElMessage.success(`已绑定「${tpl.name}」，该组订阅将跟随模板更新`)
      await loadGroups()
      await syncEditorFromGroup()
    } else {
      ElMessage.error(data.message)
    }
  } catch (e) {
    ElMessage.error(errMsg(e, '绑定模板失败'))
  } finally {
    bindingSaving.value = false
  }
}

async function copyPreview() {
  if (!previewData.value?.rendered) return
  try {
    await navigator.clipboard.writeText(previewData.value.rendered)
    ElMessage.success('预览配置已复制到剪贴板')
  } catch {
    ElMessage.warning('复制失败')
  }
}

// ==================== 我的模板库（可绑定给权限组，改模板影响绑定组） ====================
const subTemplates = ref<SubTemplate[]>([])
const libraryLoading = ref(false)
const librarySaving = ref(false)
const libEditorRef = ref<InstanceType<typeof CodeEditor> | null>(null)
const libLint = reactive({ errors: 0, warnings: 0 })
// 右栏是否打开（选中了某条或正在新建）；模板内容本身放在 libForm
const libPaneOpen = ref(false)
const libForm = reactive<{ id?: number; name: string; content: string }>({
  id: undefined,
  name: '',
  content: '',
})

const libIsNew = computed(() => libForm.id === undefined)
const libOrigin = computed(() => subTemplates.value.find((t) => t.id === libForm.id))

const libDirty = computed(() => {
  if (libForm.id === undefined) return libForm.name.trim() !== '' || libForm.content !== ''
  const o = libOrigin.value
  if (!o) return false
  return libForm.name !== o.name || libForm.content !== o.content
})

// 整页是否有未保存修改（离开页面时用；页内切换按各自作用域分别判定）
const unsaved = computed(() => groupDirty.value || libDirty.value)

async function loadLibrary() {
  libraryLoading.value = true
  const wasClean = !groupDirty.value
  try {
    const { data } = await getSubTemplates()
    if (data.code === 0) {
      subTemplates.value = data.data || []
      libraryLoaded.value = true
      // 模板库刷新后，绑定组若没有未保存修改，编辑器跟随最新模板内容（避免基线漂移成「假脏」）。
      // editorSyncing 时说明已有一次 sync 在途，由它读取更新后的列表，避免递归。
      if (wasClean && !editorSyncing.value && isGroupBound(selectedGroup.value)) {
        await syncEditorFromGroup()
      }
    } else {
      ElMessage.error(data.message)
    }
  } catch (e) {
    ElMessage.error(errMsg(e, '加载模板库失败'))
  } finally {
    libraryLoading.value = false
  }
}

function openLibForm(tpl?: SubTemplate) {
  libPaneOpen.value = true
  libForm.id = tpl?.id
  libForm.name = tpl?.name ?? ''
  libForm.content = tpl?.content ?? ''
  libLint.errors = 0
  libLint.warnings = 0
}

// 回到「未选择」状态（右栏显示空态提示）
function closeLibForm() {
  libPaneOpen.value = false
  libForm.id = undefined
  libForm.name = ''
  libForm.content = ''
  libLint.errors = 0
  libLint.warnings = 0
}

// 点选库条目 = 加载右侧编辑器（同页内，不存在「加载到哪个编辑器」的歧义）
async function selectLibrary(tpl: SubTemplate) {
  if (libForm.id === tpl.id && !libDirty.value) return
  const message =
    libForm.id === tpl.id
      ? '重新加载会丢弃当前模板编辑器中未保存的修改。'
      : '切换模板后，当前模板编辑器中未保存的修改会丢失。'
  if (!(await allowDiscard(libDirty.value, message))) return
  openLibForm(tpl)
}

async function startNewLibrary() {
  if (!(await allowDiscard(libDirty.value, '新建模板后，当前模板编辑器中未保存的修改会丢失。'))) return
  openLibForm()
}

async function saveLibrary() {
  const name = libForm.name.trim()
  if (!name) {
    ElMessage.warning('请填写模板名称')
    return
  }
  // 与组级模板同因：素材里留着语法错误，加载到权限组时会一路带下去
  const lintNow = libEditorRef.value?.validate()
  const errCount = lintNow ? lintNow.errors : libLint.errors
  if (errCount > 0) {
    const detail = lintNow?.messages.length ? `\n首条：${lintNow.messages[0]}` : ''
    try {
      await ElMessageBox.confirm(
        `当前模板有 ${errCount} 处 YAML 语法错误（编辑器内已标红）。绑定到权限组后会原样下发给客户端，确认保存？${detail}`,
        '模板存在语法错误',
        { type: 'error', confirmButtonText: '仍然保存', cancelButtonText: '返回修改' }
      )
    } catch {
      return
    }
  }
  const isNew = libIsNew.value
  const usage = isNew ? 0 : libOrigin.value?.usage_count ?? 0
  if (usage > 0) {
    const names = (libOrigin.value?.group_names || []).join('、')
    try {
      await ElMessageBox.confirm(
        `模板「${name}」正被 ${usage} 个权限组绑定${names ? `（${names}）` : ''}。保存后这些组的下一次订阅请求立即使用新内容，确认保存？`,
        '保存将影响绑定组',
        { type: 'warning', confirmButtonText: '保存并生效', cancelButtonText: '取消' }
      )
    } catch {
      return
    }
  }
  librarySaving.value = true
  try {
    const { data } = isNew
      ? await createSubTemplate({ name, content: libForm.content })
      : await updateSubTemplate(libForm.id as number, { name, content: libForm.content })
    if (data.code === 0) {
      ElMessage.success(isNew ? '已保存到模板库' : '模板已更新')
      // 以服务端返回值回填：新建后 id 生效，脏标记随之消失
      libForm.id = data.data.id
      libForm.name = data.data.name
      libForm.content = data.data.content
      await loadLibrary()
    } else {
      ElMessage.error(data.message)
    }
  } catch (e) {
    ElMessage.error(errMsg(e, '保存模板失败'))
  } finally {
    librarySaving.value = false
  }
}

// 删除：被绑定也允许——服务端先把模板当前内容落进各绑定组的副本，再清空绑定。
async function removeLibrary() {
  const tpl = libOrigin.value
  if (!tpl) return
  const usage = tpl.usage_count ?? 0
  const names = (tpl.group_names || []).join('、')
  try {
    await ElMessageBox.confirm(
      usage > 0
        ? `模板「${tpl.name}」正被 ${usage} 个权限组绑定${names ? `（${names}）` : ''}。删除后这些组将转为自定义模板（保留当前内容），此后模板库改动不再影响它们。此操作不可撤销。`
        : `删除模板库中的「${tpl.name}」？此操作不可撤销。`,
      '删除模板',
      { type: 'error', confirmButtonText: '删除', cancelButtonText: '取消' }
    )
  } catch {
    return
  }
  const usedBySelected = !!selectedGroup.value?.sub_template_id && selectedGroup.value.sub_template_id === tpl.id
  try {
    const { data } = await deleteSubTemplate(tpl.id)
    if (data.code === 0) {
      const detached = data.data.detached_group_count ?? 0
      ElMessage.success(detached > 0 ? `模板已删除，${detached} 个权限组已转为自定义模板` : '模板已删除')
      // 先刷新权限组（拿到解绑后的 mode/内容），再刷新模板库使用计数，避免编辑器先闪回旧快照。
      await loadGroups()
      await loadLibrary()
      if (usedBySelected) await syncEditorFromGroup()
      // 回到「未选择」而不是自动跳到下一条：避免看起来像内容被替换
      closeLibForm()
    } else {
      ElMessage.error(data.message)
    }
  } catch (e) {
    ElMessage.error(errMsg(e, '删除模板失败'))
  }
}

// 把组编辑器当前内容沉淀为模板库条目（组 → 库）。纯写入：不切标签页、不动模板编辑器。
async function saveAsFromEditor() {
  if (!templateCode.value.trim()) {
    ElMessage.warning('当前编辑器内容为空，无需另存')
    return
  }
  const name = await ElMessageBox.prompt('请输入模板名称', '另存为模板', {
    confirmButtonText: '保存',
    cancelButtonText: '取消',
    inputPattern: /\S+/,
    inputErrorMessage: '模板名不能为空',
  })
    .then((r) => r.value as string)
    .catch(() => null)
  if (name === null) return
  try {
    const { data } = await createSubTemplate({ name: name.trim(), content: templateCode.value })
    if (data.code === 0) {
      await loadLibrary()
      ElMessage.success(`已另存为模板库条目「${data.data.name}」，可在「我的模板库」中查看`)
    } else {
      ElMessage.error(data.message)
    }
  } catch (e) {
    ElMessage.error(errMsg(e, '保存模板失败'))
  }
}
</script>

<template>
  <CsPage class="sub-templates-page">
    <CsToolbar>
      <CsBox variant="span" font-size="heading-s" font-weight="bold">
        订阅模板
        <TipIcon
          content="模板决定权限组用户在 Clash / Mihomo 客户端看到的配置骨架。权限组可以在「模板来源」里绑定模板库条目（改模板即更新所有绑定组），也可以使用自定义模板；在组编辑器里保存会自动转为自定义。"
        />
      </CsBox>
    </CsToolbar>

    <CsTabs v-model="activeTab" :tabs="outerTabs" disable-content-paddings>
      <!-- ==================== TAB 1：组级订阅模板（生效位置） ==================== -->
      <div v-show="activeTab === 'groups'" class="tpl-layout">
          <!-- 左：权限组选择 -->
          <CsContainer header="权限组">
            <template #actions>
              <CsInput
                v-model="groupKeyword"
                size="small"
                placeholder="搜索权限组"
                prefix-icon="search"
                clearable
                style="width: 150px"
              />
            </template>

            <div v-if="groupsLoading" class="list-hint">正在加载…</div>
            <div v-else-if="groups.length === 0" class="list-hint">
              尚无权限组，请先在「订阅与财务 · 权限组管理」中创建
            </div>
            <div v-else-if="filteredGroups.length === 0" class="list-hint">没有匹配的权限组</div>
            <div v-else class="pick-list">
              <button
                v-for="g in filteredGroups"
                :key="g.id"
                type="button"
                class="pick-row"
                :class="{ active: g.id === selectedGroupId }"
                @click="selectGroup(g.id)"
              >
                <span class="pick-main">
                  <span class="pick-name">{{ g.name }}</span>
                  <span class="pick-remark">{{ g.remark || '—' }}</span>
                </span>
                <span v-if="g.id === selectedGroupId && groupDirty" class="dirty-mark">未保存</span>
                <CsBadge :color="groupBadgeColor(g)">{{ groupBadge(g) }}</CsBadge>
              </button>
            </div>
            <div class="list-foot">绑定模板的组会跟随模板库更新；再次点击已选中的权限组可放弃修改、重新加载生效模板。</div>
          </CsContainer>

          <!-- 右：模板编辑器 -->
          <CsContainer :header="selectedGroup ? `配置订阅模板 · ${selectedGroup.name}` : '配置订阅模板'">
            <template #actions>
              <CsBadge v-if="editorSyncing" color="grey">加载中</CsBadge>
              <CsBadge v-else-if="groupDirty" color="severity-medium">未保存</CsBadge>
              <CsBadge :color="isBoundGroup || selectedGroup?.template_source === 'custom' ? 'blue' : 'grey'">
                {{ templateSourceLabel }}
              </CsBadge>
            </template>

            <div v-if="!selectedGroup" class="list-hint" style="padding: 60px 0">
              请在左侧选择一个权限组
            </div>

            <template v-else>
              <div class="preset-section">
                <div class="preset-row">
                  <span class="preset-label">模板来源：</span>
                  <CsSelect
                    :model-value="templateSourceValue"
                    :options="bindingOptions"
                    :disabled="bindingSaving || editorSyncing"
                    style="width: 240px"
                    @change="onTemplateSourceChange"
                  />
                  <span v-if="bindingSaving" class="cs-text-secondary" style="font-size: 12px">
                    正在保存…
                  </span>
                  <span v-else class="cs-text-secondary" style="font-size: 12px">
                    选择模板即绑定（模板库改动同步到本组）；在此编辑内容后保存会转为自定义
                  </span>
                </div>
                <div v-if="isBoundGroup" class="bind-hint">
                  当前绑定「{{ selectedGroup.sub_template_name || '模板' }}」：内容随模板库更新；在此编辑并保存会自动转为自定义，此后不再跟随模板库。
                </div>

                <div class="preset-row" style="margin-top: 8px">
                  <span class="preset-label">模板动作：</span>
                  <CsButton @click="saveAsFromEditor">将当前内容另存为模板</CsButton>
                  <CsButton @click="clearTemplate">清空</CsButton>
                </div>

                <div class="preset-row" style="margin-top: 8px">
                  <span class="preset-label">常用占位符：</span>
                  <TipIcon
                    content="$PROXIES$ 自动展开为当前权限组所有可用 VLESS 节点；$ALL_PROXIES$ 展开为全部节点名称；$FILTER_PROXIES(关键词)$ 自动过滤匹配该地区的节点名称（匹配为空时使用默认规则）。占位符按后端支持的写法校验，不会被误判为语法错误。"
                  />
                  <div class="preset-chips">
                    <button type="button" class="preset-chip code" @mousedown.prevent @click="insertPlaceholder('$PROXIES$')">
                      + $PROXIES$（节点池）
                    </button>
                    <button type="button" class="preset-chip code" @mousedown.prevent @click="insertPlaceholder('$ALL_PROXIES$')">
                      + $ALL_PROXIES$（全部节点）
                    </button>
                    <button type="button" class="preset-chip code" @mousedown.prevent @click="insertPlaceholder('$FILTER_PROXIES(关键词)$')">
                      + $FILTER_PROXIES(关键词)$
                    </button>
                    <button type="button" class="preset-chip code" @mousedown.prevent @click="insertPlaceholder('$PANEL_HOST$')">
                      + $PANEL_HOST$（面板防回环）
                    </button>
                  </div>
                </div>
              </div>

              <CsTabs v-model="editorTab" :tabs="editorTabs" disable-content-paddings @change="onTabChange">
                <div v-show="editorTab === 'edit'">
                  <CodeEditor
                    ref="groupEditorRef"
                    v-model="templateCode"
                    height="330px"
                    resizable
                    placeholder="留空则使用系统内置默认模板。填写后将在 proxies 和 proxy-groups 处按占位符注入该权限组的节点。"
                    @lint="(e, w) => { groupLint.errors = e; groupLint.warnings = w }"
                  />
                  <CsAlert
                    v-if="groupLint.errors > 0 || groupLint.warnings > 0"
                    class="lint-banner"
                    :type="groupLint.errors ? 'error' : 'warning'"
                    header="YAML 校验未通过"
                  >
                    {{ groupLint.errors }} 处错误<span v-if="groupLint.warnings">、{{ groupLint.warnings }} 处警告</span>。
                    错误行已在编辑器中标红，悬停可看原因；此处保存会把问题一起下发给客户端。
                  </CsAlert>
                </div>

                <div v-if="editorTab === 'preview'">
                  <CsLoading :loading="previewLoading">
                    <div v-if="previewData" class="preview-header">
                      <div class="preview-stats">
                        <span class="stat-badge">
                          注入节点数：<strong>{{ previewData.proxy_count }}</strong>
                        </span>
                        <span v-if="previewData.is_sample_nodes" class="stat-badge warning">
                          该权限组暂无可用接入点，以下为样例模拟结果
                        </span>
                      </div>
                      <div style="display: flex; gap: 8px">
                        <CsButton icon-name="refresh" @click="fetchPreview">刷新预览</CsButton>
                        <CsButton variant="primary" icon-name="copy" @click="copyPreview">复制预览配置</CsButton>
                      </div>
                    </div>

                    <div v-if="previewData?.proxy_names && previewData.proxy_names.length" class="matched-nodes-box">
                      <span class="box-label">组内可用节点池：</span>
                      <span v-for="name in previewData.proxy_names" :key="name" class="node-chip cs-text-mono">
                        {{ name }}
                      </span>
                    </div>

                    <CodeEditor
                      v-if="previewData"
                      :model-value="previewData.rendered"
                      height="300px"
                      readonly
                      resizable
                      @lint="(e, w) => { previewLint.errors = e; previewLint.warnings = w }"
                    />
                    <div v-else class="list-hint" style="padding: 30px 0">正在编译渲染…</div>
                    <CsAlert v-if="previewLint.errors > 0" class="lint-banner" type="error" header="编译结果不合法">
                      有 {{ previewLint.errors }} 处 YAML 语法错误：模板本身可能没问题，但占位符位置或缩进让最终配置不合法，客户端会解析失败。
                    </CsAlert>
                  </CsLoading>
                </div>
              </CsTabs>

              <div class="save-bar">
                <span class="cs-text-secondary" style="font-size: 12px">
                  <template v-if="isBoundGroup">
                    当前绑定「{{ selectedGroup.sub_template_name || '模板' }}」：保存会把编辑内容写为该组自定义模板并解除绑定
                  </template>
                  <template v-else>
                    保存后权限组「{{ selectedGroup.name }}」的用户请求订阅时将直接返回此配置
                  </template>
                </span>
                <CsButton
                  variant="primary"
                  icon-name="check"
                  :loading="templateSaving"
                  :disabled="editorSyncing || (isBoundGroup && !groupDirty)"
                  @click="saveTemplate"
                >
                  {{ isBoundGroup ? '保存并转为自定义' : '保存到该权限组' }}
                </CsButton>
              </div>
            </template>
          </CsContainer>
        </div>

      <!-- ==================== TAB 2：我的模板库（可绑定给权限组，与 TAB 1 同一套双列骨架） ==================== -->
      <div v-show="activeTab === 'library'" class="tpl-layout">
          <!-- 左：模板列表 -->
          <CsContainer header="模板库">
            <template #info>
              <TipIcon content="模板库条目可被多个权限组绑定：绑定后改模板即更新这些组的订阅。删除模板时，绑定组会转为自定义并保留模板当前内容。" />
            </template>
            <template #actions>
              <CsButton variant="primary" icon-name="add-plus" @click="startNewLibrary">
                新建模板
              </CsButton>
            </template>

            <div v-if="libraryLoading" class="list-hint">正在加载…</div>
            <div v-else-if="subTemplates.length === 0" class="list-hint">
              模板库还是空的。点「新建模板」，或在「组级订阅模板」里把当前内容另存为模板。
            </div>
            <div v-else class="pick-list">
              <button
                v-for="tpl in subTemplates"
                :key="tpl.id"
                type="button"
                class="pick-row"
                :class="{ active: tpl.id === libForm.id }"
                @click="selectLibrary(tpl)"
              >
                <span class="pick-main">
                  <span class="pick-name">{{ tpl.name }}</span>
                  <span class="pick-remark">
                    {{ (tpl.content || '').length }} 字符 · 更新于 {{ formatDateTime(tpl.updated_at) }}
                  </span>
                </span>
                <span v-if="tpl.id === libForm.id && libDirty" class="dirty-mark">未保存</span>
                <CsBadge v-if="tpl.usage_count" color="blue">{{ tpl.usage_count }} 组</CsBadge>
              </button>
            </div>
          </CsContainer>

          <!-- 右：模板编辑器（与组级页共用同一套骨架，不再用弹窗） -->
          <CsContainer
            :header="libPaneOpen ? (libIsNew ? '新建模板' : '编辑模板') : '模板编辑器'"
          >
            <template #info>
              <TipIcon content="与组级模板语法一致：$PROXIES$（节点池）、$ALL_PROXIES$（全部节点名称）、$FILTER_PROXIES(关键词)$（按地区过滤）、$PANEL_HOST$（面板域名）。" />
            </template>
            <template #actions>
              <span v-if="libPaneOpen && libDirty" class="dirty-mark">未保存</span>
              <CsBadge v-if="libOrigin?.usage_count" color="blue">
                被 {{ libOrigin.usage_count }} 个组使用
              </CsBadge>
            </template>

            <div v-if="!libPaneOpen" class="list-hint" style="padding: 60px 0">
              请在左侧选择要编辑的模板，或点「新建模板」
            </div>

            <template v-else>
              <div class="preset-section">
                <div class="preset-row">
                  <span class="preset-label">模板名称：</span>
                  <CsInput
                    v-model="libForm.name"
                    size="small"
                    :maxlength="64"
                    show-word-limit
                    placeholder="如 通用精简模板 / 流媒体分流模板"
                    style="width: 280px"
                  />
                </div>
              </div>

              <CsAlert
                v-if="!libIsNew && (libOrigin?.usage_count ?? 0) > 0"
                class="lint-banner"
                type="warning"
                header="该模板正在被权限组绑定"
              >
                保存后 {{ libOrigin?.usage_count }} 个绑定组的下一次订阅请求立即使用新内容<template
                  v-if="libOrigin?.group_names?.length"
                >（{{ libOrigin.group_names.join('、') }}）</template>。
              </CsAlert>

              <CodeEditor
                ref="libEditorRef"
                v-model="libForm.content"
                height="330px"
                resizable
                placeholder="留空则使用系统内置默认模板"
                @lint="(e, w) => { libLint.errors = e; libLint.warnings = w }"
              />
              <CsAlert
                v-if="libLint.errors > 0 || libLint.warnings > 0"
                class="lint-banner"
                :type="libLint.errors ? 'error' : 'warning'"
                header="YAML 校验未通过"
              >
                {{ libLint.errors }} 处错误<span v-if="libLint.warnings">、{{ libLint.warnings }} 处警告</span>。
                错误行已在编辑器中标红，悬停可看原因。
              </CsAlert>

              <div class="save-bar">
                <span class="cs-text-secondary" style="font-size: 12px">
                  <template v-if="!libIsNew && (libOrigin?.usage_count ?? 0) > 0">
                    保存后立即影响 {{ libOrigin?.usage_count }} 个绑定组的下一次订阅
                  </template>
                  <template v-else>
                    保存只写入模板库，不会改变任何权限组的生效模板
                  </template>
                </span>
                <div style="display: flex; gap: 8px">
                  <CsButton v-if="!libIsNew" icon-name="remove" @click="removeLibrary">删除</CsButton>
                  <CsButton variant="primary" icon-name="check" :loading="librarySaving" @click="saveLibrary">
                    {{ libIsNew ? '保存到模板库' : '保存修改' }}
                  </CsButton>
                </div>
              </div>
            </template>
          </CsContainer>
        </div>
    </CsTabs>
  </CsPage>
</template>

<style scoped lang="scss">

/* ===== 两栏骨架：两个标签页共用（左列表 + 右编辑器） ===== */
.tpl-layout {
  display: grid;
  grid-template-columns: 300px minmax(0, 1fr);
  gap: 14px;
  align-items: start;
}
.pick-list {
  display: flex;
  flex-direction: column;
  gap: 6px;
  max-height: 620px;
  overflow-y: auto;
}
.pick-row {
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: 8px;
  width: 100%;
  text-align: left;
  background: var(--color-background-container-content-mjfil9);
  border: 1px solid var(--color-border-divider-default-diwd4w);
  border-radius: 8px;
  padding: 8px 10px;
  cursor: pointer;
  transition: all 0.18s ease;
  font: inherit;
  &:hover {
    border-color: var(--color-text-link-default-qabnuk);
  }
  &.active {
    border-color: var(--color-text-link-default-qabnuk);
    background: var(--color-background-item-selected-dotkup);
  }
  .pick-main {
    display: flex;
    flex-direction: column;
    gap: 2px;
    min-width: 0;
  }
  .pick-name {
    font-size: 13px;
    font-weight: 600;
    color: var(--color-text-body-default-b2us04);
    overflow: hidden;
    text-overflow: ellipsis;
    white-space: nowrap;
  }
  .pick-remark {
    font-size: 11.5px;
    color: var(--color-text-body-secondary-66ygbb);
    overflow: hidden;
    text-overflow: ellipsis;
    white-space: nowrap;
  }
}
/* 列表行里的「未保存」标记；卡片标题栏改用 CsBadge severity-medium */
.dirty-mark {
  flex: none;
  font-size: 10.5px;
  line-height: 1.5;
  padding: 1px 6px;
  border-radius: 4px;
  background: var(--color-background-alert-warning-021z3n);
  color: var(--color-text-status-warning-fabq6h);
  white-space: nowrap;
}
.list-hint {
  padding: 24px 0;
  text-align: center;
  color: var(--color-text-body-secondary-66ygbb);
  font-size: 13px;
}
.list-foot {
  margin-top: 10px;
  font-size: 11px;
  line-height: 1.5;
  color: var(--color-text-body-secondary-66ygbb);
}
.save-bar {
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: 12px;
  flex-wrap: wrap;
  border-top: 1px solid var(--color-border-divider-default-diwd4w);
  margin-top: 14px;
  padding-top: 12px;
}

/* ===== 编辑器工具条 ===== */
.preset-section {
  background: var(--color-background-cell-shaded-twxsbz);
  border: 1px solid var(--color-border-divider-default-diwd4w);
  border-radius: 8px;
  padding: 10px 14px;
  margin-bottom: 12px;
}
.preset-row {
  display: flex;
  align-items: center;
  gap: 8px;
  flex-wrap: wrap;
}
.preset-label {
  font-size: 12px;
  font-weight: 600;
  color: var(--color-text-link-default-qabnuk);
  white-space: nowrap;
}
.bind-hint {
  margin-top: 8px;
  padding: 6px 10px;
  border-left: 3px solid var(--color-text-link-default-qabnuk);
  background: var(--color-background-item-selected-dotkup);
  border-radius: 4px;
  font-size: 12px;
  line-height: 1.6;
  color: var(--color-text-body-secondary-66ygbb);
}
.preset-chips {
  display: flex;
  gap: 6px;
  flex-wrap: wrap;
}
.preset-chip {
  display: inline-flex;
  align-items: center;
  background: var(--color-background-container-content-mjfil9);
  border: 1px solid var(--color-border-divider-default-diwd4w);
  padding: 3px 8px;
  border-radius: 6px;
  font-size: 11.5px;
  color: var(--color-text-body-default-b2us04);
  cursor: pointer;
  transition: all 0.2s ease;
  &:hover {
    border-color: var(--color-text-link-default-qabnuk);
    color: var(--color-text-link-default-qabnuk);
    background: rgba(99, 102, 241, 0.06);
  }
  &.code {
    font-family: var(--font-family-monospace-842lr2);
    font-size: 11px;
  }
}
/* YAML 校验结果改用 CsAlert；这里只补编辑器与提示条之间的间距 */
.lint-banner {
  margin-top: 8px;
}
.preview-header {
  display: flex;
  justify-content: space-between;
  align-items: center;
  margin-bottom: 8px;
  flex-wrap: wrap;
  gap: 8px;
}
.preview-stats {
  display: flex;
  gap: 8px;
  align-items: center;
}
.stat-badge {
  font-size: 12px;
  background: var(--color-background-cell-shaded-twxsbz);
  border: 1px solid var(--color-border-divider-default-diwd4w);
  padding: 3px 8px;
  border-radius: 6px;
  color: var(--color-text-body-default-b2us04);
  &.warning {
    background: var(--color-background-alert-warning-021z3n);
    border-color: #fde68a;
    color: #92400e;
  }
}
.matched-nodes-box {
  display: flex;
  align-items: center;
  gap: 6px;
  flex-wrap: wrap;
  background: var(--color-background-cell-shaded-twxsbz);
  border: 1px solid var(--color-border-divider-default-diwd4w);
  border-radius: 6px;
  padding: 6px 10px;
  font-size: 11.5px;
}
.box-label {
  color: var(--color-text-body-secondary-66ygbb);
  font-weight: 500;
}
.node-chip {
  background: var(--color-background-container-content-mjfil9);
  border: 1px solid var(--color-border-divider-default-diwd4w);
  border-radius: 4px;
  padding: 1px 6px;
  font-size: 11px;
}

/* ===== 移动端：改为上下堆叠 ===== */
@media (max-width: 900px) {
  .tpl-layout {
    grid-template-columns: 1fr;
  }
  .pick-list {
    max-height: 300px;
  }
}

/* 订阅模板页是双栏（列表 + 编辑器），内容上限比其它页更宽；
   仍复用同一个布局常量，只是在这里放大到 1.36 倍。 */
.sub-templates-page {
  max-width: 1800px;
}
</style>
