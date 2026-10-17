package webhook

import (
	"context"

	"github.com/chenggegedeid/catflap/core/component"
)

// Name 组件名。
const Name = "webhook"

// webhookComponent 把 Webhook 模块接到组件注册表上。
//
// 它是最小的一个模块（只有模板的增删改查），灰度接入顺序里排第一：
// 先拿它验证「注册 → 启动 → 挂路由 → 能开关」整条链路，再动 stun 那种大的。
type webhookComponent struct {
	// LocalSwitch 提供 Enabled / SetEnabled，状态存在
	// config/components.json（见 core/component/state.go）。
	component.LocalSwitch
}

func (webhookComponent) Name() string { return Name }

// Title 见 core/component.Describer。
func (webhookComponent) Title() string { return "Webhook 通知" }

// Summary 见 core/component.Describer。
func (webhookComponent) Summary() string { return "把事件推送到自定义的 HTTP 地址" }

func (webhookComponent) Init(context.Context) error { return InitWebhook() }

func init() {
	component.MustRegister(webhookComponent{
		LocalSwitch: component.NewLocalSwitch(Name),
	})
}
