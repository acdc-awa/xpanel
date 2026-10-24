<script setup lang="ts">
import { Handle, Position } from '@vue-flow/core'
import { CsIcon } from '@/components/cloudscape'
import type { TopologyData } from '@/api/admin'
import type { AccessLayer, InboundItem } from '@/api/types'
import {
  DEFAULT_BOX_W,
  inboundSummary,
  isOutboundDraft,
  typeInfo,
  type BoxData,
  type BoxOutbound,
  type LayerGroup,
} from '../model'

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
</script>

<template>
  <!-- 标准 Xray 节点服务器卡片 -->
  <div
    class="server-box"
    :class="{ offline: data.server.status !== 1 }"
    :style="{ width: (data.boxWidth || DEFAULT_BOX_W) + 'px' }"
  >
    <div
      v-show="selected"
      class="custom-resizer-right"
      @mousedown.stop.prevent="emit('resize-start', $event, data)"
    />
    <div class="sb-head">
      <div class="sb-head-left">
        <span class="status-dot" :class="data.server.status === 1 ? 'online' : 'offline'" />
        <span class="name" :title="data.server.name">{{ data.server.name }}</span>
        <span v-if="data.server.status !== 1" class="offline-badge">离线</span>
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
        <button class="sb-detail-btn" title="查看服务器详情与监控" @click.stop="emit('open-server', data.server.id)">
          <CsIcon name="settings" size="x-small" />&nbsp;详情
        </button>
      </div>
    </div>

    <div class="sb-cols">
      <!-- 入站列：包含 Caddy 认知层容器与常规入站 -->
      <div class="sb-col">
        <div class="sb-col-head">
          <span class="sb-title">入站 (Inbound)</span>
          <div class="sb-head-actions">
            <button
              v-if="editable"
              class="sb-add-btn layer-add-btn"
              title="新建对外接入层（订阅地址分组：内部实现对外不可见）"
              @click.stop="emit('create-layer', data.server.id)"
            >
              <CsIcon name="add-plus" size="x-small" />&nbsp;层
            </button>
            <button
              v-if="editable"
              class="sb-add-btn"
              title="为该服务器新建入站"
              @click.stop="emit('create-inbound', data.server.id)"
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
              <div class="layer-title">
                <span class="layer-icon">🌐</span>
                <span class="layer-name" :title="g.layer.name">{{ g.layer.name }}</span>
                <code class="layer-endpoint">{{ g.layer.host }}:{{ g.layer.port }}</code>
                <span class="layer-sec" :class="g.layer.security">
                  {{ g.layer.security === 'tls' ? 'TLS' : '明文' }}
                </span>
              </div>
              <div class="layer-actions">
                <button
                  class="layer-copy-btn"
                  title="一键复制该层的 Caddyfile 反代配置片段"
                  @click.stop="emit('copy-caddyfile', g.layer)"
                >
                  <CsIcon name="copy" size="x-small" />&nbsp;Caddyfile
                </button>
                <button
                  v-if="editable"
                  class="layer-act-btn"
                  title="将本机未挂层入站挂载到该层"
                  @click.stop="emit('bind-layer', g.layer)"
                >
                  <CsIcon name="add-plus" size="x-small" />&nbsp;挂载
                </button>
                <button v-if="editable" class="layer-act-btn" title="编辑对外层" @click.stop="emit('edit-layer', g.layer)">
                  <CsIcon name="settings" size="x-small" />
                </button>
                <button
                  v-if="editable"
                  class="layer-act-btn danger"
                  title="删除对外层（挂载入站回退原生行）"
                  @click.stop="emit('delete-layer', g.layer)"
                >
                  <CsIcon name="remove" size="x-small" />
                </button>
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
                  v-if="typeInfo(inb.type).text"
                  class="type-tag"
                  :class="typeInfo(inb.type).cls"
                  :title="`类型：${typeInfo(inb.type).text}`"
                >
                  {{ typeInfo(inb.type).text }}
                </span>
                <span v-if="inb.share_path" class="path-badge" :title="`反代至 127.0.0.1:${inb.port}`">
                  {{ inb.share_path }}
                </span>
                <span class="port-badge">:{{ inb.port }}</span>
                <span class="tag clickable" :title="`编辑入站「${inb.tag}」`">{{ inb.tag }}</span>
                <span class="proto-badge" :class="inboundSummary(inb).cls">{{ inboundSummary(inb).proto }}</span>
                <CsIcon class="row-edit-icon" name="edit" size="x-small" variant="subtle" title="编辑入站" />
                <button
                  v-if="editable"
                  class="layer-unbind-btn"
                  title="移出对外层（回退直连端点）"
                  @click.stop="emit('unbind-inbound', inb)"
                >
                  ×
                </button>
                <div v-if="editable && g.inbounds.length > 1" class="tag-order-actions">
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
              v-if="typeInfo(inb.type).text"
              class="type-tag"
              :class="typeInfo(inb.type).cls"
              :title="`类型：${typeInfo(inb.type).text}`"
            >
              {{ typeInfo(inb.type).text }}
            </span>
            <span class="port-badge">:{{ inb.port }}</span>
            <span class="tag clickable" :title="`编辑入站「${inb.tag}」`">{{ inb.tag }}</span>
            <span class="proto-badge" :class="inboundSummary(inb).cls">{{ inboundSummary(inb).proto }}</span>
            <CsIcon class="row-edit-icon" name="edit" size="x-small" variant="subtle" title="编辑入站" />

            <div v-if="editable && nativeInbounds.length > 1" class="tag-order-actions">
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
          <span class="sb-title">出站 (Outbound)</span>
          <button
            v-if="editable"
            class="sb-add-btn"
            title="为该服务器新建出站"
            @click.stop="emit('create-outbound', data.server.id)"
          >
            <CsIcon name="add-plus" size="x-small" />&nbsp;出站
          </button>
        </div>

        <template v-if="data.outbounds.length > 0">
          <div
            v-for="(out, outIdx) in data.outbounds"
            :key="out.id"
            class="sb-row"
            :class="{ 'direct-row': out.virtual, clickable: !out.virtual }"
            :title="out.virtual ? '' : '点击编辑出站'"
            @click="!out.virtual && emit('edit-outbound', out)"
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
              :class="out.isDraftTunnel ? 'draft' : out.owner_inbound_id ? 'tunnel' : out.inbound_ref ? 'ref' : out.virtual ? 'direct' : out.protocol === 'blackhole' ? 'blocked' : isOutboundDraft(out) ? 'draft' : ''"
              :title="out.isDraftTunnel ? `直通草稿「${out.tag}」（拖拽右侧端点至落地入站建立管道）` : out.owner_inbound_id ? `四层直通链路出口「${out.tag}」（点击编辑对应直通管道）` : isOutboundDraft(out) ? `待连线出站「${out.tag}」（拖拽右侧端点至目标入站完成绑定）` : `出站「${out.tag}」`"
            >
              {{ out.isDraftTunnel ? '🔗 ' + out.tag : isOutboundDraft(out) ? '⚠️ ' + out.tag : out.tag }}
            </span>
            <span v-if="out.isDraftTunnel" class="out-proto-badge draft" title="草稿直通：拖拽右侧端点至落地入站建立管道">待连线</span>
            <span v-else-if="out.owner_inbound_id" class="out-proto-badge tunnel">LINK</span>
            <span v-else-if="out.inbound_ref" class="out-proto-badge ref">InboundRef</span>
            <span v-else-if="out.virtual" class="out-proto-badge direct">DIRECT</span>
            <span v-else-if="out.protocol === 'blackhole'" class="out-proto-badge blocked">BLOCKED</span>
            <span v-else-if="isOutboundDraft(out)" class="out-proto-badge draft" title="草稿出站：请拖拽右侧端点至目标入站完成绑定">待连线</span>
            <span v-else-if="out.protocol === 'custom'" class="out-proto-badge" title="自定义 Xray 出站 JSON">JSON</span>
            <span v-else class="out-proto-badge">{{ out.protocol }}</span>
            <CsIcon v-if="!out.virtual" class="row-edit-icon" name="edit" size="x-small" variant="subtle" title="编辑出站" />

            <div v-if="editable && data.outbounds.length > 1" class="tag-order-actions">
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
  width: 440px;
  min-width: 340px;
  position: relative;
  background: rgba(24, 33, 47, 0.95);
  border: 1px solid rgba(255, 255, 255, 0.1);
  border-radius: 16px;
  font-size: 12px;
  color: #e2e8f0;
  box-shadow: 0 6px 24px rgba(0, 0, 0, 0.35), inset 0 1px 0 rgba(255, 255, 255, 0.1);
  overflow: visible;
  transition: border-color 0.2s, box-shadow 0.2s;
  contain: paint;
  transform: translateZ(0);

  &.offline {
    border-color: rgba(239, 68, 68, 0.25);
    box-shadow: 0 6px 20px rgba(0, 0, 0, 0.4);
    .sb-head {
      background: linear-gradient(180deg, rgba(239, 68, 68, 0.1) 0%, transparent 100%);
    }
  }

  .sb-head {
    display: flex;
    align-items: center;
    justify-content: space-between;
    padding: 0 16px;
    height: 48px;
    border-bottom: 1px solid rgba(255, 255, 255, 0.06);
    background: linear-gradient(180deg, rgba(255, 255, 255, 0.06) 0%, transparent 100%);
    border-radius: 16px 16px 0 0;

    .sb-head-left {
      display: flex;
      align-items: center;
      gap: 8px;
      min-width: 0;
    }

    .sb-head-right {
      display: flex;
      align-items: center;
      gap: 6px;
      flex: none;
    }

    .sb-detail-btn {
      display: inline-flex;
      align-items: center;
      background: rgba(255, 255, 255, 0.08);
      border: 1px solid rgba(255, 255, 255, 0.15);
      border-radius: 6px;
      color: #cbd5e1;
      font-size: 11px;
      padding: 3px 8px;
      cursor: pointer;
      transition: all 0.2s;
      &:hover {
        background: rgba(56, 189, 248, 0.2);
        border-color: #38bdf8;
        color: #38bdf8;
      }
    }

    .status-dot {
      width: 8px;
      height: 8px;
      border-radius: 50%;
      flex: none;
      &.online {
        background: #34d399;
        box-shadow: 0 0 8px #34d399;
      }
      &.offline {
        background: #ef4444;
        box-shadow: 0 0 6px rgba(239, 68, 68, 0.6);
      }
    }
    .name {
      font-weight: 600;
      font-size: 14px;
      white-space: nowrap;
      overflow: hidden;
      text-overflow: ellipsis;
      max-width: 140px;
      text-shadow: 0 2px 4px rgba(0, 0, 0, 0.5);
    }
    .offline-badge {
      font-size: 10px;
      padding: 1px 6px;
      border-radius: 4px;
      background: rgba(239, 68, 68, 0.15);
      color: #f87171;
      border: 1px solid rgba(239, 68, 68, 0.3);
      font-weight: 500;
    }
    .host {
      color: #94a3b8;
      font-size: 11px;
      overflow: hidden;
      text-overflow: ellipsis;
      white-space: nowrap;
      max-width: 140px;
    }
  }

  /* 四层直通管道卡片：横贯整机宽度 (Full-width Tunnel Card) */
  .sb-tunnels-wrap {
    padding: 0 16px 10px;
    display: flex;
    flex-direction: column;
    gap: 8px;
    width: 100%;
    box-sizing: border-box;
  }

  .tunnel-card {
    position: relative;
    background: linear-gradient(90deg, rgba(6, 182, 212, 0.12) 0%, rgba(15, 23, 42, 0.75) 100%);
    border: 1px solid rgba(6, 182, 212, 0.35);
    border-radius: 10px;
    padding: 6px 10px;
    box-sizing: border-box;
    transition: all 0.2s ease;
    box-shadow: 0 2px 8px rgba(0, 0, 0, 0.25);

    &:hover {
      border-color: rgba(6, 182, 212, 0.6);
      box-shadow: 0 4px 14px rgba(6, 182, 212, 0.2);
    }

    .tunnel-card-content {
      display: flex;
      align-items: center;
      justify-content: space-between;
      gap: 10px;
      width: 100%;
    }

    .tunnel-entry {
      display: flex;
      align-items: center;
      gap: 6px;
      flex: none;
    }

    .tunnel-badge {
      font-size: 10px;
      font-weight: 700;
      padding: 1px 6px;
      border-radius: 8px;
      background: rgba(6, 182, 212, 0.2);
      color: #22d3ee;
      border: 1px solid rgba(6, 182, 212, 0.4);
    }

    .tunnel-tag {
      font-weight: 600;
      font-size: 12px;
      color: #f1f5f9;
      max-width: 100px;
      white-space: nowrap;
      overflow: hidden;
      text-overflow: ellipsis;
    }

    .tunnel-pipe {
      display: flex;
      flex-direction: column;
      align-items: center;
      gap: 2px;
      flex: 1;
      min-width: 0;

      .pipe-line {
        display: flex;
        align-items: center;
        gap: 4px;
        padding: 2px 8px;
        border-radius: 4px;
        background: rgba(15, 23, 42, 0.6);
        border: 1px dashed rgba(148, 163, 184, 0.3);
        font-size: 10.5px;
        color: #94a3b8;
        &.is-active {
          border-style: solid;
          border-color: rgba(6, 182, 212, 0.4);
          background: rgba(6, 182, 212, 0.1);
          color: #67e8f9;
        }
      }

      .pipe-proto {
        font-size: 9.5px;
        font-weight: 700;
        padding: 0 4px;
        border-radius: 3px;
        background: rgba(56, 189, 248, 0.2);
        color: #38bdf8;
      }

      .pipe-target-info {
        font-size: 10px;
        white-space: nowrap;
        overflow: hidden;
        text-overflow: ellipsis;
        max-width: 100%;
        .target-desc {
          color: #38bdf8;
          font-weight: 500;
        }
        .target-draft {
          color: #fbbf24;
        }
      }
    }

    .tunnel-actions {
      display: flex;
      align-items: center;
      gap: 4px;
      flex: none;

      .tunnel-btn {
        background: rgba(255, 255, 255, 0.08);
        border: 1px solid rgba(255, 255, 255, 0.15);
        color: #cbd5e1;
        border-radius: 4px;
        padding: 2px 5px;
        font-size: 11px;
        cursor: pointer;
        transition: all 0.15s;
        &:hover {
          background: rgba(56, 189, 248, 0.2);
          border-color: #38bdf8;
          color: #38bdf8;
        }
        &.danger:hover {
          background: rgba(239, 68, 68, 0.2);
          border-color: #ef4444;
          color: #ef4444;
        }
      }
    }
  }

  .sb-cols {
    display: flex;
    gap: 36px;
    justify-content: space-between;
    padding: 0 16px 16px;
    width: 100%;
    box-sizing: border-box;

    .sb-col {
      flex: 1;
      min-width: 0;
      display: flex;
      flex-direction: column;
      gap: 6px;
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
    }
    .sb-col-head {
      display: flex;
      align-items: center;
      justify-content: space-between;
      width: 100%;
      height: 32px;
      padding: 6px 4px 2px;
      box-sizing: border-box;
    }
    .sb-title {
      color: #94a3b8;
      font-size: 12px;
      letter-spacing: 0.5px;
      font-weight: 600;
      text-transform: uppercase;
    }
    .sb-add-btn {
      display: inline-flex;
      align-items: center;
      background: rgba(56, 189, 248, 0.12);
      border: 1px solid rgba(56, 189, 248, 0.25);
      border-radius: 4px;
      color: #38bdf8;
      font-size: 11px;
      padding: 2px 6px;
      cursor: pointer;
      transition: all 0.2s;
      &:hover {
        background: rgba(56, 189, 248, 0.25);
        border-color: #38bdf8;
        transform: translateY(-1px);
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
    background: rgba(45, 212, 191, 0.12);
    border-color: rgba(45, 212, 191, 0.25);
    color: #2dd4bf;
    &:hover {
      background: rgba(45, 212, 191, 0.25);
      border-color: #2dd4bf;
    }
  }
  .layer-capsule {
    width: 100%;
    background: rgba(15, 23, 42, 0.85);
    border: 1px solid rgba(45, 212, 191, 0.25);
    border-radius: 12px;
    padding: 6px 8px 8px;
    box-sizing: border-box;
    box-shadow: 0 2px 10px rgba(0, 0, 0, 0.25);
    display: flex;
    flex-direction: column;
    gap: 6px;

    .layer-capsule-head {
      display: flex;
      justify-content: space-between;
      align-items: center;
      gap: 8px;
      padding: 2px 4px 4px;
      border-bottom: 1px dashed rgba(45, 212, 191, 0.2);
      flex-wrap: wrap;

      .layer-title {
        display: flex;
        align-items: center;
        gap: 4px;
        font-size: 11.5px;
        font-weight: 600;
        color: #2dd4bf;
        min-width: 0;
      }
      .layer-icon {
        flex: none;
      }
      .layer-name {
        white-space: nowrap;
        overflow: hidden;
        text-overflow: ellipsis;
        max-width: 90px;
      }
      .layer-endpoint {
        font-family: monospace;
        font-size: 10.5px;
        color: #a5f3fc;
        background: rgba(45, 212, 191, 0.1);
        padding: 0 5px;
        border-radius: 4px;
        flex: none;
      }
      .layer-sec {
        font-size: 9px;
        padding: 1px 5px;
        border-radius: 4px;
        font-weight: 600;
        flex: none;
        &.tls {
          background: rgba(45, 212, 191, 0.15);
          color: #2dd4bf;
          border: 1px solid rgba(45, 212, 191, 0.3);
        }
        &.none {
          background: rgba(148, 163, 184, 0.15);
          color: #94a3b8;
          border: 1px solid rgba(148, 163, 184, 0.3);
        }
      }
      .layer-actions {
        display: flex;
        align-items: center;
        gap: 4px;
        flex: none;
      }
      .layer-copy-btn,
      .layer-act-btn {
        display: inline-flex;
        align-items: center;
        background: rgba(45, 212, 191, 0.15);
        border: 1px solid rgba(45, 212, 191, 0.35);
        border-radius: 4px;
        color: #2dd4bf;
        font-size: 10.5px;
        padding: 1px 6px;
        cursor: pointer;
        transition: all 0.15s;
        &:hover {
          background: rgba(45, 212, 191, 0.3);
          border-color: #2dd4bf;
        }
      }
      .layer-act-btn.danger {
        background: rgba(248, 113, 113, 0.15);
        border-color: rgba(248, 113, 113, 0.35);
        color: #f87171;
        &:hover {
          background: rgba(248, 113, 113, 0.3);
          border-color: #f87171;
        }
      }
    }

    .layer-capsule-body {
      display: flex;
      flex-direction: column;
      gap: 6px;
    }

    .layer-subrow {
      background: rgba(45, 212, 191, 0.06);
      border-color: rgba(45, 212, 191, 0.15);
      &:hover {
        background: rgba(45, 212, 191, 0.12);
        border-color: rgba(45, 212, 191, 0.3);
      }
    }
    .layer-unbind-btn {
      flex: none;
      width: 16px;
      height: 16px;
      line-height: 14px;
      text-align: center;
      border-radius: 50%;
      font-size: 12px;
      color: #94a3b8;
      background: rgba(148, 163, 184, 0.15);
      border: 1px solid rgba(148, 163, 184, 0.3);
      cursor: pointer;
      padding: 0;
      transition: all 0.15s;
      &:hover {
        color: #f87171;
        background: rgba(248, 113, 113, 0.2);
        border-color: #f87171;
      }
    }
    .layer-empty {
      font-style: italic;
    }
  }

  .path-badge {
    font-size: 10px;
    color: #2dd4bf;
    font-family: monospace;
    background: rgba(45, 212, 191, 0.15);
    padding: 1px 5px;
    border-radius: 4px;
    border: 1px solid rgba(45, 212, 191, 0.25);
    flex: none;
  }

  .sb-row {
    position: relative;
    display: flex;
    align-items: center;
    gap: 6px;
    padding: 0 8px;
    height: 34px;
    max-width: 100%;
    box-sizing: border-box;
    background: rgba(15, 23, 42, 0.55);
    border: 1px solid rgba(255, 255, 255, 0.05);
    border-radius: 17px;
    transition: all 0.2s;

    &:hover {
      background: rgba(15, 23, 42, 0.85);
      border-color: rgba(255, 255, 255, 0.15);
    }

    .tag {
      font-weight: 600;
      white-space: nowrap;
      overflow: hidden;
      text-overflow: ellipsis;
      max-width: 110px;
      &.clickable {
        cursor: pointer;
        &:hover {
          color: #38bdf8;
          text-decoration: underline;
        }
      }
    }
    .out-tag {
      font-weight: 600;
      color: #93c5fd;
      &.clickable {
        cursor: pointer;
        &:hover {
          text-decoration: underline;
        }
      }
      &.tunnel {
        color: #c084fc;
      }
      &.ref {
        color: #fbbf24;
      }
      &.direct {
        color: #34d399;
      }
      &.draft {
        color: #fbbf24;
        border: 1px dashed rgba(251, 191, 36, 0.4);
        padding: 1px 6px;
        border-radius: 6px;
      }
    }
    &.direct-row {
      background: rgba(52, 211, 153, 0.1);
      border-color: rgba(52, 211, 153, 0.2);
    }
    &.clickable {
      cursor: pointer;
      transition: background 0.15s ease;
      &:hover {
        background: var(--x-bg-hover, rgba(59, 130, 246, 0.08));
        .row-edit-icon {
          opacity: 0.85;
        }
      }
    }

    .row-edit-icon {
      font-size: 11px;
      opacity: 0;
      margin-left: 3px;
      color: var(--x-text-3);
      transition: opacity 0.15s ease;
      flex: none;
    }

    .port-badge {
      font-size: 11px;
      font-weight: 600;
      color: #38bdf8;
      font-family: monospace;
      flex: none;
    }

    .proto-badge {
      font-size: 9px;
      padding: 1px 4px;
      border-radius: 4px;
      font-weight: 600;
      flex: none;
      &.vless {
        background: rgba(59, 130, 246, 0.12);
        color: #60a5fa;
        border: 1px solid rgba(59, 130, 246, 0.25);
      }
      &.hy2 {
        background: rgba(20, 184, 166, 0.15);
        color: #2dd4bf;
        border: 1px solid rgba(20, 184, 166, 0.3);
      }
      &.mixed {
        background: rgba(168, 85, 247, 0.15);
        color: #c084fc;
        border: 1px solid rgba(168, 85, 247, 0.3);
      }
      &.tunnel {
        background: rgba(249, 115, 22, 0.15);
        color: #fb923c;
        border: 1px solid rgba(249, 115, 22, 0.3);
      }
      &.other {
        background: rgba(148, 163, 184, 0.15);
        color: #cbd5e1;
      }
      &.sec {
        background: rgba(168, 85, 247, 0.15);
        color: #c084fc;
        border: 1px solid rgba(168, 85, 247, 0.3);
      }
      &.net {
        background: rgba(56, 189, 248, 0.12);
        color: #7dd3fc;
      }
    }

    .out-proto-badge {
      font-size: 9px;
      padding: 1px 5px;
      border-radius: 4px;
      font-weight: 600;
      flex: none;
      background: rgba(148, 163, 184, 0.15);
      color: #cbd5e1;
      &.tunnel {
        background: rgba(168, 85, 247, 0.15);
        color: #c084fc;
        border: 1px solid rgba(168, 85, 247, 0.3);
      }
      &.ref {
        background: rgba(251, 191, 36, 0.15);
        color: #fbbf24;
        border: 1px solid rgba(251, 191, 36, 0.3);
      }
      &.direct {
        background: rgba(52, 211, 153, 0.15);
        color: #34d399;
        border: 1px solid rgba(52, 211, 153, 0.3);
      }
      &.blocked {
        background: rgba(248, 113, 113, 0.15);
        color: #f87171;
        border: 1px solid rgba(248, 113, 113, 0.3);
      }
      &.draft {
        background: rgba(251, 191, 36, 0.15);
        color: #fbbf24;
        border: 1px dashed rgba(251, 191, 36, 0.4);
      }
    }

    .type-tag {
      flex: none;
      font-size: 10px;
      padding: 1px 5px;
      border-radius: 10px;
      font-weight: 600;
      &.user {
        background: rgba(52, 211, 153, 0.15);
        color: #34d399;
      }
      &.relay {
        background: rgba(251, 191, 36, 0.15);
        color: #fbbf24;
      }
      &.tunnel {
        background: rgba(168, 85, 247, 0.15);
        color: #c084fc;
      }
      &.forward {
        background: rgba(56, 189, 248, 0.15);
        color: #38bdf8;
      }
      &.channel {
        background: rgba(244, 114, 182, 0.15);
        color: #f472b6;
      }
    }

    .tag-order-actions {
      display: inline-flex;
      flex-direction: column;
      gap: 1px;
      opacity: 0;
      transition: opacity 0.15s;
      flex: none;
      margin-left: auto;
      z-index: 4;
      .order-btn {
        background: rgba(255, 255, 255, 0.06);
        border: 1px solid rgba(255, 255, 255, 0.1);
        color: #94a3b8;
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
          color: #38bdf8;
          border-color: #38bdf8;
          background: rgba(56, 189, 248, 0.15);
        }
        &:disabled {
          opacity: 0.15;
          cursor: not-allowed;
          border-color: transparent;
          background: transparent;
        }
      }
    }
    &:hover .tag-order-actions,
    .tag-order-actions:focus-within {
      opacity: 1;
    }
  }




  .sb-empty {
    padding: 4px 10px;
    color: #64748b;
    font-size: 11px;
    height: 34px;
    display: flex;
    align-items: center;
    background: rgba(15, 23, 42, 0.3);
    border-radius: 17px;
    border: 1px dashed rgba(255, 255, 255, 0.05);
  }
}
</style>
