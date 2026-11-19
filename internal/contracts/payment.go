package contracts

import (
	"context"
	"net/http"
	"sort"
	"sync"

	"github.com/acdc-awa/xpanel/internal/models"
)

// PaymentFlowType 支付交互流程类型。
type PaymentFlowType string

const (
	PaymentFlowSync     PaymentFlowType = "sync"     // 同步即时结算（如余额、卡密）
	PaymentFlowRedirect PaymentFlowType = "redirect" // 跳转第三方收银台 URL
	PaymentFlowQRCode   PaymentFlowType = "qrcode"   // 展示扫码二维码并轮询状态
)

// PaymentFieldSchema 支付插件配置字段描述（供未来管理端动态渲染配置表单）。
type PaymentFieldSchema struct {
	Key         string `json:"key"`
	Label       string `json:"label"`
	Type        string `json:"type"` // text / secret / number / select / switch
	Required    bool   `json:"required"`
	Placeholder string `json:"placeholder,omitempty"`
	Description string `json:"description,omitempty"`
}

// PaymentChannelInfo 支付渠道公开元数据（前端充值弹窗与收银台共用）。
type PaymentChannelInfo struct {
	ID               string          `json:"id"`
	Name             string          `json:"name"`
	Description      string          `json:"description,omitempty"`
	FlowType         PaymentFlowType `json:"flow_type"`
	SupportsRecharge bool            `json:"supports_recharge"` // 是否支持账户充值
	SupportsPlanPay  bool            `json:"supports_plan_pay"` // 是否支持套餐直付
	RequiresAmount   bool            `json:"requires_amount"`   // 发起充值前是否需要用户指定金额（卡密为 false）
	MinAmountCents   int64           `json:"min_amount_cents,omitempty"`
	MaxAmountCents   int64           `json:"max_amount_cents,omitempty"`
	Enabled          bool            `json:"enabled"`
}

// PaymentInitiateRequest 发起支付请求上下文。
type PaymentInitiateRequest struct {
	Order     *models.Order
	UserID    uint64
	Code      string            // 卡密等同步凭证
	ReturnURL string            // 异步支付回跳地址
	NotifyURL string            // 异步支付回调地址
	Extra     map[string]string // 扩展参数
}

// PaymentInitiateResult 发起支付结果。
type PaymentInitiateResult struct {
	Paid    bool            `json:"paid"`               // true = 同步立即完成（如卡密/余额）
	Flow    PaymentFlowType `json:"flow"`               // sync / redirect / qrcode
	TradeNo string          `json:"trade_no,omitempty"` // 外部交易号
	PayURL  string          `json:"pay_url,omitempty"`  // redirect 跳转地址
	QRCode  string          `json:"qr_code,omitempty"`  // qrcode 内容（URL 或支付码）
	HTML    string          `json:"html,omitempty"`     // 表单自动提交 HTML（可选）
	Payload string          `json:"-"`                  // 落库保存的 JSON 载体
}

// PaymentCallbackResult 第三方支付异步回调验签结果。
type PaymentCallbackResult struct {
	OrderNo     string
	TradeNo     string
	AmountCents int64
	Paid        bool
	AckBody     string // 返回给第三方网关的成功应答体（如 "success"）
	AckStatus   int    // HTTP 状态码，默认 http.StatusOK
}

// PaymentProvider 标准支付插件契约接口。
// 后续接入第三方支付（易支付/支付宝/微信/Stripe/USDT 等）只需实现本接口并注册即可。
type PaymentProvider interface {
	Info() PaymentChannelInfo
	ConfigSchema() []PaymentFieldSchema
	Enabled() bool
	Initiate(ctx context.Context, req PaymentInitiateRequest) (*PaymentInitiateResult, error)
	VerifyCallback(ctx context.Context, r *http.Request) (*PaymentCallbackResult, error)
	QueryStatus(ctx context.Context, order *models.Order) (paid bool, tradeNo string, err error)
}

// PaymentRegistry 实例级支付插件注册表（挂在服务与 API 依赖上，避免进程级全局状态污染）。
type PaymentRegistry struct {
	mu        sync.RWMutex
	providers map[string]PaymentProvider
}

// NewPaymentRegistry 创建空的支付插件注册表。
func NewPaymentRegistry() *PaymentRegistry {
	return &PaymentRegistry{
		providers: make(map[string]PaymentProvider),
	}
}

// Register 注册或替换一个支付插件。
func (r *PaymentRegistry) Register(p PaymentProvider) {
	if r == nil || p == nil {
		return
	}
	info := p.Info()
	if info.ID == "" {
		return
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.providers == nil {
		r.providers = make(map[string]PaymentProvider)
	}
	r.providers[info.ID] = p
}

// Get 按渠道 ID 获取已注册的支付插件。
func (r *PaymentRegistry) Get(id string) (PaymentProvider, bool) {
	if r == nil {
		return nil, false
	}
	r.mu.RLock()
	defer r.mu.RUnlock()
	p, ok := r.providers[id]
	return p, ok
}

// List 列出全部已注册的支付插件（按 ID 排序保证输出稳定）。
func (r *PaymentRegistry) List() []PaymentProvider {
	if r == nil {
		return nil
	}
	r.mu.RLock()
	defer r.mu.RUnlock()
	ids := make([]string, 0, len(r.providers))
	for id := range r.providers {
		ids = append(ids, id)
	}
	sort.Strings(ids)
	out := make([]PaymentProvider, 0, len(ids))
	for _, id := range ids {
		out = append(out, r.providers[id])
	}
	return out
}
