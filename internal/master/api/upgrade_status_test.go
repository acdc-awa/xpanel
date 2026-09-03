package api

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"

	"github.com/acdc-awa/xpanel-node/pkg/protocol"
	"github.com/acdc-awa/xpanel/internal/master/nodegate"
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
