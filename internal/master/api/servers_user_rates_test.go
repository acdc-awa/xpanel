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

	"github.com/acdc-awa/xpanel/internal/master/nodegate"
	"github.com/acdc-awa/xpanel/internal/models"
)

// TestAdminServerUserRates_OldAgent 旧 agent（从未发过快车道帧）必须是 supported=false，
// 前端据此隐藏速率列与入站筛选——不能因为「列表为空」就隐藏，无人在线时同样为空。
func TestAdminServerUserRates_OldAgent(t *testing.T) {
	gin.SetMode(gin.TestMode)
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	if err != nil {
		t.Fatal(err)
	}
	if err := db.AutoMigrate(&models.Server{}); err != nil {
		t.Fatal(err)
	}
	srv := models.Server{Name: "n", Host: "1.1.1.1"}
	if err := db.Create(&srv).Error; err != nil {
		t.Fatal(err)
	}

	deps := &Deps{DB: db, Hub: nodegate.NewHub(db, nil, nil)}
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest(http.MethodGet, "/", nil)
	c.Params = gin.Params{{Key: "id", Value: strconv.FormatUint(srv.ID, 10)}}
	deps.AdminServerUserRates(c)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d", w.Code)
	}
	var body struct {
		Code int `json:"code"`
		Data struct {
			Supported bool             `json:"supported"`
			SampledAt int64            `json:"sampled_at"`
			Inbounds  []map[string]any `json:"inbounds"`
			Rates     []map[string]any `json:"rates"`
		} `json:"data"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &body); err != nil {
		t.Fatal(err)
	}
	if body.Code != 0 || body.Data.Supported || len(body.Data.Rates) != 0 {
		t.Fatalf("旧 agent 应为 supported=false 且无速率，实际 %s", w.Body.String())
	}
}

// TestAdminServerUserRates_ResolveNames 快车道速率行：统计键反解出用户与入站，
// 回填真实邮箱与入站名称；同一用户多入站各占一行（合并交给前端，接口保留原始维度）。
func TestAdminServerUserRates_ResolveNames(t *testing.T) {
	gin.SetMode(gin.TestMode)
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	if err != nil {
		t.Fatal(err)
	}
	if err := db.AutoMigrate(&models.Server{}, &models.User{}, &models.Inbound{}); err != nil {
		t.Fatal(err)
	}
	user := models.User{Email: "alice@t.com", UUID: "u-1", PasswordHash: "h", SubscribeToken: "tok", Status: models.StatusActive}
	if err := db.Create(&user).Error; err != nil {
		t.Fatal(err)
	}
	hk := models.Inbound{ServerID: 1, Tag: "vless-hk", Remark: "香港入口", Port: 443}
	us := models.Inbound{ServerID: 1, Tag: "vless-us", Remark: "美国入口", Port: 8443}
	if err := db.Create(&hk).Error; err != nil {
		t.Fatal(err)
	}
	if err := db.Create(&us).Error; err != nil {
		t.Fatal(err)
	}
	srv := models.Server{Name: "n", Host: "1.1.1.1"}
	if err := db.Create(&srv).Error; err != nil {
		t.Fatal(err)
	}

	uid := strconv.FormatUint(user.ID, 10)
	hub := nodegate.NewHub(db, nil, nil)
	hub.SetOnlineForTest(srv.ID, time.Now()) // 离线节点的速率会被 getter 归零，先标在线
	hub.SetMetricsForTest(srv.ID, &nodegate.NodeMetricsSnapshot{
		ServerID:  srv.ID,
		MetricsAt: time.Now(),
		UserRates: []protocol.UserRatePayload{
			{Email: "u" + uid + ".i" + strconv.FormatUint(hk.ID, 10) + "@panel.local", Up: 100, Down: 200},
			{Email: "u" + uid + ".i" + strconv.FormatUint(us.ID, 10) + "@panel.local", Up: 10, Down: 20},
			{Email: "user-" + uid + "@panel.local", Up: 1, Down: 2}, // 旧统计键：反解不出入站
		},
	})

	deps := &Deps{DB: db, Hub: hub}
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest(http.MethodGet, "/", nil)
	c.Params = gin.Params{{Key: "id", Value: strconv.FormatUint(srv.ID, 10)}}
	deps.AdminServerUserRates(c)

	var body struct {
		Data struct {
			Supported bool  `json:"supported"`
			SampledAt int64 `json:"sampled_at"`
			Inbounds  []struct {
				ID   uint64 `json:"id"`
				Tag  string `json:"tag"`
				Name string `json:"name"`
			} `json:"inbounds"`
			Rates []struct {
				Email       string  `json:"email"`
				UserID      uint64  `json:"user_id"`
				InboundID   uint64  `json:"inbound_id"`
				InboundName string  `json:"inbound_name"`
				Up          float64 `json:"up"`
				Down        float64 `json:"down"`
			} `json:"rates"`
		} `json:"data"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &body); err != nil {
		t.Fatal(err)
	}
	if !body.Data.Supported || body.Data.SampledAt == 0 {
		t.Fatalf("快车道数据应 supported=true 且带采样时刻: %s", w.Body.String())
	}
	if len(body.Data.Rates) != 3 {
		t.Fatalf("速率行数 = %d, want 3（同一用户多入站各一行）", len(body.Data.Rates))
	}
	var hkRate, legacy bool
	for _, r := range body.Data.Rates {
		if r.Email != "alice@t.com" || r.UserID != user.ID {
			t.Fatalf("邮箱/用户未回填: %+v", r)
		}
		switch {
		case r.InboundID == hk.ID && r.InboundName == "香港入口" && r.Up == 100 && r.Down == 200:
			hkRate = true
		case r.InboundID == 0:
			legacy = true
		}
	}
	if !hkRate || !legacy {
		t.Fatalf("入站名称/旧统计键处理不正确: %s", w.Body.String())
	}
	if len(body.Data.Inbounds) != 2 {
		t.Fatalf("入站筛选源 = %d 项, want 2", len(body.Data.Inbounds))
	}
}
