package home

import (
	"context"

	"github.com/chenggegedeid/catflap/core/component"
)

// Name 组件名。
const Name = "home"

// homeComponent 把导航主页模块接到组件注册表上。
//
// 什么依赖都不声明是有意的：门户页要显示「哪些服务开着」，靠的是订阅
// stun 的服务删除事件（见 stun_hook.go）。事件总线不要求发布方先起来 ——
// 订阅关系在各自的 Init 里建立，谁先谁后都能收到后续事件。
// 若在这里写 dependsOn("stun")，就会让门户页白等 stun 那几秒的网络探测。
//
// 不实现 Enablable：主页是访客打开域名第一眼看到的东西，也是桌面版之外
// 唯一的入口；关掉它留下的空白页比「功能少一个」严重得多。
type homeComponent struct{}

func (homeComponent) Name() string { return Name }

func (homeComponent) Init(context.Context) error { return InitHome() }

// Stop 退掉事件订阅。
//
// 单进程里进程退出也就一起没了，看似多余；但组件若将来支持「关掉再打开」
// （或者测试里反复建注册表），不退订就会出现「订阅者列表越来越长、
// 每次事件被处理好几遍」——这种问题极难从现象倒推原因，不如一开始就做对。
func (homeComponent) Stop(context.Context) error {
	unregisterStunHooks()
	return nil
}

func init() { component.MustRegister(homeComponent{}) }
