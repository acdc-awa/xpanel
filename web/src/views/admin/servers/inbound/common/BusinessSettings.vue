<script setup lang="ts">
import { QuestionFilled, Plus, Delete, MagicStick, CopyDocument } from '@element-plus/icons-vue'
import { ElMessage } from '@/components/cloudscape'
import type { AccessLayer } from '@/api/types'

defineProps<{
  inboundType: string
  layerId: number
  layers: AccessLayer[]
  serverId?: number
  shareStrategy: string
  shareAddr: string
  sharePort: number
  shareSecurity: string
  shareSni: string
  shareHost: string
  sharePath: string
  shareAllowInsecure: boolean
  caddySnippet: string
  layerDeleting?: boolean
}>()

const emit = defineEmits<{
  (e: 'update:layerId', value: number): void
  (e: 'update:shareStrategy', value: string): void
  (e: 'update:shareAddr', value: string): void
  (e: 'update:sharePort', value: number): void
  (e: 'update:shareSecurity', value: string): void
  (e: 'update:shareSni', value: string): void
  (e: 'update:shareHost', value: string): void
  (e: 'update:sharePath', value: string): void
  (e: 'update:shareAllowInsecure', value: boolean): void
  (e: 'open-layer-quick'): void
  (e: 'delete-layer'): void
  (e: 'apply-caddy-preset'): void
}>()

function copyText(txt: string, label = '内容') {
  if (!txt) return
  navigator.clipboard.writeText(txt)
  ElMessage.success(`已复制 ${label}`)
}
</script>

<template>
  <div class="business-settings-box">
    <!-- 微卡片 1: 对外接入层绑定 -->
    <div class="x-micro-card">
      <div class="x-micro-card__head">
        <div class="head-title">
          <span class="w-2 h-2 rounded-full bg-indigo-500"></span>
          <span>对外接入层绑定 (Access Layer)</span>
          <el-tooltip content="用于向客户端分发订阅时解析的连接地址与参数。若挂载对外接入层，则 host/port/安全层由该层决议。" placement="top">
            <el-icon class="help-icon"><QuestionFilled /></el-icon>
          </el-tooltip>
        </div>
        <div class="head-meta">
          <el-button size="small" type="primary" plain :disabled="!serverId" @click="emit('apply-caddy-preset')">
            <el-icon><MagicStick /></el-icon>&nbsp;应用前置反代预设
          </el-button>
        </div>
      </div>
      <div class="x-micro-card__body">
        <el-form-item label="挂载接入层">
          <div style="display: flex; gap: 10px; width: 100%; align-items: center; flex-wrap: wrap">
            <el-select
              :model-value="layerId || undefined"
              style="flex: 1; min-width: 240px"
              :disabled="!serverId"
              placeholder="选择对外层（直连请留空）"
              clearable
              @update:model-value="(v: any) => emit('update:layerId', Number(v || 0))"
            >
              <el-option :value="0" label="不使用（直连端点）" />
              <el-option
                v-for="l in layers"
                :key="l.id"
                :value="l.id"
                :label="`${l.name} (${l.host}:${l.port} ${l.security === 'tls' ? 'TLS' : '明文'})`"
              />
            </el-select>
            <el-button :disabled="!serverId" :icon="Plus" @click="emit('open-layer-quick')">新建对外层</el-button>
            <el-button
              :disabled="!serverId || !layerId"
              :loading="layerDeleting"
              :icon="Delete"
              type="danger"
              plain
              @click="emit('delete-layer')"
            >
              删除该层
            </el-button>
          </div>
        </el-form-item>

        <el-alert
          v-if="layerId > 0"
          type="success"
          :closable="false"
          style="margin-top: 10px"
          title="已挂载对外接入层"
          description="订阅中的连接 host、port 和安全层将自动取自该层。下方自定义地址组已自动失效，仅 SNI/Path 等覆写项继续生效。"
        />
      </div>
    </div>

    <!-- 微卡片 2: 订阅下发覆写与自定义参数 -->
    <div class="x-micro-card">
      <div class="x-micro-card__head">
        <div class="head-title">
          <span class="w-2 h-2 rounded-full bg-emerald-500"></span>
          <span>订阅下发参数覆写 (Subscription Overrides)</span>
        </div>
        <div class="head-meta">
          <span>下发至客户端的连接配置</span>
        </div>
      </div>
      <div class="x-micro-card__body">
        <div class="form-grid-2">
          <template v-if="!layerId">
            <el-form-item label="分享地址策略">
              <el-select
                :model-value="shareStrategy"
                style="width: 100%"
                @update:model-value="(v: string) => emit('update:shareStrategy', v)"
              >
                <el-option label="跟随服务器 IP / 域名" value="node" />
                <el-option label="自定义域名或 IP" value="custom" />
              </el-select>
            </el-form-item>

            <el-form-item label="订阅安全层 (TLS)">
              <el-select
                :model-value="shareSecurity"
                style="width: 100%"
                @update:model-value="(v: string) => emit('update:shareSecurity', v)"
              >
                <el-option label="auto (跟随传输层配置)" value="auto" />
                <el-option label="tls (强制要求 TLS)" value="tls" />
                <el-option label="none (明文传输)" value="none" />
              </el-select>
            </el-form-item>

            <template v-if="shareStrategy === 'custom'">
              <el-form-item label="自定义分享地址 (Host)">
                <el-input
                  :model-value="shareAddr"
                  placeholder="如 cdn.example.com"
                  class="font-mono-num"
                  @input="(v: string) => emit('update:shareAddr', v)"
                />
              </el-form-item>
              <el-form-item label="自定义分享端口 (Port)">
                <el-input-number
                  :model-value="sharePort"
                  :min="0"
                  :max="65535"
                  placeholder="0 = 默认入站端口"
                  class="font-mono-num"
                  style="width: 100%"
                  @update:model-value="(v: any) => emit('update:sharePort', Number(v || 0))"
                />
              </el-form-item>
            </template>
          </template>

          <el-form-item label="客户端覆写 SNI">
            <el-input
              :model-value="shareSni"
              placeholder="留空默认跟随服务器配置"
              class="font-mono-num"
              @input="(v: string) => emit('update:shareSni', v)"
            />
          </el-form-item>

          <el-form-item label="客户端覆写 HTTP Host">
            <el-input
              :model-value="shareHost"
              placeholder="留空默认跟随传输配置"
              class="font-mono-num"
              @input="(v: string) => emit('update:shareHost', v)"
            />
          </el-form-item>

          <el-form-item label="客户端覆写分流 Path">
            <el-input
              :model-value="sharePath"
              placeholder="留空默认跟随传输路径"
              class="font-mono-num"
              @input="(v: string) => emit('update:sharePath', v)"
            />
          </el-form-item>

          <el-form-item label="客户端证书校验">
            <div style="padding-top: 4px">
              <el-switch
                :model-value="shareAllowInsecure"
                active-text="跳过客户端证书校验 (allowInsecure)"
                @update:model-value="(v: any) => emit('update:shareAllowInsecure', Boolean(v))"
              />
            </div>
          </el-form-item>
        </div>
      </div>
    </div>

    <!-- 微卡片 3: Caddyfile 片段参考 -->
    <div v-if="caddySnippet" class="x-micro-card">
      <div class="x-micro-card__head">
        <div class="head-title">
          <span class="w-2 h-2 rounded-full bg-cyan-500"></span>
          <span>Caddyfile 反向代理配置参考</span>
        </div>
        <div class="head-meta">
          <el-button size="small" text type="primary" @click="copyText(caddySnippet, 'Caddyfile 配置')">
            <el-icon><CopyDocument /></el-icon>&nbsp;复制 Caddyfile 片段
          </el-button>
        </div>
      </div>
      <div class="x-micro-card__body">
        <pre class="caddy-code"><code>{{ caddySnippet }}</code></pre>
      </div>
    </div>
  </div>
</template>

<style scoped lang="scss">
.business-settings-box {
  display: flex;
  flex-direction: column;
}

:deep(.el-form-item) {
  margin-bottom: 0;
}

.help-icon {
  color: var(--x-text-3);
  font-size: 13px;
  margin-left: 4px;
  cursor: help;
  &:hover { color: var(--x-primary); }
}

.caddy-code {
  background: var(--x-code-bg, #0f172a);
  color: var(--x-code-text, #e2e8f0);
  border-radius: var(--x-radius-xs, 6px);
  padding: 12px 14px;
  font-family: var(--x-font-mono);
  font-size: 12px;
  line-height: 1.5;
  overflow-x: auto;
  border: 1px solid var(--x-border);
}
</style>
