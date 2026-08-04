<script setup lang="ts">
import { onMounted, ref } from 'vue'
import { useRouter } from 'vue-router'
import { CopyDocument, Key, Wallet } from '@element-plus/icons-vue'
import { useAuthStore } from '@/stores/auth'
import { errMsg } from '@/api/http'
import { formatMoney } from '@/utils/format'

const auth = useAuthStore()
const router = useRouter()

const profile = ref({ email: '' })
const pwd = ref({ old: '', next: '', confirm: '' })

const me = auth.user
const createdText = ref('—')

onMounted(async () => {
  try {
    if (!auth.user) await auth.fetchMe()
    profile.value.email = auth.user?.email ?? ''
    createdText.value = (auth.user?.created_at ?? '').replace('T', ' ').slice(0, 16) || '—'
  } catch (e) {
    ElMessage.error(errMsg(e, '加载资料失败'))
  }
})

const subscribeUrl = me?.subscribe_token ? `${location.origin}/api/v1/sub/${me.subscribe_token}` : '—'

function copySubscribe() {
  navigator.clipboard?.writeText(subscribeUrl).then(
    () => ElMessage.success('已复制订阅链接'),
    () => ElMessage.warning('复制失败，请手动复制'),
  )
}

function comingSoon() {
  ElMessage.info('该功能随 P4/P5 开放')
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
        <!-- 余额 -->
        <div class="x-usage-hero">
          <div style="font-size: 12.5px; opacity: 0.85">账户余额</div>
          <div style="font-size: 28px; font-weight: 700; margin-top: 2px">{{ formatMoney(0) }}</div>
          <div class="x-plan-meta">
            <span>余额与充值功能随 P4 上线</span>
            <el-button size="small" style="background: #fff; color: #6366f1; font-weight: 600; border: none" @click="comingSoon">充值</el-button>
          </div>
        </div>

        <!-- 订阅信息 -->
        <div class="x-card">
          <div class="x-card-head"><span>订阅信息</span></div>
          <div class="x-card-body">
            <div class="x-row-line"><span class="k">订阅链接</span><span class="v"><code class="cell-mono">{{ subscribeUrl }}</code></span></div>
            <div class="x-row-line"><span class="k">Token</span><span class="v"><code class="cell-mono">{{ me?.subscribe_token ?? '—' }}</code></span></div>
            <div class="x-row-line"><span class="k">到期时间</span><span class="v">{{ me?.expire_at ? String(me.expire_at).replace('T', ' ').slice(0, 16) : '—' }}</span></div>
            <div style="display: flex; gap: 10px; margin-top: 16px; flex-wrap: wrap">
              <el-button size="small" type="primary" @click="copySubscribe"><el-icon><CopyDocument /></el-icon>&nbsp;复制订阅链接</el-button>
              <el-button size="small" @click="comingSoon"><el-icon><Key /></el-icon>&nbsp;重置 Token</el-button>
            </div>
            <p class="muted" style="font-size: 12px; margin-top: 10px">订阅链接由 P3 订阅生成器提供（Clash YAML / Base64，按 UA 区分）</p>
          </div>
        </div>
      </div>

      <div>
        <!-- 用户资料 -->
        <div class="x-card">
          <div class="x-card-head"><span>用户资料</span></div>
          <div class="x-card-body">
            <el-form label-position="top">
              <el-form-item label="用户名"><el-input :model-value="me?.username ?? auth.username" disabled /></el-form-item>
              <el-form-item label="邮箱"><el-input v-model="profile.email" /></el-form-item>
              <el-form-item label="注册时间"><span class="muted">{{ createdText }}</span></el-form-item>
              <el-button type="primary" style="width: 100%" @click="comingSoon">保存资料</el-button>
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
              <el-button style="width: 100%" @click="comingSoon">修改密码</el-button>
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
</style>