package contracts

import (
	"context"
	"errors"
	"sync"
	"time"
)

// DomainEvent 领域事件接口（Stage 5：事件与通知解耦）。
type DomainEvent interface {
	EventName() string
}

// EventPublisher 事件发布口——领域服务依赖的最小面，便于测试替身。
type EventPublisher interface {
	Publish(ctx context.Context, ev DomainEvent) error
}

// EventHandler 事件处理器；返回错误被聚合，不阻断其他订阅者。
type EventHandler func(ctx context.Context, ev DomainEvent) error

// EventBus 进程内同步事件总线：订阅者在 Publish 调用栈内顺序执行。
// 设计约束（验收）：事件必须在源事务提交后发布；handler 失败仅聚合返回，
// 由发布方决定记日志/忽略，不得回滚已提交的源事务。
type EventBus struct {
	mu       sync.RWMutex
	handlers map[string][]EventHandler
}

// NewEventBus 创建空事件总线。
func NewEventBus() *EventBus {
	return &EventBus{handlers: map[string][]EventHandler{}}
}

// 编译期接口断言。
var _ EventPublisher = (*EventBus)(nil)

// Subscribe 订阅指定事件名；nil handler 忽略。
func (b *EventBus) Subscribe(eventName string, h EventHandler) {
	if h == nil {
		return
	}
	b.mu.Lock()
	defer b.mu.Unlock()
	b.handlers[eventName] = append(b.handlers[eventName], h)
}

// Publish 同步顺序调用该事件的所有订阅者；无订阅者返回 nil。
// 单个 handler 的错误不阻断后续 handler，最终 errors.Join 聚合返回。
func (b *EventBus) Publish(ctx context.Context, ev DomainEvent) error {
	if ev == nil {
		return nil
	}
	b.mu.RLock()
	list := append([]EventHandler(nil), b.handlers[ev.EventName()]...)
	b.mu.RUnlock()
	var errs []error
	for _, h := range list {
		if err := h(ctx, ev); err != nil {
			errs = append(errs, err)
		}
	}
	return errors.Join(errs...)
}

// EventOrderPaid 订单支付成功事件名。
const EventOrderPaid = "billing.order_paid"

// OrderPaidEvent 订单支付成功事件（事务提交后发布；幂等复用旧订单不重复发布）。
type OrderPaidEvent struct {
	OrderID uint64
	OrderNo string
	UserID  uint64
	PlanID  uint64
	PaidAt  time.Time
}

// EventName 实现 DomainEvent。
func (OrderPaidEvent) EventName() string { return EventOrderPaid }
