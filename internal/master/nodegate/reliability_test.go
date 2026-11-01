package nodegate

import (
	"path/filepath"
	"sync"
	"testing"
	"time"

	"github.com/glebarez/sqlite"
	"github.com/gorilla/websocket"
	"gorm.io/gorm"

	"github.com/acdc-awa/xpanel/internal/models"
	"github.com/acdc-awa/xpanel-node/pkg/protocol"
)

func newTestHub(t *testing.T) *Hub {
	t.Helper()
	path := filepath.Join(t.TempDir(), "test.db")
	db, err := gorm.Open(sqlite.Open(path), &gorm.Config{})
	if err != nil {
		t.Fatal(err)
	}
	if err := db.AutoMigrate(&models.Server{}); err != nil {
		t.Fatal(err)
	}
	sqlDB, err := db.DB()
	if err != nil {
		t.Fatal(err)
	}
	// 用真实 NewHub：它有 quit 通道与 watchdog，Cleanup 里才能按「先停 Hub、再关库」收尾。
	// ServeWS 在回 auth_ok 之后仍会写在线状态并起若干后台推送，若不先停就开清理，这些收尾
	// 会与 t.TempDir 的 RemoveAll 并发——Linux CI 上偶发
	// "TempDir RemoveAll cleanup: directory not empty"（2026-11-01 打 tag 时命中）。
	h := NewHub(db, nil, nil)
	t.Cleanup(func() {
		h.Shutdown()
		_ = sqlDB.Close() // Windows 文件锁：必须在 TempDir 清理前关闭
	})
	return h
}

// closeWSAndWait 关掉客户端连接并等网关注销它。
//
// readPump 退出的顺序在「在线状态落库」之后，所以一旦注销，ServeWS 里的同步写就已完成；
// 只剩被取消的后台推送在跑。测试直接 return 而不等的话，t.TempDir 清理会与这些收尾并发。
func closeWSAndWait(t *testing.T, h *Hub, ws *websocket.Conn, serverID uint64) {
	t.Helper()
	_ = ws.Close()
	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) {
		if !h.IsOnline(serverID) {
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatalf("网关未在 3s 内注销连接（server=%d）", serverID)
}

// newTestConn 构造测试连接（WS 可为 nil，closeSafe 对 nil 容错）。
func newTestConn(serverID uint64) *Conn {
	c := &Conn{
		ServerID: serverID,
		NodeID:   "test-node",
		Send:     make(chan []byte, 64),
		done:     make(chan struct{}),
	}
	c.LastSeen.Store(time.Now().Unix())
	return c
}

// TestSendRaceWithUnregister 复现"节点重连替换旧连接"与并发 Send/Ask 的竞态：
// 旧连接 readPump 退出 → unregister 关闭 conn.Send 通道，与此同时
// 每小时校准/API 推送的 Send 可能正在向该通道发送 —— 修复前会导致
// "send on closed channel" panic，直接崩溃整个主控进程。
func TestSendRaceWithUnregister(t *testing.T) {
	h := newTestHub(t)
	for i := 0; i < 50; i++ {
		conn := newTestConn(1)
		h.mu.Lock()
		h.conns[1] = conn
		h.mu.Unlock()

		var wg sync.WaitGroup
		wg.Add(1)
		go func() { // 模拟旧连接 readPump 退出 → unregister（旧代码会 close(conn.Send)）
			defer wg.Done()
			h.unregister(conn)
		}()
		for j := 0; j < 20; j++ {
			wg.Add(1)
			go func() { // 模拟并发 Ask/Send（每小时校准、API 推送）
				defer wg.Done()
				_ = h.Send(1, []byte(`{"type":"push_config"}`))
			}()
		}
		wg.Wait()
	}
}

// TestAskAbortedOnUnregister 验证节点断连时，正在等待该节点回执的 Ask 立即被唤醒返回错误，
// 而不需要死等超时。
func TestAskAbortedOnUnregister(t *testing.T) {
	h := newTestHub(t)
	conn := newTestConn(1)
	h.mu.Lock()
	h.conns[1] = conn
	h.mu.Unlock()

	// 消耗 conn.Send 中的消息，避免 Ask 写入 Send 阻塞
	go func() {
		for {
			select {
			case <-conn.Send:
			case <-conn.done:
				return
			}
		}
	}()

	resCh := make(chan *protocol.ResultPayload, 1)
	errCh := make(chan error, 1)
	go func() {
		// 发起一个超时长达 10 秒的 Ask 指令
		res, err := h.Ask(1, "test_cmd", nil, 10*time.Second)
		resCh <- res
		errCh <- err
	}()

	// 稍作等待确保 Ask 已经注册到 pending 映射并写入 Send
	time.Sleep(50 * time.Millisecond)

	// 模拟节点连接断开
	h.unregister(conn)

	select {
	case res := <-resCh:
		if res == nil || res.OK || res.Error != "服务器连接已断开" {
			t.Fatalf("预期收到节点断开回执, 实际: %+v", res)
		}
	case <-time.After(500 * time.Millisecond):
		t.Fatal("unregister 未能立即唤醒正在等待的 Ask，仍在阻塞等待！")
	}
}
