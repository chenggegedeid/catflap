package stun

import "sync/atomic"

// ===================== 通知：由通知中心接管 =====================
//
// **依赖方向是 notify → stun，stun 不能反过来 import notify**
// （见 notify_bridge.go 里的说明：业务模块不互相依赖）。
//
// 所以这里只定义"我有什么要通知、发到哪些渠道、内容模板是什么"，
// 真正的发送交给通知中心通过 hook 注入进来 —— 和 device 的
// SetIPChangedHook 是同一个套路。
//
// 于是：
//   - 通知方式和 token 全在**通知页**配（全局渠道），服务不存任何密钥
//   - 服务里只写"我想说什么"（模板），渠道自己决定格式
//     （钉钉 markdown、PushPlus、Bark 各不相同，那是渠道的事）

// NotifyRequest 一次通知请求（stun 侧的普通数据结构）
type NotifyRequest struct {
	// ChannelIDs 引用的全局渠道（在服务上选的）。空 = 这个服务不发。
	ChannelIDs []string
	// Template 用户自定义的正文模板，支持 #{xxx} 占位符。空 = 用默认文案。
	Template string
	// Title 一句话标题
	Title string
	// Fields 占位符取值：#{address} #{service_name} #{external_ip} ...
	Fields map[string]string
	// Key 去重键（同一 Key 在冷却窗口内只发一次，防刷屏）
	Key string
}

// notifyHook 由通知中心注册（nil = 通知中心没启用，退回旧的内联 webhook）。
//
// 用 atomic 而不是普通变量：通知中心在**组件初始化时**装 hook，
// 而穿透的收包 goroutine 随时可能在读它 —— 两边没有先后保证，
// 普通变量就是数据竞争（go test -race 会报）。
// 这里存的是同一函数类型的值，类型一致，atomic.Value 用得起来。
var notifyHook atomic.Value

type notifyHookFunc func(NotifyRequest)

// SetNotifyHook 注册通知发送器（通知中心启动时调）。
// 传 nil 表示退回到旧的内联 webhook 路径。
func SetNotifyHook(fn func(NotifyRequest)) {
	notifyHook.Store(notifyHookFunc(fn))
}

// currentNotifyHook 取当前注册的发送器，没装返回 nil
func currentNotifyHook() notifyHookFunc {
	v := notifyHook.Load()
	if v == nil {
		return nil
	}
	return v.(notifyHookFunc)
}

// hasNotifyCenter 通知中心有没有接管
func hasNotifyCenter() bool {
	return currentNotifyHook() != nil
}
