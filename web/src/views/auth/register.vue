<script setup lang="ts">
import { onMounted, reactive, ref } from 'vue'
import { useRouter } from 'vue-router'
import { Lock, Message, Ticket } from '@element-plus/icons-vue'
import { useAuthStore } from '@/stores/auth'
import { errMsg } from '@/api/http'
import { getPublicConfig } from '@/api/config'
import TurnstileWidget from '@/components/TurnstileWidget.vue'

const auth = useAuthStore()
const router = useRouter()

const form = reactive({
  email: '',
  password: '',
  confirm: '',
  invite_code: '',
  turnstile_token: '',
})
const loading = ref(false)
const captchaEnabled = ref(false)
const siteKey = ref('')

const EMAIL_RE = /^[^\s@]+@[^\s@]+\.[^\s@]+$/

onMounted(async () => {
  try {
    const { data } = await getPublicConfig()
    if (data.code === 0) {
      captchaEnabled.value = data.data.captcha_enable
      siteKey.value = data.data.turnstile_site_key
    }
  } catch {
    // 配置接口失败不阻断注册
  }
})

async function onSubmit() {
  if (!form.email || !EMAIL_RE.test(form.email)) {
    ElMessage.warning('请输入正确的邮箱（用户名即邮箱）')
    return
  }
  if (!form.invite_code) {
    ElMessage.warning('请填写邀请码（注册需要邀请码）')
    return
  }
  if (form.password.length < 8) {
    ElMessage.warning('密码至少 8 位')
    return
  }
  if (form.password !== form.confirm) {
    ElMessage.warning('两次输入的密码不一致')
    return
  }
  if (captchaEnabled.value && !form.turnstile_token) {
    ElMessage.warning('请完成人机验证')
    return
  }
  loading.value = true
  try {
    await auth.register({
      email: form.email.trim().toLowerCase(),
      password: form.password,
      invite_code: form.invite_code.trim(),
      turnstile_token: form.turnstile_token,
    })
    ElMessage.success('注册成功，已自动登录')
    router.replace(auth.homePath())
  } catch (e) {
    ElMessage.error(errMsg(e, '注册失败'))
  } finally {
    loading.value = false
  }
}
</script>

<template>
  <div class="auth-stage">
    <div class="auth-card">
      <div class="auth-brand">
        <span class="auth-logo">X</span>
        <div>
          <div class="auth-title">注册账号</div>
          <div class="auth-sub">需要邀请码才能注册</div>
        </div>
      </div>

      <el-form label-position="top" size="large" @submit.prevent="onSubmit">
        <el-form-item label="邮箱（用户名）">
          <el-input v-model="form.email" placeholder="you@example.com" :prefix-icon="Message" autofocus />
        </el-form-item>
        <el-form-item label="邀请码">
          <el-input v-model="form.invite_code" placeholder="请输入邀请码" :prefix-icon="Ticket" />
        </el-form-item>
        <el-form-item label="密码">
          <el-input v-model="form.password" type="password" show-password placeholder="至少 8 位" :prefix-icon="Lock" />
        </el-form-item>
        <el-form-item label="确认密码">
          <el-input v-model="form.confirm" type="password" show-password placeholder="再次输入密码" :prefix-icon="Lock" @keyup.enter="onSubmit" />
        </el-form-item>
        <TurnstileWidget
          v-if="captchaEnabled && siteKey"
          :site-key="siteKey"
          @token="(t) => (form.turnstile_token = t)"
        />
        <el-button type="primary" size="large" class="auth-submit" :loading="loading" native-type="submit">
          注 册
        </el-button>
      </el-form>

      <div class="auth-foot">
        已有账号？
        <router-link class="auth-link" to="/login">直接登录</router-link>
      </div>
    </div>
  </div>
</template>

<style scoped lang="scss">
.auth-stage {
  min-height: 100vh;
  display: flex;
  align-items: center;
  justify-content: center;
  padding: 24px;
  background:
    radial-gradient(1200px 600px at 20% -10%, rgba(99, 102, 241, 0.35), transparent 60%),
    radial-gradient(1000px 500px at 110% 110%, rgba(168, 85, 247, 0.28), transparent 60%),
    var(--x-bg);
}
.auth-card {
  width: 100%;
  max-width: 420px;
  background: var(--x-card);
  border: 1px solid var(--x-border);
  border-radius: 16px;
  box-shadow: var(--x-shadow-lg);
  padding: 30px 28px;
}
.auth-brand { display: flex; align-items: center; gap: 12px; margin-bottom: 22px; }
.auth-logo {
  width: 44px;
  height: 44px;
  border-radius: 13px;
  background: linear-gradient(135deg, #6366f1, #a855f7);
  color: #fff;
  font-size: 21px;
  font-weight: 700;
  display: flex;
  align-items: center;
  justify-content: center;
  flex: none;
  box-shadow: 0 6px 16px rgba(99, 102, 241, 0.35);
}
.auth-title { font-size: 19px; font-weight: 700; }
.auth-sub { font-size: 12.5px; color: var(--x-text-3); margin-top: 3px; }
.auth-submit { width: 100%; margin-top: 6px; font-weight: 600; letter-spacing: 4px; }
.auth-foot { margin-top: 18px; text-align: center; font-size: 13px; color: var(--x-text-2); }
.auth-link { color: var(--x-primary); font-weight: 600; }
</style>