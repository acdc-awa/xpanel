<script setup lang="ts">
// 订阅模板：从权限组页面独立出来的专门页面（2026-09-15）。
//
// 分层语义（页面内已显式呈现，避免误用）：
//   - 订阅生成只读 PermissionGroup.ClashTemplate 一个字段，因此「组级订阅模板」才是生效位置；
//   - 「我的模板库」是可复用的素材，改动它不会影响任何用户的订阅，必须载入并保存到某个权限组才生效。
import { computed, onMounted, reactive, ref } from 'vue'
import { useRoute } from 'vue-router'
import {
  Check,
  CopyDocument,
  Delete,
  Document,
  Edit,
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

// 基础预设模板（系统推荐起点）
const BASIC_TEMPLATE = `mixed-port: 7890
allow-lan: true
mode: rule
log-level: info
ipv6: false

dns:
  enable: true
  listen: 0.0.0.0:1053
  enhanced-mode: fake-ip
  nameserver:
    - 223.5.5.5
    - 119.29.29.29

proxies:
$PROXIES$

proxy-groups:
  - { name: 节点选择, type: select, proxies: [DIRECT, $ALL_PROXIES$] }
  - { name: 自动选择, type: url-test, url: http://cp.cloudflare.com/generate_204, interval: 300, proxies: [$ALL_PROXIES$] }

rules:
  - 'DOMAIN,$PANEL_HOST$,DIRECT'
  - 'MATCH,节点选择'
`

const activeTab = ref<'groups' | 'library'>('groups')

// ==================== 权限组（组级模板的归属目标） ====================
const groups = ref<PermissionGroup[]>([])
const groupsLoading = ref(false)
const groupKeyword = ref('')
const selectedGroupId = ref<number | undefined>(undefined)

const selectedGroup = computed(() => groups.value.find((g) => g.id === selectedGroupId.value))

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
        selectGroup(want)
      } else if (!selectedGroupId.value && groups.value.length) {
        selectGroup(groups.value[0].id)
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

function selectGroup(id: number) {
  selectedGroupId.value = id
  const g = groups.value.find((x) => x.id === id)
  templateCode.value = g?.clash_template || ''
  editorTab.value = 'edit'
  previewData.value = null
  groupLint.errors = 0
  groupLint.warnings = 0
  previewLint.errors = 0
  previewLint.warnings = 0
  selectedLibraryId.value = undefined
}

onMounted(() => {
  loadGroups()
  loadLibrary()
})

// ==================== 组级模板编辑 ====================
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

function loadPreset(type: 'basic' | 'clear') {
  if (type === 'basic') {
    templateCode.value = BASIC_TEMPLATE
    ElMessage.success('已加载「极简基础模板」')
  } else {
    templateCode.value = ''
    ElMessage.info('已清空模板（将使用系统内置默认模板）')
  }
}

function insertPlaceholder(placeholder: string) {
  // 交给编辑器在光标处插入（替换选中内容），不再从 DOM 反查 textarea 选区
  groupEditorRef.value?.insertAtCursor(placeholder)
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
const selectedLibraryId = ref<number | undefined>(undefined)
const libraryDialogOpen = ref(false)
const libraryEditing = ref<SubTemplate | null>(null)
const librarySaving = ref(false)
const libraryForm = reactive({ name: '', content: '' })

const selectedLibrary = computed(() => subTemplates.value.find((t) => t.id === selectedLibraryId.value))

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

// 把模板库内容载入组级编辑器（载入不等于生效，仍需保存到目标权限组）
function applyToEditor(tpl: SubTemplate) {
  if (!selectedGroup.value) {
    ElMessage.warning('请先在「组级订阅模板」中选择目标权限组')
    activeTab.value = 'groups'
    return
  }
  templateCode.value = tpl.content
  previewData.value = null
  selectedLibraryId.value = tpl.id
  activeTab.value = 'groups'
  editorTab.value = 'edit'
  ElMessage.success(`已载入「${tpl.name}」，保存后对权限组「${selectedGroup.value.name}」生效`)
}

function openLibraryCreate() {
  libraryEditing.value = null
  libraryForm.name = ''
  // 新建时以当前编辑器内容为初值：最常见的用法就是「把正在编的这份存下来」
  libraryForm.content = templateCode.value
  libraryDialogOpen.value = true
}

function openLibraryEdit(tpl: SubTemplate) {
  libraryEditing.value = tpl
  libraryForm.name = tpl.name
  libraryForm.content = tpl.content
  libraryDialogOpen.value = true
}

async function saveLibrary() {
  const name = libraryForm.name.trim()
  if (!name) {
    ElMessage.warning('请填写模板名称')
    return
  }
  librarySaving.value = true
  try {
    const { data } = libraryEditing.value
      ? await updateSubTemplate(libraryEditing.value.id, { name, content: libraryForm.content })
      : await createSubTemplate({ name, content: libraryForm.content })
    if (data.code === 0) {
      ElMessage.success(libraryEditing.value ? '模板已更新' : '已保存到模板库')
      libraryDialogOpen.value = false
      await loadLibrary()
      selectedLibraryId.value = data.data.id
    } else {
      ElMessage.error(data.message)
    }
  } catch (e) {
    ElMessage.error(errMsg(e, '保存模板失败'))
  } finally {
    librarySaving.value = false
  }
}

async function removeLibrary(tpl: SubTemplate) {
  try {
    await ElMessageBox.confirm(
      `删除模板库中的「${tpl.name}」？已保存到权限组的模板不受影响。`,
      '删除模板',
      { type: 'error' }
    )
  } catch {
    return
  }
  try {
    const { data } = await deleteSubTemplate(tpl.id)
    if (data.code === 0) {
      ElMessage.success('模板已删除')
      if (selectedLibraryId.value === tpl.id) selectedLibraryId.value = undefined
      loadLibrary()
    } else {
      ElMessage.error(data.message)
    }
  } catch (e) {
    ElMessage.error(errMsg(e, '删除模板失败'))
  }
}

// 把编辑器当前内容另存为模板库条目（沿用原有快捷用法）
async function saveAsFromEditor() {
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
      ElMessage.success('已保存到模板库')
      await loadLibrary()
      selectedLibraryId.value = data.data.id
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
            <div v-else class="group-pick-list">
              <button
                v-for="g in filteredGroups"
                :key="g.id"
                type="button"
                class="group-pick-row"
                :class="{ active: g.id === selectedGroupId }"
                @click="selectGroup(g.id)"
              >
                <span class="pick-main">
                  <span class="pick-name">{{ g.name }}</span>
                  <span class="pick-remark">{{ g.remark || '—' }}</span>
                </span>
                <span
                  class="x-chip"
                  :class="g.clash_template && g.clash_template.trim() ? 'purple' : 'gray'"
                  style="font-size: 10.5px; flex: none"
                >
                  {{ g.clash_template && g.clash_template.trim() ? '自定义' : '系统默认' }}
                </span>
              </button>
            </div>
          </BaseCard>

          <!-- 右：模板编辑器 -->
          <BaseCard :title="selectedGroup ? `配置订阅模板 · ${selectedGroup.name}` : '配置订阅模板'">
            <template #extra>
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
                  <el-select
                    v-model="selectedLibraryId"
                    placeholder="选择已保存的模板"
                    style="width: 200px"
                    size="small"
                    clearable
                  >
                    <el-option v-for="t in subTemplates" :key="t.id" :label="t.name" :value="t.id" />
                  </el-select>
                  <el-button
                    size="small"
                    type="primary"
                    plain
                    :disabled="!selectedLibrary"
                    @click="selectedLibrary && (templateCode = selectedLibrary.content)"
                  >
                    载入到编辑器
                  </el-button>
                  <el-button size="small" @click="saveAsFromEditor">将当前内容另存为模板</el-button>
                </div>

                <div class="preset-row" style="margin-top: 8px">
                  <span class="preset-label">快捷加载：</span>
                  <div class="preset-chips">
                    <button type="button" class="preset-chip primary" @click="loadPreset('basic')">
                      极简基础模板
                    </button>
                    <button type="button" class="preset-chip danger" @click="loadPreset('clear')">
                      清空（恢复系统默认）
                    </button>
                  </div>
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

      <!-- ==================== TAB 2：我的模板库（素材库） ==================== -->
      <el-tab-pane label="我的模板库" name="library">
        <BaseCard title="模板库">
          <template #extra>
            <el-button type="primary" size="small" @click="openLibraryCreate">
              <el-icon><Plus /></el-icon>&nbsp;新建模板
            </el-button>
          </template>

          <div class="tip-banner" style="margin-bottom: 12px">
            模板库只是可复用素材：在这里新增、改名或修改内容<strong>不会影响任何用户的订阅</strong>。
            需要生效时，请把模板载入到「组级订阅模板」并保存到目标权限组。
          </div>

          <div v-if="libraryLoading" style="padding: 40px 0; text-align: center" class="muted">正在加载…</div>
          <div v-else-if="subTemplates.length === 0" class="list-hint" style="padding: 40px 0">
            模板库还是空的。可在「组级订阅模板」里点「将当前内容另存为模板」，或在此新建。
          </div>
          <div v-else class="library-list">
            <div v-for="tpl in subTemplates" :key="tpl.id" class="library-row">
              <el-icon class="library-icon"><Document /></el-icon>
              <div class="library-main">
                <div class="library-name">{{ tpl.name }}</div>
                <div class="library-meta">
                  {{ (tpl.content || '').length }} 字符 · 更新于 {{ formatDateTime(tpl.updated_at) }}
                </div>
              </div>
              <div class="library-actions">
                <el-button size="small" text type="primary" @click="applyToEditor(tpl)">载入到编辑器</el-button>
                <el-button size="small" text :icon="Edit" @click="openLibraryEdit(tpl)">编辑</el-button>
                <el-button size="small" text type="danger" :icon="Delete" @click="removeLibrary(tpl)" />
              </div>
            </div>
          </div>
        </BaseCard>
      </el-tab-pane>
    </el-tabs>

    <!-- ===== 模板库条目编辑弹窗 ===== -->
    <el-dialog
      v-model="libraryDialogOpen"
      :title="libraryEditing ? '编辑模板' : '新建模板'"
      width="720px"
      :append-to-body="true"
    >
      <el-form label-position="top">
        <el-form-item label="模板名称">
          <el-input v-model="libraryForm.name" placeholder="如 通用精简模板 / 流媒体分流模板" maxlength="64" show-word-limit />
        </el-form-item>
        <el-form-item label="模板内容 (YAML)">
          <CodeEditor v-model="libraryForm.content" height="300px" placeholder="留空则使用系统内置默认模板" />
        </el-form-item>
      </el-form>
      <template #footer>
        <el-button @click="libraryDialogOpen = false">取消</el-button>
        <el-button type="primary" :loading="librarySaving" @click="saveLibrary">保存</el-button>
      </template>
    </el-dialog>
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

/* ===== 左右两栏：权限组选择 + 模板编辑 ===== */
.tpl-layout {
  display: grid;
  grid-template-columns: 300px minmax(0, 1fr);
  gap: 14px;
  align-items: start;
}
.group-pick-list {
  display: flex;
  flex-direction: column;
  gap: 6px;
  max-height: 620px;
  overflow-y: auto;
}
.group-pick-row {
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
.list-hint {
  padding: 24px 0;
  text-align: center;
  color: var(--x-text-3);
  font-size: 13px;
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

/* ===== 模板库列表 ===== */
.library-list {
  display: flex;
  flex-direction: column;
  gap: 8px;
}
.library-row {
  display: flex;
  align-items: center;
  gap: 12px;
  background: var(--x-card, #fff);
  border: 1px solid var(--x-border);
  border-radius: 8px;
  padding: 10px 12px;
}
.library-icon {
  font-size: 18px;
  color: var(--x-primary);
  flex: none;
}
.library-main {
  flex: 1;
  min-width: 0;
}
.library-name {
  font-size: 13.5px;
  font-weight: 600;
  color: var(--x-text);
}
.library-meta {
  font-size: 11.5px;
  color: var(--x-text-3);
  margin-top: 2px;
}
.library-actions {
  display: flex;
  align-items: center;
  gap: 2px;
  flex: none;
}

/* ===== 编辑器与预览（自权限组页迁移） ===== */
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
  &.primary {
    border-color: var(--x-primary);
    color: var(--x-primary);
    background: var(--x-primary-soft);
  }
  &.danger {
    color: var(--x-danger);
    &:hover {
      border-color: var(--x-danger);
      background: var(--x-danger-soft);
    }
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
  .group-pick-list {
    max-height: 300px;
  }
  .library-row {
    flex-wrap: wrap;
  }
  .library-actions {
    width: 100%;
    justify-content: flex-end;
  }
}
</style>
