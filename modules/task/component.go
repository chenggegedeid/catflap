package task

import (
	"context"

	"github.com/chenggegedeid/catflap/core/component"
)

// Name 组件名。
const Name = "task"

// taskComponent 把计划任务模块接到组件注册表上。
//
// 关掉它会停掉调度循环（Init 里的 goroutine 随 ctx 取消而退出），
// 路由也会 404。
type taskComponent struct {
	component.LocalSwitch
}

func (taskComponent) Name() string { return Name }

func (taskComponent) Init(context.Context) error { return InitTask() }

func init() {
	component.MustRegister(taskComponent{
		LocalSwitch: component.NewLocalSwitch(Name),
	})
}
