<script setup lang="ts">
import { computed, onMounted, reactive, ref } from 'vue'
import { useRouter } from 'vue-router'
import {
  CopyDocument,
  Wallet,
  Promotion,
  Cellphone,
  Link,
  DataLine,
  User,
  Calendar,
  Lock,
  Ticket,
  List,
} from '@element-plus/icons-vue'
import QRCode from 'qrcode'
import { useAuthStore } from '@/stores/auth'
import { buildSubscribeUrl } from '@/config/site'
import { changePassword, updateProfile } from '@/api/user'
import { redeemGiftCard, getMyBalanceLogs } from '@/api/gift_card'
import { errMsg } from '@/api/http'
import { formatBytes } from '@/utils/format'
import type { BalanceLog } from '@/api/types'

const auth = useAuthStore()
const router = useRouter()

const email = ref('')
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
  return t ? String(t).replace('T', ' ').slice(0, 16) : '永久有效'
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
  return token ? buildSubscribeUrl(token) : ''
})
const vlessBase64Url = computed(() => {
  if (!subscribeUrl.value) return ''
  const sep = subscribeUrl.value.includes('?') ? '&' : '?'
  return `${subscribeUrl.value}${sep}format=base64`
})

async function refresh() {
  try {
    await auth.fetchMe()
    email.value = auth.user?.email ?? ''
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
      ElMessage.success(`🎉 充值成功！增加余额 ¥ ${(data.data.face_value_cents / 100).toFixed(2)}`)
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

// 资料
const savingProfile = ref(false)
async function saveProfile() {
  savingProfile.value = true
  try {
    const { data } = await updateProfile(email.value)
    if (data.code === 0) {
      ElMessage.success('资料已保存')
      await auth.fetchMe()
    } else {
      ElMessage.error(data.message)
    }
  } catch (e) {
    ElMessage.error(errMsg(e, '保存失败'))
  } finally {
    savingProfile.value = false
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

function onLogout() {
  auth.logout()
  router.replace('/login')
}
</script>

<template>
  <div class="x-client-body">
    <div class="x-acct-grid">
      <!-- 左侧：用量、钱包与礼品卡充值 -->
      <div>
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
              <el-button text size="small" @click="copy(vlessBase64Url, 'VLESS Base64 订阅')">
                <el-icon><Link /></el-icon>&nbsp;复制 VLESS 通用订阅 (Base64)
              </el-button>
              <el-button text size="small" @click="qrModalOpen = true">
                <el-icon><Cellphone /></el-icon>&nbsp;手机扫码
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
              <el-form-item label="用户名">
                <el-input :model-value="auth.username" disabled />
              </el-form-item>
              <el-form-item label="邮箱">
                <el-input v-model="email" placeholder="用于接收通知（选填）" />
              </el-form-item>
              <el-form-item label="注册时间">
                <span class="muted cell-mono" style="font-size: 13px">{{ createdText }}</span>
              </el-form-item>
              <el-button type="primary" style="width: 100%" :loading="savingProfile" @click="saveProfile">
                保存资料
              </el-button>
            </el-form>
          </div>
        </div>

        <!-- 修改密码 -->
        <div class="x-card">
          <div class="x-card-head">
            <span><el-icon><Lock /></el-icon>&nbsp;修改登录密码</span>
          </div>
          <div class="x-card-body">
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

        <el-button type="danger" plain style="width: 100%; margin-bottom: 24px" @click="onLogout">
          <el-icon><Wallet /></el-icon>&nbsp;退出登录
        </el-button>
      </div>
    </div>

    <!-- 余额明细抽屉 -->
    <el-drawer v-model="balanceLogsOpen" title="💰 账户余额变动明细" size="460px">
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
  </div>
</template>

<style scoped lang="scss">
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
</style>