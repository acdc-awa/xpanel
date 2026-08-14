<script setup lang="ts">
import { reactive, ref } from 'vue'
import { useRouter } from 'vue-router'
import { Message, Lock, Key } from '@element-plus/icons-vue'
import { forgotPassword, resetPassword } from '@/api/auth'
import { errMsg } from '@/api/http'

const router = useRouter()

const step = ref<'email' | 'reset'>('email')
const email = ref('')
const form = reactive({ code: '', password: '', confirm: '' })
const loading = ref(false)
const submitting = ref(false)
const hint = ref('')

const EMAIL_RE = /^[^\s@]+@[^\s@]+\.[^\s@]+$/

async function submitEmail() {
  if (!email.value || !EMAIL_RE.test(email.value)) {
    ElMessage.warning('请输入正确的邮箱')
    return
  }
  loading.value = true
  try {
    const { data } = await forgotPassword(email.value.trim().toLowerCase())
    if (data.code === 0) {
      hint.value = data.data.message
      step.value = 'reset'
    } else {
      ElMessage.error(data.message)
    }
  } catch (e) {
    ElMessage.error(errMsg(e, '提交失败'))
  } finally {
    loading.value = false
  }
}

async function submitReset() {
  if (!form.code) {
    ElMessage.warning('请输入两步验证动态码或恢复码')
    return
  }
  if (form.password.length < 8) {
    ElMessage.warning('新密码至少 8 位')
    return
  }
  if (form.password !== form.confirm) {
    ElMessage.warning('两次输入的新密码不一致')
    return
  }
  submitting.value = true
  try {
    const { data } = await resetPassword(email.value.trim().toLowerCase(), form.code.trim(), form.password)
    if (data.code === 0) {
      ElMessage.success('密码已重置，请重新登录')
      router.replace('/login')
    } else {
      ElMessage.error(data.message)
    }
  } catch (e) {
    ElMessage.error(errMsg(e, '重置失败'))
  } finally {
    submitting.value = false
  }
}
</script>

<template>
  <div class="auth-stage">
    <div class="auth-card">
      <div class="auth-brand">
        <span class="auth-logo">X</span>
        <div>
          <div class="auth-title">重置密码</div>
          <div class="auth-sub">邮箱 + 两步验证码（未开启两步验证的账号请联系管理员）</div>
        </div>
      </div>

      <!-- 第一步：邮箱 -->
      <el-form v-if="step === 'email'" label-position="top" size="large" @submit.prevent="submitEmail">
        <el-form-item label="注册邮箱">
          <el-input v-model="email" placeholder="you@example.com" :prefix-icon="Message" autofocus />
        </el-form-item>
        <el-button type="primary" size="large" class="auth-submit" :loading="loading" native-type="submit">
          下一步
        </el-button>
      </el-form>

      <!-- 第二步：验证码 + 新密码 -->
      <el-form v-else label-position="top" size="large" @submit.prevent="submitReset">
        <p class="hint">{{ hint }}</p>
        <el-form-item label="动态验证码 / 恢复码">
          <el-input
            v-model="form.code"
            placeholder="Google Authenticator 6 位码或恢复码"
            :prefix-icon="Key"
            maxlength="16"
            style="font-family: var(--x-font-mono)"
          />
        </el-form-item>
        <el-form-item label="新密码">
          <el-input v-model="form.password" type="password" show-password placeholder="至少 8 位" :prefix-icon="Lock" />
        </el-form-item>
        <el-form-item label="确认新密码">
          <el-input
            v-model="form.confirm"
            type="password"
            show-password
            placeholder="再次输入新密码"
            :prefix-icon="Lock"
            @keyup.enter="submitReset"
          />
        </el-form-item>
        <el-button type="primary" size="large" class="auth-submit" :loading="submitting" native-type="submit">
          重置密码
        </el-button>
      </el-form>

      <div class="auth-foot">
        <router-link class="auth-link" to="/login">返回登录</router-link>
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
.hint { font-size: 12.5px; color: var(--x-text-2); margin: -6px 0 12px; line-height: 1.6; }
.auth-submit { width: 100%; margin-top: 6px; font-weight: 600; letter-spacing: 4px; }
.auth-foot { margin-top: 18px; text-align: center; font-size: 13px; color: var(--x-text-2); }
.auth-link { color: var(--x-primary); font-weight: 600; }
</style>