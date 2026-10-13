package api

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strconv"
	"testing"

	"github.com/gin-gonic/gin"

	"github.com/acdc-awa/xpanel/internal/models"
	"github.com/acdc-awa/xpanel/internal/pkg/certutil"
)

func TestAdminCACertLifecycle(t *testing.T) {
	deps, db := setupTestDeps(t)
	gin.SetMode(gin.TestMode)

	// 1. 生成新 CA
	genReq := generateCACertForm{
		Name:         "test-root-ca",
		CommonName:   "Test Root CA",
		Organization: "Test Org",
		Country:      "CN",
		KeyType:      "ecdsa_p256",
		ValidDays:    365,
		Remark:       "自动化测试 CA",
	}
	body, _ := json.Marshal(genReq)
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest(http.MethodPost, "/api/v1/admin/ca-certs/generate", bytes.NewReader(body))
	c.Request.Header.Set("Content-Type", "application/json")

	deps.AdminGenerateCACert(c)
	if w.Code != http.StatusOK {
		t.Fatalf("AdminGenerateCACert failed: %d, body=%s", w.Code, w.Body.String())
	}

	var caResp struct {
		Code int `json:"code"`
		Data struct {
			CACert caCertView `json:"ca_cert"`
		} `json:"data"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &caResp); err != nil {
		t.Fatalf("json parse: %v", err)
	}
	caID := caResp.Data.CACert.ID
	if caID == 0 || !caResp.Data.CACert.HasKey {
		t.Fatalf("CA ID 为 0 或 HasKey 为 false: %+v", caResp.Data.CACert)
	}

	// 2. 基于 CA 签发服务端证书
	issueReq := issueCertFromCAForm{
		Domain:       "relay.example.com",
		SANDNS:       []string{"relay.example.com", "*.relay.example.com"},
		SANIP:        []string{"127.0.0.1"},
		Organization: "Relay Node",
		KeyType:      "ecdsa_p256",
		ValidDays:    90,
		Remark:       "中转落地证书",
	}
	body, _ = json.Marshal(issueReq)
	w = httptest.NewRecorder()
	c, _ = gin.CreateTestContext(w)
	c.Params = gin.Params{{Key: "id", Value: strconv.FormatUint(caID, 10)}}
	c.Request = httptest.NewRequest(http.MethodPost, "/api/v1/admin/ca-certs/"+strconv.FormatUint(caID, 10)+"/issue", bytes.NewReader(body))
	c.Request.Header.Set("Content-Type", "application/json")

	deps.AdminIssueCertFromCA(c)
	if w.Code != http.StatusOK {
		t.Fatalf("AdminIssueCertFromCA failed: %d, body=%s", w.Code, w.Body.String())
	}

	var issueResp struct {
		Code int `json:"code"`
		Data struct {
			Cert certView `json:"cert"`
		} `json:"data"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &issueResp); err != nil {
		t.Fatalf("json parse issue: %v", err)
	}
	certID := issueResp.Data.Cert.ID
	if certID == 0 || issueResp.Data.Cert.CAID == nil || *issueResp.Data.Cert.CAID != caID {
		t.Fatalf("Cert CAID 不匹配: %+v", issueResp.Data.Cert)
	}
	if issueResp.Data.Cert.CAName != "test-root-ca" {
		t.Errorf("CAName want test-root-ca, got %s", issueResp.Data.Cert.CAName)
	}

	// 3. 校验 CA 列表计数
	w = httptest.NewRecorder()
	c, _ = gin.CreateTestContext(w)
	c.Request = httptest.NewRequest(http.MethodGet, "/api/v1/admin/ca-certs", nil)
	deps.AdminCACerts(c)

	var listResp struct {
		Code int `json:"code"`
		Data struct {
			Items []caCertView `json:"items"`
		} `json:"data"`
	}
	_ = json.Unmarshal(w.Body.Bytes(), &listResp)
	if len(listResp.Data.Items) != 1 || listResp.Data.Items[0].IssuedCount != 1 {
		t.Errorf("CA issued count want 1, got %+v", listResp.Data.Items)
	}

	// 4. 重签证书
	reissueBody, _ := json.Marshal(map[string]any{"valid_days": 180})
	w = httptest.NewRecorder()
	c, _ = gin.CreateTestContext(w)
	c.Params = gin.Params{{Key: "id", Value: strconv.FormatUint(certID, 10)}}
	c.Request = httptest.NewRequest(http.MethodPost, "/api/v1/admin/certs/"+strconv.FormatUint(certID, 10)+"/reissue", bytes.NewReader(reissueBody))
	c.Request.Header.Set("Content-Type", "application/json")

	deps.AdminReissueCert(c)
	if w.Code != http.StatusOK {
		t.Fatalf("AdminReissueCert failed: %d, body=%s", w.Code, w.Body.String())
	}

	// 5. 试图删除有签发记录的 CA（应被拒绝）
	w = httptest.NewRecorder()
	c, _ = gin.CreateTestContext(w)
	c.Params = gin.Params{{Key: "id", Value: strconv.FormatUint(caID, 10)}}
	c.Request = httptest.NewRequest(http.MethodDelete, "/api/v1/admin/ca-certs/"+strconv.FormatUint(caID, 10), nil)
	deps.AdminDeleteCACert(c)
	if w.Code == http.StatusOK {
		t.Fatalf("删除有签发记录的 CA 应被拒绝，但返回了 200")
	}

	// 6. 删除证书后再删除 CA（应成功）
	if err := db.Delete(&models.Cert{}, certID).Error; err != nil {
		t.Fatal(err)
	}
	w = httptest.NewRecorder()
	c, _ = gin.CreateTestContext(w)
	c.Params = gin.Params{{Key: "id", Value: strconv.FormatUint(caID, 10)}}
	c.Request = httptest.NewRequest(http.MethodDelete, "/api/v1/admin/ca-certs/"+strconv.FormatUint(caID, 10), nil)
	deps.AdminDeleteCACert(c)
	if w.Code != http.StatusOK {
		t.Fatalf("删除空闲 CA 失败: %d, body=%s", w.Code, w.Body.String())
	}
}

func TestAdminCreateCACert_PureVerifyMode(t *testing.T) {
	deps, _ := setupTestDeps(t)
	gin.SetMode(gin.TestMode)

	// 先在内存里生成一个纯 CA，仅导入其公钥
	caGen, err := certutil.GenerateCA(certutil.CAGenRequest{
		CommonName: "Third-party CA",
		KeyType:    "ecdsa_p256",
	})
	if err != nil {
		t.Fatal(err)
	}

	req := createCACertForm{
		Name:      "pure-verify-ca",
		CACertPEM: caGen.CertPEM,
		CAKeyPEM:  "", // 留空，纯公钥
		Remark:    "仅用于验签",
	}
	body, _ := json.Marshal(req)
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest(http.MethodPost, "/api/v1/admin/ca-certs", bytes.NewReader(body))
	c.Request.Header.Set("Content-Type", "application/json")

	deps.AdminCreateCACert(c)
	if w.Code != http.StatusOK {
		t.Fatalf("AdminCreateCACert failed: %d, body=%s", w.Code, w.Body.String())
	}

	var resp struct {
		Data struct {
			CACert caCertView `json:"ca_cert"`
		} `json:"data"`
	}
	_ = json.Unmarshal(w.Body.Bytes(), &resp)
	if resp.Data.CACert.HasKey {
		t.Errorf("纯公钥 CA 的 HasKey 应为 false")
	}

	// 尝试用纯公钥 CA 签发（应被拒绝）
	issueReq := issueCertFromCAForm{Domain: "fail.example.com"}
	issueBody, _ := json.Marshal(issueReq)
	w = httptest.NewRecorder()
	c, _ = gin.CreateTestContext(w)
	c.Params = gin.Params{{Key: "id", Value: strconv.FormatUint(resp.Data.CACert.ID, 10)}}
	c.Request = httptest.NewRequest(http.MethodPost, "/api/v1/admin/ca-certs/"+strconv.FormatUint(resp.Data.CACert.ID, 10)+"/issue", bytes.NewReader(issueBody))
	c.Request.Header.Set("Content-Type", "application/json")

	deps.AdminIssueCertFromCA(c)
	if w.Code == http.StatusOK {
		t.Fatalf("纯公钥 CA 签发证书应报错，但返回了 200")
	}
}
