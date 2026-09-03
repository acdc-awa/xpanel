package nodegate

import (
	"encoding/json"
	"testing"
	"time"

	"github.com/acdc-awa/xpanel-node/pkg/protocol"
	"github.com/acdc-awa/xpanel/internal/models"
)

// fakeAgentReply 消费 conn.Send，对 agent_settings 请求立即回 OK result——
// 模拟真实 agent 收到指令→handle→回执的链路，纯进程内无需真实 WS。
// 连接关闭（done）后退出；Send 刻意不被关闭（closeSafe 语义），必须监听 done。
func fakeAgentReply(h *Hub, conn *Conn) {
	go func() {
		for {
			select {
			case data := <-conn.Send:
				msg, err := protocol.Decode(data)
				if err != nil || msg.Type != protocol.MsgAgentSettings {
					continue
				}
				raw, _ := json.Marshal(protocol.ResultPayload{OK: true, Data: "设置无变更"})
				h.handleResult(&protocol.Message{Type: protocol.MsgResult, ID: msg.ID, Payload: raw})
			case <-conn.done:
				return
			}
		}
	}()
}

// TestPushAgentSettingsMarksSyncedOnAck 成功回执后应置位 settingsSynced，
// watchdog 2 分钟补推循环据此停止对该连接的重试。
func TestPushAgentSettingsMarksSyncedOnAck(t *testing.T) {
	h := newTestHub(t)
	if err := h.DB.AutoMigrate(&models.Setting{}); err != nil {
		t.Fatal(err)
	}
	conn := newTestConn(1)
	h.mu.Lock()
	h.conns[1] = conn
	h.mu.Unlock()
	fakeAgentReply(h, conn)

	if conn.settingsSynced.Load() {
		t.Fatal("下发前应为未同步（零值 false）")
	}
	h.PushAgentSettings(1) // 同步：Ask 阻塞至收到回执
	if !conn.settingsSynced.Load() {
		t.Fatal("成功回执后应置位 settingsSynced")
	}
}

// TestPushAgentSettingsFailureStaysUnsynced Ask 失败（此处用断连立即唤醒而非死等
// 30s 超时）后不得置位——watchdog 会继续每 2 分钟补推直至成功。
func TestPushAgentSettingsFailureStaysUnsynced(t *testing.T) {
	h := newTestHub(t)
	if err := h.DB.AutoMigrate(&models.Setting{}); err != nil {
		t.Fatal(err)
	}
	conn := newTestConn(1)
	h.mu.Lock()
	h.conns[1] = conn
	h.mu.Unlock()
	// 只消费不回执：请求发出后节点沉默
	go func() {
		for {
			select {
			case <-conn.Send:
			case <-conn.done:
				return
			}
		}
	}()

	done := make(chan struct{})
	go func() {
		h.PushAgentSettings(1)
		close(done)
	}()
	time.Sleep(50 * time.Millisecond) // 等待 Ask 注册 pending 并写入 Send
	h.unregister(conn)                // 断连立即唤醒等待中的 Ask

	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("断连后 Ask 未被唤醒，PushAgentSettings 仍在阻塞")
	}
	if conn.settingsSynced.Load() {
		t.Fatal("下发失败不应置位 settingsSynced")
	}
}
