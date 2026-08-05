package services

import (
	"path/filepath"
	"testing"

	"github.com/glebarez/sqlite"
	"gorm.io/gorm"

	"github.com/zhx/xray-panel/internal/models"
)

func newTestDB(t *testing.T) *gorm.DB {
	t.Helper()
	path := filepath.Join(t.TempDir(), "test.db")
	db, err := gorm.Open(sqlite.Open(path), &gorm.Config{})
	if err != nil {
		t.Fatal(err)
	}
	if err := db.AutoMigrate(&models.PendingConfig{}); err != nil {
		t.Fatal(err)
	}
	sqlDB, err := db.DB()
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = sqlDB.Close() }) // Windows 文件锁：必须在 TempDir 清理前关闭
	return db
}

// TestSavePendingOverwriteRace 复现 pending 覆盖竞态：
// 旧内容 cfgA 推送回执到达时，pending 行已被 cfgB 覆盖，MarkPushedIfSame(cfgA)
// 不得把 cfgB 误标为 pushed（否则节点永远收不到 cfgB）。
func TestSavePendingOverwriteRace(t *testing.T) {
	s := &ConfigService{DB: newTestDB(t)}
	const cfgA = `{"version":1}`
	const cfgB = `{"version":2}`

	if err := s.SavePending(7, cfgA); err != nil {
		t.Fatal(err)
	}
	p, err := s.GetPending(7)
	if err != nil || p == nil {
		t.Fatal("应存在 pending 记录")
	}

	// 并发覆盖：同一行被 SavePending(cfgB) 覆盖（行 ID 不变）
	if err := s.SavePending(7, cfgB); err != nil {
		t.Fatal(err)
	}

	// 旧内容 cfgA 的推送回执到达：不得标记 pushed
	marked, err := s.MarkPushedIfSame(p.ID, cfgA)
	if err != nil {
		t.Fatal(err)
	}
	if marked {
		t.Fatal("cfgA 已被 cfgB 覆盖，MarkPushedIfSame 不应标记成功（否则 cfgB 未下发却被标为已推送）")
	}
	p2, err := s.GetPending(7)
	if err != nil || p2 == nil {
		t.Fatal("应存在 pending 记录")
	}
	if p2.Status != "pending" {
		t.Fatalf("新内容 cfgB 应保持 pending（等待下一轮推送），实际 status=%s", p2.Status)
	}
	if p2.ConfigJSON != cfgB {
		t.Fatalf("pending 内容应为 cfgB，实际 %q", p2.ConfigJSON)
	}

	// 新内容 cfgB 下发成功 → 标记成功
	marked, err = s.MarkPushedIfSame(p2.ID, cfgB)
	if err != nil {
		t.Fatal(err)
	}
	if !marked {
		t.Fatal("内容未变化的正常标记应成功")
	}
	p3, err := s.GetPending(7)
	if err != nil || p3 == nil {
		t.Fatal("应存在 pending 记录")
	}
	if p3.Status != "pushed" {
		t.Fatalf("cfgB 已下发，期望 pushed，实际 status=%s", p3.Status)
	}

	// 已 pushed 后重复标记：不应再次生效（status != pending）
	marked, err = s.MarkPushedIfSame(p3.ID, cfgB)
	if err != nil || marked {
		t.Fatalf("已 pushed 的记录不应重复标记 (marked=%v err=%v)", marked, err)
	}
}

// TestMarkPushedByServerIfSame 按 server 维度的同语义回归测试。
func TestMarkPushedByServerIfSame(t *testing.T) {
	s := &ConfigService{DB: newTestDB(t)}
	const cfgA = `{"v":1}`
	const cfgB = `{"v":2}`

	if err := s.SavePending(9, cfgA); err != nil {
		t.Fatal(err)
	}
	// 覆盖竞态：cfgB 覆盖 cfgA
	if err := s.SavePending(9, cfgB); err != nil {
		t.Fatal(err)
	}
	// 旧内容回执：不得标记
	marked, err := s.MarkPushedByServerIfSame(9, cfgA)
	if err != nil || marked {
		t.Fatalf("覆盖后旧内容不应标记成功 (marked=%v err=%v)", marked, err)
	}
	// 新内容：标记成功
	marked, err = s.MarkPushedByServerIfSame(9, cfgB)
	if err != nil || !marked {
		t.Fatalf("新内容应标记成功 (marked=%v err=%v)", marked, err)
	}
	p, _ := s.GetPending(9)
	if p.Status != "pushed" {
		t.Fatalf("期望 pushed，实际 %s", p.Status)
	}
}

// TestSavePendingOverwriteKeepsRowID SavePending 覆盖必须更新同一行（ID 不变），
// 这是 MarkPushedIfSame 竞态修复依赖的不变量。
func TestSavePendingOverwriteKeepsRowID(t *testing.T) {
	s := &ConfigService{DB: newTestDB(t)}
	if err := s.SavePending(11, `{"v":1}`); err != nil {
		t.Fatal(err)
	}
	p1, err := s.GetPending(11)
	if err != nil || p1 == nil {
		t.Fatal("应存在 pending 记录")
	}
	if err := s.SavePending(11, `{"v":2}`); err != nil {
		t.Fatal(err)
	}
	p2, _ := s.GetPending(11)
	if p1.ID != p2.ID {
		t.Fatalf("覆盖后行 ID 应不变（竞态窗口基于同一行），实际 %d != %d", p1.ID, p2.ID)
	}
	if p2.Status != "pending" {
		t.Fatalf("覆盖后应回到 pending，实际 %s", p2.Status)
	}
	if p2.PushedAt != nil {
		t.Fatal("覆盖后 pushed_at 应清空")
	}
}

