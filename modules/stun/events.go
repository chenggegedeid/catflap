package stun

import (
	"context"

	"github.com/chenggegedeid/catflap/core/bus"

	"github.com/sirupsen/logrus"
)

// 本模块对外只发两种事件，都走 core/bus。
//
// 以前这里是 hooks.go：一个全局切片加上 RegisterOnXxx / EmitXxx 三件套，
// 每加一个订阅关系就得往 stun 里再塞一组函数和一片内存 —— 发布方被迫知道
// 「有多少种别人关心我」。现在 stun 只管发布，谁在听、听去做什么都与它无关，
// 也不会因为别人订阅而改这个包。

// NotifyServiceDeleted 通知「某个服务被删了」。
//
// **同步**投递：调用方是删除接口的 handler，紧随其后还要清重定向规则与落地
// 记录。订阅方（门户页把卡片摘掉）必须在那之前做完，否则用户会看到
// 「服务已经没了、卡片还在」的中间态。
//
// 订阅方报错不当作失败：服务本身已经删掉了，回滚不了；但要说出来，
// 免得「卡片一直没消」变成一桩无头悬案。
func NotifyServiceDeleted(deviceID, serviceID uint) {
	if err := bus.Publish(context.Background(), bus.TopicServiceDeleted, bus.ServiceDeleted{
		DeviceID:  deviceID,
		ServiceID: serviceID,
	}); err != nil {
		logrus.Warnf("[stun] 广播服务删除事件（device=%d service=%d）失败：%v", deviceID, serviceID, err)
	}
}

// notifyPublicIPChanged 通知「公网 IP 有了 / 变了」。
//
// **异步**投递：调用点在网络更新的 goroutine 里，订阅方（DDNS 立刻推一轮
// 解析）可能要几秒；同步等它会把网络更新循环卡住，而这件事并不影响主流程。
//
// 空 IP 直接返回：那不是「变了」，是「还没有」。
func notifyPublicIPChanged(ip, previous string) {
	if ip == "" {
		return
	}
	bus.PublishAsync(context.Background(), bus.TopicPublicIPChanged, bus.PublicIPChanged{
		IP:       ip,
		Previous: previous,
	})
}

// CurrentPublicIP 当前已知的公网 IP，还没有时返回空串。
//
// 给「晚订阅者」准备：事件总线的语义是「发的那一刻谁在听谁收到」，补不上
// 历史。DDNS 若在 STUN 探到 IP 之后才订阅，那一次通知就永远收不到，只能干等
// 下个周期 —— 所以它订阅完要主动读一次这里补上（见 modules/ddns/ddns_init.go）。
func CurrentPublicIP() string {
	return Runtime.Network.PublicIP
}
