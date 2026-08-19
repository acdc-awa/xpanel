<script setup lang="ts">
import { computed, onMounted, reactive, ref } from 'vue'
import { useRoute, useRouter } from 'vue-router'
import { Lock, Message, Ticket } from '@element-plus/icons-vue'
import { useAuthStore } from '@/stores/auth'
import { useSiteStore } from '@/stores/site'
import { errMsg } from '@/api/http'
import TurnstileWidget from '@/components/TurnstileWidget.vue'

const auth = useAuthStore()
const site = useSiteStore()
const route = useRoute()
const router = useRouter()

const autoFilledCode = ref(false)

const form = reactive({
  email: '',
  password: '',
  confirm: '',
  invite_code: '',
  turnstile_token: '',
})
const loading = ref(false)

const EMAIL_RE = /^[^\s@]+@[^\s@]+\.[^\s@]+$/

// 二次密码匹配状态：空不提示；匹配绿色；不匹配红色
const confirmStatus = computed(() => {
  if (!form.confirm) return ''
  if (form.password && form.confirm === form.password) return 'success'
  return 'error'
})

onMounted(async () => {
  await site.fetchConfig()
  const codeParam = (route.query.code || route.query.invite || route.query.invite_code) as string | undefined
  if (codeParam && codeParam.trim()) {
    form.invite_code = codeParam.trim()
    autoFilledCode.value = true
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
  if (site.captchaEnable && !form.turnstile_token) {
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
        <span v-if="site.logo" class="auth-logo-img"><img :src="site.logo" alt="logo" /></span>
        <span v-else class="auth-logo">X</span>
        <div>
          <div class="auth-title">注册 {{ site.appName }}</div>
          <div class="auth-sub">需要邀请码才能注册</div>
        </div>
      </div>

      <el-form label-position="top" size="large" @submit.prevent="onSubmit">
        <el-form-item label="邮箱（用户名）">
          <el-input v-model="form.email" placeholder="you@example.com" :prefix-icon="Message" autofocus />
        </el-form-item>
        <el-form-item label="邀请码">
          <el-input
            v-model="form.invite_code"
            placeholder="请输入邀请码"
            :prefix-icon="Ticket"
            :disabled="autoFilledCode"
            :clearable="!autoFilledCode"
          />
        </el-form-item>
        <el-form-item label="密码">
          <el-input v-model="form.password" type="password" show-password placeholder="至少 8 位" :prefix-icon="Lock" />
        </el-form-item>
        <el-form-item
          label="确认密码"
          :class="['confirm-pwd-item', confirmStatus]"
        >
          <el-input
            v-model="form.confirm"
            type="password"
            show-password
            placeholder="再次输入密码"
            :prefix-icon="Lock"
            @keyup.enter="onSubmit"
          />
        </el-form-item>
        <TurnstileWidget
          v-if="site.captchaEnable && site.turnstileSiteKey"
          :site-key="site.turnstileSiteKey"
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
.auth-logo-img {
  width: 44px;
  height: 44px;
  border-radius: 13px;
  overflow: hidden;
  display: flex;
  align-items: center;
  justify-content: center;
  background: var(--x-bg);
  border: 1px solid var(--x-border);
  flex: none;
  img {
    width: 100%;
    height: 100%;
    object-fit: contain;
  }
}
.auth-title { font-size: 19px; font-weight: 700; }
.auth-sub { font-size: 12.5px; color: var(--x-text-3); margin-top: 3px; }
.auth-submit { width: 100%; margin-top: 6px; font-weight: 600; letter-spacing: 4px; }
.auth-foot { margin-top: 18px; text-align: center; font-size: 13px; color: var(--x-text-2); }
.auth-link { color: var(--x-primary); font-weight: 600; }

:deep(.confirm-pwd-item) {
  &.success .el-input__wrapper {
    box-shadow: 0 0 0 1px var(--el-color-success, #10b981) inset !important;
    background-color: rgba(16, 185, 129, 0.04);
    &:focus-within {
      box-shadow: 0 0 0 1px var(--el-color-success, #10b981) inset, 0 0 0 3px rgba(16, 185, 129, 0.2) !important;
    }
  }

  &.error .el-input__wrapper {
    box-shadow: 0 0 0 1px var(--el-color-danger, #ef4444) inset !important;
    background-color: rgba(239, 68, 68, 0.04);
    &:focus-within {
      box-shadow: 0 0 0 1px var(--el-color-danger, #ef4444) inset, 0 0 0 3px rgba(239, 68, 68, 0.2) !important;
    }
  }
}
</style>