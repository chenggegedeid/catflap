package ddns

import (
	"context"

	"github.com/chenggegedeid/catflap/core/bus"
	"github.com/chenggegedeid/catflap/modules/stun"

	"github.com/sirupsen/logrus"
)

// cancelPublicIPHook 取消对「公网 IP 变了」的订阅，组件停止时调用。
var cancelPublicIPHook = func() {}

// subscribePublicIPChanged 订阅「公网 IP 有了 / 变了」，一到手就催一轮同步。
//
// 为什么必须有这条订阅：开机时 DDNS 与 STUN 是并排起的，DDNS 先跑那一轮
// 常常赶在 STUN 探到 IP 之前，那次同步只能记一次失败。没人叫它的话就得干等
// 下一个周期（默认 5 分钟），域名在这段时间里一直指着旧地址。
//
// 订阅完还要**回头读一次当前值**：两个模块在同一层并发启动，STUN 完全可能
// 在我们订阅之前就已经探到 IP 并广播过了。事件总线的语义是「发的那一刻谁在听
// 谁收到」，补不上历史 —— 不补这一下，那种时序下这次通知就永远丢了。
// 这正是把「订阅」放进 Init 之后新出现的时序窗口：改造前这个回调是在
// app.go 里、任何模块启动之前就注册好的。
func subscribePublicIPChanged() {
	cancel, err := bus.SubscribeTyped(bus.TopicPublicIPChanged,
		func(_ context.Context, e bus.PublicIPChanged) error {
			logrus.Infof("[ddns] 收到公网 IP 变化（%s → %s），立即同步一轮解析记录", e.Previous, e.IP)
			Runtime.TriggerSync()
			return nil
		})
	if err != nil {
		logrus.Errorf("[ddns] 订阅公网 IP 变化失败，IP 变更后将只能等下一个同步周期：%v", err)
		return
	}
	cancelPublicIPHook = cancel

	if ip := stun.CurrentPublicIP(); ip != "" {
		logrus.Infof("[ddns] 启动时 STUN 已探到公网 IP %s，先补一轮同步", ip)
		Runtime.TriggerSync()
	}
}

// unsubscribePublicIPChanged 退订，别让停掉的模块还在收事件。
func unsubscribePublicIPChanged() {
	cancelPublicIPHook()
	cancelPublicIPHook = func() {}
}
