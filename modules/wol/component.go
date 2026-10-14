package wol

import (
	"context"

	"github.com/chenggegedeid/catflap/core/component"
)

// Name 组件名。
const Name = "wol"

// wolComponent 把 WOL 模块接到组件注册表上。
//
// 它是「可开关」的：关掉之后路由整体 404，界面入口也消失。
// 不做后台定时器，只在用户点「唤醒」时才发包 —— 定时唤醒属于计划任务的范畴，
// 等 task 组件起来再挂上去，别在这里塞一个用不上的调度器。
type wolComponent struct {
	component.LocalSwitch
}

func (wolComponent) Name() string { return Name }

func (wolComponent) Init(context.Context) error { return InitWOL() }

func init() {
	component.MustRegister(wolComponent{
		LocalSwitch: component.NewLocalSwitch(Name),
	})
}
