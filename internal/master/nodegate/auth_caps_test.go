package nodegate

import (
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/acdc-awa/xpanel-node/pkg/protocol"
	"github.com/acdc-awa/xpanel/internal/models"
	"github.com/acdc-awa/xpanel/internal/pkg/util"
	"github.com/gin-gonic/gin"
	"github.com/gorilla/websocket"
)

// TestServeWSAuthOKAdvertisesTrafficAck 真实握手：auth_ok 必须声明 traffic_ack 能力。
//
// 这是节点侧「等落库回执才删批」的唯一开关。一旦漏发（或回退成裸 {ok:true}），节点会静默
// 降级为发完即删——数据不丢但失去至少一次投递，且没有任何报错能提示这件事。所以这里走
// ServeWS 的真实握手把线格式钉住，而不是断言构造出的结构体（那样测不到回归）。
func TestServeWSAuthOKAdvertisesTrafficAck(t *testing.T) {
	h := newTestHub(t)
	srv := models.Server{NodeID: "node-caps", Secret: util.HashSecret("sec-caps"), Name: "caps"}
	if err := h.DB.Create(&srv).Error; err != nil {
		t.Fatalf("建服务器记录: %v", err)
	}

	gin.SetMode(gin.TestMode)
	r := gin.New()
	r.GET("/node/ws", h.ServeWS)
	ts := httptest.NewServer(r)
	defer ts.Close()

	wsURL := "ws" + strings.TrimPrefix(ts.URL, "http") + "/node/ws?node_id=node-caps"
	ws, _, err := websocket.DefaultDialer.Dial(wsURL, nil)
	if err != nil {
		t.Fatalf("dial: %v", err)
	}
	defer ws.Close()

	auth, err := protocol.Encode(protocol.MsgAuth, "",
		protocol.AuthPayload{NodeID: "node-caps", Secret: "sec-caps"})
	if err != nil {
		t.Fatal(err)
	}
	if err := ws.WriteMessage(websocket.TextMessage, auth); err != nil {
		t.Fatalf("写 auth: %v", err)
	}
	_, data, err := ws.ReadMessage()
	if err != nil {
		t.Fatalf("读 auth_ok: %v", err)
	}
	msg, err := protocol.Decode(data)
	if err != nil {
		t.Fatal(err)
	}
	if msg.Type != protocol.MsgAuthOK {
		t.Fatalf("应回 auth_ok，实际 %s（payload=%s）", msg.Type, string(msg.Payload))
	}

	// 旧节点只看 ok：这个字段必须保留，否则旧节点判定认证失败
	var generic protocol.ResultPayload
	if err := msg.PayloadTo(&generic); err != nil {
		t.Fatalf("auth_ok 应按旧结构可解析（旧节点兼容）: %v", err)
	}
	if !generic.OK {
		t.Fatalf("auth_ok 必须带 ok=true，实际 payload=%s", string(msg.Payload))
	}

	var caps protocol.AuthOKPayload
	if err := msg.PayloadTo(&caps); err != nil {
		t.Fatal(err)
	}
	if !caps.HasCap(protocol.CapTrafficAck) {
		t.Fatalf("auth_ok 必须声明 %s（否则节点降级为发完即删），实际 payload=%s",
			protocol.CapTrafficAck, string(msg.Payload))
	}
}

// TestServeWSAuthBadRejectsWrongSecret 认证失败仍回 bad_auth（能力声明不得影响失败路径）。
func TestServeWSAuthBadRejectsWrongSecret(t *testing.T) {
	h := newTestHub(t)
	srv := models.Server{NodeID: "node-bad", Secret: util.HashSecret("right"), Name: "bad"}
	if err := h.DB.Create(&srv).Error; err != nil {
		t.Fatalf("建服务器记录: %v", err)
	}

	gin.SetMode(gin.TestMode)
	r := gin.New()
	r.GET("/node/ws", h.ServeWS)
	ts := httptest.NewServer(r)
	defer ts.Close()

	ws, _, err := websocket.DefaultDialer.Dial("ws"+strings.TrimPrefix(ts.URL, "http")+"/node/ws", nil)
	if err != nil {
		t.Fatalf("dial: %v", err)
	}
	defer ws.Close()

	auth, err := protocol.Encode(protocol.MsgAuth, "",
		protocol.AuthPayload{NodeID: "node-bad", Secret: "wrong"})
	if err != nil {
		t.Fatal(err)
	}
	if err := ws.WriteMessage(websocket.TextMessage, auth); err != nil {
		t.Fatalf("写 auth: %v", err)
	}
	_, data, err := ws.ReadMessage()
	if err != nil {
		t.Fatalf("读 bad_auth: %v", err)
	}
	msg, err := protocol.Decode(data)
	if err != nil {
		t.Fatal(err)
	}
	if msg.Type != protocol.MsgAuthBad {
		t.Fatalf("密钥错误应回 bad_auth，实际 %s", msg.Type)
	}
}
