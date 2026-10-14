package proxy

import (
	"context"

	"github.com/chenggegedeid/catflap/core/component"
)

// Name 组件名。与 routers/proxy_routers.go 里 ContributeRoutes 用的名字一致。
const Name = "proxy"

// proxyComponent 把反向代理模块接到组件注册表上。
//
// # 为什么是 Enablable，但关掉不等于停监听
//
// 内嵌 LocalSwitch 后，用户可以在界面上把反向代理「关掉」。这里的关闭语义是
// **接口拒绝服务**（该组件的 /api/proxy/* 全返回 404），而不是把已经起来的
// 站点监听停掉 —— 后者会让正在跑的站点瞬间中断，用户只是想收掉一个入口，
// 不该顺手把线上流量掐了。真要停用某个站点，去站点配置里关它。
//
// 注意「关掉接口」和「监听还在」这个组合是刻意的：把开关做成「停服务」，
// 就必须回答「再打开时原来的监听能不能原样恢复」，而这类状态机在没有
// 明确需求前不该提前造。
type proxyComponent struct {
	// LocalSwitch 提供 Enabled / SetEnabled，状态存在
	// config/components.json（见 core/component/state.go）。
	component.LocalSwitch
}

func (proxyComponent) Name() string { return Name }

// Init 读配置、建站点表、开监听。
//
// 监听失败不算初始化失败（InitProxy 里已处理）：站点配置都在，用户到
// 页面上补端口就行，没必要因为一个站点起不来就让整个面板不启动。
func (proxyComponent) Init(context.Context) error { return InitProxy() }

func init() {
	component.MustRegister(proxyComponent{
		LocalSwitch: component.NewLocalSwitch(Name),
	})
}
