package nodegate

// 回归用例（2026-11-15）：磁盘偏离必须走冷推修复。
//
// 事故现场：把节点从旧服务器迁移到新服务器（重置密钥复用同一 server 记录）后，新机 agent
// 首次启动写自举最小配置，磁盘哈希立刻偏离主控记录；而 pending 行仍是 pushed，PushPending
// 直接早退，漂移自愈又走 SyncUsers（热更）——热更因 handler not found 整条失败、连带丢掉
// 顺带下发的整份配置，偏离永远修不回来。只有改一个入站（SavePending → pending）才恢复。
//
// 本用例锁住两条语义：
//  1. repairDisk 会把 pushed 行重建为 pending（否则 PushPending 永远早退），内容为现场重算；
//  2. 期间若已有更新的待推送编辑，repairDisk 不得覆盖它（内容 CAS）。

import (
	"testing"
	"time"

	"gorm.io/gorm"

	"github.com/acdc-awa/xpanel/internal/models"
)

// markSynced 造出「冷推成功、主控记录与节点一致」的 SYNCED 状态。
func markSynced(t *testing.T, h *Hub, db *gorm.DB, serverID uint64, content string) {
	t.Helper()
	if err := h.Config.SavePending(serverID, content); err != nil {
		t.Fatal(err)
	}
	var p models.PendingConfig
	if err := db.Where("server_id = ?", serverID).First(&p).Error; err != nil {
		t.Fatal(err)
	}
	if _, err := h.Config.MarkPushedIfSame(p.ID, content); err != nil {
		t.Fatal(err)
	}
	if err := h.Config.MarkApplied(serverID, content); err != nil {
		t.Fatal(err)
	}
}

// TestRepairDiskEnqueuesColdPush 偏离修复必须重建待推送行：status=pushed 会让
// PushPending 在入口直接返回，冷推根本发不出去。
func TestRepairDiskEnqueuesColdPush(t *testing.T) {
	h, db := newApplyHub(t)
	srv := seedServer(t, db, "node-1", "in-a")
	const stale = `{"inbounds":[{"tag":"in-a"}],"note":"stale-disk"}`
	markSynced(t, h, db, srv.ID, stale)

	want, err := h.Config.Generate(srv.ID)
	if err != nil {
		t.Fatal(err)
	}
	if want == stale {
		t.Fatal("前置条件：现场重算的内容应与陈旧内容不同")
	}

	h.repairDisk(srv.ID)

	p, err := h.Config.GetPending(srv.ID)
	if err != nil || p == nil {
		t.Fatalf("应有待推送记录: %v", err)
	}
	if p.Status != "pending" {
		t.Fatalf("偏离修复必须把行重建为待推送（否则 PushPending 早退），实际 status=%q", p.Status)
	}
	if p.ConfigJSON != want {
		t.Fatalf("待推送内容应为现场重算的整份配置，实际:\n%s", p.ConfigJSON)
	}
}

// TestRepairDiskDoesNotOverwriteNewerEdit 修复期间已有的更新编辑（pending）不得被覆盖：
// 用户刚改了结构，正常推送流程会带着新内容下发，偏离修复不能抢。
func TestRepairDiskDoesNotOverwriteNewerEdit(t *testing.T) {
	h, db := newApplyHub(t)
	srv := seedServer(t, db, "node-1", "in-a")
	markSynced(t, h, db, srv.ID, `{"inbounds":[{"tag":"in-a"}],"note":"old"}`)

	const newer = `{"inbounds":[{"tag":"in-a"}],"note":"newer-edit-pending"}`
	if err := h.Config.SavePending(srv.ID, newer); err != nil {
		t.Fatal(err)
	}

	h.repairDisk(srv.ID)

	p, err := h.Config.GetPending(srv.ID)
	if err != nil || p == nil {
		t.Fatal(err)
	}
	if p.ConfigJSON != newer || p.Status != "pending" {
		t.Fatalf("不得覆盖更新的待推送编辑，实际 status=%q content=%s", p.Status, p.ConfigJSON)
	}
}

// TestBeginDriftRepairThrottles 冷推包含 Stop+Start，偏离期间心跳高频命中必须限频；
// 失败由 pending 补推循环兜底，不靠心跳每帧重开。
func TestBeginDriftRepairThrottles(t *testing.T) {
	now := time.Now()
	c := &Conn{}
	if !c.beginDriftRepair(now) {
		t.Fatal("首次应放行")
	}
	if c.beginDriftRepair(now.Add(30 * time.Second)) {
		t.Fatal("间隔内应拒绝（否则偏离期间每次心跳都重启一次 xray）")
	}
	if !c.beginDriftRepair(now.Add(diskRepairInterval)) {
		t.Fatal("超过间隔应重新放行")
	}
}
