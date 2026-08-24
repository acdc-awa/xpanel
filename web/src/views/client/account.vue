<script setup lang="ts">
import { computed, onMounted, reactive, ref } from 'vue'
import { useRouter } from 'vue-router'
import {
  CopyDocument,
  Wallet,
  Promotion,
  Cellphone,
  DataLine,
  User,
  Calendar,
  Lock,
  Key,
  Unlock,
  RefreshRight,
  Ticket,
  List,
  WarningFilled,
  InfoFilled,
} from '@element-plus/icons-vue'
import QRCode from 'qrcode'
import { useAuthStore } from '@/stores/auth'
import { useSiteStore } from '@/stores/site'
import { buildSubscribeUrl } from '@/config/site'
import { changePassword, setup2FA, confirm2FA, disable2FA, resetSubscribeToken } from '@/api/user'
import { redeemGiftCard, getMyBalanceLogs } from '@/api/gift_card'
import { errMsg } from '@/api/http'
import { formatBytes } from '@/utils/format'
import type { BalanceLog } from '@/api/types'

const auth = useAuthStore()
const site = useSiteStore()
const router = useRouter()

const createdText = ref('—')
const qrDataUrl = ref('')
const qrModalOpen = ref(false)

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

const usedBytes = computed(() => auth.user?.used_bytes ?? 0)
const totalBytes = computed(() => auth.user?.total_bytes ?? 0)
const usagePercent = computed(() => {
  if (!totalBytes.value) return 0
  return Math.min(100, Math.round((usedBytes.value / totalBytes.value) * 100))
})
const expireText = computed(() => {
  const t = auth.user?.expire_at
  if (!t) return auth.user?.plan_id ? '永久有效' : '未开通套餐'
  return String(t).replace('T', ' ').slice(0, 16)
})

// U23：到期/未开通判定（横幅 CTA，避免无套餐用户看到误导性的「永久有效」）
const isExpired = computed(() => {
  const t = auth.user?.expire_at
  return !!t && new Date(t).getTime() < Date.now()
})
const daysLeft = computed(() => {
  if (!auth.user?.expire_at) return null
  const exp = new Date(auth.user.expire_at).getTime()
  const now = Date.now()
  const diff = Math.ceil((exp - now) / (1000 * 60 * 60 * 24))
  return diff > 0 ? diff : 0
})

const planLabel = computed(() => (auth.user?.plan_id ? `套餐 #${auth.user.plan_id}` : '暂无套餐'))
const subscribeUrl = computed(() => {
  const token = auth.user?.subscribe_token
  return token ? buildSubscribeUrl(token, site.subscribeUrl, site.subscribePath) : ''
})

async function refresh() {
  try {
    await auth.fetchMe()
    createdText.value = (auth.user?.created_at ?? '').replace('T', ' ').slice(0, 16) || '—'
    if (subscribeUrl.value) {
      qrDataUrl.value = await QRCode.toDataURL(subscribeUrl.value, {
        width: 240,
        margin: 2,
        color: { dark: '#171b2e', light: '#ffffff' },
      })
    }
  } catch (e) {
    ElMessage.error(errMsg(e, '加载资料失败'))
  }
}
onMounted(refresh)

// 兑换礼品卡
async function submitRedeem() {
  const code = redeemCode.value.trim()
  if (!code) {
    ElMessage.warning('请输入礼品卡卡密')
    return
  }
  redeeming.value = true
  try {
    const { data } = await redeemGiftCard(code)
    if (data.code === 0) {
      ElMessage.success(`充值成功！增加余额 ¥ ${(data.data.face_value_cents / 100).toFixed(2)}`)
      redeemCode.value = ''
      await auth.fetchMe()
    } else {
      ElMessage.error(data.message)
    }
  } catch (e) {
    ElMessage.error(errMsg(e, '兑换失败'))
  } finally {
    redeeming.value = false
  }
}

// 查看余额流水
async function openBalanceLogs() {
  balanceLogsOpen.value = true
  loadingLogs.value = true
  try {
    const { data } = await getMyBalanceLogs(1, 50)
    if (data.code === 0) {
      balanceLogs.value = data.data.items
    }
  } catch (e) {
    ElMessage.error(errMsg(e, '加载流水失败'))
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
    ElMessage.warning('新密码至少 8 位')
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
      ElMessage.success('密码已修改')
      pwd.old = ''
      pwd.next = ''
      pwd.confirm = ''
    } else {
      ElMessage.error(data.message)
    }
  } catch (e) {
    ElMessage.error(errMsg(e, '修改失败'))
  } finally {
    savingPwd.value = false
  }
}

function copy(text: string, label: string) {
  if (!text) return
  navigator.clipboard?.writeText(text).then(
    () => ElMessage.success(`${label}已复制`),
    () => ElMessage.warning('复制失败，请手动复制'),
  )
}

function importClash() {
  if (!subscribeUrl.value) return
  const url = `clash://install-config?url=${encodeURIComponent(subscribeUrl.value)}&name=XrayPanel`
  window.location.href = url
  ElMessage.info('正在唤醒 Clash 客户端…')
}

// ---- 两步验证（TOTP，2026-08-14 方向③）----
const twofaOpen = ref(false)
const twofaStep = ref<'setup' | 'confirm' | 'backup' | 'disable'>('setup')
const twofaSecret = ref('')
const twofaQrUrl = ref('')
const twofaCode = ref('')
const twofaLoading = ref(false)
const backupCodes = ref<string[]>([])

// 关闭验证：需验证码/恢复码/密码任其一
const disableForm = reactive({ code: '', password: '' })

async function openTwofaSetup() {
  twofaStep.value = 'setup'
  twofaCode.value = ''
  backupCodes.value = []
  twofaOpen.value = true
  twofaLoading.value = true
  try {
    const { data } = await setup2FA()
    if (data.code === 0) {
      twofaSecret.value = data.data.secret
      twofaQrUrl.value = await QRCode.toDataURL(data.data.otpauth_url, { width: 220, margin: 1 })
      twofaStep.value = 'confirm'
    } else {
      ElMessage.error(data.message)
      twofaOpen.value = false
    }
  } catch (e) {
    ElMessage.error(errMsg(e, '获取绑定参数失败'))
    twofaOpen.value = false
  } finally {
    twofaLoading.value = false
  }
}

async function submitTwofaConfirm() {
  if (!twofaCode.value) {
    ElMessage.warning('请输入 6 位动态验证码')
    return
  }
  twofaLoading.value = true
  try {
    const { data } = await confirm2FA(twofaSecret.value, twofaCode.value.trim())
    if (data.code === 0) {
      backupCodes.value = data.data.backup_codes
      twofaStep.value = 'backup'
      await auth.fetchMe()
    } else {
      ElMessage.error(data.message)
    }
  } catch (e) {
    ElMessage.error(errMsg(e, '绑定失败'))
  } finally {
    twofaLoading.value = false
  }
}

async function submitTwofaDisable() {
  if (!disableForm.code && !disableForm.password) {
    ElMessage.warning('请输入动态验证码/恢复码或当前密码')
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

// ---- 重置订阅密钥（17 号 P0 ⑤）----
const resettingSub = ref(false)
async function onResetSubscribe() {
  try {
    await ElMessageBox.confirm(
      '重置后旧订阅链接立即失效，所有客户端需重新导入新链接。确认重置？',
      '重置订阅密钥',
      { type: 'warning' },
    )
  } catch {
    return
  }
  resettingSub.value = true
  try {
    const { data } = await resetSubscribeToken()
    if (data.code === 0) {
      ElMessage.success('订阅密钥已重置，请使用新链接重新导入客户端')
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
      <!-- 左侧：用量、钱包与礼品卡充值 -->
      <div>
        <!-- U23：到期/未开通 CTA 横幅 -->
        <div v-if="isExpired" class="acct-alert danger">
          <el-icon><WarningFilled /></el-icon>&nbsp;
          <span>套餐已到期，节点将无法使用，请及时续费</span>
          <router-link to="/shop">
            <el-button size="small" type="primary" round>立即续费</el-button>
          </router-link>
        </div>
        <div v-else-if="!auth.user?.plan_id" class="acct-alert">
          <el-icon><InfoFilled /></el-icon>&nbsp;
          <span>尚未开通套餐，开通后即可获取节点订阅</span>
          <router-link to="/shop">
            <el-button size="small" type="primary" round>去开通</el-button>
          </router-link>
        </div>

        <!-- 账户余额与用量 Hero 卡片 -->
        <div class="acct-hero">
          <div class="acct-hero-top">
            <span class="hero-tag">当前套餐 · {{ planLabel }}</span>
            <span v-if="daysLeft !== null" class="hero-expire-pill">
              <el-icon><Calendar /></el-icon>&nbsp;剩余 {{ daysLeft }} 天
            </span>
          </div>

          <div class="acct-hero-stats">
            <div class="stat-item">
              <div class="stat-lbl">账户可用余额</div>
              <div class="stat-money">¥ {{ balanceYuan }}</div>
            </div>
            <div class="stat-divider-v" />
            <div class="stat-item">
              <div class="stat-lbl">已用 / 总量</div>
              <div class="stat-traffic">
                {{ formatBytes(usedBytes) }} <small>/ {{ totalBytes ? formatBytes(totalBytes) : '0 B' }}</small>
              </div>
            </div>
          </div>

          <div class="acct-progress-bg">
            <div class="acct-progress-bar" :style="{ width: usagePercent + '%' }" />
          </div>

          <div class="acct-plan-meta">
            <span>剩余流量: {{ totalBytes ? formatBytes(Math.max(0, totalBytes - usedBytes)) : '0 B' }}</span>
            <span>到期: {{ expireText }}</span>
          </div>
        </div>

        <!-- 礼品卡充值兑换卡片 -->
        <div class="x-card">
          <div class="x-card-head">
            <span><el-icon><Ticket /></el-icon>&nbsp;礼品卡充值余额</span>
            <el-button text size="small" type="primary" @click="openBalanceLogs">
              <el-icon><List /></el-icon>&nbsp;余额明细
            </el-button>
          </div>
          <div class="x-card-body">
            <p class="muted" style="font-size: 13px; margin-bottom: 12px">
              输入管理员发放或购买的充值卡密（格式如 <code>GIFT-XXXX-XXXX-XXXX-XXXX</code>），充值后余额即时到账，可在套餐商店一键免审核开通套餐。
            </p>
            <div class="redeem-box">
              <el-input
                v-model="redeemCode"
                placeholder="请输入礼品卡充值卡密"
                clearable
                class="cell-mono"
                @keyup.enter="submitRedeem"
              />
              <el-button type="primary" :loading="redeeming" @click="submitRedeem">
                立即充值
              </el-button>
            </div>
          </div>
        </div>

        <!-- 订阅管理卡片 -->
        <div class="x-card">
          <div class="x-card-head">
            <span><el-icon><Promotion /></el-icon>&nbsp;Clash 订阅信息</span>
          </div>
          <div class="x-card-body">
            <div class="sub-link-preview">
              <span class="sub-link-lbl">订阅链接</span>
              <code class="cell-mono sub-link-val">{{ subscribeUrl || '暂无可用订阅链接' }}</code>
            </div>

            <div class="sub-btns">
              <el-button type="primary" class="glow-btn" @click="importClash">
                <el-icon><Promotion /></el-icon>&nbsp;一键导入 Clash
              </el-button>
              <el-button @click="copy(subscribeUrl, 'Clash 订阅链接')">
                <el-icon><CopyDocument /></el-icon>&nbsp;复制链接
              </el-button>
              <router-link to="/subscribe">
                <el-button plain>
                  <el-icon><DataLine /></el-icon>&nbsp;订阅中心
                </el-button>
              </router-link>
            </div>

            <div class="sub-extra-links">
              <el-button text size="small" @click="qrModalOpen = true">
                <el-icon><Cellphone /></el-icon>&nbsp;手机扫码
              </el-button>
              <el-button text size="small" type="danger" :loading="resettingSub" @click="onResetSubscribe">
                <el-icon><RefreshRight /></el-icon>&nbsp;重置订阅密钥
              </el-button>
            </div>
          </div>
        </div>
      </div>

      <!-- 右侧：资料与安全 -->
      <div>
        <!-- 用户资料 -->
        <div class="x-card">
          <div class="x-card-head">
            <span><el-icon><User /></el-icon>&nbsp;用户资料</span>
          </div>
          <div class="x-card-body">
            <el-form label-position="top">
              <el-form-item label="邮箱（用户名）">
                <el-input :model-value="auth.user?.email ?? ''" disabled />
              </el-form-item>
              <el-form-item label="注册时间">
                <span class="muted cell-mono" style="font-size: 13px">{{ createdText }}</span>
              </el-form-item>
            </el-form>
            <p class="muted" style="font-size: 12.5px; margin-top: -4px">
              邮箱即登录用户名，如需修改请联系管理员
            </p>
          </div>
        </div>

        <!-- 修改密码 -->
        <div class="x-card">
          <div class="x-card-head">
            <span><el-icon><Lock /></el-icon>&nbsp;修改登录密码</span>
            <el-tag v-if="auth.user?.must_change_pwd" type="danger" size="small" effect="plain">首次登录需修改</el-tag>
          </div>
          <div class="x-card-body">
            <el-alert
              v-if="auth.user?.must_change_pwd"
              type="warning"
              :closable="false"
              style="margin-bottom: 14px"
              title="当前为初始密码，为保障账号安全请立即修改"
            />
            <el-form label-position="top">
              <el-form-item label="当前密码">
                <el-input v-model="pwd.old" type="password" show-password placeholder="请输入当前密码" />
              </el-form-item>
              <el-form-item label="新密码">
                <el-input v-model="pwd.next" type="password" show-password placeholder="至少 8 位" />
              </el-form-item>
              <el-form-item label="确认新密码">
                <el-input v-model="pwd.confirm" type="password" show-password placeholder="再次输入新密码" />
              </el-form-item>
              <el-button style="width: 100%" :loading="savingPwd" @click="savePwd">
                修改密码
              </el-button>
            </el-form>
          </div>
        </div>

        <!-- 两步验证（TOTP） -->
        <div class="x-card">
          <div class="x-card-head">
            <span><el-icon><Key /></el-icon>&nbsp;两步验证（Google Authenticator）</span>
            <el-tag v-if="auth.user?.totp_enabled" type="success" size="small" effect="plain">已开启</el-tag>
            <el-tag v-else type="info" size="small" effect="plain">未开启</el-tag>
          </div>
          <div class="x-card-body">
            <p class="muted" style="font-size: 13px; margin-bottom: 12px; line-height: 1.7">
              开启后登录需要动态验证码；<b>忘记密码重置必须使用两步验证码</b>（未开启的账号请联系管理员重置密码）。建议同时妥善保存恢复码。
            </p>
            <el-button
              v-if="!auth.user?.totp_enabled"
              type="primary"
              plain
              style="width: 100%"
              @click="openTwofaSetup"
            >
              <el-icon><Unlock /></el-icon>&nbsp;开启两步验证
            </el-button>
            <el-button
              v-else
              type="danger"
              plain
              style="width: 100%"
              @click="twofaStep = 'disable'; disableForm.code = ''; disableForm.password = ''; twofaOpen = true"
            >
              <el-icon><Key /></el-icon>&nbsp;关闭两步验证
            </el-button>
          </div>
        </div>

        <el-button type="danger" plain style="width: 100%; margin-bottom: 24px" @click="onLogout">
          <el-icon><Wallet /></el-icon>&nbsp;退出登录
        </el-button>
      </div>
    </div>

    <!-- 余额明细抽屉 -->
    <el-drawer v-model="balanceLogsOpen" title="账户余额变动明细" size="460px">
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

    <!-- 扫码弹窗 -->
    <el-dialog v-model="qrModalOpen" title="手机扫码导入订阅" width="320px" append-to-body center>
      <div style="display: flex; flex-direction: column; align-items: center; gap: 12px; padding: 10px 0">
        <img v-if="qrDataUrl" :src="qrDataUrl" alt="订阅二维码" style="width: 220px; height: 220px; border-radius: 8px; border: 1px solid var(--x-border)" />
        <p class="muted" style="font-size: 12px; text-align: center">
          支持 Stash / Flclash 手机扫码一键添加配置。
        </p>
      </div>
    </el-dialog>

    <!-- 两步验证弹窗 -->
    <el-dialog
      v-model="twofaOpen"
      :title="twofaStep === 'disable' ? '关闭两步验证' : '开启两步验证'"
      width="400px"
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
            使用 Google Authenticator / Authy 等扫码，或手动输入密钥：
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
          请立即保存以下恢复码（仅此一次展示）。设备丢失时用恢复码登录或重置密码，每个恢复码只能使用一次。
        </el-alert>
        <div class="backup-codes">
          <code v-for="c in backupCodes" :key="c" class="cell-mono">{{ c }}</code>
        </div>
        <el-button type="primary" style="width: 100%; margin-top: 14px" @click="twofaOpen = false">
          我已保存
        </el-button>
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
.acct-alert {
  display: flex;
  align-items: center;
  gap: 8px;
  padding: 12px 16px;
  border-radius: var(--x-radius);
  margin-bottom: 14px;
  font-size: 13.5px;
  background: rgba(56, 189, 248, 0.1);
  border: 1px solid rgba(56, 189, 248, 0.3);
  color: var(--x-text);
  > span { flex: 1; }
  &.danger {
    background: rgba(244, 63, 94, 0.1);
    border-color: rgba(244, 63, 94, 0.35);
    color: #fb7185;
  }
}
.acct-hero {
  background: linear-gradient(135deg, #4f46e5 0%, #7c3aed 100%);
  border-radius: var(--x-radius);
  padding: 24px;
  color: #fff;
  box-shadow: 0 8px 24px rgba(79, 70, 229, 0.22);
  margin-bottom: 16px;

  .acct-hero-top {
    display: flex;
    justify-content: space-between;
    align-items: center;
    margin-bottom: 14px;

    .hero-tag {
      font-size: 13px;
      font-weight: 600;
      opacity: 0.9;
    }
    .hero-expire-pill {
      display: inline-flex;
      align-items: center;
      background: rgba(16, 185, 129, 0.25);
      border: 1px solid rgba(16, 185, 129, 0.4);
      padding: 2px 9px;
      border-radius: 12px;
      font-size: 11.5px;
      font-weight: 600;
      color: #a7f3d0;
    }
  }

  .acct-hero-stats {
    display: flex;
    align-items: center;
    justify-content: space-between;
    margin-bottom: 16px;

    .stat-item {
      flex: 1;
    }

    .stat-divider-v {
      width: 1px;
      height: 32px;
      background: rgba(255, 255, 255, 0.2);
      margin: 0 16px;
    }

    .stat-lbl {
      font-size: 11.5px;
      color: rgba(255, 255, 255, 0.75);
    }
    .stat-money {
      font-family: var(--x-font-mono);
      font-size: 24px;
      font-weight: 800;
      color: #fde047;
      margin-top: 2px;
    }
    .stat-traffic {
      font-family: var(--x-font-mono);
      font-size: 20px;
      font-weight: 800;
      color: #ffffff;
      margin-top: 2px;
      small {
        font-size: 13px;
        opacity: 0.8;
      }
    }
  }

  .acct-progress-bg {
    height: 8px;
    background: rgba(255, 255, 255, 0.2);
    border-radius: 4px;
    overflow: hidden;
    margin-bottom: 10px;
  }
  .acct-progress-bar {
    height: 100%;
    background: linear-gradient(90deg, #38bdf8 0%, #fde047 100%);
    border-radius: 4px;
    transition: width 0.4s ease;
  }

  .acct-plan-meta {
    display: flex;
    justify-content: space-between;
    font-size: 12px;
    opacity: 0.85;
    font-family: var(--x-font-mono);
  }
}

.redeem-box {
  display: flex;
  gap: 10px;
}

.sub-link-preview {
  background: var(--x-bg);
  border: 1px solid var(--x-border);
  border-radius: var(--x-radius);
  padding: 10px 14px;
  margin-bottom: 14px;

  .sub-link-lbl {
    display: block;
    font-size: 11px;
    color: var(--x-text-3);
    text-transform: uppercase;
    font-weight: 600;
    margin-bottom: 3px;
  }
  .sub-link-val {
    display: block;
    word-break: break-all;
    font-size: 12px;
    color: var(--x-text);
  }
}

.sub-btns {
  display: flex;
  gap: 8px;
  flex-wrap: wrap;
  margin-bottom: 10px;
}

.sub-extra-links {
  display: flex;
  gap: 8px;
  flex-wrap: wrap;
}

.glow-btn {
  box-shadow: 0 4px 12px rgba(99, 102, 241, 0.35);
}

.log-list {
  display: flex;
  flex-direction: column;
  gap: 12px;
}

.log-item {
  display: flex;
  justify-content: space-between;
  align-items: center;
  padding: 12px 14px;
  background: var(--x-bg);
  border: 1px solid var(--x-border);
  border-radius: var(--x-radius);

  .log-title {
    font-size: 13.5px;
    font-weight: 600;
    color: var(--x-text);
  }
  .log-time {
    font-size: 11.5px;
    color: var(--x-text-3);
    margin-top: 2px;
  }

  .log-amount {
    font-size: 15px;
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
    font-size: 11.5px;
    color: var(--x-text-3);
    text-align: right;
    margin-top: 2px;
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
  background: var(--x-bg);
  border: 1px dashed var(--x-border);
  border-radius: 10px;
}
.backup-codes code {
  font-family: var(--x-font-mono);
  font-size: 12.5px;
  padding: 4px 8px;
  background: var(--x-card);
  border-radius: 6px;
  border: 1px solid var(--x-border);
  user-select: all;
}
</style>