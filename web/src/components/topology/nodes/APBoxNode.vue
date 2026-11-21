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

const protoBadge = computed(() => {
  const raw = protocolDisplayName(props.data.ap.resolved_protocol)
  if (raw === 'hysteria') return { label: 'HY2', cls: 'neutral' }
  if (raw === 'tunnel') return { label: 'TUNNEL', cls: 'teal' }
  return { label: raw ? raw.toUpperCase() : 'VLESS', cls: 'blue' }
})
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

    <div class="ap-head">
      <div class="ap-head-left">
        <CsIcon name="globe" size="small" class="ap-icon" />
        <span class="ap-name-text" :title="data.ap.name">{{ data.ap.name }}</span>
      </div>
      <div class="ap-head-badges">
        <span class="ap-badge role">接入点</span>
        <span v-if="!data.ap.enabled" class="ap-badge gray">禁用</span>
      </div>
    </div>

    <div class="ap-body">
      <div class="ap-node-perms">
        <template v-if="data.ap.permission_group_ids && data.ap.permission_group_ids.length > 0">
          <span
            v-for="gid in data.ap.permission_group_ids.slice(0, 3)"
            :key="gid"
            class="ap-badge blue perm-item"
            :title="groupName(gid)"
          >
            {{ groupName(gid) }}
          </span>
          <span v-if="data.ap.permission_group_ids.length > 3" class="ap-badge gray">
            +{{ data.ap.permission_group_ids.length - 3 }}
          </span>
        </template>
        <span v-else class="ap-perm-warn">未分配权限组（全员不可见）</span>
      </div>

      <!-- 管道消费的实时端点展示 -->
      <div v-if="data.ap.resolved_host" class="ap-endpoint-row">
        <span class="ap-dot online" />
        <span class="ap-endpoint-text" :title="`${data.ap.resolved_host}:${data.ap.resolved_port}`">
          {{ data.ap.resolved_host }}:{{ data.ap.resolved_port }}
        </span>
        <span class="ap-proto-badge" :class="protoBadge.cls">{{ protoBadge.label }}</span>
      </div>
      <div v-else class="ap-waiting-text">
        <span class="ap-dot offline" />
        <span>未绑定：从右侧端点拖出连线</span>
      </div>

      <div v-if="data.ap.resolved_target_desc" class="ap-desc-text" :title="data.ap.resolved_target_desc">
        直连 → {{ data.ap.resolved_target_desc }}
      </div>
    </div>
  </div>
</template>

<style scoped lang="scss">
.ap-node {
  position: relative;
  width: 280px;
  box-sizing: border-box;
  background: var(--topo-card-bg);
  border: 1px solid var(--topo-card-border);
  border-radius: var(--topo-radius, 16px);
  cursor: pointer;
  overflow: visible;
  box-shadow: var(--topo-card-shadow);
  transition: border-color 0.2s, box-shadow 0.2s;

  &:hover {
    border-color: var(--topo-card-border-hover);
    box-shadow: var(--topo-card-shadow-hover);
    .ap-endpoint-row {
      background: var(--topo-row-hover);
      border-color: var(--topo-accent-border);
    }
  }
  &.unlinked {
    border: 1px dashed var(--topo-warning);
    &:hover {
      border-color: var(--topo-warning);
    }
  }
  &.disabled {
    .ap-head,
    .ap-body {
      opacity: 0.65;
    }
  }

  .ap-head {
    height: 40px;
    padding: 0 14px;
    display: flex;
    align-items: center;
    justify-content: space-between;
    gap: 8px;
    background: var(--topo-head-bg);
    border-bottom: 1px solid var(--topo-divider);
    border-radius: 16px 16px 0 0;
    box-sizing: border-box;
  }
  .ap-head-left {
    display: flex;
    align-items: center;
    gap: 6px;
    min-width: 0;
    flex: 1;
  }
  .ap-icon {
    flex: none;
    color: var(--topo-accent);
  }
  .ap-name-text {
    font-size: 13px;
    font-weight: 600;
    color: var(--topo-text-1);
    white-space: nowrap;
    overflow: hidden;
    text-overflow: ellipsis;
    min-width: 0;
  }
  .ap-head-badges {
    display: flex;
    align-items: center;
    gap: 4px;
    flex: none;
  }

  .ap-body {
    padding: 12px 14px;
    display: flex;
    flex-direction: column;
    gap: 8px;
  }

  .ap-node-perms {
    display: flex;
    align-items: center;
    gap: 4px;
    min-height: 20px;
    overflow: hidden;

    .perm-item {
      display: inline-block;
      flex: 0 1 auto;
      min-width: 28px;
      max-width: 112px;
      line-height: 16px;
      overflow: hidden;
      text-overflow: ellipsis;
    }
  }
  .ap-perm-warn {
    font-size: 11px;
    line-height: 20px;
    color: var(--topo-warning);
  }

  .ap-endpoint-row {
    height: 32px;
    padding: 0 10px;
    display: flex;
    align-items: center;
    gap: 6px;
    background: var(--topo-row-bg);
    border: 1px solid var(--topo-row-border);
    border-radius: 8px;
    box-sizing: border-box;
    transition: background-color 0.15s, border-color 0.15s;
  }
  .ap-endpoint-text {
    font-size: 12px;
    font-weight: 600;
    font-variant-numeric: tabular-nums;
    color: var(--topo-text-1);
    flex: 1;
    min-width: 0;
    white-space: nowrap;
    overflow: hidden;
    text-overflow: ellipsis;
  }
  .ap-proto-badge {
    font-size: 9px;
    padding: 1px 5px;
    border-radius: 4px;
    font-weight: 600;
    flex: none;
    border: 1px solid transparent;
    &.blue {
      background: var(--topo-accent-soft);
      color: var(--topo-accent);
      border-color: var(--topo-accent-border);
    }
    &.neutral {
      background: var(--topo-surface);
      color: var(--topo-text-2);
      border-color: var(--topo-card-border);
    }
    &.teal {
      background: var(--topo-teal-soft);
      color: var(--topo-teal);
      border-color: var(--topo-teal-border);
    }
  }

  .ap-waiting-text {
    display: flex;
    align-items: center;
    gap: 6px;
    min-height: 20px;
    font-size: 11px;
    line-height: 20px;
    color: var(--topo-warning);
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
    font-size: 11px;
    line-height: 16px;
    color: var(--topo-text-2);
    white-space: nowrap;
    overflow: hidden;
    text-overflow: ellipsis;
  }

  .ap-badge {
    display: inline-flex;
    align-items: center;
    height: 18px;
    line-height: 1;
    padding: 0 6px;
    box-sizing: border-box;
    border-radius: 4px;
    border: 1px solid transparent;
    font-size: 10px;
    font-weight: 600;
    white-space: nowrap;
    flex: none;

    &.role,
    &.blue {
      background: var(--topo-accent-soft);
      border-color: var(--topo-accent-border);
      color: var(--topo-accent);
    }
    &.gray {
      background: var(--topo-surface);
      border-color: var(--topo-card-border);
      color: var(--topo-text-2);
    }
  }
}
</style>
