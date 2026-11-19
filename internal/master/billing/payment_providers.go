package billing

import (
	"context"
	"errors"
	"net/http"

	"github.com/acdc-awa/xpanel/internal/contracts"
	"github.com/acdc-awa/xpanel/internal/models"
)

// RegisterBuiltinPaymentProviders 注册内置支付插件（余额支付与卡密兑换）到指定实例级注册表。
func RegisterBuiltinPaymentProviders(reg *contracts.PaymentRegistry, orders contracts.OrderService, giftCards contracts.GiftCardService) {
	if reg == nil {
		return
	}
	if orders != nil {
		reg.Register(&BalanceProvider{orders: orders})
	}
	if giftCards != nil {
		reg.Register(&GiftCardProvider{giftCards: giftCards})
	}
}

// BalanceProvider 账户余额支付插件（仅用于套餐订单）。
type BalanceProvider struct {
	orders contracts.OrderService
}

func (p *BalanceProvider) Info() contracts.PaymentChannelInfo {
	return contracts.PaymentChannelInfo{
		ID:               models.PaymentMethodBalance,
		Name:             "账户余额",
		Description:      "使用当前账户可用余额即时支付",
		FlowType:         contracts.PaymentFlowSync,
		SupportsRecharge: false,
		SupportsPlanPay:  true,
		RequiresAmount:   false,
		Enabled:          true,
	}
}

func (p *BalanceProvider) ConfigSchema() []contracts.PaymentFieldSchema { return nil }
func (p *BalanceProvider) Enabled() bool                              { return true }

func (p *BalanceProvider) Initiate(_ context.Context, req contracts.PaymentInitiateRequest) (*contracts.PaymentInitiateResult, error) {
	if req.Order == nil || req.Order.OrderType == models.OrderTypeRecharge {
		return nil, errors.New("充值订单不支持余额支付")
	}
	o, err := p.orders.PayOrderWithBalance(req.UserID, req.Order.OrderNo)
	if err != nil {
		return nil, err
	}
	return &contracts.PaymentInitiateResult{
		Paid:    o.Status == models.OrderPaid,
		Flow:    contracts.PaymentFlowSync,
		TradeNo: o.TradeNo,
	}, nil
}

func (p *BalanceProvider) VerifyCallback(_ context.Context, _ *http.Request) (*contracts.PaymentCallbackResult, error) {
	return nil, errors.New("余额支付不支持异步回调")
}

func (p *BalanceProvider) QueryStatus(_ context.Context, order *models.Order) (bool, string, error) {
	if order == nil {
		return false, "", ErrOrderNotFound
	}
	return order.Status == models.OrderPaid, order.TradeNo, nil
}

// GiftCardProvider 礼品卡/卡密兑换插件（同步原子入账并生成已支付充值订单）。
type GiftCardProvider struct {
	giftCards contracts.GiftCardService
}

func (p *GiftCardProvider) Info() contracts.PaymentChannelInfo {
	return contracts.PaymentChannelInfo{
		ID:               models.PaymentMethodGiftCard,
		Name:             "卡密兑换",
		Description:      "输入充值卡密，核销后按面值即时计入账户余额",
		FlowType:         contracts.PaymentFlowSync,
		SupportsRecharge: true,
		SupportsPlanPay:  true,
		RequiresAmount:   false,
		Enabled:          true,
	}
}

func (p *GiftCardProvider) ConfigSchema() []contracts.PaymentFieldSchema { return nil }
func (p *GiftCardProvider) Enabled() bool                              { return true }

func (p *GiftCardProvider) Initiate(_ context.Context, req contracts.PaymentInitiateRequest) (*contracts.PaymentInitiateResult, error) {
	if req.Order != nil && req.Order.OrderType == models.OrderTypeRecharge {
		return nil, errors.New("卡密仅支持通过充值入口直接兑换，不能用于支付已创建的待支付充值订单")
	}
	if req.Code == "" {
		return nil, errors.New("请输入卡密")
	}
	_, order, _, err := p.giftCards.RedeemWithOrder(req.UserID, req.Code)
	if err != nil {
		return nil, err
	}
	return &contracts.PaymentInitiateResult{
		Paid:    true,
		Flow:    contracts.PaymentFlowSync,
		TradeNo: order.TradeNo,
	}, nil
}

func (p *GiftCardProvider) VerifyCallback(_ context.Context, _ *http.Request) (*contracts.PaymentCallbackResult, error) {
	return nil, errors.New("卡密兑换不支持异步回调")
}

func (p *GiftCardProvider) QueryStatus(_ context.Context, order *models.Order) (bool, string, error) {
	if order == nil {
		return false, "", ErrOrderNotFound
	}
	return order.Status == models.OrderPaid, order.TradeNo, nil
}
