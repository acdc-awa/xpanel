<script setup lang="ts">
import { nextTick, onMounted, reactive, ref } from 'vue'
import TipIcon from '@/components/base/TipIcon.vue'
import {
  CsBadge,
  CsButton,
  CsCard,
  CsCardFact,
  CsCardFacts,
  CsCardGrid,
  CsCheckbox,
  CsContainer,
  CsForm,
  CsFormItem,
  CsIcon,
  CsInput,
  CsInputNumber,
  CsModal,
  CsOption,
  CsSelect,
  CsSpinner,
  CsToggle,
  ElMessage,
  ElMessageBox,
} from '@/components/cloudscape'
import { createPlan, deletePlan, getPermissionGroups, getPlans, updatePlan, type PermissionGroup, type Plan } from '@/api/admin'
import { errMsg } from '@/api/http'

const list = ref<Plan[]>([])
const loading = ref(false)

async function load() {
  loading.value = true
  try {
    const { data } = await getPlans()
    if (data.code === 0) list.value = data.data.items
    else ElMessage.error(data.message)
  } catch (e) {
    ElMessage.error(errMsg(e, '加载套餐失败'))
  } finally {
    loading.value = false
  }
}
onMounted(load)

// Phase T：权限组下拉（套餐自动授权）
const groups = ref<PermissionGroup[]>([])
async function loadGroups() {
  try {
    const { data } = await getPermissionGroups()
    if (data.code === 0) groups.value = data.data.items
  } catch { /* 权限组加载失败不阻塞 */ }
}
onMounted(loadGroups)

const formOpen = ref(false)
const editing = ref(false)
const descInputRef = ref<any>(null)
const form = reactive({
  id: 0,
  name: '',
  description: '',
  price_yuan: 25,
  traffic_gb: 200,
  duration_days: 30,
  device_limit: 0,
  permission_group_id: 0,
  sort_order: 0,
  is_featured: false,
  purchasable: true, // 可新购：商店展示并允许非持有者购买
  renewable: true, // 可续费：持有者余额直付顺延
  sync_users: false, // 编辑保存时是否把新套餐值同步到存量用户（默认关：只影响新购/续费）
})
const saving = ref(false)

const defaultPlanDesc = `包含 $TRAFFIC$ 高速流量
有效期 $DURATION$ 天
$DEVICE_LIMIT$
全量优质节点与中转链路授权
全面兼容 Mihomo / Clash 生态`

function insertPlaceholder(placeholder: string) {
  const el = (descInputRef.value?.textarea ||
    descInputRef.value?.$el?.querySelector('textarea')) as HTMLTextAreaElement | undefined

  if (!el) {
    // 降级：未获取到 textarea 元素时追加到末尾
    form.description += (form.description.endsWith('\n') || !form.description ? '' : '\n') + placeholder
    ElMessage.success(`已插入占位符 ${placeholder}`)
    return
  }

  const start = el.selectionStart ?? form.description.length
  const end = el.selectionEnd ?? form.description.length
  const text = form.description || ''

  const before = text.substring(0, start)
  const after = text.substring(end)
  form.description = before + placeholder + after

  ElMessage.success(`已在光标处插入 ${placeholder}`)

  // 恢复光标至插入内容之后并保持聚焦
  nextTick(() => {
    el.focus()
    const newPos = start + placeholder.length
    el.setSelectionRange(newPos, newPos)
  })
}

function openCreate() {
  editing.value = false
  Object.assign(form, {
    id: 0,
    name: '',
    description: defaultPlanDesc,
    price_yuan: 25,
    traffic_gb: 200,
    duration_days: 30,
    device_limit: 0,
    permission_group_id: 0,
    sort_order: 0,
    is_featured: false,
    purchasable: true,
    renewable: true,
    sync_users: false,
  })
  formOpen.value = true
}

function openEdit(row: any) {
  editing.value = true
  Object.assign(form, {
    id: row.id,
    name: row.name,
    description: row.description || '',
    price_yuan: Number((row.price_cents / 100).toFixed(2)),
    traffic_gb: row.traffic_gb,
    duration_days: row.duration_days,
    device_limit: row.device_limit || 0,
    permission_group_id: row.permission_group_id || 0,
    sort_order: row.sort_order || 0,
    is_featured: !!row.is_featured,
    purchasable: !!row.purchasable,
    renewable: !!row.renewable,
    sync_users: false, // 每次打开默认不勾，避免误触发批量同步
  })
  formOpen.value = true
}

async function save() {
  if (!form.name || form.price_yuan < 0 || form.traffic_gb < 1 || form.duration_days < 1) {
    ElMessage.warning('请填写完整套餐信息')
    return
  }
  saving.value = true
  try {
    const payload: Record<string, any> = {
      name: form.name,
      description: form.description,
      price_cents: Math.round(form.price_yuan * 100),
      traffic_gb: form.traffic_gb,
      duration_days: form.duration_days,
      device_limit: form.device_limit,
      permission_group_id: form.permission_group_id || undefined,
      sort_order: form.sort_order,
      is_featured: form.is_featured,
      purchasable: form.purchasable,
      renewable: form.renewable,
    }
    if (editing.value) payload.sync_users = form.sync_users
    const { data } = editing.value ? await updatePlan(form.id, payload) : await createPlan(payload)
    if (data.code === 0) {
      ElMessage.success(editing.value ? '已保存' : '已创建')
      formOpen.value = false
      load()
    } else {
      ElMessage.error(data.message)
    }
  } catch (e) {
    ElMessage.error(errMsg(e, '保存失败'))
  } finally {
    saving.value = false
  }
}

// 卡片徽标快捷切换（可新购/可续费）
async function toggleFlag(row: any, flag: 'purchasable' | 'renewable') {
  try {
    const { data } = await updatePlan(row.id, { [flag]: !row[flag] })
    if (data.code === 0) load()
  } catch (e) {
    ElMessage.error(errMsg(e, '操作失败'))
  }
}

function priceLabel(cents: number) {
  return `¥ ${(cents / 100).toFixed(2)}`
}

function trafficLabel(gb: number) {
  return gb >= 1024 ? `${(gb / 1024).toFixed(1)} TB` : `${gb} GB`
}

function groupNameOf(id?: number) {
  if (!id) return '未绑定'
  return groups.value.find((g) => g.id === id)?.name || `#${id}`
}

async function remove(row: any) {
  try {
    await ElMessageBox.confirm(`确认删除套餐「${row.name}」？`, '删除套餐', { type: 'error' })
  } catch {
    return
  }
  try {
    const { data } = await deletePlan(row.id)
    if (data.code === 0) {
      ElMessage.success('已删除')
      load()
    }
  } catch (e) {
    ElMessage.error(errMsg(e, '删除失败'))
  }
}
</script>

<template>
  <div class="x-page">
    <div class="x-toolbar">
      <div class="x-toolbar-left">
        <CsButton icon-name="refresh" @click="load">刷新</CsButton>
      </div>
      <CsButton variant="primary" icon-name="add-plus" @click="openCreate">新增套餐</CsButton>
    </div>

    <CsContainer header="套餐列表">
      <div v-if="loading" style="padding: 48px 0; text-align: center">
        <CsSpinner size="large" />
      </div>

      <div v-else-if="list.length === 0" style="text-align: center; padding: 48px 0; color: var(--x-text-3); font-size: 13.5px">
        <CsIcon name="ticket" size="large" />
        <p style="margin-top: 8px">暂无套餐，点击右上角「新增套餐」</p>
      </div>

      <!-- 官方 ItemCard 卡片网格：卡头 16/700，正文 14/20，价格作为卡片主指标 -->
      <CsCardGrid v-else>
        <CsCard v-for="row in list" :key="row.id">
          <template #header>
            <button type="button" class="plan-name" title="点击编辑套餐" @click="openEdit(row)">
              {{ row.name }}
            </button>
            <div class="plan-meta">
              <span class="plan-id">#{{ row.id }}</span>
              <button
                type="button"
                class="plan-flag"
                title="可新购：商店展示并允许非持有者购买，点击切换"
                @click="toggleFlag(row, 'purchasable')"
              >
                <CsBadge :color="row.purchasable ? 'green' : 'grey'">
                  {{ row.purchasable ? '可新购' : '停售' }}
                </CsBadge>
              </button>
              <button
                type="button"
                class="plan-flag"
                title="可续费：持有者余额直付顺延，点击切换"
                @click="toggleFlag(row, 'renewable')"
              >
                <CsBadge :color="row.renewable ? 'blue' : 'grey'">
                  {{ row.renewable ? '可续费' : '停续' }}
                </CsBadge>
              </button>
              <CsBadge v-if="row.is_featured" color="severity-medium">热门推荐</CsBadge>
            </div>
          </template>
          <template #actions>
            <span class="plan-price" :class="{ 'plan-price--muted': !row.purchasable && !row.renewable }">
              {{ priceLabel(row.price_cents) }}
            </span>
          </template>

          <div v-if="row.description" class="card-desc-box">{{ row.description }}</div>

          <CsCardFacts>
            <CsCardFact label="包含流量" :value="trafficLabel(row.traffic_gb)" mono />
            <CsCardFact label="有效期" :value="`${row.duration_days} 天`" mono />
            <CsCardFact label="设备限制" :value="row.device_limit ? `${row.device_limit} 台` : '不限设备'" />
            <CsCardFact label="展示排序" :value="`#${row.sort_order ?? 0}`" mono />
            <CsCardFact label="权限组" :value="groupNameOf(row.permission_group_id)" full-width />
          </CsCardFacts>

          <template #footer>
            <CsButton icon-name="edit" @click="openEdit(row)">编辑套餐</CsButton>
            <CsButton icon-name="remove" @click="remove(row)">删除</CsButton>
          </template>
        </CsCard>
      </CsCardGrid>
    </CsContainer>

    <CsModal v-model:visible="formOpen" :header="editing ? '编辑套餐' : '新增套餐'" size="medium">
      <CsForm label-position="top">
        <CsFormItem label="名称"><CsInput v-model="form.name" placeholder="如 月付 200G" /></CsFormItem>
        <CsFormItem label="套餐文案 / 特性描述（每行一条特性，支持占位符）">
          <div class="placeholder-chips">
            <span class="chip-label">快捷插入占位符：</span>
            <CsBadge color="blue" class="placeholder-tag" @click="insertPlaceholder('$TRAFFIC$')">
              $TRAFFIC$ (流量)
            </CsBadge>
            <CsBadge color="blue" class="placeholder-tag" @click="insertPlaceholder('$DURATION$')">
              $DURATION$ (周期)
            </CsBadge>
            <CsBadge color="blue" class="placeholder-tag" @click="insertPlaceholder('$DEVICE_LIMIT$')">
              $DEVICE_LIMIT$ (设备限制)
            </CsBadge>
          </div>
          <CsInput
            ref="descInputRef"
            v-model="form.description"
            type="textarea"
            :rows="5"
            placeholder="如：&#10;包含 $TRAFFIC$ 高速流量&#10;有效期 $DURATION$ 天&#10;$DEVICE_LIMIT$&#10;全量优质节点与中转链路授权&#10;全面兼容 Mihomo / Clash 生态"
          />
        </CsFormItem>
        <div class="form-grid-2">
          <CsFormItem label="价格（元）">
            <CsInputNumber v-model="form.price_yuan" :min="0" :step="1" :precision="2" style="width: 100%" />
          </CsFormItem>
          <CsFormItem label="有效期（天）">
            <CsInputNumber v-model="form.duration_days" :min="1" style="width: 100%" />
          </CsFormItem>
          <CsFormItem label="流量配额（GB）">
            <CsInputNumber v-model="form.traffic_gb" :min="1" style="width: 100%" />
          </CsFormItem>
          <CsFormItem label="并发设备上限（台，0 为不限）">
            <CsInputNumber v-model="form.device_limit" :min="0" style="width: 100%" />
          </CsFormItem>
          <CsFormItem label="展示排序（越小越靠前）">
            <CsInputNumber v-model="form.sort_order" :min="0" :step="10" style="width: 100%" />
          </CsFormItem>
          <CsFormItem label="热门推荐">
            <div style="display: flex; align-items: center; gap: 8px; width: 100%">
              <CsToggle v-model="form.is_featured" />
              <TipIcon content="商城展示「热门推荐」徽标并高亮（多个标记时仅排最前的生效）" />
            </div>
          </CsFormItem>
        </div>
        <CsFormItem>
          <template #label>
            上架状态
            <TipIcon
              content="商店按身份过滤：非持有者只见「可新购」套餐；持有者额外可见自己「可续费」的当前套餐（用于续费）。停售老套餐但保留续费：关闭「可新购」，保留「可续费」。"
            />
          </template>
          <div style="display: grid; grid-template-columns: 1fr 1fr; gap: 8px 16px; width: 100%">
            <div style="display: flex; align-items: center; gap: 8px">
              <CsToggle v-model="form.purchasable" />
              <span style="font-size: 12.5px">可新购</span>
              <span class="muted" style="font-size: 11.5px">商店展示并允许非持有者购买</span>
            </div>
            <div style="display: flex; align-items: center; gap: 8px">
              <CsToggle v-model="form.renewable" />
              <span style="font-size: 12.5px">可续费</span>
              <span class="muted" style="font-size: 11.5px">持有者余额直付顺延</span>
            </div>
          </div>
        </CsFormItem>
        <CsFormItem label="权限组（选填，购买后自动授权组内入站）">
          <CsSelect v-model="form.permission_group_id" style="width: 100%" clearable placeholder="不绑定">
            <CsOption v-for="g in groups" :key="g.id" :label="g.name" :value="g.id" />
          </CsSelect>
        </CsFormItem>
        <CsFormItem v-if="editing">
          <div style="display: flex; align-items: center; gap: 6px">
            <CsCheckbox v-model="form.sync_users">同步存量用户</CsCheckbox>
            <TipIcon
              type="warn"
              content="默认关闭：本次修改仅影响新购买 / 续费，存量用户按购买时的快照额度与权限组继续使用，直到下次分配或续费。勾选后立即把新的额度 / 设备限制 / 权限组应用到当前所有订阅用户并同步至服务器（超量用户会被即时踢下线）。"
            />
          </div>
        </CsFormItem>
      </CsForm>
      <template #footer>
        <CsButton @click="formOpen = false">取消</CsButton>
        <CsButton variant="primary" :loading="saving" @click="save">保存</CsButton>
      </template>
    </CsModal>
  </div>
</template>

<style scoped lang="scss">
.plan-name {
  appearance: none;
  border: 0;
  background: transparent;
  margin: 0;
  padding: 0;
  font: inherit;
  color: inherit;
  text-align: start;
  cursor: pointer;
  overflow-wrap: anywhere;

  &:hover {
    color: var(--color-text-link-default-qabnuk, #006ce0);
  }
}

.plan-meta {
  display: flex;
  align-items: center;
  flex-wrap: wrap;
  gap: 6px;
  margin-block-start: 4px;
}

.plan-id {
  font-family: var(--font-family-monospace, ui-monospace, Menlo, Consolas, monospace);
  font-size: 12px;
  line-height: 16px;
  font-weight: 400;
  color: var(--color-text-body-secondary-66ygbb, #656871);
}

.plan-flag {
  appearance: none;
  border: 0;
  background: transparent;
  margin: 0;
  padding: 0;
  cursor: pointer;
  line-height: 0;
}

.plan-price {
  font-size: 18px;
  line-height: 22px;
  font-weight: 700;
  font-variant-numeric: tabular-nums;
  white-space: nowrap;
  color: var(--color-text-status-success-1i2jf8, #00802f);
}

.plan-price--muted {
  color: var(--color-text-body-secondary-66ygbb, #656871);
}

.card-desc-box {
  margin-block-end: 12px;
  padding: 8px 10px;
  background: var(--x-bg);
  border-radius: 8px;
  font-size: var(--font-size-body-s, 12px);
  line-height: var(--line-height-body-s, 16px);
  color: var(--color-text-body-secondary-66ygbb, #656871);
  white-space: pre-line;
}

.placeholder-chips {
  display: flex;
  align-items: center;
  gap: 6px;
  flex-wrap: wrap;
  margin-bottom: 6px;
  font-size: 12px;

  .chip-label {
    color: var(--x-text-3);
    font-size: 11.5px;
  }

  .placeholder-tag {
    cursor: pointer;
    font-family: var(--x-font-mono, monospace);
    font-size: 11px;
    user-select: none;
    transition: all 0.15s ease;
    &:hover {
      background: var(--x-primary-soft, #eef2ff);
      border-color: var(--x-primary, #6366f1);
      color: var(--x-primary, #6366f1);
    }
  }
}
</style>
