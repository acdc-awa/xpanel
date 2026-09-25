package api

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strconv"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/glebarez/sqlite"
	"gorm.io/gorm"

	"github.com/acdc-awa/xpanel-node/pkg/protocol"

	"github.com/acdc-awa/xpanel/internal/models"
	"github.com/acdc-awa/xpanel/internal/master/nodegate"
)

// TestAdminServerOnlineIPs_MergeUserAcrossInbounds 在线面板按用户合并去重（2026-09-06）：
// 统计键按入站区分后同一用户每入站一个 email 条目，合并为一行（IP 并集去重）、
// email 回填真实邮箱；旧格式统计键同样归类为面板用户；relay/自定义 email 各保留一行；
// online_users = 去重后的条目数。
func TestAdminServerOnlineIPs_MergeUserAcrossInbounds(t *testing.T) {
	gin.SetMode(gin.TestMode)
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	if err := db.AutoMigrate(&models.Server{}, &models.User{}); err != nil {
		t.Fatalf("migrate: %v", err)
	}

	user := models.User{Username: "alice", Email: "Alice@T.com", UUID: "11111111-1111-1111-1111-111111111111", PasswordHash: "h", SubscribeToken: "tok", Status: models.StatusActive}
	if err := db.Create(&user).Error; err != nil {
		t.Fatal(err)
	}
	raw, _ := json.Marshal([]map[string]any{
		{"email": "u" + strconv.FormatUint(user.ID, 10) + ".i7@panel.local", "ips": []string{"1.1.1.1"}},
		{"email": "u" + strconv.FormatUint(user.ID, 10) + ".i9@panel.local", "ips": []string{"1.1.1.1", "2.2.2.2"}},
		{"email": "user-" + strconv.FormatUint(user.ID, 10) + "@panel.local", "ips": []string{"3.3.3.3"}},
		{"email": "relay-in-relay@panel.local", "ips": []string{"4.4.4.4"}},
		{"email": "custom@x.com", "ips": []string{"5.5.5.5"}},
	})
	srv := models.Server{Name: "node1", Host: "1.2.3.4", OnlineIPs: string(raw)}
	if err := db.Create(&srv).Error; err != nil {
		t.Fatal(err)
	}

	deps := &Deps{DB: db}
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest(http.MethodGet, "/", nil)
	c.Params = gin.Params{{Key: "id", Value: strconv.FormatUint(srv.ID, 10)}}
	deps.AdminServerOnlineIPs(c)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", w.Code)
	}
	var resp struct {
		Code int `json:"code"`
		Data struct {
			OnlineUsers int `json:"online_users"`
			Users       []struct {
				Email  string   `json:"email"`
				Kind   string   `json:"kind"`
				Name   string   `json:"name"`
				UserID uint64   `json:"user_id"`
				IPs    []string `json:"ips"`
			} `json:"users"`
		} `json:"data"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}

	if resp.Data.OnlineUsers != 3 {
		t.Fatalf("online_users = %d, want 3（用户合并 1 + relay 1 + 自定义 1）", resp.Data.OnlineUsers)
	}
	var merged, relay, other *int
	for i := range resp.Data.Users {
		switch resp.Data.Users[i].Kind {
		case "user":
			merged = &i
		case "relay":
			relay = &i
		case "other":
			other = &i
		}
	}
	if merged == nil {
		t.Fatal("缺 user 条目")
	}
	u := resp.Data.Users[*merged]
	if u.UserID != user.ID || u.Name != "alice" {
		t.Fatalf("user 条目 = %+v, want user_id=%d name=alice", u, user.ID)
	}
	if u.Email != "Alice@T.com" {
		t.Fatalf("user email = %q, want 回填真实邮箱 Alice@T.com", u.Email)
	}
	if len(u.IPs) != 3 || u.IPs[0] != "1.1.1.1" || u.IPs[1] != "2.2.2.2" || u.IPs[2] != "3.3.3.3" {
		t.Fatalf("user IPs = %v, want 三入口并集去重 [1.1.1.1 2.2.2.2 3.3.3.3]", u.IPs)
	}
	if relay == nil || other == nil {
		t.Fatalf("缺 relay/other 条目: %+v", resp.Data.Users)
	}
	if resp.Data.Users[*relay].Email != "relay-in-relay@panel.local" || resp.Data.Users[*other].Email != "custom@x.com" {
		t.Fatalf("relay/other 条目错: %+v", resp.Data.Users)
	}
}

// TestAdminServerOnlineIPs_MemoryFirstAndOfflineZero 数据源统一（2026-09-25）：
// 在线节点读网关内存快照（与 dashboard 人数同源同帧，DB 列旧值不得漏出）；
// 离线节点读归零后的内存快照（不给死节点展示残影，同样不得回退 DB）；
// 主控无快照（如刚重启）时仅对已回连的在线节点回退 servers.online_ips 列，
// 离线节点一律返回空（重启前的冻结名单就是残影，不能展示）。
func TestAdminServerOnlineIPs_MemoryFirstAndOfflineZero(t *testing.T) {
	gin.SetMode(gin.TestMode)
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	if err := db.AutoMigrate(&models.Server{}, &models.User{}); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	// DB 列放一份"残影"数据：在线时应被内存覆盖、离线时应被归零遮蔽
	stale, _ := json.Marshal([]protocol.OnlineUserIPs{
		{Email: "ghost@panel.local", IPs: []string{"9.9.9.9"}},
	})
	srv := models.Server{Name: "node1", Host: "1.2.3.4", OnlineIPs: string(stale)}
	if err := db.Create(&srv).Error; err != nil {
		t.Fatal(err)
	}

	memory := []protocol.OnlineUserIPs{{Email: "u1.i1@panel.local", IPs: []string{"1.1.1.1"}}}
	h := nodegate.NewHub(db, nil, nil)
	h.SetOnlineForTest(srv.ID, time.Now())
	h.SetMetricsForTest(srv.ID, &nodegate.NodeMetricsSnapshot{
		ServerID:    srv.ID,
		OnlineUsers: 1,
		OnlineIPs:   memory,
		XrayRunning: true,
		ReportedAt:  time.Now(),
	})

	fetch := func(hub *nodegate.Hub) (int, []struct {
		Email string `json:"email"`
		IPs   []string
	}) {
		w := httptest.NewRecorder()
		c, _ := gin.CreateTestContext(w)
		c.Request = httptest.NewRequest(http.MethodGet, "/", nil)
		c.Params = gin.Params{{Key: "id", Value: strconv.FormatUint(srv.ID, 10)}}
		(&Deps{DB: db, Hub: hub}).AdminServerOnlineIPs(c)
		if w.Code != http.StatusOK {
			t.Fatalf("status = %d, want 200", w.Code)
		}
		var resp struct {
			Code int `json:"code"`
			Data struct {
				OnlineUsers int `json:"online_users"`
				Users       []struct {
					Email string `json:"email"`
					IPs   []string
				} `json:"users"`
			} `json:"data"`
		}
		if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
			t.Fatalf("unmarshal: %v", err)
		}
		return resp.Data.OnlineUsers, resp.Data.Users
	}

	// 在线：内存权威，DB 残影不漏出
	if n, users := fetch(h); n != 1 || len(users) != 1 || users[0].Email != "u1.i1@panel.local" || users[0].IPs[0] != "1.1.1.1" {
		t.Fatalf("在线应读内存快照, got n=%d users=%+v", n, users)
	}

	// 离线：内存归零遮蔽 DB 残影
	h.SetOnlineForTest(srv.ID, time.Now().Add(-2*nodegate.HeartbeatTimeout))
	if n, users := fetch(h); n != 0 || len(users) != 0 {
		t.Fatalf("离线应归零不回退 DB, got n=%d users=%+v", n, users)
	}

	// 主控重启（无快照）+ 节点已回连在线：回退 DB 列（最近一次节流落库值）
	h2 := nodegate.NewHub(db, nil, nil)
	h2.SetOnlineForTest(srv.ID, time.Now())
	if n, users := fetch(h2); n != 1 || len(users) != 1 || users[0].Email != "ghost@panel.local" {
		t.Fatalf("重启后节点在线应回退 DB 列, got n=%d users=%+v", n, users)
	}

	// 主控重启（无快照）+ 节点未回连（离线）：返回空，不漏出重启前的冻结名单
	h3 := nodegate.NewHub(db, nil, nil)
	if n, users := fetch(h3); n != 0 || len(users) != 0 {
		t.Fatalf("重启后节点离线应返回空不回退 DB, got n=%d users=%+v", n, users)
	}
}

