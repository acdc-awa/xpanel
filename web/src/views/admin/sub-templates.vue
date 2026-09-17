<script setup lang="ts">
// 订阅模板：从权限组页面独立出来的专门页面（2026-09-15，2026-09-17 重构交互）。
//
// 分层语义（页面内已显式呈现，避免误用）：
//   - 订阅生成只读 PermissionGroup.ClashTemplate 一个字段，因此「组级订阅模板」才是生效位置；
//   - 「我的模板库」是可复用的素材，改动它不会影响任何用户的订阅，必须加载并保存到某个权限组才生效。
//
// 交互结构：两个标签页共用同一套「左列表 + 右编辑器」骨架——组级页是权限组列表 + 组模板编辑器，
// 模板库页是模板列表 + 模板编辑器。此前模板库用「整宽列表 + 弹窗编辑」，与组级页两套范式并存，
// 且列表行的「载入到编辑器」实际会切走标签页、写入另一个编辑器的缓冲，目标不可见，故一并收掉。
// 跨层动作只保留方向明确的两个入口：组级页「从模板库加载」（库 → 组，取副本写入缓冲）、
// 「将当前内容另存为模板」（组 → 库）。
//
// 全页不自动保存：切换到别的权限组/模板、新建、清空、离开页面之前都会先确认，绝不静默丢弃修改。
import { computed, onBeforeUnmount, onMounted, reactive, ref } from 'vue'
import { onBeforeRouteLeave, useRoute } from 'vue-router'
import {
  ArrowDown,
  Check,
  CopyDocument,
  Delete,
  Plus,
  Refresh,
  Search,
} from '@element-plus/icons-vue'
import { ElMessage, ElMessageBox } from 'element-plus'
import BaseCard from '@/components/base/BaseCard.vue'
import {
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

const hasCustomTemplate = computed(() => !!templateCode.value.trim())

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

// 编辑缓冲与权限组已存模板不一致 = 有未保存修改（保存成功后 loadGroups 会刷新已存值，标记随之消失）
const groupDirty = computed(
  () => !!selectedGroup.value && templateCode.value !== (selectedGroup.value.clash_template || '')
)

const filteredGroups = computed(() => {
  const kw = groupKeyword.value.trim().toLowerCase()
  if (!kw) return groups.value
  return groups.value.filter(
    (g) => g.name.toLowerCase().includes(kw) || (g.remark || '').toLowerCase().includes(kw)
  )
})

async function loadGroups() {
  groupsLoading.value = true
  try {
    const { data } = await getPermissionGroups()
    if (data.code === 0) {
      groups.value = data.data.items
      // 深链 ?group=id（权限组卡片徽标跳转过来）；否则默认选中第一个
      const want = Number(route.query.group)
      if (want && groups.value.some((g) => g.id === want)) {
        await selectGroup(want)
      } else if (!selectedGroupId.value && groups.value.length) {
        await selectGroup(groups.value[0].id)
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

// 选中权限组 = 把该组已存模板读进编辑器。再次点击当前组则从服务端重新加载（放弃修改的退路）。
async function selectGroup(id: number) {
  if (id === selectedGroupId.value && !groupDirty.value) return
  const message =
    id === selectedGroupId.value
      ? '重新加载会丢弃当前编辑器中未保存的修改，回到服务端已存的模板。'
      : '切换权限组后，当前编辑器中未保存的模板修改会丢失。'
  if (!(await allowDiscard(groupDirty.value, message))) return
  selectedGroupId.value = id
  templateCode.value = groups.value.find((x) => x.id === id)?.clash_template || ''
  editorTab.value = 'edit'
  previewData.value = null
  clearLint()
}

onMounted(() => {
  window.addEventListener('beforeunload', onBeforeUnload)
  loadGroups()
  loadLibrary()
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
  ElMessage.info('已清空模板（将使用系统内置默认模板），保存到权限组后生效')
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

function onTabChange(tab: any) {
  if (tab === 'preview') fetchPreview()
}

async function saveTemplate() {
  if (!selectedGroup.value) {
    ElMessage.warning('请先选择目标权限组')
    return
  }
  const target = selectedGroup.value
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
      `将当前模板保存到权限组「${target.name}」？保存后该组用户请求订阅时直接返回此配置。`,
      '保存订阅模板',
      { type: 'warning' }
    )
  } catch {
    return
  }
  templateSaving.value = true
  try {
    const { data } = await updatePermissionGroup(target.id, { clash_template: templateCode.value })
    if (data.code === 0) {
      ElMessage.success('订阅模板已保存')
      await loadGroups()
    } else {
      ElMessage.error(data.message)
    }
  } catch (e) {
    ElMessage.error(errMsg(e, '保存模板失败'))
  } finally {
    templateSaving.value = false
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

// ==================== 我的模板库（素材库，不直接生效） ====================
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
  try {
    const { data } = await getSubTemplates()
    if (data.code === 0) subTemplates.value = data.data || []
    else ElMessage.error(data.message)
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
        `当前模板有 ${errCount} 处 YAML 语法错误（编辑器内已标红）。加载到权限组后会原样下发给客户端，确认保存？${detail}`,
        '模板存在语法错误',
        { type: 'error', confirmButtonText: '仍然保存', cancelButtonText: '返回修改' }
      )
    } catch {
      return
    }
  }
  const isNew = libIsNew.value
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

// 删除：删的是模板库素材，已保存到权限组的 clash_template 是副本，不受影响
async function removeLibrary() {
  const tpl = libOrigin.value
  if (!tpl) return
  try {
    await ElMessageBox.confirm(
      `删除模板库中的「${tpl.name}」？已保存到权限组的模板不受影响，此操作不可撤销。`,
      '删除模板',
      { type: 'error', confirmButtonText: '删除', cancelButtonText: '取消' }
    )
  } catch {
    return
  }
  try {
    const { data } = await deleteSubTemplate(tpl.id)
    if (data.code === 0) {
      ElMessage.success('模板已删除')
      await loadLibrary()
      // 回到「未选择」而不是自动跳到下一条：避免看起来像内容被替换
      closeLibForm()
    } else {
      ElMessage.error(data.message)
    }
  } catch (e) {
    ElMessage.error(errMsg(e, '删除模板失败'))
  }
}

// 从模板库取一份副本写入组编辑器（库 → 组）。加载只是填缓冲，保存到权限组才对用户生效。
async function loadFromLibrary(id: number) {
  const tpl = subTemplates.value.find((t) => t.id === id)
  if (!tpl) return
  if (!(await allowDiscard(groupDirty.value, '加载模板会覆盖当前编辑器中未保存的修改。'))) return
  templateCode.value = tpl.content
  previewData.value = null
  editorTab.value = 'edit'
  clearLint()
  ElMessage.success(
    `已加载「${tpl.name}」，保存后对权限组「${selectedGroup.value?.name ?? ''}」生效`
  )
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
  <div class="x-page">
    <div class="x-toolbar">
      <div class="x-toolbar-left">
        <div>
          <div class="page-title">订阅模板</div>
          <div class="muted" style="font-size: 12px; margin-top: 2px">
            模板决定权限组用户在 Clash / Mihomo 客户端看到的配置骨架。模板库是可复用素材，
            只有保存到具体权限组后才会对该组用户的订阅生效。
          </div>
        </div>
      </div>
    </div>

    <el-tabs v-model="activeTab">
      <!-- ==================== TAB 1：组级订阅模板（生效位置） ==================== -->
      <el-tab-pane label="组级订阅模板" name="groups">
        <div class="tpl-layout">
          <!-- 左：权限组选择 -->
          <BaseCard title="权限组">
            <template #extra>
              <el-input
                v-model="groupKeyword"
                size="small"
                placeholder="搜索权限组"
                :prefix-icon="Search"
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
                <span
                  class="x-chip"
                  :class="g.clash_template && g.clash_template.trim() ? 'purple' : 'gray'"
                  style="font-size: 10.5px; flex: none"
                >
                  {{ g.clash_template && g.clash_template.trim() ? '自定义' : '系统默认' }}
                </span>
              </button>
            </div>
            <div class="list-foot">再次点击已选中的权限组可放弃修改、重新加载服务端已存的模板。</div>
          </BaseCard>

          <!-- 右：模板编辑器 -->
          <BaseCard :title="selectedGroup ? `配置订阅模板 · ${selectedGroup.name}` : '配置订阅模板'">
            <template #extra>
              <span v-if="groupDirty" class="dirty-mark">未保存</span>
              <span class="x-chip" :class="hasCustomTemplate ? 'purple' : 'gray'" style="font-size: 10.5px">
                {{ hasCustomTemplate ? '自定义模板' : '系统默认模板' }}
              </span>
            </template>

            <div v-if="!selectedGroup" class="list-hint" style="padding: 60px 0">
              请在左侧选择一个权限组
            </div>

            <template v-else>
              <div class="preset-section">
                <div class="preset-row">
                  <span class="preset-label">模板库：</span>
                  <el-dropdown trigger="click" @command="loadFromLibrary">
                    <el-button size="small" plain>
                      从模板库加载<el-icon class="el-icon--right"><ArrowDown /></el-icon>
                    </el-button>
                    <template #dropdown>
                      <el-dropdown-menu>
                        <el-dropdown-item v-for="t in subTemplates" :key="t.id" :command="t.id">
                          {{ t.name }}
                        </el-dropdown-item>
                        <el-dropdown-item v-if="subTemplates.length === 0" disabled>
                          模板库为空
                        </el-dropdown-item>
                      </el-dropdown-menu>
                    </template>
                  </el-dropdown>
                  <el-button size="small" @click="saveAsFromEditor">将当前内容另存为模板</el-button>
                  <el-button size="small" type="danger" plain @click="clearTemplate">
                    清空（恢复系统默认）
                  </el-button>
                </div>

                <div class="preset-row" style="margin-top: 8px">
                  <span class="preset-label">常用占位符：</span>
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

              <el-tabs v-model="editorTab" @tab-change="onTabChange">
                <el-tab-pane label="模板代码 (YAML)" name="edit">
                  <CodeEditor
                    ref="groupEditorRef"
                    v-model="templateCode"
                    height="330px"
                    placeholder="留空则使用系统内置默认模板。填写后将在 proxies 和 proxy-groups 处按占位符注入该权限组的节点。"
                    @lint="(e, w) => { groupLint.errors = e; groupLint.warnings = w }"
                  />
                  <div v-if="groupLint.errors > 0 || groupLint.warnings > 0" class="lint-banner" :class="groupLint.errors ? 'error' : 'warn'">
                    <span>
                      YAML 校验：{{ groupLint.errors }} 处错误<span v-if="groupLint.warnings">、{{ groupLint.warnings }} 处警告</span>。
                      错误行已在编辑器中标红，悬停可看原因；此处保存会把问题一起下发给客户端。
                    </span>
                  </div>
                  <div class="tip-banner" style="margin-top: 10px">
                    占位符说明：<code>$PROXIES$</code> 自动展开为当前权限组所有可用 VLESS 节点；<code>$ALL_PROXIES$</code> 展开为全部节点名称；<code>$FILTER_PROXIES(关键词)$</code> 自动过滤匹配该地区的节点名称（匹配为空时使用默认规则）。占位符按后端支持的写法校验，不会被误判为语法错误。
                  </div>
                </el-tab-pane>

                <el-tab-pane label="实时编译预览 (Preview)" name="preview" lazy>
                  <div v-loading="previewLoading">
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
                        <el-button size="small" :icon="Refresh" @click="fetchPreview">刷新预览</el-button>
                        <el-button size="small" type="primary" plain :icon="CopyDocument" @click="copyPreview">复制预览配置</el-button>
                      </div>
                    </div>

                    <div v-if="previewData?.proxy_names && previewData.proxy_names.length" class="matched-nodes-box">
                      <span class="box-label">组内可用节点池：</span>
                      <span v-for="name in previewData.proxy_names" :key="name" class="node-chip cell-mono">
                        {{ name }}
                      </span>
                    </div>

                    <CodeEditor
                      v-if="previewData"
                      :model-value="previewData.rendered"
                      height="300px"
                      readonly
                      @lint="(e, w) => { previewLint.errors = e; previewLint.warnings = w }"
                    />
                    <div v-else class="list-hint" style="padding: 30px 0">正在编译渲染…</div>
                    <div v-if="previewLint.errors > 0" class="lint-banner error" style="margin-top: 8px">
                      <span>
                        编译结果有 {{ previewLint.errors }} 处 YAML 语法错误：模板本身可能没问题，但占位符位置或缩进让最终配置不合法，客户端会解析失败。
                      </span>
                    </div>
                  </div>
                </el-tab-pane>
              </el-tabs>

              <div class="save-bar">
                <span class="muted" style="font-size: 12px">
                  保存后权限组「{{ selectedGroup.name }}」的用户请求订阅时将直接返回此配置
                </span>
                <el-button type="primary" :loading="templateSaving" @click="saveTemplate">
                  <el-icon><Check /></el-icon>&nbsp;保存到该权限组
                </el-button>
              </div>
            </template>
          </BaseCard>
        </div>
      </el-tab-pane>

      <!-- ==================== TAB 2：我的模板库（素材库，与 TAB 1 同一套双列骨架） ==================== -->
      <el-tab-pane label="我的模板库" name="library">
        <div class="tpl-layout">
          <!-- 左：模板列表 -->
          <BaseCard title="模板库">
            <template #extra>
              <el-button type="primary" size="small" @click="startNewLibrary">
                <el-icon><Plus /></el-icon>&nbsp;新建模板
              </el-button>
            </template>

            <div class="tip-banner" style="margin-bottom: 10px">
              这里的改动<strong>不会影响任何用户的订阅</strong>：它只是素材，要生效需加载到某个权限组。
            </div>

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
              </button>
            </div>
          </BaseCard>

          <!-- 右：模板编辑器（与组级页共用同一套骨架，不再用弹窗） -->
          <BaseCard :title="libPaneOpen ? (libIsNew ? '新建模板' : '编辑模板') : '模板编辑器'">
            <template #extra>
              <span v-if="libPaneOpen && libDirty" class="dirty-mark">未保存</span>
            </template>

            <div v-if="!libPaneOpen" class="list-hint" style="padding: 60px 0">
              请在左侧选择要编辑的模板，或点「新建模板」
            </div>

            <template v-else>
              <div class="preset-section">
                <div class="preset-row">
                  <span class="preset-label">模板名称：</span>
                  <el-input
                    v-model="libForm.name"
                    size="small"
                    maxlength="64"
                    show-word-limit
                    placeholder="如 通用精简模板 / 流媒体分流模板"
                    style="width: 280px"
                  />
                </div>
              </div>

              <CodeEditor
                ref="libEditorRef"
                v-model="libForm.content"
                height="330px"
                placeholder="留空则使用系统内置默认模板"
                @lint="(e, w) => { libLint.errors = e; libLint.warnings = w }"
              />
              <div v-if="libLint.errors > 0 || libLint.warnings > 0" class="lint-banner" :class="libLint.errors ? 'error' : 'warn'">
                <span>
                  YAML 校验：{{ libLint.errors }} 处错误<span v-if="libLint.warnings">、{{ libLint.warnings }} 处警告</span>。
                  错误行已在编辑器中标红，悬停可看原因。
                </span>
              </div>
              <div class="tip-banner" style="margin-top: 10px">
                与组级模板语法一致：<code>$PROXIES$</code>（节点池）、<code>$ALL_PROXIES$</code>（全部节点名称）、<code>$FILTER_PROXIES(关键词)$</code>（按地区过滤）、<code>$PANEL_HOST$</code>（面板域名）。
              </div>

              <div class="save-bar">
                <span class="muted" style="font-size: 12px">
                  保存只更新这条素材，不会改变任何权限组当前使用的订阅模板
                </span>
                <div style="display: flex; gap: 8px">
                  <el-button v-if="!libIsNew" type="danger" plain @click="removeLibrary">
                    <el-icon><Delete /></el-icon>&nbsp;删除
                  </el-button>
                  <el-button type="primary" :loading="librarySaving" @click="saveLibrary">
                    <el-icon><Check /></el-icon>&nbsp;{{ libIsNew ? '保存到模板库' : '保存修改' }}
                  </el-button>
                </div>
              </div>
            </template>
          </BaseCard>
        </div>
      </el-tab-pane>
    </el-tabs>
  </div>
</template>

<style scoped lang="scss">
.muted { color: var(--x-text-3); }
.cell-mono {
  font-family: var(--x-font-mono, monospace);
  font-size: 12px;
}
.page-title {
  font-size: 16px;
  font-weight: 600;
  color: var(--x-text);
}

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
  background: var(--x-card, #fff);
  border: 1px solid var(--x-border);
  border-radius: 8px;
  padding: 8px 10px;
  cursor: pointer;
  transition: all 0.18s ease;
  font: inherit;
  &:hover {
    border-color: var(--x-primary);
  }
  &.active {
    border-color: var(--x-primary);
    background: var(--x-primary-soft);
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
    color: var(--x-text);
    overflow: hidden;
    text-overflow: ellipsis;
    white-space: nowrap;
  }
  .pick-remark {
    font-size: 11.5px;
    color: var(--x-text-3);
    overflow: hidden;
    text-overflow: ellipsis;
    white-space: nowrap;
  }
}
/* 有未保存修改的标记：列表行与卡片标题栏共用（与 x-chip.orange 同色系） */
.dirty-mark {
  flex: none;
  font-size: 10.5px;
  line-height: 1.5;
  padding: 1px 6px;
  border-radius: 4px;
  background: var(--x-warning-soft);
  color: var(--x-warning);
  white-space: nowrap;
}
.list-hint {
  padding: 24px 0;
  text-align: center;
  color: var(--x-text-3);
  font-size: 13px;
}
.list-foot {
  margin-top: 10px;
  font-size: 11px;
  line-height: 1.5;
  color: var(--x-text-3);
}
.save-bar {
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: 12px;
  flex-wrap: wrap;
  border-top: 1px solid var(--x-border-light);
  margin-top: 14px;
  padding-top: 12px;
}

/* ===== 编辑器工具条 ===== */
.preset-section {
  background: var(--x-bg);
  border: 1px solid var(--x-border);
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
  color: var(--x-primary);
  white-space: nowrap;
}
.preset-chips {
  display: flex;
  gap: 6px;
  flex-wrap: wrap;
}
.preset-chip {
  display: inline-flex;
  align-items: center;
  background: var(--x-card, #fff);
  border: 1px solid var(--x-border);
  padding: 3px 8px;
  border-radius: 6px;
  font-size: 11.5px;
  color: var(--x-text);
  cursor: pointer;
  transition: all 0.2s ease;
  &:hover {
    border-color: var(--x-primary);
    color: var(--x-primary);
    background: rgba(99, 102, 241, 0.06);
  }
  &.code {
    font-family: var(--x-font-mono, monospace);
    font-size: 11px;
  }
}
/* YAML 校验结果提示条（错误红色 / 警告琥珀色，与编辑器行内标记同色系） */
.lint-banner {
  margin-top: 8px;
  font-size: 11.5px;
  line-height: 1.5;
  padding: 6px 10px;
  border-radius: 0 4px 4px 0;
}
.lint-banner.error {
  color: var(--x-danger);
  background: var(--x-code-error-bg);
  border-left: 3px solid var(--x-danger);
}
.lint-banner.warn {
  color: var(--x-warning);
  background: var(--x-code-warn-bg);
  border-left: 3px solid var(--x-warning);
}
.tip-banner {
  font-size: 11.5px;
  color: var(--x-text-2);
  line-height: 1.5;
  background: rgba(99, 102, 241, 0.06);
  border-left: 3px solid var(--x-primary);
  padding: 6px 10px;
  border-radius: 0 4px 4px 0;
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
  background: var(--x-bg);
  border: 1px solid var(--x-border);
  padding: 3px 8px;
  border-radius: 6px;
  color: var(--x-text);
  &.warning {
    background: var(--x-warning-soft);
    border-color: #fde68a;
    color: #92400e;
  }
}
.matched-nodes-box {
  display: flex;
  align-items: center;
  gap: 6px;
  flex-wrap: wrap;
  background: var(--x-bg);
  border: 1px solid var(--x-border);
  border-radius: 6px;
  padding: 6px 10px;
  font-size: 11.5px;
}
.box-label {
  color: var(--x-text-2);
  font-weight: 500;
}
.node-chip {
  background: var(--x-card, #fff);
  border: 1px solid var(--x-border);
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
</style>
