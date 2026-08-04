<script setup lang="ts">
import { computed, onMounted, reactive, ref } from 'vue'
import { useRouter } from 'vue-router'
import { CopyDocument, Key, Wallet, DataLine } from '@element-plus/icons-vue'
import { useAuthStore } from '@/stores/auth'
import { buildSubscribeUrl } from '@/config/site'
import { changePassword, updateProfile } from '@/api/user'
import { errMsg } from '@/api/http'
import { formatBytes } from '@/utils/format'

const auth = useAuthStore()
const router = useRouter()

const email = ref('')
const createdText = ref('—')

const usedBytes = computed(() => auth.user?.used_bytes ?? 0)
const totalBytes = computed(() => auth.user?.total_bytes ?? 0)
const usagePercent = computed(() => {
  if (!totalBytes.value) return 0
  return Math.min(100, Math.round((usedBytes.value / totalBytes.value) * 100))
})
const expireText = computed(() => {
  const t = auth.user?.expire_at
  return t ? String(t).replace('T', ' ').slice(0, 16) : '—'
})
const planLabel = computed(() => (auth.user?.plan_id ? `套餐 #${auth.user.plan_id}` : '暂无套餐'))
const subscribeUrl = computed(() => {
  const token = auth.user?.subscribe_token
  return token ? buildSubscribeUrl(token) : ''
})

async function refresh() {
  try {
    if (!auth.user) await auth.fetchMe()
    else await auth.fetchMe()
    email.value = auth.user?.email ?? ''
    createdText.value = (auth.user?.created_at ?? '').replace('T', ' ').slice(0, 16) || '—'
  } catch (e) {
    ElMessage.error(errMsg(e, '加载资料失败'))
  }
}
onMounted(refresh)

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
  navigator.clipboard?.writeText(text).then(
    () => ElMessage.success(`${label}已复制`),
    () => ElMessage.warning('复制失败，请手动复制'),
  )
}

function onLogout() {
  auth.logout()
  router.replace('/login')
}
</script>

<template>
  <div class="x-client-body">
    <div class="x-acct-grid">
      <div>
        <!-- 套餐用量 -->
        <div class="x-usage-hero">
          <div style="font-size: 12.5px; opacity: 0.85">当前套餐 · {{ planLabel }}</div>
          <div style="font-size: 24px; font-weight: 700; margin-top: 4px">{{ formatBytes(usedBytes) }} / {{ totalBytes ? formatBytes(totalBytes) : '—' }}</div>
          <el-progress :percentage="usagePercent" :show-text="false" :stroke-width="7" color="#fff" class="hero-progress" />
          <div class="x-plan-meta">
            <span>剩余 {{ totalBytes ? formatBytes(Math.max(0, totalBytes - usedBytes)) : '—' }}</span>
            <span>到期 {{ expireText }}</span>
          </div>
        </div>

        <!-- 订阅信息 -->
        <div class="x-card">
          <div class="x-card-head"><span>订阅信息</span></div>
          <div class="x-card-body">
            <div class="x-row-line"><span class="k">订阅链接</span><span class="v"><code class="cell-mono">{{ subscribeUrl || '—' }}</code></span></div>
            <div style="display: flex; gap: 10px; margin-top: 16px; flex-wrap: wrap">
              <el-button size="small" type="primary" @click="copy(subscribeUrl, '订阅链接')"><el-icon><CopyDocument /></el-icon>&nbsp;复制订阅链接</el-button>
              <router-link to="/subscribe"><el-button size="small"><el-icon><DataLine /></el-icon>&nbsp;订阅中心</el-button></router-link>
            </div>
            <p class="muted" style="font-size: 12px; margin-top: 10px">导入 Clash 系客户端后即可连接节点，流量自动统计</p>
          </div>
        </div>
      </div>

      <div>
        <!-- 用户资料 -->
        <div class="x-card">
          <div class="x-card-head"><span>用户资料</span></div>
          <div class="x-card-body">
            <el-form label-position="top">
              <el-form-item label="用户名"><el-input :model-value="auth.username" disabled /></el-form-item>
              <el-form-item label="邮箱"><el-input v-model="email" placeholder="用于接收通知（选填）" /></el-form-item>
              <el-form-item label="注册时间"><span class="muted">{{ createdText }}</span></el-form-item>
              <el-button type="primary" style="width: 100%" :loading="savingProfile" @click="saveProfile">保存资料</el-button>
            </el-form>
          </div>
        </div>

        <!-- 修改密码 -->
        <div class="x-card">
          <div class="x-card-head"><span>修改密码</span></div>
          <div class="x-card-body">
            <el-form label-position="top">
              <el-form-item label="当前密码"><el-input v-model="pwd.old" type="password" show-password /></el-form-item>
              <el-form-item label="新密码"><el-input v-model="pwd.next" type="password" show-password placeholder="至少 8 位" /></el-form-item>
              <el-form-item label="确认新密码"><el-input v-model="pwd.confirm" type="password" show-password /></el-form-item>
              <el-button style="width: 100%" :loading="savingPwd" @click="savePwd">修改密码</el-button>
            </el-form>
          </div>
        </div>

        <el-button type="danger" plain style="width: 100%" @click="onLogout"><el-icon><Wallet /></el-icon>&nbsp;退出登录</el-button>
      </div>
    </div>
  </div>
</template>

<style scoped lang="scss">
.cell-mono { font-family: ui-monospace, Menlo, Consolas, monospace; font-size: 12px; color: var(--x-text-2); word-break: break-all; }
.muted { color: var(--x-text-3); }
.hero-progress { margin-top: 10px; --el-progress-bg-color: #fff; }
</style>