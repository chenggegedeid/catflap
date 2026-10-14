package home

import (
	"context"

	"github.com/chenggegedeid/catflap/core/bus"

	"github.com/sirupsen/logrus"
)

// cancelStunHooks 取消对 stun 事件的订阅，组件停止时调用。
var cancelStunHooks = func() {}

// registerStunHooks 订阅「服务被删除」事件：门户页要把对应卡片一起摘掉。
//
// 这里只认识 bus.TopicServiceDeleted，**不知道 stun 这个包存在** ——
// 以前是 import modules/stun 调它的 RegisterOnServiceDeleted，那让门户页
// 和穿透模块绑在一起；现在两边都只认事件总线，谁加谁删都不影响对方。
func registerStunHooks() {
	cancel, err := bus.SubscribeTyped(bus.TopicServiceDeleted,
		func(_ context.Context, e bus.ServiceDeleted) error {
			Runtime.RemoveAppByStunService(e.DeviceID, e.ServiceID)
			logrus.Debugf("[home] 同步删除 stun app device=%d service=%d", e.DeviceID, e.ServiceID)
			return nil
		})
	if err != nil {
		// 载荷类型没在 topics.go 登记只可能是开发期出错。真发生了的表现是
		// 「服务删了、门户页卡片还在」，必须让人能在日志里看见原因。
		logrus.Errorf("[home] 订阅服务删除事件失败，门户页将不会自动清理卡片：%v", err)
		return
	}
	cancelStunHooks = cancel
}

// unregisterStunHooks 退订，别让停掉的模块还在收事件。
func unregisterStunHooks() {
	cancelStunHooks()
	cancelStunHooks = func() {}
}
