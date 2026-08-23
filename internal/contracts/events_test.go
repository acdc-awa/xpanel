package contracts

import (
	"context"
	"errors"
	"testing"
)

func TestEventBus_PublishSubscribe(t *testing.T) {
	bus := NewEventBus()

	var calls []string
	bus.Subscribe(EventOrderPaid, func(_ context.Context, ev DomainEvent) error {
		paid, ok := ev.(OrderPaidEvent)
		if !ok {
			t.Fatalf("事件类型错误: %T", ev)
		}
		if paid.OrderNo != "NO-1" {
			t.Fatalf("OrderNo = %q", paid.OrderNo)
		}
		calls = append(calls, "h1")
		return nil
	})
	bus.Subscribe(EventOrderPaid, func(_ context.Context, _ DomainEvent) error {
		calls = append(calls, "h2")
		return nil
	})
	// 其他事件名的订阅者不应被触发
	bus.Subscribe("other.event", func(_ context.Context, _ DomainEvent) error {
		calls = append(calls, "other")
		return nil
	})
	bus.Subscribe(EventOrderPaid, nil) // nil handler 忽略

	err := bus.Publish(context.Background(), OrderPaidEvent{OrderID: 1, OrderNo: "NO-1", UserID: 2, PlanID: 3})
	if err != nil {
		t.Fatalf("Publish: %v", err)
	}
	if len(calls) != 2 || calls[0] != "h1" || calls[1] != "h2" {
		t.Fatalf("订阅者应按注册顺序同步执行且无串扰, got %v", calls)
	}

	// 无订阅者事件 → nil
	if err := bus.Publish(context.Background(), nil); err != nil {
		t.Fatalf("nil 事件应返回 nil, got %v", err)
	}
}

func TestEventBus_ErrorAggregation(t *testing.T) {
	bus := NewEventBus()
	e1 := errors.New("handler1 失败")
	bus.Subscribe(EventOrderPaid, func(context.Context, DomainEvent) error { return e1 })
	ran := false
	bus.Subscribe(EventOrderPaid, func(context.Context, DomainEvent) error {
		ran = true // 前序 handler 失败不阻断后续
		return nil
	})

	err := bus.Publish(context.Background(), OrderPaidEvent{OrderNo: "NO-2"})
	if !errors.Is(err, e1) {
		t.Fatalf("错误应聚合并可被 errors.Is 检出, got %v", err)
	}
	if !ran {
		t.Fatal("前序 handler 失败不应阻断后续订阅者")
	}
}
