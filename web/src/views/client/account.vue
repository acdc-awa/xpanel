<script setup lang="ts">
import { computed, onMounted, reactive, ref } from 'vue'
import { useRouter } from 'vue-router'
import {
  Wallet,
  User,
  Lock,
  Key,
  Unlock,
  RefreshRight,
  Ticket,
  List,
  CopyDocument,
} from '@element-plus/icons-vue'
import QRCode from 'qrcode'
import { useAuthStore } from '@/stores/auth'
import { changePassword, setup2FA, confirm2FA, disable2FA, resetSubscribeToken, setAutoRenew } from '@/api/user'
import { redeemGiftCard, getMyBalanceLogs } from '@/api/gift_card'
import { errMsg } from '@/api/http'
import type { BalanceLog } from '@/api/types'
import { ElMessage } from 'element-plus'

const auth = useAuthStore()
const router = useRouter()

const createdText = ref('—')

// 自动续费双开关（到期 / 流量耗尽；触发即按现有购买语义扣费重购）
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

const balanceYuan = computed(() => {
  const cents = auth.user?.balance_cents ?? 0
  return (cents / 100).toFixed(2)
})

async function refresh() {
  try {
    await auth.fetchMe()
    createdText.value = (auth.user?.created_at ?? '').replace('T', ' ').slice(0, 16) || '—'
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

// 兑换礼品卡
async function submitRedeem() {
  const code = redeemCode.value.trim()
  if (!code) {
    ElMessage.warning('请输入充值卡密')
    return
  }
  redeeming.value = true
  try {
    const { data } = await redeemGiftCard(code)
    if (data.code === 0) {
      ElMessage.success(`充值成功！已到账 ¥ ${(data.data.face_value_cents / 100).toFixed(2)}`)
      redeemCode.value = ''
      await refresh()
    } else {
      ElMessage.error(data.message)
    }
  } catch (e) {
    ElMessage.error(errMsg(e, '充值核销失败'))
  } finally {
    redeeming.value = false
  }
}

// 查看全部余额流水
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

// ---- 两步验证（TOTP）----
const twofaOpen = ref(false)
const twofaStep = ref<'setup' | 'confirm' | 'backup' | 'disable'>('setup')
const twofaSecret = ref('')
const twofaQrUrl = ref('')
const twofaCode = ref('')
const twofaLoading = ref(false)
const backupCodes = ref<string[]>([])

// 关闭验证：需验证码/恢复码/密码任其一
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
    ElMessage.error(errMsg(e, '绑定失败'))
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
    ElMessage.error('复制失败，请手动选择复制')
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
    ElMessage.error(errMsg(e, '解绑失败'))
  } finally {
    twofaLoading.value = false
  }
}

// ---- 重置订阅凭据 ----
const resettingSub = ref(false)
async function onResetSubscribe() {
  try {
    await ElMessageBox.confirm(
      '重置后旧订阅链接与已下发的节点密钥将立即失效，所有 Mihomo 客户端需重新同步新链接。确认重置？',
      '重置订阅凭据',
      { type: 'warning' },
    )
  } catch {
    return
  }
  resettingSub.value = true
  try {
    const { data } = await resetSubscribeToken()
    if (data.code === 0) {
      ElMessage.success('订阅凭据已重置，请在控制台重新同步至 Mihomo 客户端')
      await auth.fetchMe()
    } else {
      ElMessage.error(data.message)
    }
  } catch (e) {
    ElMessage.error(errMsg(e, '重置失败'))
  } finally {
    resettingSub.value = false
  }
}

async function onLogout() {
  await auth.logout()
  await router.replace('/login')
}
</script>

<template>
  <div class="x-client-body">
    <div class="x-acct-grid">
      <!-- 左侧：账户资产与充值流水 -->
      <div>
        <!-- 可用余额卡片 -->
        <div class="x-card">
          <div class="x-card-head">
            <span><el-icon><Wallet /></el-icon>&nbsp;账户资产</span>
            <el-button text size="small" type="primary" @click="openBalanceLogs">
              <el-icon><List /></el-icon>&nbsp;收支明细
            </el-button>
          </div>
          <div class="x-card-body">
            <div class="balance-overview-box">
              <div class="bal-col">
                <span class="bal-label">可用余额</span>
                <span class="bal-value cell-mono">¥ {{ balanceYuan }}</span>
              </div>
              <router-link to="/shop">
                <el-button type="primary" size="default">选购服务计划</el-button>
              </router-link>
            </div>

            <!-- 卡密充值区域 -->
            <div class="topup-form-section">
              <div class="section-label">
                <el-icon><Ticket /></el-icon>&nbsp;充值卡核销
              </div>
              <p class="muted" style="font-size: 12px; margin-bottom: 10px">
                输入充值卡密（格式如 <code>GIFT-XXXX-XXXX-XXXX-XXXX</code>），核销后余额即时到账。
              </p>
              <div class="redeem-box">
                <el-input
                  v-model="redeemCode"
                  placeholder="请输入 16 位充值卡密"
                  clearable
                  class="cell-mono"
                  @keyup.enter="submitRedeem"
                />
                <el-button type="primary" plain :loading="redeeming" @click="submitRedeem">
                  立即核销
                </el-button>
              </div>
            </div>
          </div>
        </div>

        <!-- 自动续费设置 -->
        <div class="x-card">
          <div class="x-card-head">
            <span><el-icon><RefreshRight /></el-icon>&nbsp;自动续费</span>
          </div>
          <div class="x-card-body">
            <div class="autorenew-row">
              <div class="autorenew-info">
                <div class="autorenew-title">到期自动续费</div>
                <div class="muted" style="font-size: 12px">套餐到期前 1 小时内自动使用余额续购当前套餐</div>
              </div>
              <el-switch
                :model-value="autoRenewExpire"
                :loading="autoRenewSaving"
                @change="(v: any) => toggleAutoRenew('expire', !!v)"
              />
            </div>
            <div class="autorenew-row" style="margin-top: 12px">
              <div class="autorenew-info">
                <div class="autorenew-title">流量耗尽自动续费</div>
                <div class="muted" style="font-size: 12px">流量用尽后自动使用余额续购当前套餐（重新计时+清零流量）</div>
              </div>
              <el-switch
                :model-value="autoRenewExhaust"
                :loading="autoRenewSaving"
                @change="(v: any) => toggleAutoRenew('exhaust', !!v)"
              />
            </div>
            <el-alert type="info" :closable="false" show-icon style="margin-top: 12px">
              <p style="font-size: 12px; line-height: 1.6">
                续购按购买规则执行：现有剩余时长作废、自购买时刻重新计时、流量周期重置。余额不足时不扣费，
                充值后下一轮自动补续；余额充足是前提，请保持账户余额充裕。
              </p>
            </el-alert>
          </div>
        </div>

        <!-- 最近收支记录 -->
        <div class="x-card">
          <div class="x-card-head">
            <span>最近变动记录</span>
            <span class="muted" style="font-size: 12px">显示近 5 条记录</span>
          </div>
          <div class="x-card-body" style="padding: 12px 16px;">
            <div v-if="balanceLogs.length" class="log-list">
              <div v-for="log in balanceLogs.slice(0, 5)" :key="log.id" class="log-item">
                <div class="log-left">
                  <div class="log-title">{{ log.remark || log.type }}</div>
                  <div class="log-time cell-mono">{{ String(log.created_at).replace('T', ' ').slice(0, 16) }}</div>
                </div>
                <div class="log-right">
                  <div class="log-amount cell-mono" :class="{ plus: log.amount_cents > 0, minus: log.amount_cents < 0 }">
                    {{ log.amount_cents > 0 ? '+' : '' }}¥ {{ (log.amount_cents / 100).toFixed(2) }}
                  </div>
                  <div class="log-balance cell-mono">结余: ¥ {{ (log.balance_after / 100).toFixed(2) }}</div>
                </div>
              </div>
            </div>
            <div v-else class="muted" style="text-align: center; padding: 24px 0; font-size: 12.5px;">
              暂无收支明细记录
            </div>
          </div>
        </div>
      </div>

      <!-- 右侧：账户安全与访问凭证 -->
      <div>
        <!-- 账号基本资料 -->
        <div class="x-card">
          <div class="x-card-head">
            <span><el-icon><User /></el-icon>&nbsp;账号信息</span>
          </div>
          <div class="x-card-body">
            <div class="form-grid-2">
              <div class="profile-info-item">
                <span class="p-label">登录账号 / 邮箱</span>
                <span class="p-val cell-mono">{{ auth.user?.email || auth.username }}</span>
              </div>
              <div class="profile-info-item">
                <span class="p-label">权限级别</span>
                <span class="p-val">
                  <span class="x-chip" :class="auth.role === 'admin' ? 'purple' : 'gray'">
                    {{ auth.role === 'admin' ? '管理员' : '普通订阅用户' }}
                  </span>
                </span>
              </div>
            </div>
            <div class="profile-info-item" style="margin-top: 12px;">
              <span class="p-label">注册时间</span>
              <span class="p-val cell-mono">{{ createdText }}</span>
            </div>
          </div>
        </div>

        <!-- 修改登录密码 -->
        <div class="x-card">
          <div class="x-card-head">
            <span><el-icon><Lock /></el-icon>&nbsp;修改登录密码</span>
          </div>
          <div class="x-card-body">
            <el-form label-position="top">
              <el-form-item label="当前密码" style="margin-bottom: 12px">
                <el-input v-model="pwd.old" type="password" show-password placeholder="请输入当前密码以验证身份" />
              </el-form-item>
              <div class="form-grid-2">
                <el-form-item label="新密码" style="margin-bottom: 12px">
                  <el-input v-model="pwd.next" type="password" show-password placeholder="至少 8 位字符" />
                </el-form-item>
                <el-form-item label="确认新密码" style="margin-bottom: 12px">
                  <el-input v-model="pwd.confirm" type="password" show-password placeholder="再次输入新密码" />
                </el-form-item>
              </div>
              <el-button type="primary" :loading="savingPwd" style="margin-top: 4px" @click="savePwd">
                更新登录密码
              </el-button>
            </el-form>
          </div>
        </div>

        <!-- 两步验证与凭据安全 -->
        <div class="x-card">
          <div class="x-card-head">
            <span><el-icon><Key /></el-icon>&nbsp;安全认证与凭证</span>
          </div>
          <div class="x-card-body" style="display: flex; flex-direction: column; gap: 14px;">
            <!-- 2FA -->
            <div class="x-toggle-card">
              <div class="toggle-info">
                <div class="toggle-title">双因素身份验证 (2FA / TOTP)</div>
                <div class="toggle-desc">使用 Google Authenticator 或 1Password 等身份验证器生成动态 6 位验证码。</div>
              </div>
              <div>
                <el-button
                  v-if="!auth.user?.totp_enabled"
                  type="primary"
                  plain
                  size="small"
                  :loading="twofaLoading"
                  @click="startTwofaSetup"
                >
                  <el-icon><Unlock /></el-icon>&nbsp;开启验证
                </el-button>
                <el-button
                  v-else
                  type="danger"
                  plain
                  size="small"
                  @click="twofaStep = 'disable'; disableForm.code = ''; disableForm.password = ''; twofaOpen = true"
                >
                  <el-icon><Key /></el-icon>&nbsp;关闭验证
                </el-button>
              </div>
            </div>

            <!-- 重置订阅 Token -->
            <div class="x-toggle-card">
              <div class="toggle-info">
                <div class="toggle-title">Mihomo 订阅访问凭据</div>
                <div class="toggle-desc">若怀疑订阅地址外泄，可重置凭据。重置后旧链接失效，需重新导入客户端。</div>
              </div>
              <div>
                <el-button
                  type="danger"
                  plain
                  size="small"
                  :loading="resettingSub"
                  @click="onResetSubscribe"
                >
                  <el-icon><RefreshRight /></el-icon>&nbsp;重置凭据
                </el-button>
              </div>
            </div>
          </div>
        </div>

        <el-button type="danger" plain style="width: 100%; margin-bottom: 24px" @click="onLogout">
          退出当前登录账号
        </el-button>
      </div>
    </div>

    <!-- 余额明细抽屉 -->
    <el-drawer v-model="balanceLogsOpen" title="账户收支明细" size="460px">
      <div v-loading="loadingLogs">
        <div v-if="balanceLogs.length" class="log-list">
          <div v-for="log in balanceLogs" :key="log.id" class="log-item">
            <div class="log-left">
              <div class="log-title">{{ log.remark || log.type }}</div>
              <div class="log-time cell-mono">{{ String(log.created_at).replace('T', ' ').slice(0, 16) }}</div>
            </div>
            <div class="log-right">
              <div class="log-amount cell-mono" :class="{ plus: log.amount_cents > 0, minus: log.amount_cents < 0 }">
                {{ log.amount_cents > 0 ? '+' : '' }}¥ {{ (log.amount_cents / 100).toFixed(2) }}
              </div>
              <div class="log-balance cell-mono">结余: ¥ {{ (log.balance_after / 100).toFixed(2) }}</div>
            </div>
          </div>
        </div>
        <div v-else class="muted" style="text-align: center; padding: 40px 0">
          暂无余额变动记录
        </div>
      </div>
    </el-drawer>

    <!-- 两步验证弹窗 -->
    <el-dialog
      v-model="twofaOpen"
      :title="twofaStep === 'disable' ? '关闭两步验证' : '开启两步验证'"
      width="400px"
      class="twofa-dialog"
      :close-on-click-modal="false"
      append-to-body
    >
      <!-- 绑定：扫码 + 输入验证码 -->
      <template v-if="twofaStep === 'confirm'">
        <div style="display: flex; flex-direction: column; align-items: center; gap: 10px">
          <img
            v-if="twofaQrUrl"
            :src="twofaQrUrl"
            alt="TOTP 二维码"
            style="width: 200px; height: 200px; border-radius: 10px; border: 1px solid var(--x-border)"
          />
          <p class="muted" style="font-size: 12px; text-align: center">
            使用 Google Authenticator / 1Password 扫码，或手动输入密钥：
            <code class="cell-mono" style="user-select: all">{{ twofaSecret }}</code>
          </p>
          <el-input
            v-model="twofaCode"
            placeholder="输入 App 显示的 6 位动态验证码"
            maxlength="6"
            size="large"
            style="font-family: var(--x-font-mono)"
          />
          <el-button type="primary" style="width: 100%" :loading="twofaLoading" @click="submitTwofaConfirm">
            确认绑定
          </el-button>
        </div>
      </template>

      <!-- 备份码（仅展示一次） -->
      <template v-else-if="twofaStep === 'backup'">
        <el-alert type="warning" :closable="false" style="margin-bottom: 14px">
          请立即妥善保存以下恢复码（仅展示一次）。设备丢失时可用恢复码登录，每个恢复码只能使用一次。
        </el-alert>
        <div class="backup-codes">
          <code v-for="c in backupCodes" :key="c" class="cell-mono">{{ c }}</code>
        </div>
        <div style="display: flex; gap: 10px; margin-top: 14px">
          <el-button style="flex: 1" @click="copyBackupCodes">
            <el-icon><CopyDocument /></el-icon>&nbsp;复制全部
          </el-button>
          <el-button type="primary" style="flex: 1" @click="twofaOpen = false">
            我已保存
          </el-button>
        </div>
      </template>

      <!-- 关闭：验证 -->
      <template v-else-if="twofaStep === 'disable'">
        <p class="muted" style="font-size: 13px; margin-bottom: 12px">
          请输入动态验证码、恢复码或当前密码（任选其一）以确认身份
        </p>
        <el-input
          v-model="disableForm.code"
          placeholder="动态验证码或恢复码"
          maxlength="16"
          style="font-family: var(--x-font-mono); margin-bottom: 10px"
        />
        <el-input
          v-model="disableForm.password"
          type="password"
          show-password
          placeholder="或当前密码"
          style="margin-bottom: 14px"
        />
        <el-button type="danger" style="width: 100%" :loading="twofaLoading" @click="submitTwofaDisable">
          关闭两步验证
        </el-button>
      </template>
    </el-dialog>
  </div>
</template>

<style scoped lang="scss">
.balance-overview-box {
  display: flex;
  justify-content: space-between;
  align-items: center;
  padding: 16px 18px;
  background: var(--x-card-soft);
  border: 1px solid var(--x-border);
  border-radius: var(--x-radius-sm);
  margin-bottom: 16px;

  .bal-col {
    display: flex;
    flex-direction: column;
    gap: 3px;
  }
  .bal-label {
    font-size: 12px;
    color: var(--x-text-3);
    font-weight: 500;
  }
  .bal-value {
    font-size: 24px;
    font-weight: 800;
    color: var(--x-text);
  }
}

.autorenew-row {
  display: flex;
  justify-content: space-between;
  align-items: center;
  gap: 12px;

  .autorenew-title {
    font-size: 13.5px;
    font-weight: 600;
    margin-bottom: 2px;
  }
}

.topup-form-section {
  padding-top: 12px;
  border-top: 1px dashed var(--x-border);

  .section-label {
    font-size: 13px;
    font-weight: 600;
    color: var(--x-text);
    margin-bottom: 4px;
    display: flex;
    align-items: center;
  }
}

.redeem-box {
  display: flex;
  gap: 10px;
}

.profile-info-item {
  display: flex;
  flex-direction: column;
  gap: 3px;

  .p-label {
    font-size: 11.5px;
    color: var(--x-text-3);
    font-weight: 500;
  }
  .p-val {
    font-size: 13.5px;
    font-weight: 600;
    color: var(--x-text);
  }
}

.log-list {
  display: flex;
  flex-direction: column;
  gap: 8px;
}

.log-item {
  display: flex;
  justify-content: space-between;
  align-items: center;
  padding: 10px 12px;
  background: var(--x-card-soft);
  border: 1px solid var(--x-border);
  border-radius: var(--x-radius-sm);

  .log-title {
    font-size: 13px;
    font-weight: 600;
    color: var(--x-text);
  }
  .log-time {
    font-size: 11px;
    color: var(--x-text-3);
    margin-top: 1px;
  }

  .log-amount {
    font-size: 14px;
    font-weight: 700;
    text-align: right;
    &.plus {
      color: var(--x-success);
    }
    &.minus {
      color: var(--x-text);
    }
  }
  .log-balance {
    font-size: 11px;
    color: var(--x-text-3);
    text-align: right;
  }
}

.muted {
  color: var(--x-text-3);
}

.backup-codes {
  display: flex;
  flex-wrap: wrap;
  gap: 8px;
  padding: 12px;
  background: var(--x-card-soft);
  border: 1px dashed var(--x-border);
  border-radius: 8px;
}
.backup-codes code {
  font-family: var(--x-font-mono);
  font-size: 12.5px;
  padding: 4px 8px;
  background: var(--x-card);
  border-radius: 4px;
  border: 1px solid var(--x-border);
  user-select: all;
}
</style>