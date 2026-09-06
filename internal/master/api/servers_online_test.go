package api

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strconv"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/glebarez/sqlite"
	"gorm.io/gorm"

	"github.com/acdc-awa/xpanel/internal/models"
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

