package task

import (
	"context"

	"github.com/chenggegedeid/catflap/core/component"
)

// Name 组件名。
const Name = "task"

// taskComponent 把计划任务模块接到组件注册表上。
//
// 关掉它之后接口会 404、侧栏入口会消失，调度循环也不再执行任何动作
// （门禁加在 RunDue 上，见 modules/task/enter.go）。
//
// 注意循环**本身不停**：goroutine 只是每 30 秒醒来看一眼开关。
// 曾经这里的注释写着「关掉会停掉调度循环（goroutine 随 ctx 取消而退出）」，
// 那是错的 —— cancel 只在 core.OnShutdown 里调，开关翻位跟它没关系，
// 结果就是关掉了还在定时同步解析。
//
// 刻意不去补「关掉真停 goroutine、再打开重启」那套状态机：那要回答
// 「重启时正在跑的任务怎么办、lastRun 要不要保留」，在没有明确需求前
// 不提前造（同一个取舍见 modules/proxy/component.go 里关于监听的说明）。
// 空转一圈的成本是每 30 秒一次 map 读取，可以忽略。
type taskComponent struct {
	component.LocalSwitch
}

func (taskComponent) Name() string { return Name }

// Title 见 core/component.Describer。
func (taskComponent) Title() string { return "计划任务" }

// Summary 见 core/component.Describer。
func (taskComponent) Summary() string { return "定时同步解析记录或唤醒机器" }

func (taskComponent) Init(context.Context) error { return InitTask() }

func init() {
	component.MustRegister(taskComponent{
		LocalSwitch: component.NewLocalSwitch(Name),
	})
}
