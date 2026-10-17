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

// Init 不需要做任何事：备份是直接读配置文件的，没有运行时状态要准备。
func (backupComponent) Init(context.Context) error { return nil }

func (backupComponent) Stop(context.Context) error { return nil }

func init() { component.MustRegister(backupComponent{}) }
