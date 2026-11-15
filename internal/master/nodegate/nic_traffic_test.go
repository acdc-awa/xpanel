package nodegate

import (
	"path/filepath"
	"testing"
	"time"

	"github.com/acdc-awa/xpanel-node/pkg/protocol"
	"github.com/glebarez/sqlite"
	"gorm.io/gorm"

	"github.com/acdc-awa/xpanel/internal/master/services"
	"github.com/acdc-awa/xpanel/internal/models"
)

func newNICTestHub(t *testing.T) (*Hub, *gorm.DB) {
	t.Helper()
	path := filepath.Join(t.TempDir(), "nic.db")
	db, err := gorm.Open(sqlite.Open(path), &gorm.Config{})
	if err != nil {
		t.Fatal(err)
	}
	if err := db.AutoMigrate(&models.Setting{}, &models.Server{}, &models.NodeReport{}, &models.ServerNICTrafficDaily{}); err != nil {
		t.Fatal(err)
	}
	sqlDB, err := db.DB()
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = sqlDB.Close() })
	if err := services.SetSetting(db, services.SettingBusinessTimezone, "UTC"); err != nil {
		t.Fatal(err)
	}
	if err := db.Create(&models.Server{ID: 1, Name: "n1", Host: "10.0.0.1", NodeID: "node-1", Secret: "s"}).Error; err != nil {
		t.Fatal(err)
	}
	h := &Hub{
		DB:            db,
		conns:         make(map[uint64]*Conn),
		latestMetrics: make(map[uint64]*NodeMetricsSnapshot),
		nicAcc:        make(map[uint64]*nicAccumulator),
	}
	return h, db
}

// 累计器：基线 → 正常差分 → 重置归零；flush 后日表累加且记录 last_raw。
func TestHubNICAccumulateAndFlush(t *testing.T) {
	h, db := newNICTestHub(t)
	day := time.Date(2026, 11, 15, 10, 0, 0, 0, time.UTC)

	h.accumulateNIC(1, 1000, 500) // 首帧只建基线
	if a := h.nicAcc[1]; a == nil || a.pendingRx != 0 || a.pendingTx != 0 || !a.have {
		t.Fatalf("首帧基线异常: %+v", a)
	}
	h.accumulateNIC(1, 1500, 700) // +500/+200
	h.accumulateNIC(1, 200, 100)  // 计数器归零 → +200/+100
	h.flushNIC(day)

	var row models.ServerNICTrafficDaily
	if err := db.Where("server_id = ? AND date = ?", 1, "2026-11-15").First(&row).Error; err != nil {
		t.Fatal(err)
	}
	if row.RxBytes != 700 || row.TxBytes != 300 || row.LastRawRx != 200 || row.LastRawTx != 100 {
		t.Fatalf("第一次落库 = %+v, want rx=700 tx=300 last=200/100", row)
	}

	h.accumulateNIC(1, 400, 200) // 同一天继续 +200/+100
	h.flushNIC(day.Add(time.Minute))
	if err := db.Where("server_id = ? AND date = ?", 1, "2026-11-15").First(&row).Error; err != nil {
		t.Fatal(err)
	}
	if row.RxBytes != 900 || row.TxBytes != 400 || row.LastRawRx != 400 || row.LastRawTx != 200 {
		t.Fatalf("第二次落库 = %+v, want rx=900 tx=400 last=400/200", row)
	}
	// pending 已扣掉，不重复累计
	if a := h.nicAcc[1]; a.pendingRx != 0 || a.pendingTx != 0 {
		t.Fatalf("flush 后 pending 未清零: %+v", a)
	}
}

// 主控重启：从日表最新一行的 last_raw 续算，不把停机期间的流量漏掉或重复计入。
func TestHubNICResumeFromBaseline(t *testing.T) {
	h, db := newNICTestHub(t)
	if err := db.Create(&models.ServerNICTrafficDaily{
		ServerID: 1, Date: "2026-11-14", RxBytes: 111, TxBytes: 222, LastRawRx: 5000, LastRawTx: 6000,
	}).Error; err != nil {
		t.Fatal(err)
	}
	// 新进程：没有内存累计器，loadNICBaselines 应从日表恢复基线
	h2 := &Hub{
		DB:            db,
		conns:         make(map[uint64]*Conn),
		latestMetrics: make(map[uint64]*NodeMetricsSnapshot),
		nicAcc:        make(map[uint64]*nicAccumulator),
	}
	h2.loadNICBaselines()
	if a := h2.nicAcc[1]; a == nil || !a.have || a.lastRx != 5000 || a.lastTx != 6000 {
		t.Fatalf("基线未恢复: %+v", a)
	}
	// 停机期间涨到 5600/6400：下一帧差分为 600/400
	h2.accumulateNIC(1, 5600, 6400)
	if a := h2.nicAcc[1]; a.pendingRx != 600 || a.pendingTx != 400 {
		t.Fatalf("重启后续算 = %+v, want +600/+400", a)
	}
	_ = h
}

// 快慢车道归属：快车道只认 metrics，旧 agent 只认心跳；0/0 帧不建立基线。
func TestHubNICLaneAttribution(t *testing.T) {
	h, _ := newNICTestHub(t)
	conn := newTestConn(1)
	h.conns[1] = conn

	// 旧 agent：心跳携带累计字节
	feed(t, h, conn, protocol.MsgHeartbeat, protocol.HeartbeatPayload{
		RxBytes: 1000, TxBytes: 500, XrayRunning: true,
	})
	if a := h.nicAcc[1]; a == nil || !a.have || a.pendingRx != 0 {
		t.Fatalf("旧 agent 首帧基线异常: %+v", a)
	}
	feed(t, h, conn, protocol.MsgHeartbeat, protocol.HeartbeatPayload{
		RxBytes: 1500, TxBytes: 700, XrayRunning: true,
	})
	if a := h.nicAcc[1]; a.pendingRx != 500 || a.pendingTx != 200 {
		t.Fatalf("旧 agent 差分异常: %+v", a)
	}

	// 首帧 metrics 后连接切到快车道；之后的 0/0 心跳不能把基线打回 0
	feed(t, h, conn, protocol.MsgMetrics, protocol.MetricsPayload{
		RxBytes: 1600, TxBytes: 800, XrayRunning: true,
	})
	if a := h.nicAcc[1]; a.pendingRx != 600 || a.pendingTx != 300 {
		t.Fatalf("快车道差分异常: %+v", a)
	}
	feed(t, h, conn, protocol.MsgHeartbeat, protocol.HeartbeatPayload{XrayRunning: true})
	if a := h.nicAcc[1]; a.pendingRx != 600 || a.pendingTx != 300 || a.lastRx != 1600 || a.lastTx != 800 {
		t.Fatalf("快车道连接上的空心跳污染了累计器: %+v", a)
	}

	// 真正的 0/0 帧（旧 agent 没上报）不建立基线
	h2, _ := newNICTestHub(t)
	h2.accumulateNIC(1, 0, 0)
	if a := h2.nicAcc[1]; a != nil {
		t.Fatalf("0/0 帧不应建立基线: %+v", a)
	}
}
