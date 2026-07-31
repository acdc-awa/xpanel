<script setup lang="ts">
import { ref } from 'vue'
import { CopyDocument, Key, Wallet } from '@element-plus/icons-vue'
import { mockClient } from '@/mock/data'
import { formatMoney } from '@/utils/format'

const profile = ref({ nickname: '', email: mockClient.username + '@example.com' })
const pwd = ref({ old: '', next: '', confirm: '' })
</script>

<template>
  <div class="x-client-body">
    <div class="x-acct-grid">
      <div>
        <!-- 余额 -->
        <div class="x-usage-hero">
          <div style="font-size: 12.5px; opacity: 0.85">账户余额</div>
          <div style="font-size: 28px; font-weight: 700; margin-top: 2px">{{ formatMoney(mockClient.balance) }}</div>
          <div class="x-plan-meta">
            <span>余额可用于购买套餐</span>
            <el-button size="small" style="background: #fff; color: #6366f1; font-weight: 600; border: none">充值</el-button>
          </div>
        </div>

        <!-- 订阅信息 -->
        <div class="x-card">
          <div class="x-card-head"><span>订阅信息</span></div>
          <div class="x-card-body">
            <div class="x-row-line"><span class="k">订阅链接</span><span class="v"><code class="cell-mono">{{ mockClient.subscribeUrl }}</code></span></div>
            <div class="x-row-line"><span class="k">Token</span><span class="v"><code class="cell-mono">{{ mockClient.subscribeToken }}</code></span></div>
            <div style="display: flex; gap: 10px; margin-top: 16px; flex-wrap: wrap">
              <el-button size="small" type="primary"><el-icon><CopyDocument /></el-icon>&nbsp;复制订阅链接</el-button>
              <el-button size="small"><el-icon><Key /></el-icon>&nbsp;重置 Token</el-button>
            </div>
          </div>
        </div>
      </div>

      <div>
        <!-- 用户资料 -->
        <div class="x-card">
          <div class="x-card-head"><span>用户资料</span></div>
          <div class="x-card-body">
            <el-form label-position="top">
              <el-form-item label="用户名"><el-input :model-value="mockClient.username" disabled /></el-form-item>
              <el-form-item label="昵称"><el-input v-model="profile.nickname" placeholder="请输入昵称" /></el-form-item>
              <el-form-item label="邮箱"><el-input v-model="profile.email" /></el-form-item>
              <el-form-item label="注册时间"><span class="muted">2026-07-20 14:02</span></el-form-item>
              <el-button type="primary" style="width: 100%">保存资料</el-button>
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
              <el-button style="width: 100%">修改密码</el-button>
            </el-form>
          </div>
        </div>

        <el-button type="danger" plain style="width: 100%"><el-icon><Wallet /></el-icon>&nbsp;退出登录</el-button>
      </div>
    </div>
  </div>
</template>

<style scoped lang="scss">
.cell-mono { font-family: ui-monospace, Menlo, Consolas, monospace; font-size: 12px; color: var(--x-text-2); word-break: break-all; }
.muted { color: var(--x-text-3); }
</style>
