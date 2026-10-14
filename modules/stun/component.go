package stun

import (
	"context"

	"github.com/chenggegedeid/catflap/core/component"
	"github.com/chenggegedeid/catflap/modules/cert"
)

// Name 组件名。与 routers/stun_routers.go 里 ContributeRoutes 用的名字一致。
const Name = "stun"

// stunComponent 把 STUN/端口映射模块接到组件注册表上。
//
// # 为什么依赖 cert
//
// STUN 起来之后要往网关/防火墙推映射规则，走的是 HTTPS 站点入口；证书没就绪
// 时推出去的是个浏览器会拦的站点。改造前这个顺序靠 app.go 里的注释和启动列表
// 的排列维持，属于「谁改谁知道」的知识；写成显式依赖后，注册表按拓扑排序保证
// cert 先完成，注释就不再是唯一的守卫。
//
// 用 cert.Name 而不是手写 "cert"：依赖名写错时注册表只会在启动时报
// 「依赖 X 没有注册」，不会告诉你是拼错了。引用常量让编译器把这件事管起来。
type stunComponent struct {
	// LocalSwitch 提供 Enabled / SetEnabled，状态存在
	// config/components.json（见 core/component/state.go）。
	component.LocalSwitch
}

func (stunComponent) Name() string { return Name }

func (stunComponent) Init(context.Context) error { return InitSTUN() }

func init() {
	component.MustRegister(stunComponent{
		LocalSwitch: component.NewLocalSwitch(Name),
	}, cert.Name)
}
