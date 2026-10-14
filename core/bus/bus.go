// Package bus 是模块之间的事件总线。
//
// # 为什么需要它
//
// 引入本包之前，跨模块的「发生了一件事，通知别人」是靠各模块自己维护一组回调实现的
// （见 modules/stun 里曾经的 hooks.go：RegisterOnServiceDeleted /
// RegisterOnPublicIPChanged 加一个全局切片）。那套做法能用，但有两个问题：
//
//  1. 每加一个订阅关系，就要在**发布方**的包里再加一组 Register/Emit 函数与一个
//     全局切片，发布方被迫知道「有多少种别人关心我」；
//  2. 谁来把订阅方和发布方接上？只能靠在 app.go 里手写一段注入代码
//     （见 app.go 的 registerSTUNRedirectSyncer）。加一个模块要改 app.go。
//
// 换成事件总线之后：发布方只管 Publish，订阅方只管 Subscribe，
// **app.go 不再需要知道它们之间存在关系**。
//
// # 用法
//
//	// 发布方（modules/stun）
//	bus.Publish(ctx, bus.TopicPublicIPChanged, bus.PublicIPChanged{IP: ip, Previous: old})
//
//	// 订阅方（modules/ddns），在自己的 Init 里注册
//	cancel, err := bus.SubscribeTyped(bus.TopicPublicIPChanged,
//	    func(ctx context.Context, e bus.PublicIPChanged) error {
//	        Runtime.TriggerSync()
//	        return nil
//	    })
//	if err != nil { return err }
//	defer cancel()   // 组件 Stop 时取消订阅，避免停掉的组件还收事件
//
// # 设计取舍
//
//   - **同步投递**：Publish 依次调用订阅者并等待返回。这样「IP 变了 → DDNS 推记录」
//     才是有序的；需要不阻塞调用方的场合用 PublishAsync。
//   - **订阅者之间互不影响**：某个订阅者返回 error 或 panic，不会中断其它订阅者
//     （panic 会被 recover 成 error 并记日志）。一个模块的 bug 不应该连带弄挂别人。
//   - **不加消息队列**：进程内、无持久化、不重试。这是通知机制，不是任务系统。
//     需要可靠执行的任务请用 component.Task。
package bus

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"time"

	"github.com/sirupsen/logrus"
)

// DefaultAsyncTimeout PublishAsync 给异步投递的兜底超时。
//
// 异步投递会脱离调用方的 ctx（用 context.WithoutCancel），否则调用方一返回
// ctx 被取消，通知就全丢了。脱离之后就必须要有个自己的上限，否则万一订阅者卡住，
// goroutine 会一直挂着。
const DefaultAsyncTimeout = 30 * time.Second

// Handler 事件处理器。payload 的具体类型由主题约定（见 topics.go 里各主题的说明）。
type Handler func(ctx context.Context, payload any) error

// subscriber 一个订阅者。带 id 是为了取消时能精确定位——
// Go 里函数值不能相互比较，只能跟 nil 比，没法靠函数值找到是哪一条。
type subscriber struct {
	id uint64
	h  Handler
}

// Bus 事件总线。一般用包级的 DefaultBus 即可；需要隔离时用 New。
type Bus struct {
	mu sync.RWMutex
	// 每个主题的订阅者按**注册顺序**存在切片里，不用 map。
	//
	// 用 map 的话遍历顺序是随机的，而本包承诺「投递顺序 = 订阅顺序」
	// （见 Publish 的说明），「IP 变了 → DDNS 推记录」这类有先后要求的链路
	// 就会时对时错。CI 上曾因此出现「投递顺序不对：[second first]」。
	subs map[Topic][]subscriber
	next uint64
}

// DefaultBus 全局默认总线，包级 Publish/Subscribe 操作的就是它。
var DefaultBus = New()

// New 建一个独立的总线（测试用）。
func New() *Bus {
	return &Bus{subs: make(map[Topic][]subscriber)}
}

// Subscribe 订阅主题，返回取消订阅的函数（可重复调用，幂等）。
// 同一个主题的多个订阅者按注册先后依次收到事件。
func (b *Bus) Subscribe(topic Topic, h Handler) func() {
	if h == nil || topic == "" {
		return func() {}
	}

	b.mu.Lock()
	b.next++
	id := b.next
	b.subs[topic] = append(b.subs[topic], subscriber{id: id, h: h})
	b.mu.Unlock()

	var once sync.Once
	return func() {
		once.Do(func() {
			b.mu.Lock()
			defer b.mu.Unlock()
			list := b.subs[topic]
			for i := range list {
				if list[i].id == id {
					b.subs[topic] = append(list[:i], list[i+1:]...)
					return
				}
			}
		})
	}
}

// Publish 同步投递给该主题的所有订阅者，返回聚合后的错误。
//
// 投递顺序是订阅顺序。任何一个订阅者出错都不影响后续订阅者。
func (b *Bus) Publish(ctx context.Context, topic Topic, payload any) error {
	handlers := b.handlersOf(topic)
	if len(handlers) == 0 {
		return nil
	}

	var errs []error
	for _, h := range handlers {
		if err := callHandler(ctx, topic, h, payload); err != nil {
			errs = append(errs, err)
		}
	}
	return errors.Join(errs...)
}

// PublishAsync 异步投递，立刻返回。
//
// 用于「通知一声就行、调用方不关心结果」的场景。异步投递会脱离调用方的 ctx，
// 并施加 DefaultAsyncTimeout 的上限。
func (b *Bus) PublishAsync(ctx context.Context, topic Topic, payload any) {
	handlers := b.handlersOf(topic)
	if len(handlers) == 0 {
		return
	}

	base := context.WithoutCancel(ctx)
	go func() {
		actx, cancel := context.WithTimeout(base, DefaultAsyncTimeout)
		defer cancel()
		for _, h := range handlers {
			if err := callHandler(actx, topic, h, payload); err != nil {
				logrus.Errorf("bus: 异步投递 %s 失败：%v", topic, err)
			}
		}
	}()
}

// SubscriberCount 某主题当前的订阅者数量。排查「事件发了但没人收」时有用。
func (b *Bus) SubscriberCount(topic Topic) int {
	b.mu.RLock()
	defer b.mu.RUnlock()
	return len(b.subs[topic])
}

// Reset 清空所有订阅。仅测试使用。
func (b *Bus) Reset() {
	b.mu.Lock()
	b.subs = make(map[Topic][]subscriber)
	b.next = 0
	b.mu.Unlock()
}

// handlersOf 在锁内拷一份出来，锁外调用。
//
// 这一步不能省：订阅者如果在处理过程中又订阅/取消订阅，持锁调用会死锁。
// 现有 modules/stun/hooks.go 也是同样的写法。
//
// 返回值保持订阅顺序——切片本身就是按注册顺序追加的，顺序就是遍历顺序。
func (b *Bus) handlersOf(topic Topic) []Handler {
	b.mu.RLock()
	defer b.mu.RUnlock()

	list := b.subs[topic]
	if len(list) == 0 {
		return nil
	}
	out := make([]Handler, 0, len(list))
	for _, s := range list {
		out = append(out, s.h)
	}
	return out
}

// callHandler 兜住 panic：一个订阅者的崩溃不应该连带影响其它订阅者和发布方。
func callHandler(ctx context.Context, topic Topic, h Handler, payload any) (err error) {
	defer func() {
		if r := recover(); r != nil {
			err = fmt.Errorf("bus: 主题 %s 的订阅者 panic：%v", topic, r)
		}
	}()
	return h(ctx, payload)
}

// ── 包级快捷方法（操作 DefaultBus）──────────────────────────────────────────

// Subscribe 订阅 DefaultBus 上的主题。
func Subscribe(topic Topic, h Handler) func() {
	return DefaultBus.Subscribe(topic, h)
}

// Publish 向 DefaultBus 同步发布事件。
func Publish(ctx context.Context, topic Topic, payload any) error {
	return DefaultBus.Publish(ctx, topic, payload)
}

// PublishAsync 向 DefaultBus 异步发布事件。
func PublishAsync(ctx context.Context, topic Topic, payload any) {
	DefaultBus.PublishAsync(ctx, topic, payload)
}

// SubscriberCount DefaultBus 上某主题的订阅者数量。
func SubscriberCount(topic Topic) int {
	return DefaultBus.SubscriberCount(topic)
}
