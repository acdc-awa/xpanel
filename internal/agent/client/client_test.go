package client

import (
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/pem"
	"math/big"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/acdc/xray-panel/internal/agent/accounts"
	"github.com/acdc/xray-panel/internal/pkg/protocol"
)

func testCert(t *testing.T) (certPEM, keyPEM string) {
	t.Helper()
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	tmpl := &x509.Certificate{
		SerialNumber: big.NewInt(1),
		Subject:      pkix.Name{CommonName: "test.local"},
		NotBefore:    time.Now().Add(-time.Hour),
		NotAfter:     time.Now().Add(time.Hour),
	}
	der, err := x509.CreateCertificate(rand.Reader, tmpl, tmpl, &key.PublicKey, key)
	if err != nil {
		t.Fatal(err)
	}
	keyDER, _ := x509.MarshalECPrivateKey(key)
	return string(pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: der})),
		string(pem.EncodeToMemory(&pem.Block{Type: "EC PRIVATE KEY", Bytes: keyDER}))
}

func mustMsg(t *testing.T, typ string, payload any) *protocol.Message {
	t.Helper()
	raw, err := protocol.Encode(typ, "req-1", payload)
	if err != nil {
		t.Fatal(err)
	}
	m, err := protocol.Decode(raw)
	if err != nil {
		t.Fatal(err)
	}
	return m
}

func setupClient(t *testing.T) *Client {
	t.Helper()
	return &Client{
		Accounts: accounts.New(filepath.Join(t.TempDir(), "internal_accounts.json")),
		CertsDir: filepath.Join(t.TempDir(), "certs"),
	}
}

func TestDispatchSetupInternalAccount(t *testing.T) {
	c := setupClient(t)
	res := c.dispatch(mustMsg(t, protocol.MsgSetupInternalAccount, protocol.SetupInternalAccountPayload{Tag: "relay-a"}))
	if res == nil || !res.OK {
		t.Fatalf("setup 应成功: %+v", res)
	}
	data, ok := res.Data.(protocol.SetupInternalResult)
	if !ok {
		t.Fatalf("回执 Data 类型不符: %T", res.Data)
	}
	uuid := data.UUID
	if uuid == "" || data.Tag != "relay-a" {
		t.Fatalf("回执不符: %+v", data)
	}
	// 幂等：再次 setup 返回同一 UUID（不重新生成）
	res2 := c.dispatch(mustMsg(t, protocol.MsgSetupInternalAccount, protocol.SetupInternalAccountPayload{Tag: "relay-a"}))
	uuid2 := res2.Data.(protocol.SetupInternalResult).UUID
	if uuid2 != uuid {
		t.Errorf("setup 应幂等复用: %s vs %s", uuid, uuid2)
	}
	// 持久化验证
	m, _ := c.Accounts.Load()
	if m["relay-a"] != uuid {
		t.Errorf("持久化不符: %v", m)
	}
}

func TestDispatchRotateInternalAccount(t *testing.T) {
	c := setupClient(t)
	res := c.dispatch(mustMsg(t, protocol.MsgSetupInternalAccount, protocol.SetupInternalAccountPayload{Tag: "relay-a"}))
	oldUUID := res.Data.(protocol.SetupInternalResult).UUID

	res2 := c.dispatch(mustMsg(t, protocol.MsgRotateInternalAccount, protocol.SetupInternalAccountPayload{Tag: "relay-a"}))
	if !res2.OK {
		t.Fatalf("rotate 应成功: %+v", res2)
	}
	newUUID := res2.Data.(protocol.SetupInternalResult).UUID
	if newUUID == "" || newUUID == oldUUID {
		t.Errorf("rotate 应生成新 UUID: old=%s new=%s", oldUUID, newUUID)
	}
	m, _ := c.Accounts.Load()
	if m["relay-a"] != newUUID {
		t.Errorf("rotate 后持久化不符: %v", m)
	}
}

func TestDispatchSetupErrors(t *testing.T) {
	c := setupClient(t)
	if res := c.dispatch(mustMsg(t, protocol.MsgSetupInternalAccount, protocol.SetupInternalAccountPayload{Tag: ""})); res == nil || res.OK {
		t.Error("空 tag 应失败")
	}
	// Accounts 未初始化
	c2 := &Client{}
	if res := c2.dispatch(mustMsg(t, protocol.MsgSetupInternalAccount, protocol.SetupInternalAccountPayload{Tag: "a"})); res == nil || res.OK {
		t.Error("Accounts 为 nil 应失败")
	}
}

func TestDispatchPushCert(t *testing.T) {
	c := setupClient(t)
	certPEM, keyPEM := testCert(t)

	res := c.dispatch(mustMsg(t, protocol.MsgPushCert, protocol.PushCertPayload{Domain: "test.local", CertPEM: certPEM, KeyPEM: keyPEM}))
	if res == nil || !res.OK {
		t.Fatalf("push_cert 应成功: %+v", res)
	}
	for _, p := range []string{"fullchain.pem", "key.pem"} {
		f := filepath.Join(c.CertsDir, "test.local", p)
		if _, err := os.Stat(f); err != nil {
			t.Fatalf("缺少 %s: %v", f, err)
		}
	}
	if runtime.GOOS != "windows" {
		info, _ := os.Stat(filepath.Join(c.CertsDir, "test.local", "key.pem"))
		if info.Mode().Perm() != 0o600 {
			t.Errorf("key.pem 权限 = %o, want 600", info.Mode().Perm())
		}
	}
}

func TestDispatchPushCertRejectsBadPair(t *testing.T) {
	c := setupClient(t)
	certPEM, _ := testCert(t)
	_, keyPEM2 := testCert(t) // 不匹配的私钥
	res := c.dispatch(mustMsg(t, protocol.MsgPushCert, protocol.PushCertPayload{Domain: "test.local", CertPEM: certPEM, KeyPEM: keyPEM2}))
	if res == nil || res.OK {
		t.Fatalf("证书/私钥不匹配应失败: %+v", res)
	}
	if !strings.Contains(res.Error, "不匹配") {
		t.Errorf("错误信息应说明不匹配: %s", res.Error)
	}
	// 目录不应被创建
	if _, err := os.Stat(filepath.Join(c.CertsDir, "test.local")); !os.IsNotExist(err) {
		t.Error("失败时不应落盘")
	}
}

func TestDispatchPushCertRejectsTraversal(t *testing.T) {
	c := setupClient(t)
	certPEM, keyPEM := testCert(t)
	res := c.dispatch(mustMsg(t, protocol.MsgPushCert, protocol.PushCertPayload{Domain: "../evil", CertPEM: certPEM, KeyPEM: keyPEM}))
	if res == nil || res.OK {
		t.Fatalf("路径穿越 domain 应失败: %+v", res)
	}
}

func TestDispatchUnknownType(t *testing.T) {
	c := setupClient(t)
	if res := c.dispatch(mustMsg(t, "unknown_type", map[string]any{})); res != nil {
		t.Errorf("未知类型应返回 nil: %+v", res)
	}
}
