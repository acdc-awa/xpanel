<script setup lang="ts">
import { computed, ref } from 'vue'
import { CsIcon } from '@/components/cloudscape'
import { Handle, Position } from '@vue-flow/core'
import type { PermissionGroup, UserAccessPoint } from '@/api/types'
import { protocolDisplayName } from '@/utils/protocol'
import { useGridHeight } from '../useGridHeight'

const props = defineProps<{
  data: { ap: UserAccessPoint }
  editable: boolean
  permissionGroups: PermissionGroup[]
}>()

const emit = defineEmits<{
  (e: 'edit', ap: UserAccessPoint): void
}>()

const boxEl = ref<HTMLElement | null>(null)
useGridHeight(boxEl)

const groupNameMap = computed(
  () => new Map(props.permissionGroups.map((g) => [g.id, g.name]))
)

function groupName(id: number): string {
  return groupNameMap.value.get(id) ?? `组#${id}`
}
</script>

<template>
  <div
    ref="boxEl"
    class="ap-node"
    :class="{ unlinked: !data.ap.target_type, disabled: !data.ap.enabled }"
    @click.stop="emit('edit', data.ap)"
  >
    <!-- 右侧源端点：拖线至目标入站（inb-tgt） -->
    <Handle
      type="source"
      :id="`ap-src-${data.ap.id}`"
      :position="Position.Right"
      :connectable="editable"
      class="ep ap-src-handle"
      title="拖线至目标用户入站"
    />

    <div class="ap-node-head">
      <CsIcon name="globe" class="ap-icon" />
      <span class="ap-name-text" :title="data.ap.name">{{ data.ap.name }}</span>
      <span v-if="!data.ap.enabled" class="ap-chip gray">禁用</span>
    </div>

    <div class="ap-node-perms">
      <template v-if="data.ap.permission_group_ids && data.ap.permission_group_ids.length > 0">
        <span v-for="gid in data.ap.permission_group_ids.slice(0, 3)" :key="gid" class="ap-chip blue">
          {{ groupName(gid) }}
        </span>
        <span v-if="data.ap.permission_group_ids.length > 3" class="ap-chip gray">
          +{{ data.ap.permission_group_ids.length - 3 }}
        </span>
      </template>
      <span v-else class="ap-chip orange">未分配权限组（全员不可见）</span>
    </div>

    <!-- 管道消费的实时端点展示 -->
    <div v-if="data.ap.resolved_host" class="ap-resolved-text">
      <span class="ap-dot online" />
      <code class="cell-mono">{{ data.ap.resolved_host }}:{{ data.ap.resolved_port }}</code>
      <span class="ap-chip" :class="data.ap.resolved_protocol === 'hysteria' ? 'teal' : 'blue'">
        {{ data.ap.resolved_protocol === 'hysteria' ? 'Hysteria 2' : (protocolDisplayName(data.ap.resolved_protocol).toUpperCase() || 'VLESS') }}
      </span>
    </div>
    <div v-else class="ap-resolved-text waiting">
      <span class="ap-dot offline" />
      <span style="font-size: 11px">未绑定：从右侧端点拖出连线</span>
    </div>

    <div v-if="data.ap.resolved_target_desc" class="ap-desc-text" :title="data.ap.resolved_target_desc">
      直连 → {{ data.ap.resolved_target_desc }}
    </div>
  </div>
</template>

<style scoped lang="scss">
.ap-node {
  position: relative;
  width: 280px;
  background: var(--topo-card-bg);
  border: 1px solid var(--topo-card-border);
  border-left: 3px solid var(--topo-accent);
  border-radius: var(--topo-radius, 16px);
  padding: 16px;
  cursor: pointer;
  display: flex;
  flex-direction: column;
  gap: 8px;
  box-shadow: var(--topo-card-shadow);
  transition: border-color 0.2s, box-shadow 0.2s;
  contain: paint;
  transform: translateZ(0);

  &:hover {
    border-color: var(--topo-card-border-hover);
    border-left-color: var(--topo-accent);
    box-shadow: var(--topo-card-shadow-hover);
  }
  &.unlinked {
    border: 1.5px dashed var(--topo-warning);
    &:hover {
      border-color: var(--topo-warning);
    }
  }
  &.disabled {
    opacity: 0.6;
    filter: grayscale(0.4);
  }

  .ap-node-head {
    display: flex;
    align-items: center;
    gap: 8px;
    min-height: 24px;
  }
  .ap-icon {
    flex: none;
    color: var(--topo-accent);
  }
  .ap-name-text {
    font-size: 13px;
    font-weight: 700;
    color: var(--topo-text-1);
    white-space: nowrap;
    overflow: hidden;
    text-overflow: ellipsis;
    flex: 1;
    min-width: 0;
  }
  /* 固定 24px 高、不换行：权限组数量再多也不改变卡片高度，保证底边落在 8pt 网格上 */
  .ap-node-perms {
    display: flex;
    align-items: center;
    gap: 4px;
    flex-wrap: nowrap;
    overflow: hidden;
    height: 24px;

    .ap-chip {
      flex: none;
      max-width: 120px;
      overflow: hidden;
      text-overflow: ellipsis;
      white-space: nowrap;
    }
  }
  .ap-resolved-text {
    display: flex;
    align-items: center;
    gap: 6px;
    min-height: 24px;
    font-size: 11.5px;
    color: var(--topo-accent);
    &.waiting {
      color: var(--topo-warning);
    }
  }
  .ap-dot {
    width: 6px;
    height: 6px;
    border-radius: 50%;
    flex: none;
    &.online {
      background: var(--topo-success);
    }
    &.offline {
      background: var(--topo-warning);
    }
  }
  .ap-desc-text {
    font-size: 10.5px;
    line-height: 16px;
    color: var(--topo-text-2);
    white-space: nowrap;
    overflow: hidden;
    text-overflow: ellipsis;
  }

  /* 语义徽标：全部消费画布亮/暗自适应令牌，与 Cloudscape Badge 保持一致 */
  .ap-chip {
    display: inline-flex;
    align-items: center;
    height: 18px;
    padding: 0 6px;
    border-radius: 999px;
    border: 1px solid transparent;
    font-size: 10px;
    line-height: 1;
    font-weight: 500;
    white-space: nowrap;

    &.blue {
      background: var(--topo-accent-soft);
      border-color: var(--topo-accent-border);
      color: var(--topo-accent);
    }
    &.gray {
      background: var(--topo-row-bg);
      border-color: var(--topo-row-border);
      color: var(--topo-text-2);
    }
    &.orange {
      background: var(--topo-warning-soft);
      border-color: var(--topo-warning-border);
      color: var(--topo-warning);
    }
    &.teal {
      background: var(--topo-teal-soft);
      border-color: var(--topo-teal-border);
      color: var(--topo-teal);
    }
  }
}
</style>
