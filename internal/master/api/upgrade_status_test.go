package api

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/glebarez/sqlite"
	"github.com/stretchr/testify/assert"
	"gorm.io/gorm"

	"github.com/acdc-awa/xpanel-node/pkg/protocol"
	"github.com/acdc-awa/xpanel/internal/master/nodegate"
	"github.com/acdc-awa/xpanel/internal/models"
)

func TestAdminGetServerUpgradeStatus(t *testing.T) {
	gin.SetMode(gin.TestMode)
	hub := nodegate.NewHub(nil, nil, nil)
	deps := &Deps{Hub: hub}

	r := gin.New()
	r.GET("/servers/:id/upgrade-status", deps.AdminGetServerUpgradeStatus)

	// 1. Initial status is nil
	{
		w := httptest.NewRecorder()
		req, _ := http.NewRequest(http.MethodGet, "/servers/1/upgrade-status", nil)
		r.ServeHTTP(w, req)
		assert.Equal(t, http.StatusOK, w.Code)

		var resp struct {
			Code int `json:"code"`
			Data struct {
				Status *protocol.UpgradeProgressPayload `json:"status"`
			} `json:"data"`
		}
		assert.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
		assert.Nil(t, resp.Data.Status)
	}

	// 2. Set upgrade status and query
	hub.SetUpgradeStatus(1, &protocol.UpgradeProgressPayload{
		Phase:   "downloading",
		Target:  "v0.2.0",
		Message: "正在下载...",
		TS:      time.Now().Unix(),
	})
	{
		w := httptest.NewRecorder()
		req, _ := http.NewRequest(http.MethodGet, "/servers/1/upgrade-status", nil)
		r.ServeHTTP(w, req)
		assert.Equal(t, http.StatusOK, w.Code)

		var resp struct {
			Code int `json:"code"`
			Data struct {
				Status *protocol.UpgradeProgressPayload `json:"status"`
			} `json:"data"`
		}
		assert.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
		assert.NotNil(t, resp.Data.Status)
		assert.Equal(t, "downloading", resp.Data.Status.Phase)
		assert.Equal(t, "v0.2.0", resp.Data.Status.Target)
		assert.Equal(t, "正在下载...", resp.Data.Status.Message)
	}
}

// TestAdminGetUpgradeStatuses 批量进度快照：全量与按 ids 过滤。
func TestAdminGetUpgradeStatuses(t *testing.T) {
	gin.SetMode(gin.TestMode)
	hub := nodegate.NewHub(nil, nil, nil)
	deps := &Deps{Hub: hub}
	hub.SetUpgradeStatus(1, &protocol.UpgradeProgressPayload{Phase: "downloading", Target: "v0.2.0", TS: time.Now().Unix()})
	hub.SetUpgradeStatus(2, &protocol.UpgradeProgressPayload{Phase: "success", Target: "v0.2.0", TS: time.Now().Unix()})

	r := gin.New()
	r.GET("/servers/upgrade-status", deps.AdminGetUpgradeStatuses)

	// 全量
	w := httptest.NewRecorder()
	r.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/servers/upgrade-status", nil))
	assert.Equal(t, http.StatusOK, w.Code)
	var resp struct {
		Data struct {
			Statuses map[string]*protocol.UpgradeProgressPayload `json:"statuses"`
		} `json:"data"`
	}
	assert.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
	assert.Len(t, resp.Data.Statuses, 2)
	assert.Equal(t, "downloading", resp.Data.Statuses["1"].Phase)

	// 按 ids 过滤
	var resp2 struct {
		Data struct {
			Statuses map[string]*protocol.UpgradeProgressPayload `json:"statuses"`
		} `json:"data"`
	}
	w2 := httptest.NewRecorder()
	r.ServeHTTP(w2, httptest.NewRequest(http.MethodGet, "/servers/upgrade-status?ids=2", nil))
	assert.NoError(t, json.Unmarshal(w2.Body.Bytes(), &resp2))
	assert.Len(t, resp2.Data.Statuses, 1)
	assert.Equal(t, "success", resp2.Data.Statuses["2"].Phase)
}

// TestAdminBatchUpgradeSkipsOffline 批量升级预检：不存在的 ID 与离线服务器直接跳过，
// 不进入派发队列（Hub 无真实连接，IsOnline 恒 false）。
func TestAdminBatchUpgradeSkipsOffline(t *testing.T) {
	gin.SetMode(gin.TestMode)
	db, err := gorm.Open(sqlite.Open("file:mem_batch_upgrade?mode=memory&cache=shared"), &gorm.Config{})
	assert.NoError(t, err)
	sqlDB, _ := db.DB()
	t.Cleanup(func() { _ = sqlDB.Close() })
	assert.NoError(t, models.AutoMigrate(db))
	mk := func(name, nodeID, ver string) uint64 {
		s := models.Server{Name: name, Host: "h", NodeID: nodeID, Secret: "sec", AgentVersion: ver}
		assert.NoError(t, db.Create(&s).Error)
		return s.ID
	}
	id1 := mk("s1", "n1", "")
	id2 := mk("s2", "n2", "v0.1.22")

	hub := nodegate.NewHub(nil, nil, nil)
	deps := &Deps{DB: db, Hub: hub}
	r := gin.New()
	r.POST("/servers/batch-upgrade", deps.AdminBatchUpgradeServers)

	body := `{"ids":[` + fmt.Sprint(id1) + `,` + fmt.Sprint(id2) + `,99999],"target":"v9.9.9"}`
	w := httptest.NewRecorder()
	r.ServeHTTP(w, httptest.NewRequest(http.MethodPost, "/servers/batch-upgrade", strings.NewReader(body)))
	assert.Equal(t, http.StatusOK, w.Code)

	var resp struct {
		Data struct {
			Target     string `json:"target"`
			Dispatched []struct {
				ID uint64 `json:"id"`
			} `json:"dispatched"`
			Skipped []struct {
				ID     uint64 `json:"id"`
				Reason string `json:"reason"`
			} `json:"skipped"`
		} `json:"data"`
	}
	assert.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
	assert.Empty(t, resp.Data.Dispatched)
	assert.Len(t, resp.Data.Skipped, 3)
	reasons := map[uint64]string{}
	for _, s := range resp.Data.Skipped {
		reasons[s.ID] = s.Reason
	}
	assert.Contains(t, reasons[id1], "离线")
	assert.Contains(t, reasons[id2], "离线")
	assert.Contains(t, reasons[99999], "不存在")
	// 跳过的节点不应写入升级进度
	assert.Empty(t, hub.GetUpgradeStatuses(nil))
}
