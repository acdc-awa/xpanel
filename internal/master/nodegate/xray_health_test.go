package nodegate

import (
	"encoding/json"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/glebarez/sqlite"
	"gorm.io/gorm"

	"github.com/acdc-awa/xpanel-node/pkg/protocol"
	"github.com/acdc-awa/xpanel/internal/models"
)

// xray 启动失败可观测性（2026-09-21）：节点心跳带回的状态/原因必须落库（面板据此显示
// "为什么没起来"），并在"进入 failed"与"failed→running 恢复"两个跃迁上各记一条 system 审计
// ——按心跳刷屏不算报警，跃迁才算。
func TestHeartbeatXrayFailureReportAndAlarm(t *testing.T) {
	path := filepath.Join(t.TempDir(), "test.db")
	db, err := gorm.Open(sqlite.Open(path), &gorm.Config{})
	if err != nil {
		t.Fatal(err)
	}
	if err := db.AutoMigrate(&models.Server{}, &models.NodeReport{}, &models.AuditLog{}); err != nil {
		t.Fatal(err)
	}
	sqlDB, err := db.DB()
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = sqlDB.Close() })

	if err := db.Create(&models.Server{ID: 7, Name: "NHK-Lite", Host: "10.0.0.7", NodeID: "node-7", Secret: "s"}).Error; err != nil {
		t.Fatal(err)
	}
	h := &Hub{DB: db}
	conn := newTestConn(7)

	beat := func(hb protocol.HeartbeatPayload) {
		t.Helper()
		raw, _ := json.Marshal(hb)
		h.handleHeartbeat(conn, &protocol.Message{Type: protocol.MsgHeartbeat, Payload: raw})
		conn.lastReportAt = time.Now().Add(-2 * nodeReportSampleInterval) // 每帧都落监控行，避免抽稀干扰
	}
	audits := func() []models.AuditLog {
		t.Helper()
		var list []models.AuditLog
		db.Order("id ASC").Find(&list)
		return list
	}

	// 1. 单次失败（restarting）：状态与原因落库，但还不报警（节点仍在自动重试）
	beat(protocol.HeartbeatPayload{
		XrayRunning: false, XrayState: "restarting", XrayFailures: 1,
		XrayLastError: "退出码 255: Failed to start: failed to listen TCP on 443 > bind: address already in use",
		XrayErrorAt:   time.Now().Unix(),
	})
	var srv models.Server
	if err := db.First(&srv, 7).Error; err != nil {
		t.Fatal(err)
	}
	if srv.XrayState != "restarting" || srv.XrayFailures != 1 {
		t.Fatalf("状态应落库: state=%q failures=%d", srv.XrayState, srv.XrayFailures)
	}
	if !strings.Contains(srv.XrayLastError, "bind: address already in use") {
		t.Fatalf("失败原因应落库，实际: %q", srv.XrayLastError)
	}
	if srv.XrayErrorAt == nil {
		t.Fatal("失败时刻应落库")
	}
	if len(audits()) != 0 {
		t.Fatalf("单次失败不应报警，实际审计 %d 条", len(audits()))
	}

	// 2. 达上限放弃自动拉起（failed）：记一条 system 审计，且按心跳重复上报不重复刷
	beat(protocol.HeartbeatPayload{
		XrayRunning: false, XrayState: "failed", XrayFailures: 5,
		XrayLastError: "退出码 255: Failed to start: failed to listen TCP on 443 > bind: address already in use",
		XrayErrorAt:   time.Now().Unix(),
	})
	beat(protocol.HeartbeatPayload{XrayRunning: false, XrayState: "failed", XrayFailures: 5,
		XrayLastError: "退出码 255: Failed to start: ... bind: address already in use"})
	got := audits()
	if len(got) != 1 {
		t.Fatalf("进入 failed 应恰好记一条审计，实际 %d 条", len(got))
	}
	if got[0].OperatorType != "system" || got[0].Action != "servers.xray_start_failed" {
		t.Fatalf("审计应为 system/servers.xray_start_failed，实际 %s/%s", got[0].OperatorType, got[0].Action)
	}
	if !strings.Contains(got[0].Detail, "NHK-Lite") || !strings.Contains(got[0].Detail, "已停止自动拉起") {
		t.Fatalf("审计详情应含服务器名与停手说明，实际: %q", got[0].Detail)
	}

	// 3. 恢复运行：再记一条恢复审计
	beat(protocol.HeartbeatPayload{XrayRunning: true, XrayState: "running", XrayFailures: 0,
		XrayLastError: got[0].Detail})
	got = audits()
	if len(got) != 2 || got[1].Action != "servers.xray_recovered" {
		t.Fatalf("恢复应记一条 servers.xray_recovered，实际: %+v", got)
	}
	if err := db.First(&srv, 7).Error; err != nil {
		t.Fatal(err)
	}
	if srv.XrayState != "running" || srv.XrayFailures != 0 {
		t.Fatalf("恢复后状态应刷新: state=%q failures=%d", srv.XrayState, srv.XrayFailures)
	}

	// 4. 旧 agent（不带这些字段）不得覆盖已有状态与原因
	beat(protocol.HeartbeatPayload{XrayRunning: true})
	if err := db.First(&srv, 7).Error; err != nil {
		t.Fatal(err)
	}
	if srv.XrayState != "running" || srv.XrayLastError == "" {
		t.Fatalf("旧 agent 心跳不应抹掉已存的状态/原因: state=%q err=%q", srv.XrayState, srv.XrayLastError)
	}
}
