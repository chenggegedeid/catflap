package system

import (
	"context"
	"strings"

	"github.com/chenggegedeid/catflap/core/component"
	"github.com/chenggegedeid/catflap/core/provider"

	"github.com/sirupsen/logrus"
)

// Name 组件名。
const Name = "system"

// systemComponent 是「系统信息与运行日志」这组接口的归属组件。
//
// 它没有自己的业务模块目录（handler 在 api/system_api，路由在
// routers/system_routers.go），但**仍然值得注册成一个组件**：
//
//   - 版本号与运行日志是用户在界面上看得见的功能（侧栏底部有版本、
//     导航里有「运行日志」页），不是框架内部管道，理应出现在功能列表里；
//   - 若把它们当作框架路由直接写在 routers/enter.go 里，enter.go 就又变成
//     一份需要手工维护的清单了 —— 那正是 M3 要消掉的东西。
//
// 不实现 Enablable：关掉它等于把「看日志」这件事本身关掉，
// 出问题时用户连排查手段都没有，这种开关有害无益。
type systemComponent struct{}

func (systemComponent) Name() string { return Name }

// Init 做启动摘要，把「装了哪些功能、哪些被关了、注册了哪些服务商」写进日志。
//
// 放在这里而不是 app.go：摘要是「系统」这个组件的职责，写在自己的 Init 里
// 就不用启动流程去记得调用它。这些信息全部来自各包的 init() 注册结果
// （在 main 之前就已就绪），所以不依赖其它组件是否初始化完成 ——
// 因此它也不需要声明任何依赖，不必等别人。
func (systemComponent) Init(context.Context) error {
	names := component.Names()
	disabled := component.DisabledNames()

	logrus.Infof("组件共 %d 个：%s", len(names), strings.Join(names, ", "))
	if len(disabled) > 0 {
		logrus.Warnf("以下组件已在界面上被关闭，其接口返回 404：%s", strings.Join(disabled, ", "))
	}
	provider.LogSummary()
	return nil
}

func init() { component.MustRegister(systemComponent{}) }
