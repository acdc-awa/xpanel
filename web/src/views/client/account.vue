<script setup lang="ts">
import { computed, onMounted, reactive, ref } from 'vue'
import { useRouter } from 'vue-router'
import QRCode from 'qrcode'
import { useAuthStore } from '@/stores/auth'
import { changePassword, setup2FA, confirm2FA, disable2FA, resetSubscribeToken, setAutoRenew } from '@/api/user'
import { getMyBalanceLogs } from '@/api/gift_card'
import { cancelOrder, getMyOrders } from '@/api/shop'
import RechargeModal from '@/components/RechargeModal.vue'
import { errMsg } from '@/api/http'
import { formatMoney, formatMoneyNumber, formatOrderContent, formatSignedMoney } from '@/utils/format'
import { formatDateTimeMinute } from '@/utils/timezone'
import type { BalanceLog, Order } from '@/api/types'
import {
  CsAlert,
  CsBadge,
  CsButton,
  CsColumnLayout,
  CsContainer,
  CsForm,
  CsFormItem,
  CsHeader,
  CsInput,
  CsModal,
  CsPagination,
  CsPaginationBar,
  CsSpaceBetween,
  CsStatusIndicator,
  CsTable,
  CsTabs,
  CsToggle,
  ElMessage,
  ElMessageBox,
} from '@/components/cloudscape'
import type { TableColumn } from '@/components/cloudscape'

const router = useRouter()
const auth = useAuthStore()

const createdText = ref('—')

// 自动续费双开关
const autoRenewSaving = ref(false)
const autoRenewExpire = computed(() => !!auth.user?.auto_renew_expire)
const autoRenewExhaust = computed(() => !!auth.user?.auto_renew_exhaust)

async function toggleAutoRenew(kind: 'expire' | 'exhaust', val: boolean) {
  autoRenewSaving.value = true
  try {
    const payload = kind === 'expire' ? { auto_renew_expire: val } : { auto_renew_exhaust: val }
    const { data } = await setAutoRenew(payload)
    if (data.code === 0) {
      ElMessage.success('自动续费设置已保存')
      await auth.fetchMe()
    } else {
      ElMessage.error(data.message)
    }
  } catch (e) {
    ElMessage.error(errMsg(e, '保存失败'))
  } finally {
    autoRenewSaving.value = false
  }
}

// 余额 & 充值弹窗
const rechargeModalOpen = ref(false)
const balanceLogs = ref<BalanceLog[]>([])
const loadingLogs = ref(false)

const BALANCE_TYPE_LABEL: Record<string, string> = {
  recharge_gift_card: '充值',
  order_payment: '购买套餐',
  admin_adjust: '管理员调整',
  refund: '退款',
}
function balanceLogTitle(type?: string): string {
  if (!type) return '余额变动'
  if (BALANCE_TYPE_LABEL[type]) return BALANCE_TYPE_LABEL[type]
  if (type.startsWith('recharge_')) return '充值'
  return '余额变动'
}

const balanceYuan = computed(() => {
  const cents = auth.user?.balance_cents ?? 0
  return formatMoneyNumber(cents)
})

async function refresh() {
  try {
    await auth.fetchMe()
    createdText.value = formatDateTimeMinute(auth.user?.created_at, '—')
    loadRecentLogs()
    loadOrders()
  } catch (e) {
    ElMessage.error(errMsg(e, '加载资料失败'))
  }
}
onMounted(refresh)

const activeRecordTab = ref<'orders' | 'logs'>('orders')
const recordTabs = [
  { id: 'orders', label: '我的订单' },
  { id: 'logs', label: '收支明细' },
]

const orders = ref<Order[]>([])
const orderPage = ref(1)
const orderPageSize = ref(10)
const orderTotalPages = computed(() => Math.max(1, Math.ceil(orders.value.length / orderPageSize.value)))
const pagedOrders = computed(() => {
  const start = (orderPage.value - 1) * orderPageSize.value
  return orders.value.slice(start, start + orderPageSize.value)
})
const orderStatusMap: Record<string, { className: string; text: string }> = {
  pending: { className: 'order-status-pending', text: '待支付' },
  paid: { className: 'order-status-paid', text: '已支付' },
  cancelled: { className: 'order-status-cancelled', text: '已取消' },
}

const orderColumns: TableColumn[] = [
  { id: 'plan_name', header: '订单内容' },
  { id: 'status', header: '状态' },
  { id: 'amount', header: '金额' },
  { id: 'payment_method', header: '支付方式' },
  { id: 'created_at', header: '创建时间' },
  { id: 'order_no', header: '订单号' },
  { id: 'actions', header: '操作' },
]

function orderMethodLabel(row: Order): string {
  if (row.payment_method === 'gift_card') return '卡密'
  if (row.payment_method === 'balance') return '余额'
  return row.payment_channel || row.payment_method || '—'
}

function orderHasAmountChange(row: Order): boolean {
  return row.status === 'paid' && row.amount_cents > 0
}

function formatOrderAmount(row: Order): string {
  const prefix = row.order_type === 'recharge' ? '+' : '-'
  if (row.amount_cents === 0) return formatMoney(0)
  return `${prefix}${formatMoney(row.amount_cents)}`
}

const logPage = ref(1)
const logPageSize = ref(10)
const pagedLogs = computed(() => {
  const start = (logPage.value - 1) * logPageSize.value
  return balanceLogs.value.slice(start, start + logPageSize.value)
})
const logColumns: TableColumn[] = [
  { id: 'amount', header: '变动金额' },
  { id: 'type', header: '类型' },
  { id: 'remark', header: '说明' },
  { id: 'created_at', header: '时间' },
]

async function loadOrders() {
  try {
    const { data } = await getMyOrders()
    if (data.code === 0) {
      orders.value = data.data.items
      if (orderPage.value > orderTotalPages.value) {
        orderPage.value = 1
      }
    }
  } catch {
    // 静默失败，不影响主资料展示
  }
}

async function handleCancelOrder(orderNo: string) {
  try {
    await cancelOrder(orderNo)
    ElMessage.success('订单已取消')
    await loadOrders()
  } catch (e) {
    ElMessage.error(errMsg(e, '取消订单失败'))
  }
}

async function loadRecentLogs() {
  try {
    const { data } = await getMyBalanceLogs(1, 10)
    if (data.code === 0) {
      balanceLogs.value = data.data.items
    }
  } catch {
    balanceLogs.value = []
  }
}

async function handleRecharged() {
  await refresh()
}

// 修改密码
const pwdOpen = ref(false)
const pwd = reactive({ old: '', next: '', confirm: '' })
const savingPwd = ref(false)

function openPwdModal() {
  pwd.old = ''
  pwd.next = ''
  pwd.confirm = ''
  pwdOpen.value = true
}

async function savePwd() {
  if (!pwd.old || !pwd.next) {
    ElMessage.warning('请填写当前密码与新密码')
    return
  }
  if (pwd.next.length < 8) {
    ElMessage.warning('新密码长度不能少于 8 位')
    return
  }
  if (pwd.next !== pwd.confirm) {
    ElMessage.warning('两次输入的新密码不一致')
    return
  }
  savingPwd.value = true
  try {
    const { data } = await changePassword(pwd.old, pwd.next)
    if (data.code === 0) {
      ElMessage.success('登录密码已更新')
      pwdOpen.value = false
      pwd.old = ''
      pwd.next = ''
      pwd.confirm = ''
    } else {
      ElMessage.error(data.message)
    }
  } catch (e) {
    ElMessage.error(errMsg(e, '修改密码失败'))
  } finally {
    savingPwd.value = false
  }
}

// 两步验证（TOTP）
const twofaOpen = ref(false)
const twofaStep = ref<'setup' | 'confirm' | 'backup' | 'disable'>('setup')
const twofaSecret = ref('')
const twofaQrUrl = ref('')
const twofaCode = ref('')
const twofaLoading = ref(false)
const backupCodes = ref<string[]>([])
const disableForm = reactive({ code: '', password: '' })

async function startTwofaSetup() {
  twofaLoading.value = true
  try {
    const { data } = await setup2FA()
    if (data.code === 0) {
      twofaSecret.value = data.data.secret
      twofaQrUrl.value = await QRCode.toDataURL(data.data.otpauth_url, {
        width: 220,
        margin: 2,
        color: { dark: '#0f172a', light: '#ffffff' },
      })
      twofaCode.value = ''
      twofaStep.value = 'confirm'
      twofaOpen.value = true
    } else {
      ElMessage.error(data.message)
    }
  } catch (e) {
    ElMessage.error(errMsg(e, '生成两步验证密钥失败'))
  } finally {
    twofaLoading.value = false
  }
}

function openDisable2FA() {
  disableForm.code = ''
  disableForm.password = ''
  twofaStep.value = 'disable'
  twofaOpen.value = true
}

async function submitTwofaConfirm() {
  if (!twofaCode.value || twofaCode.value.length !== 6) {
    ElMessage.warning('请输入 6 位动态验证码')
    return
  }
  twofaLoading.value = true
  try {
    const { data } = await confirm2FA(twofaSecret.value, twofaCode.value)
    if (data.code === 0) {
      backupCodes.value = data.data.backup_codes || []
      twofaStep.value = 'backup'
      await auth.fetchMe()
      ElMessage.success('两步验证已成功开启')
    } else {
      ElMessage.error(data.message)
    }
  } catch (e) {
    ElMessage.error(errMsg(e, '开启失败'))
  } finally {
    twofaLoading.value = false
  }
}

async function copyText(text: string, label: string) {
  if (!text) return
  try {
    await navigator.clipboard.writeText(text)
    ElMessage.success(`${label}已复制到剪贴板`)
  } catch {
    ElMessage.error('复制失败，请手动复制')
  }
}

async function copyBackupCodes() {
  if (!backupCodes.value.length) return
  await copyText(backupCodes.value.join('\n'), '恢复码')
}

async function copyTwofaSecret() {
  await copyText(twofaSecret.value, '密钥')
}

async function submitTwofaDisable() {
  if (!disableForm.code && !disableForm.password) {
    ElMessage.warning('请输入动态验证码、恢复码或当前密码')
    return
  }
  twofaLoading.value = true
  try {
    const { data } = await disable2FA({
      code: disableForm.code || undefined,
      password: disableForm.password || undefined,
    })
    if (data.code === 0) {
      ElMessage.success('已关闭两步验证')
      twofaOpen.value = false
      disableForm.code = ''
      disableForm.password = ''
      await auth.fetchMe()
    } else {
      ElMessage.error(data.message)
    }
  } catch (e) {
    ElMessage.error(errMsg(e, '关闭失败'))
  } finally {
    twofaLoading.value = false
  }
}

// 重置订阅凭据
const resettingSub = ref(false)
async function onResetSubscribe() {
  try {
    await ElMessageBox.confirm(
      '重置后，原订阅地址与已连接的节点凭据将立即失效，所有客户端需重新同步或更新订阅后方可继续使用。确定重置订阅地址？',
      '重置订阅地址',
      { type: 'warning', confirmButtonText: '确定重置', cancelButtonText: '取消' },
    )
  } catch {
    return
  }
  resettingSub.value = true
  try {
    const { data } = await resetSubscribeToken()
    if (data.code === 0) {
      ElMessage.success('订阅地址与连接凭据已重置，请在客户端中更新订阅')
      await auth.fetchMe()
    } else {
      ElMessage.error(data.message)
    }
  } catch (e) {
    ElMessage.error(errMsg(e, '重置订阅地址失败'))
  } finally {
    resettingSub.value = false
  }
}
</script>

<template>
  <CsSpaceBetween direction="vertical" size="l">
    <CsHeader variant="h1" description="管理账户资产、自动续费、安全设置与登录密码。">
      账户设置
    </CsHeader>

    <div class="acct-grid">
      <CsContainer fit-height class="balance-card">
          <template #header>
            <CsHeader variant="h2">账户余额</CsHeader>
          </template>

          <div class="balance-box">
            <div class="balance-value cell-mono">¥{{ balanceYuan }}</div>
            <CsButton variant="primary" @click="rechargeModalOpen = true">
              充值
            </CsButton>
          </div>
        </CsContainer>

        <CsContainer>
          <template #header>
            <CsHeader variant="h2" description="开启后自动从余额扣费。">
              自动续费设置
            </CsHeader>
          </template>
          <CsSpaceBetween direction="vertical" size="m">
            <div class="switch-row">
              <div class="switch-info">
                <div class="switch-title">到期自动续费</div>
                <div class="switch-desc">套餐到期时自动续订</div>
              </div>
              <CsToggle
                :model-value="autoRenewExpire"
                :disabled="autoRenewSaving"
                @update:model-value="(v) => toggleAutoRenew('expire', v)"
              />
            </div>

            <div class="switch-row">
              <div class="switch-info">
                <div class="switch-title">流量耗尽自动续费</div>
                <div class="switch-desc">本周期流量用尽时自动重新订阅套餐</div>
              </div>
              <CsToggle
                :model-value="autoRenewExhaust"
                :disabled="autoRenewSaving"
                @update:model-value="(v) => toggleAutoRenew('exhaust', v)"
              />
            </div>
          </CsSpaceBetween>
        </CsContainer>

      <CsContainer>
          <template #header>
            <CsHeader variant="h2">账户资料</CsHeader>
          </template>
          <CsColumnLayout :columns="2" variant="text-grid">
            <div class="kv-item">
              <div class="kv-label">登录邮箱</div>
              <div class="kv-value cell-mono">{{ auth.user?.email || '—' }}</div>
            </div>
            <div class="kv-item">
              <div class="kv-label">账户角色</div>
              <div class="kv-value">
                <CsBadge :color="auth.role === 'admin' ? 'blue' : 'grey'">
                  {{ auth.role === 'admin' ? '管理员' : '普通用户' }}
                </CsBadge>
              </div>
            </div>
            <div class="kv-item">
              <div class="kv-label">注册时间</div>
              <div class="kv-value cell-mono">{{ createdText }}</div>
            </div>
            <div class="kv-item">
              <div class="kv-label">两步验证</div>
              <div class="kv-value">
                <CsStatusIndicator :type="auth.user?.totp_enabled ? 'success' : 'stopped'">
                  {{ auth.user?.totp_enabled ? '已开启' : '未开启' }}
                </CsStatusIndicator>
              </div>
            </div>
          </CsColumnLayout>
        </CsContainer>

        <CsContainer>
          <template #header>
            <CsHeader variant="h2">安全与订阅凭据</CsHeader>
          </template>
          <CsSpaceBetween direction="vertical" size="m">
            <div class="switch-row">
              <div class="switch-info">
                <div class="switch-title">两步验证（TOTP）</div>
                <div class="switch-desc">使用身份验证器为账号开启两步验证</div>
              </div>
              <CsButton
                v-if="auth.user?.totp_enabled"
                @click="openDisable2FA"
              >
                关闭验证
              </CsButton>
              <CsButton
                v-else
                variant="primary"
                :loading="twofaLoading"
                @click="startTwofaSetup"
              >
                开启验证
              </CsButton>
            </div>

            <div class="switch-row">
              <div class="switch-info">
                <div class="switch-title">重置订阅地址</div>
                <div class="switch-desc">订阅泄露时可立即重置地址</div>
              </div>
              <CsButton :loading="resettingSub" @click="onResetSubscribe">重置订阅</CsButton>
            </div>

            <div class="switch-row">
              <div class="switch-info">
                <div class="switch-title">登录密码</div>
                <div class="switch-desc">定期修改登录密码以保障账户安全</div>
              </div>
              <CsButton @click="openPwdModal">修改密码</CsButton>
            </div>
          </CsSpaceBetween>
        </CsContainer>
    </div>

    <!-- 修改登录密码弹窗 -->
    <CsModal
      v-model:visible="pwdOpen"
      header="修改登录密码"
      size="small"
      :close-on-click-overlay="false"
    >
      <CsForm @submit="savePwd">
        <CsFormItem label="当前密码">
          <CsInput v-model="pwd.old" type="password" show-password placeholder="请输入当前密码" />
        </CsFormItem>
        <CsFormItem label="新密码" description="长度不少于 8 位。">
          <CsInput v-model="pwd.next" type="password" show-password placeholder="请输入新密码" />
        </CsFormItem>
        <CsFormItem label="确认新密码">
          <CsInput v-model="pwd.confirm" type="password" show-password placeholder="请再次输入新密码" />
        </CsFormItem>
      </CsForm>
      <template #footer>
        <CsSpaceBetween direction="horizontal" size="xs">
          <CsButton @click="pwdOpen = false">取消</CsButton>
          <CsButton variant="primary" :loading="savingPwd" @click="savePwd">
            保存新密码
          </CsButton>
        </CsSpaceBetween>
      </template>
    </CsModal>

    <!-- 两步验证设置/关闭弹窗 -->
    <CsModal
      v-model:visible="twofaOpen"
      :header="
        twofaStep === 'disable'
          ? '关闭两步验证'
          : twofaStep === 'backup'
            ? '保存恢复码'
            : '开启两步验证'
      "
      size="small"
      :close-on-click-overlay="false"
    >
      <CsSpaceBetween v-if="twofaStep === 'confirm'" direction="vertical" size="m">
        <div class="twofa-qr-wrap">
          <img v-if="twofaQrUrl" :src="twofaQrUrl" alt="TOTP 二维码" class="twofa-qr" />
        </div>
        <CsForm @submit="submitTwofaConfirm">
          <CsFormItem label="手动密钥" description="若无法扫码，可复制此密钥并在验证器中手动输入。">
            <div class="twofa-secret-box">
              <code class="twofa-secret-code cell-mono">{{ twofaSecret }}</code>
              <CsButton icon-name="copy" :wrap-text="false" @click="copyTwofaSecret">
                复制
              </CsButton>
            </div>
          </CsFormItem>
          <CsFormItem label="6 位动态验证码">
            <CsInput v-model="twofaCode" placeholder="请输入 6 位数字" class="cell-mono" />
          </CsFormItem>
        </CsForm>
      </CsSpaceBetween>

      <CsSpaceBetween v-else-if="twofaStep === 'backup'" direction="vertical" size="m">
        <CsAlert type="warning" header="请妥善保存恢复码">
          每个恢复码仅可使用一次，用于在丢失验证器时恢复登录。
        </CsAlert>
        <div class="backup-codes-grid cell-mono">
          <span v-for="code in backupCodes" :key="code" class="backup-code-item">{{ code }}</span>
        </div>
      </CsSpaceBetween>

      <CsForm v-else-if="twofaStep === 'disable'" @submit="submitTwofaDisable">
        <CsFormItem label="动态验证码或恢复码">
          <CsInput v-model="disableForm.code" placeholder="输入 6 位验证码或恢复码" class="cell-mono" />
        </CsFormItem>
        <CsFormItem label="或输入当前登录密码">
          <CsInput v-model="disableForm.password" type="password" show-password placeholder="输入当前密码" />
        </CsFormItem>
      </CsForm>

      <template #footer>
        <CsSpaceBetween direction="horizontal" size="xs">
          <template v-if="twofaStep === 'confirm'">
            <CsButton @click="twofaOpen = false">取消</CsButton>
            <CsButton variant="primary" :loading="twofaLoading" @click="submitTwofaConfirm">
              确认开启
            </CsButton>
          </template>
          <template v-else-if="twofaStep === 'backup'">
            <CsButton @click="copyBackupCodes">复制全部恢复码</CsButton>
            <CsButton variant="primary" @click="twofaOpen = false">我已保存</CsButton>
          </template>
          <template v-else-if="twofaStep === 'disable'">
            <CsButton @click="twofaOpen = false">取消</CsButton>
            <CsButton variant="primary" :loading="twofaLoading" @click="submitTwofaDisable">
              确认关闭
            </CsButton>
          </template>
        </CsSpaceBetween>
      </template>
    </CsModal>

    <CsContainer>
      <template #header>
        <CsHeader variant="h2">订单与收支记录</CsHeader>
      </template>

      <CsTabs v-model="activeRecordTab" :tabs="recordTabs">
        <div v-if="activeRecordTab === 'orders'">
          <CsSpaceBetween size="m">
            <CsTable
              compact
              :wrap-lines="false"
              :items="pagedOrders"
              :columns="orderColumns"
              track-by="id"
              empty-text="暂无订单记录"
            >
              <template #cell-order_no="{ row }">
                <code class="cell-mono">{{ row.order_no }}</code>
              </template>
              <template #cell-plan_name="{ row }">
                <span>{{ formatOrderContent(row) }}</span>
              </template>
              <template #cell-payment_method="{ row }">
                <span v-if="orderMethodLabel(row) !== '—'">{{ orderMethodLabel(row) }}</span>
                <span v-else class="cell-muted">—</span>
              </template>
              <template #cell-amount="{ row }">
                <span
                  class="cell-mono"
                  :class="orderHasAmountChange(row) ? 'amount-changed' : 'amount-unchanged'"
                >
                  <strong>{{ formatOrderAmount(row) }}</strong>
                </span>
              </template>
              <template #cell-status="{ row }">
                <span
                  :class="orderStatusMap[row.status]?.className ?? 'order-status-cancelled'"
                >
                  {{ orderStatusMap[row.status]?.text ?? row.status }}
                </span>
              </template>
              <template #cell-created_at="{ row }">
                <span class="cell-mono">{{ formatDateTimeMinute(row.created_at) }}</span>
              </template>
              <template #cell-actions="{ row }">
                <div v-if="row.status === 'pending'" class="order-actions">
                  <CsButton variant="primary" :wrap-text="false" @click="router.push(`/checkout/${row.order_no}`)">
                    去支付
                  </CsButton>
                  <CsButton variant="normal" :wrap-text="false" @click="handleCancelOrder(row.order_no)">
                    取消
                  </CsButton>
                </div>
                <span v-else class="cell-muted">—</span>
              </template>
            </CsTable>

            <CsPaginationBar v-if="orders.length > 0">
              <CsPagination
                v-model:current-page="orderPage"
                v-model:page-size="orderPageSize"
                :total="orders.length"
                :page-sizes="[10, 20, 50]"
                layout="total, sizes, prev, pager, next"
                simple
                @size-change="orderPage = 1"
              />
            </CsPaginationBar>
          </CsSpaceBetween>
        </div>

        <div v-else-if="activeRecordTab === 'logs'">
          <CsSpaceBetween size="m">
            <CsTable
              compact
              :wrap-lines="false"
              :items="pagedLogs"
              :columns="logColumns"
              :loading="loadingLogs"
              loading-text="加载中…"
              track-by="id"
              empty-text="暂无收支记录"
            >
              <template #cell-type="{ row }">
                <span :class="row.amount_cents > 0 ? 'order-status-paid' : ''">
                  {{ balanceLogTitle(row.type) }}
                </span>
              </template>
              <template #cell-remark="{ row }">
                {{ row.remark || balanceLogTitle(row.type) }}
              </template>
              <template #cell-amount="{ row }">
                <span class="log-amount cell-mono" :class="{ 'is-plus': row.amount_cents > 0 }">
                  {{ formatSignedMoney(row.amount_cents) }}
                </span>
              </template>
              <template #cell-created_at="{ row }">
                <span class="cell-mono">{{ formatDateTimeMinute(row.created_at) }}</span>
              </template>
            </CsTable>

            <CsPaginationBar v-if="balanceLogs.length > 0">
              <CsPagination
                v-model:current-page="logPage"
                v-model:page-size="logPageSize"
                :total="balanceLogs.length"
                :page-sizes="[10, 20, 50]"
                layout="total, sizes, prev, pager, next"
                simple
                @size-change="logPage = 1"
              />
            </CsPaginationBar>
          </CsSpaceBetween>
        </div>
      </CsTabs>
    </CsContainer>

    <RechargeModal v-model:visible="rechargeModalOpen" @recharged="handleRecharged" />
  </CsSpaceBetween>
</template>

<style scoped lang="scss">
.acct-grid {
  display: grid;
  grid-template-columns: repeat(2, minmax(0, 1fr));
  gap: 20px;
}

.order-list {
  display: flex;
  flex-direction: column;
  gap: 10px;
}

.order-row {
  display: flex;
  justify-content: space-between;
  align-items: center;
  gap: 12px;
  padding: 12px 14px;
  border-radius: 8px;
  border: 1px solid var(--color-border-divider-default-diwd4w, #eaeded);
}

.order-row.clickable {
  cursor: pointer;
}

.order-row.clickable:hover {
  border-color: var(--color-border-item-selected-7104rh, #0972d3);
}

.order-main {
  display: flex;
  flex-direction: column;
  gap: 4px;
}

.order-name {
  font-weight: 600;
  font-size: 14px;
}

.order-meta {
  display: flex;
  flex-wrap: wrap;
  align-items: center;
  gap: 8px;
  font-size: 12px;
  color: var(--color-text-body-secondary-66ygbb, #5f6b7a);
}

.order-right {
  display: flex;
  align-items: center;
  gap: 12px;
  flex-shrink: 0;
}

.order-amount {
  font-weight: 600;
  font-size: 14px;
}

.order-pagination {
  display: flex;
  justify-content: flex-end;
}

.balance-card :deep([class*='content-inner']) {
  block-size: 100%;
  display: flex;
  align-items: center;
  justify-content: center;
}

.balance-box {
  display: flex;
  flex-direction: column;
  align-items: center;
  justify-content: center;
  gap: 14px;
  padding: 8px 0;
}

.balance-value {
  font-size: 24px;
  font-weight: 700;
  color: var(--x-primary);
}

.log-section-title {
  font-size: 12px;
  font-weight: 600;
  color: var(--x-text-secondary);
  margin-bottom: 6px;
}

.log-list {
  display: flex;
  flex-direction: column;
}

.log-row {
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: 12px;
  padding: 8px 0;
  border-bottom: 1px solid var(--x-border);

  &:last-child {
    border-bottom: none;
    padding-bottom: 0;
  }
}

.log-main {
  min-inline-size: 0;
}

.log-title {
  font-size: 13px;
  color: var(--x-text);
}

.log-time {
  font-size: 12px;
  color: var(--x-text-secondary);
}

.log-amount {
  font-size: 14px;
  font-weight: 700;
  color: var(--x-text);
  flex-shrink: 0;

  &.is-plus {
    color: var(--x-primary);
  }
}

.switch-row {
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: 16px;
}

.switch-info {
  min-inline-size: 0;
}

.switch-title {
  font-size: 14px;
  font-weight: 600;
  color: var(--x-text);
}

.switch-desc {
  margin-top: 2px;
  font-size: 12px;
  color: var(--x-text-secondary);
}

.kv-item {
  display: flex;
  flex-direction: column;
  gap: 4px;
}

.kv-label {
  font-size: 12px;
  color: var(--x-text-secondary);
}

.kv-value {
  font-size: 14px;
  color: var(--x-text);
}

.twofa-qr-wrap {
  display: flex;
  justify-content: center;
}

.twofa-qr {
  inline-size: 180px;
  block-size: 180px;
  padding: 8px;
  border-radius: 8px;
  background-color: #ffffff;
  border: 1px solid var(--x-border);
}

.backup-codes-grid {
  display: grid;
  grid-template-columns: repeat(2, minmax(0, 1fr));
  gap: 8px;
}

.backup-code-item {
  padding: 8px;
  border-radius: 4px;
  border: 1px solid var(--x-border);
  text-align: center;
  font-size: 13px;
}

.twofa-secret-box {
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: 12px;
  padding: 8px 12px;
  border: 1px solid var(--x-border);
  border-radius: 8px;
  background-color: var(--x-bg-subtle, transparent);
}

.twofa-secret-code {
  min-inline-size: 0;
  font-size: 13px;
  color: var(--x-text);
  word-break: break-all;
  user-select: all;
}

.amount-changed {
  color: var(--color-text-link-default-qabnuk, #0972d3);
}

.amount-unchanged {
  color: var(--color-text-body-default-b2us04, #0f141a);
}

.order-status-paid {
  color: var(--color-text-status-success-1i2jf8, #00802f);
}

.order-status-pending {
  color: var(--color-text-status-warning-fabq6h, #855900);
}

.order-status-cancelled {
  color: var(--color-text-status-inactive-fqjc97, #656871);
}

.order-actions {
  display: inline-flex;
  flex-wrap: nowrap;
  align-items: center;
  gap: 8px;
  white-space: nowrap;
}

@media (max-width: $bp-two-pane) {
  .acct-grid {
    grid-template-columns: 1fr;
  }
}
</style>
