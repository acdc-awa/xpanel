<script setup lang="ts">
import { Handle, Position } from '@vue-flow/core'
import { ref } from 'vue'
import { CsIcon } from '@/components/cloudscape'
import type { TopologyData } from '@/api/admin'
import type { AccessLayer, InboundItem } from '@/api/types'
import {
  DEFAULT_BOX_W,
  inboundSummary,
  isOutboundDraft,
  xhttpPathOf,
  typeInfo,
  type BoxData,
  type BoxOutbound,
  type LayerGroup,
} from '../model'
import { useGridHeight } from '../useGridHeight'

defineProps<{
  data: BoxData
  selected: boolean
  editable: boolean
  layerGroups: LayerGroup[]
  nativeInbounds: TopologyData['inbounds']
}>()

const emit = defineEmits<{
  (e: 'open-server', serverId: number): void
  (e: 'edit-inbound', inb: InboundItem): void
  (e: 'create-inbound', serverId: number): void
  (e: 'edit-outbound', out: BoxOutbound): void
  (e: 'create-outbound', serverId: number): void
  (e: 'create-layer', serverId: number): void
  (e: 'edit-layer', layer: AccessLayer): void
  (e: 'delete-layer', layer: AccessLayer): void
  (e: 'bind-layer', layer: AccessLayer): void
  (e: 'unbind-inbound', inb: InboundItem): void
  (e: 'copy-caddyfile', layer: AccessLayer): void
  (e: 'move-inbound-order', serverId: number, inboundId: number, dir: 'up' | 'down'): void
  (e: 'move-outbound-order', serverId: number, outboundId: string, dir: 'up' | 'down'): void
  (e: 'auto-align', serverId: number): void
  (e: 'resize-start', evt: MouseEvent, data: BoxData): void
}>()

const boxEl = ref<HTMLElement | null>(null)
useGridHeight(boxEl)
</script>

<template>
  <!-- 标准 Xray 节点服务器卡片 -->
  <div
    ref="boxEl"
    class="server-box"
    :class="{ offline: data.server.status !== 1, selected }"
    :style="{ width: (data.boxWidth || DEFAULT_BOX_W) + 'px' }"
  >
    <div
      class="custom-resizer-corner nodrag"
      title="拖动调整卡片宽度"
      @mousedown.stop.prevent="emit('resize-start', $event, data)"
      @dblclick.stop
    >
      <svg width="10" height="10" viewBox="0 0 10 10" aria-hidden="true">
        <path d="M9 1L1 9M9 5L5 9" stroke="currentColor" stroke-width="1.4" stroke-linecap="round" />
      </svg>
    </div>
    <div class="sb-head">
      <div class="sb-head-left">
        <div class="sb-head-title-row">
          <span class="status-dot" :class="data.server.status === 1 ? 'online' : 'offline'" />
          <span class="name" :title="data.server.name">{{ data.server.name }}</span>
          <span v-if="data.server.status !== 1" class="offline-badge">离线</span>
        </div>
        <span class="host" :title="data.server.host">{{ data.server.host }}</span>
      </div>
      <div class="sb-head-right">
        <button
          v-if="editable"
          class="sb-detail-btn nodrag"
          title="根据内部路由规则自动对齐出入站顺序，减少连线交叉"
          @click.stop="emit('auto-align', data.server.id)"
          @dblclick.stop
        >
          <CsIcon name="gen-ai" size="x-small" />&nbsp;整理
        </button>
        <button
          class="sb-detail-btn nodrag"
          title="查看服务器详情与监控"
          @click.stop="emit('open-server', data.server.id)"
          @dblclick.stop
        >
          <CsIcon name="settings" size="x-small" />&nbsp;详情
        </button>
      </div>
    </div>

    <div class="sb-cols">
      <!-- 入站列：包含 Caddy 认知层容器与常规入站 -->
      <div class="sb-col">
        <div class="sb-col-head">
          <span class="sb-title">入站 <span class="sb-count">{{ data.inbounds.length }}</span></span>
          <div class="sb-head-actions">
            <button
              v-if="editable"
              class="sb-add-btn layer-add-btn nodrag"
              title="新建对外接入层（订阅地址分组：内部实现对外不可见）"
              @click.stop="emit('create-layer', data.server.id)"
              @dblclick.stop
            >
              <CsIcon name="add-plus" size="x-small" />&nbsp;层
            </button>
            <button
              v-if="editable"
              class="sb-add-btn nodrag"
              title="为该服务器新建入站"
              @click.stop="emit('create-inbound', data.server.id)"
              @dblclick.stop
            >
              <CsIcon name="add-plus" size="x-small" />&nbsp;入站
            </button>
          </div>
        </div>

        <template v-if="data.inbounds.length > 0">
          <!-- 1. 对外接入层胶囊（显式端点点位；挂载入站行，端点语义全由层决议） -->
          <div
            v-for="g in layerGroups"
            :key="`layer-${g.layer.id}`"
            class="layer-capsule"
          >
            <div class="layer-capsule-head">
              <div class="layer-head-top">
                <div class="layer-title">
                  <CsIcon name="globe" size="x-small" class="layer-icon" />
                  <span class="layer-name" :title="g.layer.name">{{ g.layer.name }}</span>
                  <span class="layer-sec" :class="g.layer.security">
                    {{ g.layer.security === 'tls' ? 'TLS' : '明文' }}
                  </span>
                </div>
                <div class="layer-actions">
                  <button
                    class="layer-act-btn nodrag"
                    title="一键复制该层的 Caddyfile 反代配置片段"
                    @click.stop="emit('copy-caddyfile', g.layer)"
                    @dblclick.stop
                  >
                    <CsIcon name="copy" size="x-small" />
                  </button>
                  <button
                    v-if="editable"
                    class="layer-act-btn nodrag"
                    title="将本机未挂层入站挂载到该层"
                    @click.stop="emit('bind-layer', g.layer)"
                    @dblclick.stop
                  >
                    <CsIcon name="add-plus" size="x-small" />
                  </button>
                  <button
                    v-if="editable"
                    class="layer-act-btn nodrag"
                    title="编辑对外层"
                    @click.stop="emit('edit-layer', g.layer)"
                    @dblclick.stop
                  >
                    <CsIcon name="settings" size="x-small" />
                  </button>
                  <button
                    v-if="editable"
                    class="layer-act-btn danger nodrag"
                    title="删除对外层（挂载入站回退原生行）"
                    @click.stop="emit('delete-layer', g.layer)"
                    @dblclick.stop
                  >
                    <CsIcon name="remove" size="x-small" />
                  </button>
                </div>
              </div>
              <div class="layer-head-sub">
                <span class="layer-endpoint" :title="`${g.layer.host}:${g.layer.port}`">
                  {{ g.layer.host }}:{{ g.layer.port }}
                </span>
              </div>
            </div>
            <div class="layer-capsule-body">
              <div v-if="g.inbounds.length === 0" class="sb-empty layer-empty">暂无挂载入站</div>
              <div
                v-for="(inb, inbIdx) in g.inbounds"
                :key="inb.id"
                class="sb-row layer-subrow clickable"
                title="点击编辑入站"
                @click="emit('edit-inbound', inb)"
              >
                <Handle
                  type="target"
                  :id="`inb-tgt-${inb.id}`"
                  :position="Position.Left"
                  :connectable="editable"
                  class="ep ext-tgt"
                />
                <span
                  v-if="typeInfo(inb.type).text && inb.type !== 'tunnel'"
                  class="type-tag"
                  :class="typeInfo(inb.type).cls"
                  :title="`类型：${typeInfo(inb.type).text}`"
                >
                  {{ typeInfo(inb.type).text }}
                </span>
                <span
                  v-if="xhttpPathOf(inb)"
                  class="path-badge"
                  :title="`反代至 127.0.0.1:${inb.port}（路径须与入站 xhttp 设置同值）`"
                >
                  {{ xhttpPathOf(inb) }}
                </span>
                <span class="port-badge">:{{ inb.port }}</span>
                <span class="tag clickable" :title="`编辑入站「${inb.tag}」`">{{ inb.tag }}</span>
                <span class="proto-badge" :class="inboundSummary(inb).cls">{{ inboundSummary(inb).proto }}</span>
                <div v-if="editable" class="row-hover-actions" @click.stop>
                  <button
                    class="layer-unbind-btn nodrag"
                    title="移出对外层（回退直连端点）"
                    @click.stop="emit('unbind-inbound', inb)"
                    @dblclick.stop
                  >
                    ×
                  </button>
                  <div v-if="g.inbounds.length > 1" class="tag-order-actions">
                    <button
                      class="order-btn nodrag"
                      :disabled="inbIdx === 0"
                      title="上移入站"
                      @click.stop="emit('move-inbound-order', data.server.id, inb.id, 'up')"
                      @dblclick.stop
                    >▲</button>
                    <button
                      class="order-btn nodrag"
                      :disabled="inbIdx === g.inbounds.length - 1"
                      title="下移入站"
                      @click.stop="emit('move-inbound-order', data.server.id, inb.id, 'down')"
                      @dblclick.stop
                    >▼</button>
                  </div>
                </div>
                <Handle
                  type="source"
                  :id="`inb-src-${inb.id}`"
                  :position="Position.Right"
                  :connectable="editable"
                  class="ep in-ep"
                />
              </div>
            </div>
          </div>

          <!-- 2. 未挂层原生入站行（直连端点，ShareAddrStrategy 决定对外地址） -->
          <div
            v-for="(inb, inbIdx) in nativeInbounds"
            :key="inb.id"
            class="sb-row clickable"
            title="点击编辑入站"
            @click="emit('edit-inbound', inb)"
          >
            <Handle
              type="target"
              :id="`inb-tgt-${inb.id}`"
              :position="Position.Left"
              :connectable="editable"
              class="ep ext-tgt"
            />
            <span
              v-if="typeInfo(inb.type).text && inb.type !== 'tunnel'"
              class="type-tag"
              :class="typeInfo(inb.type).cls"
              :title="`类型：${typeInfo(inb.type).text}`"
            >
              {{ typeInfo(inb.type).text }}
            </span>
            <span class="port-badge">:{{ inb.port }}</span>
            <span class="tag clickable" :title="`编辑入站「${inb.tag}」`">{{ inb.tag }}</span>
            <span class="proto-badge" :class="inboundSummary(inb).cls">{{ inboundSummary(inb).proto }}</span>

            <div v-if="editable && nativeInbounds.length > 1" class="row-hover-actions" @click.stop>
              <div class="tag-order-actions">
                <button
                  class="order-btn nodrag"
                  :disabled="inbIdx === 0"
                  title="上移入站"
                  @click.stop="emit('move-inbound-order', data.server.id, inb.id, 'up')"
                  @dblclick.stop
                >▲</button>
                <button
                  class="order-btn nodrag"
                  :disabled="inbIdx === nativeInbounds.length - 1"
                  title="下移入站"
                  @click.stop="emit('move-inbound-order', data.server.id, inb.id, 'down')"
                  @dblclick.stop
                >▼</button>
              </div>
            </div>

            <Handle
              type="source"
              :id="`inb-src-${inb.id}`"
              :position="Position.Right"
              :connectable="editable"
              class="ep in-ep"
            />
          </div>
        </template>
        <div v-else class="sb-empty">暂无入站</div>
      </div>

      <!-- 出站列：盒内点（中线右）= 收盒内规则；边缘外点（右缘）= 发引用线；direct 仅右缘绿点收线 -->
      <div class="sb-col">
        <div class="sb-col-head">
          <span class="sb-title">出站 <span class="sb-count">{{ data.outbounds.length }}</span></span>
          <button
            v-if="editable"
            class="sb-add-btn nodrag"
            title="为该服务器新建出站"
            @click.stop="emit('create-outbound', data.server.id)"
            @dblclick.stop
          >
            <CsIcon name="add-plus" size="x-small" />&nbsp;出站
          </button>
        </div>

        <template v-if="data.outbounds.length > 0">
          <div
            v-for="(out, outIdx) in data.outbounds"
            :key="out.id"
            class="sb-row clickable"
            title="点击编辑出站"
            @click="emit('edit-outbound', out)"
          >
            <!-- 左侧内端接口（所有出站包括 direct 均有，接收盒内路由规则） -->
            <Handle
              type="target"
              :id="`out-tgt-${out.id}`"
              :position="Position.Left"
              :connectable="editable"
              class="ep in-ep"
            />
            <span
              class="tag out-tag clickable"
              :class="{ draft: out.isDraftTunnel || isOutboundDraft(out) }"
              :title="out.isDraftTunnel ? `直通草稿「${out.tag}」（拖拽右侧端点至落地入站建立管道）` : out.owner_inbound_id ? `四层直通链路出口「${out.tag}」（点击编辑对应直通管道）` : isOutboundDraft(out) ? `待连线出站「${out.tag}」（拖拽右侧端点至目标入站完成绑定）` : `出站「${out.tag}」`"
            >
              {{ out.tag }}
            </span>
            <span v-if="out.isDraftTunnel" class="out-proto-badge draft" title="草稿直通：拖拽右侧端点至落地入站建立管道">待连线</span>
            <span v-else-if="out.owner_inbound_id" class="out-proto-badge tunnel">LINK</span>
            <span v-else-if="out.inbound_ref" class="out-proto-badge ref">InboundRef</span>
            <span v-else-if="out.virtual" class="out-proto-badge direct">DIRECT</span>
            <span v-else-if="out.protocol === 'blackhole'" class="out-proto-badge blocked">BLOCKED</span>
            <span v-else-if="isOutboundDraft(out)" class="out-proto-badge draft" title="草稿出站：请拖拽右侧端点至目标入站完成绑定">待连线</span>
            <span v-else-if="out.protocol === 'custom'" class="out-proto-badge" title="自定义 Xray 出站 JSON">JSON</span>
            <span v-else class="out-proto-badge">{{ out.protocol }}</span>
            <span
              v-if="out.send_proxy_protocol && out.inbound_ref && !out.owner_inbound_id"
              class="out-proto-badge pp"
              title="该出站通过派生的 freedom 向落地发送 PROXY Protocol v2，携带客户端真实 IP"
            >PPv2</span>

            <div v-if="editable && data.outbounds.length > 1" class="row-hover-actions" @click.stop>
              <div class="tag-order-actions">
                <button
                  class="order-btn nodrag"
                  :disabled="outIdx === 0"
                  title="上移出站"
                  @click.stop="emit('move-outbound-order', data.server.id, out.id, 'up')"
                  @dblclick.stop
                >▲</button>
                <button
                  class="order-btn nodrag"
                  :disabled="outIdx === data.outbounds.length - 1"
                  title="下移出站"
                  @click.stop="emit('move-outbound-order', data.server.id, out.id, 'down')"
                  @dblclick.stop
                >▼</button>
              </div>
            </div>

            <!-- 右侧外端接口：普通出站发跨盒引用；direct 行显示绿色出口圆点 -->
            <Handle
              v-if="!out.virtual"
              type="source"
              :id="`out-src-${out.id}`"
              :position="Position.Right"
              :connectable="editable"
              class="ep ext-src"
            />
            <span
              v-else
              class="ep direct-ep-dot"
              title="直连出口（freedom）"
            />
          </div>
        </template>
        <div v-else class="sb-empty">暂无出站</div>
      </div>
    </div>
  </div>
</template>

<style scoped lang="scss">
.server-box {
  width: 520px;
  min-width: 416px;
  position: relative;
  background: var(--topo-card-bg);
  border: 1px solid var(--topo-card-border);
  border-radius: var(--topo-radius, 16px);
  font-size: 12px;
  color: var(--topo-text-1);
  box-shadow: var(--topo-card-shadow);
  overflow: visible;
  transition: border-color 0.2s, box-shadow 0.2s;
  contain: paint;
  transform: translateZ(0);

  &:hover {
    border-color: var(--topo-card-border-hover);
    box-shadow: var(--topo-card-shadow-hover);
  }

  &.offline {
    border-color: var(--topo-danger-border);
  }

  .sb-head {
    display: flex;
    align-items: center;
    justify-content: space-between;
    gap: 12px;
    padding: 0 16px;
    height: 56px;
    box-sizing: border-box;
    border-bottom: 1px solid var(--topo-divider);
    background: var(--topo-head-bg);
    border-radius: 16px 16px 0 0;

    .sb-head-left {
      display: flex;
      flex-direction: column;
      justify-content: center;
      gap: 2px;
      min-width: 0;
      flex: 1;
    }

    .sb-head-title-row {
      display: flex;
      align-items: center;
      gap: 6px;
      min-width: 0;
    }

    .sb-head-right {
      display: flex;
      align-items: center;
      gap: 6px;
      flex: none;
      white-space: nowrap;
    }

    .sb-detail-btn {
      display: inline-flex;
      align-items: center;
      white-space: nowrap;
      background: var(--topo-card-bg);
      border: 1px solid var(--topo-card-border);
      border-radius: 8px;
      color: var(--topo-text-2);
      font-size: 11px;
      padding: 3px 8px;
      cursor: pointer;
      transition: background-color 0.15s, border-color 0.15s, color 0.15s;
      &:hover {
        background: var(--topo-accent-soft);
        border-color: var(--topo-accent);
        color: var(--topo-accent);
      }
    }

    .status-dot {
      width: 8px;
      height: 8px;
      border-radius: 50%;
      flex: none;
      &.online {
        background: var(--topo-success);
      }
      &.offline {
        background: var(--topo-danger);
      }
    }
    .name {
      font-weight: 600;
      font-size: 14px;
      line-height: 18px;
      color: var(--topo-text-1);
      white-space: nowrap;
      overflow: hidden;
      text-overflow: ellipsis;
      min-width: 0;
    }
    .offline-badge {
      font-size: 10px;
      line-height: 14px;
      padding: 1px 6px;
      border-radius: 4px;
      background: var(--topo-danger-soft);
      color: var(--topo-danger);
      border: 1px solid var(--topo-danger-border);
      font-weight: 500;
      white-space: nowrap;
      flex: none;
    }
    .host {
      font-size: 11px;
      line-height: 14px;
      font-variant-numeric: tabular-nums;
      color: var(--topo-text-2);
      padding-left: 14px;
      overflow: hidden;
      text-overflow: ellipsis;
      white-space: nowrap;
    }
  }

  .sb-cols {
    display: flex;
    gap: 48px;
    justify-content: space-between;
    padding: 0 16px 16px;
    width: 100%;
    box-sizing: border-box;

    .sb-col {
      flex: 0 1 auto;
      min-width: 0;
      max-width: min(280px, calc(50% - 24px));
      display: flex;
      flex-direction: column;
      gap: 8px;
      align-items: flex-start;
    }
    .sb-col:last-child {
      align-items: flex-end;
      .sb-col-head {
        flex-direction: row-reverse;
      }
      .sb-title {
        text-align: right;
      }
      .sb-empty {
        justify-content: flex-end;
      }
    }
    .sb-col-head {
      display: flex;
      align-items: center;
      justify-content: space-between;
      gap: 12px;
      width: 100%;
      height: 32px;
      padding: 6px 4px 2px;
      box-sizing: border-box;
    }
    .sb-title {
      display: inline-flex;
      align-items: center;
      gap: 4px;
      white-space: nowrap;
      color: var(--topo-text-2);
      font-size: 12px;
      font-weight: 600;
    }
    .sb-count {
      font-size: 11px;
      font-weight: 500;
      color: var(--topo-text-3);
      font-variant-numeric: tabular-nums;
    }
    .sb-add-btn {
      display: inline-flex;
      align-items: center;
      white-space: nowrap;
      flex: none;
      background: var(--topo-card-bg);
      border: 1px solid var(--topo-card-border);
      border-radius: 4px;
      color: var(--topo-accent);
      font-size: 11px;
      padding: 2px 6px;
      cursor: pointer;
      transition: background-color 0.15s, border-color 0.15s, color 0.15s;
      &:hover {
        background: var(--topo-accent-soft);
        border-color: var(--topo-accent);
      }
    }
  }

  /* 对外接入层胶囊（显式订阅端点分组；挂载入站行） */
  .sb-head-actions {
    display: flex;
    align-items: center;
    gap: 6px;
    flex: none;
  }
  .layer-add-btn {
    color: var(--topo-teal);
    &:hover {
      background: var(--topo-teal-soft);
      border-color: var(--topo-teal);
    }
  }
  .layer-capsule {
    width: 100%;
    background: var(--topo-surface);
    border: 1px solid var(--topo-surface-border);
    border-left: 2px solid var(--topo-teal);
    border-radius: 8px;
    padding: 8px;
    box-sizing: border-box;
    display: flex;
    flex-direction: column;
    gap: 8px;

    .layer-capsule-head {
      display: flex;
      flex-direction: column;
      justify-content: center;
      gap: 2px;
      height: 40px;
      box-sizing: border-box;
      padding: 0 2px 3px;
      border-bottom: 1px solid var(--topo-divider);

      .layer-head-top {
        display: flex;
        align-items: center;
        justify-content: space-between;
        gap: 6px;
        height: 20px;
        min-width: 0;
      }

      .layer-title {
        display: flex;
        align-items: center;
        gap: 5px;
        min-width: 0;
        flex: 1;
      }
      .layer-icon {
        flex: none;
        color: var(--topo-teal);
      }
      .layer-name {
        font-size: 12px;
        font-weight: 600;
        color: var(--topo-text-1);
        white-space: nowrap;
        overflow: hidden;
        text-overflow: ellipsis;
        min-width: 0;
      }
      .layer-sec {
        font-size: 9px;
        line-height: 12px;
        padding: 1px 5px;
        border-radius: 4px;
        font-weight: 600;
        flex: none;
        &.tls {
          background: var(--topo-teal-soft);
          color: var(--topo-teal);
          border: 1px solid var(--topo-teal-border);
        }
        &.none {
          background: var(--topo-row-bg);
          color: var(--topo-text-2);
          border: 1px solid var(--topo-row-border);
        }
      }
      .layer-actions {
        display: flex;
        align-items: center;
        gap: 2px;
        flex: none;
      }
      .layer-act-btn {
        display: inline-flex;
        align-items: center;
        justify-content: center;
        width: 20px;
        height: 20px;
        padding: 0;
        background: transparent;
        border: 1px solid transparent;
        border-radius: 4px;
        color: var(--topo-text-3);
        cursor: pointer;
        transition: background-color 0.15s, border-color 0.15s, color 0.15s;
        &:hover {
          background: var(--topo-accent-soft);
          border-color: var(--topo-accent-border);
          color: var(--topo-accent);
        }
      }
      .layer-act-btn.danger:hover {
        background: var(--topo-danger-soft);
        border-color: var(--topo-danger-border);
        color: var(--topo-danger);
      }

      .layer-head-sub {
        display: flex;
        align-items: center;
        min-width: 0;
      }
      .layer-endpoint {
        font-size: 11px;
        line-height: 14px;
        font-variant-numeric: tabular-nums;
        color: var(--topo-text-2);
        white-space: nowrap;
        overflow: hidden;
        text-overflow: ellipsis;
      }
    }

    .layer-capsule-body {
      display: flex;
      flex-direction: column;
      gap: 8px;
    }

    .layer-subrow {
      background: var(--topo-card-bg);
      border-color: var(--topo-row-border);
      &:hover {
        background: var(--topo-row-hover);
        border-color: var(--topo-accent-border);
      }
    }
    .layer-unbind-btn {
      flex: none;
      width: 18px;
      height: 18px;
      line-height: 16px;
      text-align: center;
      border-radius: 50%;
      font-size: 12px;
      color: var(--topo-text-3);
      background: var(--topo-card-bg);
      border: 1px solid var(--topo-card-border);
      cursor: pointer;
      padding: 0;
      transition: background-color 0.15s, border-color 0.15s, color 0.15s;
      &:hover {
        color: var(--topo-danger);
        background: var(--topo-danger-soft);
        border-color: var(--topo-danger);
      }
    }
    .layer-empty {
      font-style: italic;
    }
  }

  .path-badge {
    font-size: 10px;
    font-variant-numeric: tabular-nums;
    color: var(--topo-teal);
    background: var(--topo-teal-soft);
    padding: 1px 5px;
    border-radius: 4px;
    border: 1px solid var(--topo-teal-border);
    flex: none;
  }

  .sb-row {
    position: relative;
    display: flex;
    align-items: center;
    gap: 8px;
    padding: 0 12px;
    width: 100%;
    height: 32px;
    max-width: 100%;
    box-sizing: border-box;
    background: var(--topo-row-bg);
    border: 1px solid var(--topo-row-border);
    border-radius: 8px;
    color: var(--topo-text-1);
    transition: background-color 0.15s, border-color 0.15s;

    &:hover {
      background: var(--topo-row-hover);
      border-color: var(--topo-accent-border);
    }

    .tag {
      flex: 1;
      min-width: 0;
      font-weight: 600;
      white-space: nowrap;
      overflow: hidden;
      text-overflow: ellipsis;
      &.clickable {
        cursor: pointer;
        &:hover {
          color: var(--topo-accent);
          text-decoration: underline;
        }
      }
    }
    .out-tag {
      color: var(--topo-text-1);
      &.draft {
        color: var(--topo-text-2);
      }
    }
    &.clickable {
      cursor: pointer;
      transition: background-color 0.15s ease, border-color 0.15s ease;
      &:hover {
        background: var(--topo-row-hover);
        border-color: var(--topo-accent-border);
      }
    }

    .port-badge {
      font-size: 11px;
      font-weight: 600;
      font-variant-numeric: tabular-nums;
      color: var(--topo-accent);
      flex: none;
    }

    .proto-badge {
      font-size: 9px;
      padding: 1px 5px;
      border-radius: 4px;
      font-weight: 600;
      flex: none;
      &.vless,
      &.mixed,
      &.sec,
      &.net {
        background: var(--topo-accent-soft);
        color: var(--topo-accent);
        border: 1px solid var(--topo-accent-border);
      }
      &.hy2,
      &.other {
        background: var(--topo-surface);
        color: var(--topo-text-2);
        border: 1px solid var(--topo-card-border);
      }
      &.tunnel {
        background: var(--topo-teal-soft);
        color: var(--topo-teal);
        border: 1px solid var(--topo-teal-border);
      }
    }

    .out-proto-badge {
      font-size: 9px;
      padding: 1px 5px;
      border-radius: 4px;
      font-weight: 600;
      flex: none;
      background: var(--topo-surface);
      color: var(--topo-text-2);
      border: 1px solid var(--topo-card-border);
      &.tunnel {
        background: var(--topo-teal-soft);
        color: var(--topo-teal);
        border: 1px solid var(--topo-teal-border);
      }
      &.ref {
        background: var(--topo-warning-soft);
        color: var(--topo-warning);
        border: 1px solid var(--topo-warning-border);
      }
      &.direct {
        background: var(--topo-success-soft);
        color: var(--topo-success);
        border: 1px solid var(--topo-success-border);
      }
      &.blocked {
        background: var(--topo-danger-soft);
        color: var(--topo-danger);
        border: 1px solid var(--topo-danger-border);
      }
      &.draft {
        background: var(--topo-warning-soft);
        color: var(--topo-warning);
        border: 1px dashed var(--topo-warning-border);
      }
      &.pp {
        background: var(--topo-accent-soft);
        color: var(--topo-accent);
        border: 1px solid var(--topo-accent-border);
      }
    }

    .type-tag {
      flex: none;
      font-size: 10px;
      padding: 1px 5px;
      border-radius: 4px;
      font-weight: 600;
      &.user {
        background: var(--topo-success-soft);
        color: var(--topo-success);
      }
      &.relay {
        background: var(--topo-warning-soft);
        color: var(--topo-warning);
      }
      &.tunnel,
      &.channel {
        background: var(--topo-teal-soft);
        color: var(--topo-teal);
      }
      &.forward {
        background: var(--topo-accent-soft);
        color: var(--topo-accent);
      }
    }

    .row-hover-actions {
      position: absolute;
      right: 10px;
      top: 50%;
      transform: translateY(-50%);
      display: inline-flex;
      align-items: center;
      gap: 4px;
      padding-left: 12px;
      background: linear-gradient(90deg, transparent, var(--topo-row-hover) 12px);
      opacity: 0;
      pointer-events: none;
      transition: opacity 0.15s ease;
      z-index: 4;
    }

    &:hover .row-hover-actions,
    .row-hover-actions:focus-within {
      opacity: 1;
      pointer-events: auto;
    }

    .tag-order-actions {
      display: inline-flex;
      flex-direction: column;
      gap: 1px;
      flex: none;
      .order-btn {
        background: var(--topo-card-bg);
        border: 1px solid var(--topo-card-border);
        color: var(--topo-text-3);
        // 原来 7px / 8px 行高 / 10px 高：低于不少环境的最小字号下限
        // （部分 Android WebView 与国产套壳浏览器强制 12px），会被撑破按钮；
        // 提到 10px 后即使被强制放大到 12px 也仍在 14px 盒内
        font-size: 10px;
        line-height: 12px;
        padding: 0 4px;
        min-width: 14px;
        height: 14px;
        cursor: pointer;
        border-radius: var(--x-radius-2xs, 4px);
        display: flex;
        align-items: center;
        justify-content: center;
        &:hover:not(:disabled) {
          color: var(--topo-accent);
          border-color: var(--topo-accent);
          background: var(--topo-accent-soft);
        }
        &:disabled {
          opacity: 0.25;
          cursor: not-allowed;
          border-color: transparent;
          background: transparent;
        }
      }
    }
  }

  .sb-empty {
    width: 100%;
    box-sizing: border-box;
    padding: 0 4px;
    color: var(--topo-text-3);
    font-size: 11px;
    height: 32px;
    display: flex;
    align-items: center;
  }

  .custom-resizer-corner {
    position: absolute;
    right: 4px;
    bottom: 4px;
    width: 20px;
    height: 20px;
    display: flex;
    align-items: flex-end;
    justify-content: flex-end;
    padding: 3px;
    box-sizing: border-box;
    border-radius: 4px;
    color: var(--topo-text-3);
    cursor: ew-resize;
    opacity: 0;
    transition: opacity 0.15s ease, color 0.15s ease;
    z-index: 6;
  }

  &:hover .custom-resizer-corner,
  &.selected .custom-resizer-corner {
    opacity: 0.75;
    &:hover {
      opacity: 1;
      color: var(--topo-accent);
    }
  }
}
</style>
