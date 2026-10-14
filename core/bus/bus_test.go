package bus

import (
	"context"
	"errors"
	"strings"
	"sync"
	"testing"
)

func TestPublishReachesAllSubscribersInOrder(t *testing.T) {
	b := New()
	var got []string

	b.Subscribe(TopicPublicIPChanged, func(context.Context, any) error {
		got = append(got, "first")
		return nil
	})
	b.Subscribe(TopicPublicIPChanged, func(context.Context, any) error {
		got = append(got, "second")
		return nil
	})

	if err := b.Publish(context.Background(), TopicPublicIPChanged,
		PublicIPChanged{IP: "1.2.3.4"}); err != nil {
		t.Fatal(err)
	}
	if strings.Join(got, ",") != "first,second" {
		t.Fatalf("投递顺序不对：%v", got)
	}
	if n := b.SubscriberCount(TopicPublicIPChanged); n != 2 {
		t.Fatalf("订阅者数应为 2，实际 %d", n)
	}
}

func TestUnsubscribeIsIdempotent(t *testing.T) {
	b := New()
	hit := 0
	cancel := b.Subscribe(TopicServiceDeleted, func(context.Context, any) error {
		hit++
		return nil
	})

	cancel()
	cancel() // 重复取消不应 panic
	if err := b.Publish(context.Background(), TopicServiceDeleted, ServiceDeleted{}); err != nil {
		t.Fatal(err)
	}
	if hit != 0 {
		t.Fatalf("取消订阅后不应再收到事件，实际 %d 次", hit)
	}
}

func TestPublishIsolatesFailingAndPanickingSubscribers(t *testing.T) {
	b := New()
	reached := false

	b.Subscribe(TopicPublicIPChanged, func(context.Context, any) error {
		return errors.New("这个订阅者坏了")
	})
	b.Subscribe(TopicPublicIPChanged, func(context.Context, any) error {
		panic("这个订阅者崩了")
	})
	b.Subscribe(TopicPublicIPChanged, func(context.Context, any) error {
		reached = true
		return nil
	})

	err := b.Publish(context.Background(), TopicPublicIPChanged, PublicIPChanged{IP: "1.2.3.4"})

	// 一个模块的 bug 不应该连带弄挂别人：后两个订阅者仍要被调用
	if !reached {
		t.Fatal("前面的订阅者出错后，后面的订阅者仍应收到事件")
	}
	if err == nil {
		t.Fatal("出错与 panic 都应被聚合成 error 返回")
	}
	if !strings.Contains(err.Error(), "这个订阅者坏了") || !strings.Contains(err.Error(), "panic") {
		t.Fatalf("聚合错误里应同时包含两种失败原因，实际：%v", err)
	}
}

func TestSubscriberCanUnsubscribeDuringPublish(t *testing.T) {
	b := New()

	// 先声明再赋值：闭包里要用到自己，不能用 := 一步写完
	var cancel func()
	cancel = b.Subscribe(TopicServiceDeleted, func(context.Context, any) error {
		cancel() // 在处理过程中取消自己，不应死锁
		return nil
	})
	b.Subscribe(TopicServiceDeleted, func(context.Context, any) error { return nil })

	if err := b.Publish(context.Background(), TopicServiceDeleted, ServiceDeleted{}); err != nil {
		t.Fatal(err)
	}
	if n := b.SubscriberCount(TopicServiceDeleted); n != 1 {
		t.Fatalf("订阅者数应为 1，实际 %d", n)
	}
}

func TestPublishWithNoSubscribersIsNoop(t *testing.T) {
	b := New()
	if err := b.Publish(context.Background(), TopicCertExpiring, CertExpiring{}); err != nil {
		t.Fatalf("没有订阅者时不应报错，实际：%v", err)
	}
}

func TestSubscribeTypedChecksPayloadType(t *testing.T) {
	b := New()
	var gotIP string

	cancel, err := SubscribeTypedOn(b, TopicPublicIPChanged,
		func(_ context.Context, e PublicIPChanged) error {
			gotIP = e.IP
			return nil
		})
	if err != nil {
		t.Fatal(err)
	}
	defer cancel()

	if err := b.Publish(context.Background(), TopicPublicIPChanged,
		PublicIPChanged{IP: "5.6.7.8", Previous: "1.1.1.1"}); err != nil {
		t.Fatal(err)
	}
	if gotIP != "5.6.7.8" {
		t.Fatalf("载荷未送达，实际 %q", gotIP)
	}

	// 载荷类型不符时应返回错误，而不是静默忽略
	err = b.Publish(context.Background(), TopicPublicIPChanged, "1.2.3.4")
	if err == nil || !strings.Contains(err.Error(), "期望载荷") {
		t.Fatalf("类型不符应报错，实际：%v", err)
	}
}

func TestSubscribeTypedRejectsUnregisteredPayload(t *testing.T) {
	type unknown struct{ A int }
	if _, err := SubscribeTypedOn(New(), TopicPublicIPChanged,
		func(context.Context, unknown) error { return nil }); err == nil {
		t.Fatal("未登记的载荷类型应在订阅时就报错，而不是运行时静默收不到")
	}
}

func TestPublishAsyncDoesNotBlock(t *testing.T) {
	b := New()
	release := make(chan struct{})
	done := make(chan struct{})

	b.Subscribe(TopicTunnelUp, func(context.Context, any) error {
		<-release // 卡住异步投递，证明 PublishAsync 已经返回了
		close(done)
		return nil
	})

	b.PublishAsync(context.Background(), TopicTunnelUp, TunnelStateChanged{DeviceID: 1})
	close(release)

	<-done
}

func TestResetClearsSubscribers(t *testing.T) {
	b := New()
	var mu sync.Mutex
	calls := 0
	b.Subscribe(TopicTunnelDown, func(context.Context, any) error {
		mu.Lock()
		calls++
		mu.Unlock()
		return nil
	})
	b.Reset()
	if err := b.Publish(context.Background(), TopicTunnelDown, TunnelStateChanged{}); err != nil {
		t.Fatal(err)
	}
	mu.Lock()
	defer mu.Unlock()
	if calls != 0 {
		t.Fatal("Reset 后不应再收到事件")
	}
}
