package backup

import (
	"context"

	"github.com/chenggegedeid/catflap/core/component"
)

// Name 组件名。路由贡献要用它，否则挂载时会被判成"有路由无组件"而终止启动。
const Name = "backup"

/**
 * backupComponent 备份组件。
 *
 * 有意做得很轻：不声明依赖、Init 什么都不做。
 *
 * 备份是**随时可用、不需要预热**的功能 —— 它读的是磁盘上的配置文件，
 * 不依赖任何模块先起来。如果给它加 Init 去预加载什么，反而引入
 * "某个模块还没初始化完，备份就不可用"这种没必要的耦合。
 */
type backupComponent struct{}

func (backupComponent) Name() string { return Name }

// Title 见 core/component.Describer。
func (backupComponent) Title() string { return "配置备份" }

// Summary 见 core/component.Describer。
func (backupComponent) Summary() string { return "导出全部配置与素材，可一键还原" }

// Init 启动定时备份循环。
//
// 之前这里什么都不做 —— 那时候备份只有手动触发。加了定时之后需要一个
// 后台 goroutine，组件生命周期正好是它的位置：由框架统一管理启停，
// 不用在外面散落 go 语句。配置里没开定时的话，StartSchedule 自己会跳过。
func (backupComponent) Init(context.Context) error {
	StartSchedule()
	return nil
}

func (backupComponent) Stop(context.Context) error {
	StopSchedule()
	return nil
}

func init() { component.MustRegister(backupComponent{}) }
