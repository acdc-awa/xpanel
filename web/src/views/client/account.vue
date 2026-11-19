<script setup lang="ts">
import { computed, onMounted, reactive, ref } from 'vue'
import QRCode from 'qrcode'
import { useAuthStore } from '@/stores/auth'
import { changePassword, setup2FA, confirm2FA, disable2FA, resetSubscribeToken, setAutoRenew } from '@/api/user'
import { redeemGiftCard, getMyBalanceLogs } from '@/api/gift_card'
import { errMsg } from '@/api/http'
import { formatMoney, formatMoneyNumber, formatSignedMoney } from '@/utils/format'
import { formatDateTimeMinute } from '@/utils/timezone'
import type { BalanceLog } from '@/api/types'
import {
  CsAlert,
  CsBadge,
  CsBox,
  CsButton,
  CsColumnLayout,
  CsContainer,
  CsForm,
  CsFormItem,
  CsHeader,
  CsInput,
  CsModal,
  CsSpaceBetween,
  CsStatusIndicator,
  CsToggle,
  ElMessage,
  ElMessageBox,
} from '@/components/cloudscape'

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

// 余额 & 卡密兑换
const redeemCode = ref('')
const redeeming = ref(false)
const balanceLogsOpen = ref(false)
const balanceLogs = ref<BalanceLog[]>([])
const loadingLogs = ref(false)

const BALANCE_TYPE_LABEL: Record<string, string> = {
  recharge_gift_card: '充值',
  order_payment: '购买套餐',
  admin_adjust: '管理员调整',
  refund: '退款',
}
function balanceLogTitle(type?: string): string {
  return BALANCE_TYPE_LABEL[type ?? ''] ?? '余额变动'
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
  } catch (e) {
    ElMessage.error(errMsg(e, '加载资料失败'))
  }
}
onMounted(refresh)

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

async function submitRedeem() {
  const code = redeemCode.value.trim()
  if (!code) {
    ElMessage.warning('请输入卡密')
    return
  }
  redeeming.value = true
  try {
    const { data } = await redeemGiftCard(code)
    if (data.code === 0) {
      ElMessage.success(`充值成功，已到账 ${formatMoney(data.data.face_value_cents)}`)
      redeemCode.value = ''
      await refresh()
    } else {
      ElMessage.error(data.message)
    }
  } catch (e) {
    ElMessage.error(errMsg(e, '卡密核销失败'))
  } finally {
    redeeming.value = false
  }
}

async function openBalanceLogs() {
  balanceLogsOpen.value = true
  loadingLogs.value = true
  try {
    const { data } = await getMyBalanceLogs(1, 50)
    if (data.code === 0) {
      balanceLogs.value = data.data.items
    }
  } catch (e) {
    ElMessage.error(errMsg(e, '加载明细失败'))
  } finally {
    loadingLogs.value = false
  }
}

// 修改密码
const pwd = reactive({ old: '', next: '', confirm: '' })
const savingPwd = ref(false)

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

async function copyBackupCodes() {
  if (!backupCodes.value.length) return
  const text = backupCodes.value.join('\n')
  try {
    await navigator.clipboard.writeText(text)
    ElMessage.success('恢复码已复制到剪贴板')
  } catch {
    ElMessage.error('复制失败，请手动复制')
  }
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
      <!-- 左栏：账户资产 + 自动续费 -->
      <CsSpaceBetween direction="vertical" size="l">
        <CsContainer>
          <template #header>
            <CsHeader variant="h2">
              账户资产
              <template #actions>
                <CsButton @click="openBalanceLogs">收支明细</CsButton>
              </template>
            </CsHeader>
          </template>

          <CsSpaceBetween direction="vertical" size="l">
            <div class="balance-box">
              <div class="balance-label">可用余额</div>
              <div class="balance-value cell-mono">¥{{ balanceYuan }}</div>
            </div>

            <CsForm @submit="submitRedeem">
              <CsFormItem label="卡密充值" description="输入 16 位充值卡密，核销后余额即时到账。">
                <CsSpaceBetween direction="horizontal" size="xs">
                  <CsInput
                    v-model="redeemCode"
                    placeholder="请输入 16 位卡密"
                    class="cell-mono"
                  />
                  <CsButton variant="primary" :loading="redeeming" @click="submitRedeem">
                    立即核销
                  </CsButton>
                </CsSpaceBetween>
              </CsFormItem>
            </CsForm>

            <div v-if="balanceLogs.length" class="log-list">
              <div class="log-section-title">最近变动</div>
              <div v-for="item in balanceLogs.slice(0, 5)" :key="item.id" class="log-row">
                <div class="log-main">
                  <div class="log-title">{{ item.remark || balanceLogTitle(item.type) }}</div>
                  <div class="log-time cell-mono">{{ formatDateTimeMinute(item.created_at) }}</div>
                </div>
                <div class="log-amount cell-mono" :class="{ 'is-plus': item.amount_cents > 0 }">
                  {{ formatSignedMoney(item.amount_cents) }}
                </div>
              </div>
            </div>
          </CsSpaceBetween>
        </CsContainer>

        <CsContainer>
          <template #header>
            <CsHeader variant="h2" description="开启后将在触发条件时按当前套餐价格自动从余额扣费续订。">
              自动续费
            </CsHeader>
          </template>
          <CsSpaceBetween direction="vertical" size="m">
            <div class="switch-row">
              <div class="switch-info">
                <div class="switch-title">到期自动续费</div>
                <div class="switch-desc">套餐到期时自动使用账户余额续订</div>
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
                <div class="switch-desc">本周期流量用尽时自动扣费重置流量</div>
              </div>
              <CsToggle
                :model-value="autoRenewExhaust"
                :disabled="autoRenewSaving"
                @update:model-value="(v) => toggleAutoRenew('exhaust', v)"
              />
            </div>
          </CsSpaceBetween>
        </CsContainer>
      </CsSpaceBetween>

      <!-- 右栏：账户资料 + 安全设置 + 修改密码 -->
      <CsSpaceBetween direction="vertical" size="l">
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
                <div class="switch-desc">使用身份验证器 App 为登录增加动态验证码保护</div>
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
                <div class="switch-desc">订阅泄露时可立即作废旧地址并生成新凭据</div>
              </div>
              <CsButton :loading="resettingSub" @click="onResetSubscribe">重置订阅</CsButton>
            </div>
          </CsSpaceBetween>
        </CsContainer>

        <CsContainer>
          <template #header>
            <CsHeader variant="h2">修改登录密码</CsHeader>
          </template>
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
            <CsButton variant="primary" :loading="savingPwd" form-action="submit">
              保存新密码
            </CsButton>
          </CsForm>
        </CsContainer>
      </CsSpaceBetween>
    </div>

    <!-- 收支明细弹窗 -->
    <CsModal
      v-model:visible="balanceLogsOpen"
      header="账户收支明细"
      size="medium"
      :close-on-click-overlay="false"
    >
      <CsBox v-if="loadingLogs" color="text-body-secondary" font-size="body-s">加载中…</CsBox>
      <CsBox v-else-if="!balanceLogs.length" color="text-body-secondary" font-size="body-s">
        暂无收支记录
      </CsBox>
      <div v-else class="log-list">
        <div v-for="item in balanceLogs" :key="item.id" class="log-row">
          <div class="log-main">
            <div class="log-title">{{ item.remark || balanceLogTitle(item.type) }}</div>
            <div class="log-time cell-mono">{{ formatDateTimeMinute(item.created_at) }}</div>
          </div>
          <div class="log-amount cell-mono" :class="{ 'is-plus': item.amount_cents > 0 }">
            {{ formatSignedMoney(item.amount_cents) }}
          </div>
        </div>
      </div>
      <template #footer>
        <CsButton variant="primary" @click="balanceLogsOpen = false">关闭</CsButton>
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
          <CsFormItem label="手动密钥" description="若无法扫码，可在验证器中手动输入此密钥。">
            <CsInput :model-value="twofaSecret" readonly class="cell-mono" />
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
  </CsSpaceBetween>
</template>

<style scoped lang="scss">
.acct-grid {
  display: grid;
  grid-template-columns: repeat(2, minmax(0, 1fr));
  gap: 20px;
  align-items: start;
}

.balance-box {
  display: flex;
  flex-direction: column;
  gap: 4px;
}

.balance-label {
  font-size: 12px;
  color: var(--x-text-secondary);
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

@media (max-width: $bp-two-pane) {
  .acct-grid {
    grid-template-columns: 1fr;
  }
}
</style>
