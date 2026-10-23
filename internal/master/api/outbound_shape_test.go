package api

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"

	"github.com/acdc-awa/xpanel/internal/models"
)

func outboundReq(t *testing.T, r *gin.Engine, method string, serverID, outboundID uint64, body string) *httptest.ResponseRecorder {
	t.Helper()
	path := "/api/v1/admin/servers/" + strconv.FormatUint(serverID, 10) + "/outbounds"
	if outboundID > 0 {
		path += "/" + strconv.FormatUint(outboundID, 10)
	}
	req := httptest.NewRequest(method, path, strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	return w
}

func expectStatus(t *testing.T, w *httptest.ResponseRecorder, want int, label string) {
	t.Helper()
	if w.Code != want {
		t.Fatalf("%s: status=%d want %d body=%s", label, w.Code, want, w.Body.String())
	}
}

func TestOutboundShapeValidation(t *testing.T) {
	deps, r, srv := setupReservedOutboundsTest(t)

	other := models.Server{Name: "other", Host: "10.0.0.2", NodeID: "n2", Secret: "s2"}
	if err := deps.DB.Create(&other).Error; err != nil {
		t.Fatal(err)
	}
	mkInbound := func(tag, proto string, uuid string) models.Inbound {
		inb := models.Inbound{
			ServerID: other.ID, Tag: tag, Protocol: proto, Port: 44300,
			Type: models.InboundTypeUser, InternalUUID: uuid, Enabled: true,
		}
		if err := deps.DB.Create(&inb).Error; err != nil {
			t.Fatal(err)
		}
		return inb
	}
	vlessIn := mkInbound("land-vless", "vless", "uuid-vless")
	hyIn := mkInbound("land-hy2", "hysteria", "uuid-hy2")
	socksIn := mkInbound("land-socks", "socks", "")

	// ---- custom ----
	expectStatus(t, outboundReq(t, r, http.MethodPost, srv.ID, 0, `{"tag":"c1","protocol":"custom"}`), http.StatusBadRequest, "custom без json")
	expectStatus(t, outboundReq(t, r, http.MethodPost, srv.ID, 0,
		`{"tag":"c2","protocol":"custom","outbound_json":"{\"settings\":{}}"}`), http.StatusBadRequest, "custom без protocol")
	expectStatus(t, outboundReq(t, r, http.MethodPost, srv.ID, 0,
		`{"tag":"c3","protocol":"custom","outbound_json":"{\"protocol\":\"vless\",\"settings\":{\"vnext\":[]}}"}`), http.StatusOK, "custom valid")
	var c3 models.ServerOutbound
	deps.DB.Where("server_id = ? AND tag = ?", srv.ID, "c3").First(&c3)
	if c3.Protocol != "custom" || c3.SettingsJSON != "" || c3.StreamSettingsJSON != "" || c3.OutboundJSON == "" {
		t.Errorf("custom row not normalized: %+v", c3)
	}
	expectStatus(t, outboundReq(t, r, http.MethodPost, srv.ID, 0,
		fmt.Sprintf(`{"tag":"c4","protocol":"custom","outbound_json":"{\"protocol\":\"vless\"}","inbound_ref":%d}`, vlessIn.ID)),
		http.StatusBadRequest, "custom + ref")
	expectStatus(t, outboundReq(t, r, http.MethodPost, srv.ID, 0,
		`{"tag":"c5","protocol":"custom","outbound_json":"{\"protocol\":\"vless\"}","settings_json":"{}"}`),
		http.StatusBadRequest, "custom + settings")

	// ---- ref ----
	expectStatus(t, outboundReq(t, r, http.MethodPost, srv.ID, 0,
		fmt.Sprintf(`{"tag":"r1","protocol":"vless","inbound_ref":%d}`, vlessIn.ID)), http.StatusOK, "ref vless")
	var r1 models.ServerOutbound
	deps.DB.Where("server_id = ? AND tag = ?", srv.ID, "r1").First(&r1)
	if r1.InboundRef == nil || *r1.InboundRef != vlessIn.ID || r1.InternalUUID == "" {
		t.Errorf("ref row not wired: %+v", r1)
	}
	if r1.SettingsJSON != "" || r1.StreamSettingsJSON != "" || r1.OutboundJSON != "" {
		t.Errorf("ref row should have no payload: %+v", r1)
	}
	expectStatus(t, outboundReq(t, r, http.MethodPost, srv.ID, 0,
		fmt.Sprintf(`{"tag":"r2","protocol":"vless","inbound_ref":%d}`, hyIn.ID)), http.StatusOK, "ref hysteria")
	var r2 models.ServerOutbound
	deps.DB.Where("server_id = ? AND tag = ?", srv.ID, "r2").First(&r2)
	if r2.Protocol != "hysteria" {
		t.Errorf("ref row protocol should follow target, got %q", r2.Protocol)
	}
	expectStatus(t, outboundReq(t, r, http.MethodPost, srv.ID, 0,
		fmt.Sprintf(`{"tag":"r3","protocol":"vless","inbound_ref":%d}`, socksIn.ID)), http.StatusBadRequest, "ref socks")
	expectStatus(t, outboundReq(t, r, http.MethodPost, srv.ID, 0,
		fmt.Sprintf(`{"tag":"r4","protocol":"vless","inbound_ref":%d,"settings_json":"{\"vnext\":[]}"}`, vlessIn.ID)),
		http.StatusBadRequest, "ref + settings")

	// ---- 基础出口 / 草稿 / 旧表单 ----
	expectStatus(t, outboundReq(t, r, http.MethodPost, srv.ID, 0,
		`{"tag":"b1","protocol":"freedom","outbound_json":"{\"protocol\":\"vless\"}"}`), http.StatusBadRequest, "freedom + json")
	expectStatus(t, outboundReq(t, r, http.MethodPost, srv.ID, 0,
		`{"tag":"b2","protocol":"freedom","settings_json":"{}","stream_settings_json":"{\"network\":\"tcp\"}"}`),
		http.StatusBadRequest, "freedom + stream")
	expectStatus(t, outboundReq(t, r, http.MethodPost, srv.ID, 0,
		`{"tag":"l1","protocol":"vless","settings_json":"{\"vnext\":[{\"address\":\"1.2.3.4\",\"port\":443,\"users\":[{\"id\":\"u\"}]}]}"}`),
		http.StatusBadRequest, "legacy proxy form")
	expectStatus(t, outboundReq(t, r, http.MethodPost, srv.ID, 0,
		`{"tag":"d1","protocol":"vless"}`), http.StatusOK, "draft ref")
	var d1 models.ServerOutbound
	deps.DB.Where("server_id = ? AND tag = ?", srv.ID, "d1").First(&d1)
	if d1.InboundRef != nil || d1.OutboundJSON != "" || d1.SettingsJSON != "" {
		t.Errorf("draft should be empty: %+v", d1)
	}

	// ---- update：解绑 / 切换 custom ----
	expectStatus(t, outboundReq(t, r, http.MethodPut, srv.ID, r1.ID, `{"inbound_ref":0}`), http.StatusOK, "unbind")
	var r1After models.ServerOutbound
	deps.DB.First(&r1After, r1.ID)
	if r1After.InboundRef != nil {
		t.Error("unbind should clear inbound_ref")
	}
	expectStatus(t, outboundReq(t, r, http.MethodPut, srv.ID, r2.ID,
		`{"protocol":"custom","outbound_json":"{\"protocol\":\"hysteria\",\"settings\":{}}","settings_json":"","stream_settings_json":"","inbound_ref":0}`),
		http.StatusOK, "switch to custom")
	var r2After models.ServerOutbound
	deps.DB.First(&r2After, r2.ID)
	if r2After.InboundRef != nil || r2After.Protocol != "custom" || r2After.OutboundJSON == "" {
		t.Errorf("switch to custom failed: %+v", r2After)
	}

	// ---- 只改 tag 的旧行不应被形态校验拦截 ----
	legacy := models.ServerOutbound{
		ServerID: srv.ID, Tag: "legacy-row", Protocol: "vless", Enabled: true,
		SettingsJSON: `{"vnext":[{"address":"1.2.3.4","port":443,"users":[{"id":"u"}]}]}`,
	}
	if err := deps.DB.Create(&legacy).Error; err != nil {
		t.Fatal(err)
	}
	expectStatus(t, outboundReq(t, r, http.MethodPut, srv.ID, legacy.ID, `{"tag":"legacy-renamed"}`), http.StatusOK, "tag-only legacy update")
	var legacyAfter models.ServerOutbound
	deps.DB.First(&legacyAfter, legacy.ID)
	if legacyAfter.Tag != "legacy-renamed" || legacyAfter.SettingsJSON == "" {
		t.Errorf("tag-only update should not touch payload: %+v", legacyAfter)
	}
}
