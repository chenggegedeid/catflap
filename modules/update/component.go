package update

import (
	"context"

	"github.com/chenggegedeid/catflap/core"
	"github.com/chenggegedeid/catflap/core/component"
)

// Name 组件名。与 routers/system_routers.go 里 ContributeRoutes 用的名字一致
// （更新那一组路由挂在 /api/system/update*）。
const Name = "update"

// updateComponent 把检查更新模块接到组件注册表上。
//
// # 版本号为什么从 core 读
//
// InitUpdate 需要知道自己是哪个版本，才能跟远端比。改造前 version 由 main
// 传给 api/system_api，再由 app.go 转交给这里；但 api/system_api 又 import 了
// 本模块，让本模块反过来 import api 包就成环了。
//
// 版本号是**构建期属性**（ldflags 注入），不属于任何一层，放进 core 双方都只是
// 读它，环就没了。注入点是 `-X github.com/chenggegedeid/catflap/core.Version=v1.0.0`。
type updateComponent struct {
	// LocalSwitch 提供 Enabled / SetEnabled，状态存在
	// config/components.json（见 core/component/state.go）。
	component.LocalSwitch
}

func (updateComponent) Name() string { return Name }

func (updateComponent) Init(context.Context) error { return InitUpdate(core.Version) }

func init() {
	component.MustRegister(updateComponent{
		LocalSwitch: component.NewLocalSwitch(Name),
	})
}
