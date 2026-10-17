package ddns

import (
	"context"
	"fmt"

	"github.com/chenggegedeid/catflap/core/component"
	"github.com/chenggegedeid/catflap/modules/ddns/model"
)

// Name 组件名。与 routers/ddns_routers.go 里 ContributeRoutes 用的名字一致。
const Name = "ddns"

// ddnsComponent 把动态域名解析模块接到组件注册表上。
//
// # 为什么不声明依赖 stun
//
// 它确实要在 STUN 探到公网 IP 之后才好同步，但那是**运行期的通知关系**，
// 不是启动顺序关系 —— 订阅在 Init 里建立，谁先谁后都能收到后续事件；
// 万一订阅晚了，subscribePublicIPChanged 结尾还会回头读一次当前值补上。
// 若写成 dependsOn("stun")，DDNS 的接口就要白等 stun 那几秒的网络探测才能用，
// 而这几秒里用户打开 DDNS 页面看到的是 404。
type ddnsComponent struct {
	// LocalSwitch 提供 Enabled / SetEnabled，状态存在
	// config/components.json（见 core/component/state.go）。
	component.LocalSwitch
}

func (ddnsComponent) Name() string { return Name }

// Title 见 core/component.Describer。
func (ddnsComponent) Title() string { return "DDNS 域名解析" }

// Summary 见 core/component.Describer。
func (ddnsComponent) Summary() string { return "把变动的公网 IP 同步到域名" }

// Init 读配置、装 IP 源、建立公网 IP 订阅、启动同步调度器。
func (ddnsComponent) Init(context.Context) error { return DDNSInit() }

// Stop 退掉事件订阅。
//
// 单进程里进程退出也就一起没了，看似多余；但组件若将来支持「关掉再打开」
// （或者测试里反复建注册表），不退订就会出现「订阅者列表越来越长、
// 每次事件被处理好几遍」——这种问题极难从现象倒推原因，不如一开始就做对。
func (ddnsComponent) Stop(context.Context) error {
	unsubscribePublicIPChanged()
	return nil
}

// Health 看解析记录的上次同步结果。
//
// 只读内存快照，不发 DNS 查询 —— 健康检查会被界面轮询，真去查一遍
// 等于拿面板的刷新频率去打服务商的接口。
func (ddnsComponent) Health() component.Health {
	cfg := Runtime.Snapshot()

	failed := 0
	for _, r := range cfg.Records {
		if r.Enabled && r.LastStatus == model.DDNSRecordStatusFailed {
			failed++
		}
	}
	if failed > 0 {
		return component.Degraded(fmt.Sprintf("有 %d 条解析记录上次同步失败", failed))
	}
	return component.Healthy()
}

func init() {
	component.MustRegister(ddnsComponent{
		LocalSwitch: component.NewLocalSwitch(Name),
	})
}

// Enabled 报告 DDNS 功能当前是否开启。
//
// # 为什么调度器要看它
//
// 关掉组件做的是两件事：接口返回 404、侧栏入口消失。但 Init 时起来的
// 同步循环（Scheduler.loop）并不会因此停下 —— 组件开关只翻了
// config/components.json 里的一个位，没人去取消那个 goroutine。
//
// 结果就是界面上写着「DDNS 已关闭」，后台却还在按 interval 去调服务商接口
// 同步解析记录。用户从任何一处都看不出来，只能等下个月的服务商调用量对不上
// 才发现。所以调度器的 scan 入口上要挡一道。
//
// 这里走 component.IsEnabled 而不是另存一份 atomic：开关的真相源只有
// config/components.json 一个，多存一份就多一处会不一致的地方。
func Enabled() bool { return component.IsEnabled(Name) }
