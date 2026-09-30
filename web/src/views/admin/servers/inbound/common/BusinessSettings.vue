<script setup lang="ts">
import { QuestionFilled, Plus, Delete, MagicStick, CopyDocument } from '@element-plus/icons-vue'
import { ElMessage } from 'element-plus'
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
    <div class="form-card">
      <div class="card-head-flex">
        <div class="card-title">
          <span>订阅分享与反代设置</span>
          <el-tooltip content="用于向客户端分发订阅时解析的连接地址与参数。若挂载对外接入层，则 host/port/安全层由该层决议。" placement="top">
            <el-icon class="help-icon"><QuestionFilled /></el-icon>
          </el-tooltip>
        </div>
        <el-button size="small" type="primary" plain :disabled="!serverId" @click="emit('apply-caddy-preset')">
          <el-icon><MagicStick /></el-icon>&nbsp;应用前置反代预设
        </el-button>
      </div>

      <!-- 对外接入层 -->
      <div style="margin-bottom: 12px">
        <el-form-item label="对外接入层">
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
      </div>

      <el-alert
        v-if="layerId > 0"
        type="success"
        :closable="false"
        style="margin-bottom: 12px"
        title="已挂载对外接入层"
        description="订阅中的连接 host、port 和安全层将自动取自该层。下方自定义地址组已自动失效，仅 SNI/Path 等覆写项继续生效。"
      />

      <div class="form-grid">
        <template v-if="!layerId">
          <el-form-item label="分享地址策略">
            <el-select
              :model-value="shareStrategy"
              style="width: 100%"
              @update:model-value="(v: string) => emit('update:shareStrategy', v)"
            >
              <el-option label="跟随服务器" value="node" />
              <el-option label="自定义地址" value="custom" />
            </el-select>
          </el-form-item>

          <el-form-item label="订阅安全层 (TLS)">
            <el-select
              :model-value="shareSecurity"
              style="width: 100%"
              @update:model-value="(v: string) => emit('update:shareSecurity', v)"
            >
              <el-option label="auto (跟随传输层)" value="auto" />
              <el-option label="tls (强制启用)" value="tls" />
              <el-option label="none (明文)" value="none" />
            </el-select>
          </el-form-item>

          <template v-if="shareStrategy === 'custom'">
            <el-form-item label="自定义分享地址">
              <el-input
                :model-value="shareAddr"
                placeholder="如 cdn.example.com"
                @input="(v: string) => emit('update:shareAddr', v)"
              />
            </el-form-item>
            <el-form-item label="自定义分享端口">
              <el-input-number
                :model-value="sharePort"
                :min="0"
                :max="65535"
                placeholder="0 = 默认入站端口"
                style="width: 100%"
                @update:model-value="(v: any) => emit('update:sharePort', Number(v || 0))"
              />
            </el-form-item>
          </template>
        </template>

        <el-form-item label="分享 SNI">
          <el-input
            :model-value="shareSni"
            placeholder="留空默认跟随服务器配置"
            @input="(v: string) => emit('update:shareSni', v)"
          />
        </el-form-item>

        <el-form-item label="分享 Host">
          <el-input
            :model-value="shareHost"
            placeholder="留空默认跟随传输配置"
            @input="(v: string) => emit('update:shareHost', v)"
          />
        </el-form-item>

        <el-form-item label="分享 Path">
          <el-input
            :model-value="sharePath"
            placeholder="留空默认跟随传输路径"
            @input="(v: string) => emit('update:sharePath', v)"
          />
        </el-form-item>

        <el-form-item label="允许不安全证书">
          <div style="padding-top: 6px">
            <el-switch
              :model-value="shareAllowInsecure"
              active-text="跳过客户端证书校验 (allowInsecure)"
              @update:model-value="(v: any) => emit('update:shareAllowInsecure', Boolean(v))"
            />
          </div>
        </el-form-item>
      </div>

      <!-- Caddyfile 片段参考 -->
      <div v-if="caddySnippet" class="caddy-snippet-box">
        <div class="caddy-head">
          <span class="caddy-title">Caddyfile 反代配置参考</span>
          <el-button size="small" text type="primary" @click="copyText(caddySnippet, 'Caddyfile 配置')">
            <el-icon><CopyDocument /></el-icon>&nbsp;复制 Caddyfile
          </el-button>
        </div>
        <pre class="caddy-code"><code>{{ caddySnippet }}</code></pre>
      </div>
    </div>
  </div>
</template>

<style scoped lang="scss">
.business-settings-box {
  display: flex;
  flex-direction: column;
  gap: 14px;
}

.form-card {
  background: var(--x-bg);
  border: 1px solid var(--x-border);
  border-radius: var(--x-radius);
  padding: 16px 18px;
}

.card-title {
  font-size: 13.5px;
  font-weight: 600;
  color: var(--x-text);
  margin-bottom: 14px;
  display: flex;
  align-items: center;
  gap: 6px;
}

.card-head-flex {
  display: flex;
  align-items: center;
  justify-content: space-between;
  margin-bottom: 14px;
  .card-title { margin-bottom: 0; }
}

.help-icon {
  color: var(--x-text-3);
  font-size: 13px;
  margin-left: 4px;
  cursor: help;
  &:hover { color: var(--x-primary); }
}

.form-grid {
  display: grid;
  grid-template-columns: 1fr 1fr;
  gap: 0 18px;

  :deep(.el-form-item) {
    margin-bottom: 14px;
  }
}

.caddy-snippet-box {
  margin-top: 14px;
  background: var(--x-card-soft);
  border: 1px solid var(--x-border);
  border-radius: 8px;
  padding: 12px;
  display: flex;
  flex-direction: column;
  gap: 8px;

  .caddy-head {
    display: flex;
    align-items: center;
    justify-content: space-between;
  }

  .caddy-title {
    font-size: 12.5px;
    font-weight: 500;
    color: var(--x-text);
  }

  .caddy-code {
    margin: 0;
    padding: 10px 12px;
    background: var(--x-card);
    border: 1px solid var(--x-border);
    border-radius: 6px;
    font-family: var(--x-font-mono, monospace);
    font-size: 12px;
    color: var(--x-primary);
    line-height: 1.5;
    overflow-x: auto;
  }
}

@media (max-width: 640px) {
  .form-grid {
    grid-template-columns: 1fr;
  }
}
</style>
