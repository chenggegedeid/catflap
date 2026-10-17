// Package component 提供统一的组件契约、注册表与按依赖启动的编排。
//
// 它要解决的问题：在引入本包之前，新增一个业务功能需要手工修改四处硬编码列表
// （app.go 的启动列表、routers/enter.go 的挂载、api/enter.go 的 Api 结构体、
// 前端 App.tsx 的 import 与 nav 列表）。任何一处漏改，功能就是「半隐身」的。
//
// 用本包之后，新增一个功能 = 新增一个 modules/<名字>/component.go，
// 在 init() 里调用 component.MustRegister(...)，然后由注册表统一编排。
//
// # 三层扩展点
//
//	core/component  加业务功能     → 本包
//	core/provider   加服务商       → 见 core/provider
//	core/bus        模块间解耦     → 见 core/bus
//
// # 编写一个组件
//
//	// modules/foo/component.go
//	package foo
//
//	import (
//	    "context"
//	    "github.com/chenggegedeid/catflap/core/component"
//	)
//
//	type comp struct{}
//
//	func (comp) Name() string { return "foo" }
//
//	func (comp) Init(ctx context.Context) error { return InitFoo() }
//	func (comp) Stop(ctx context.Context) error { return StopFoo() }        // 可选
//	func (comp) Enabled() bool                  { return Runtime.Enabled() } // 可选
//	func (comp) SetEnabled(v bool) error        { return Runtime.SetEnabled(v) }
//	func (comp) Health() component.Health       { ... }                     // 可选
//
//	func init() {
//	    // 依赖 STUN：证书必须在 STUN 之前就绪（见 app.go 里原有的顺序注释）
//	    component.MustRegister(comp{}, "stun")
//	}
//
// 然后在 app.go 里只保留一行副作用导入：
//
//	_ "github.com/chenggegedeid/catflap/modules/foo"
//
// # 迁移策略
//
// **加适配层，不重写模块。** 每个既有模块只新增 60~120 行的 component.go，
// 模块内部零改动。灰度顺序：webhook → home → stun，每接一个跑一次回归。
package component

import (
	"context"
	"time"
)

// Component 是所有业务模块的统一契约。
//
// 只强制要求 Name 与 Init 两个方法；其余能力通过下面的可选接口按需实现 ——
// 注册表会做类型断言，实现了就参与对应环节，没实现就跳过，不需要写空实现。
type Component interface {
	// Name 组件的唯一标识，用短横线小写，例如 "stun"、"ddns"、"webhook"。
	// 它会出现在日志、健康检查结果和依赖声明里。
	Name() string

	// Init 初始化组件。应在其中完成配置读取、状态恢复等准备工作。
	//
	// ctx 由注册表传入，带超时；不要把 ctx 存起来长期使用。
	// 若组件需要长期后台运行，请在 Init 里自己起 goroutine，并在 Stop 里收敛。
	Init(ctx context.Context) error
}

// ── 可选能力接口 ────────────────────────────────────────────────────────────
// 实现哪个就参与哪个环节，全部可选。

// Stopper 需要在退出时清理资源（关闭监听、停掉后台协程、flush 缓冲）。
// 注册表按注册的**逆序**调用，保证依赖方先于被依赖方停止。
type Stopper interface {
	Stop(ctx context.Context) error
}

// Enablable 可以被用户在界面上开关。
//
// 关闭语义由组件自己定义，但必须满足一条：**关闭后该组件的 HTTP 接口应拒绝服务**
// （建议返回 404 而不是 500，避免向未授权者泄露「这里有个功能」）。
type Enablable interface {
	Enabled() bool
	SetEnabled(enabled bool) error
}

// HealthChecker 提供健康状态，用于面板状态页与上游健康检查。
//
// 实现要求：必须**快速返回**（建议 < 200ms），不要在其中做网络探测这类慢操作 ——
// 需要探测的话请在后台周期性刷新并缓存结果，Health 只读缓存。
type HealthChecker interface {
	Health() Health
}

// Schedulable 声明自己需要跑的定时任务。
//
// 调度器在 v1.1 的 modules/task 中实现；本接口先行定义，让组件可以提前声明意图，
// 避免 v1.1 时又要回头改各模块。调度器只需读 Tasks() 并负责执行与重试。
type Schedulable interface {
	Tasks() []Task
}

// Describer 给组件一个给人看的名字和一句说明。
//
// # 为什么需要它
//
// 「功能管理」页列出全部组件，其中有一半（webhook / traffic / update / auth…）
// 在前端没有对应页面，只能拿机器名（Name()）顶上。机器名是给日志和
// config/components.json 用的，摆在界面上用户看不出那是什么。
//
// 名字由组件自己提供而不是前端再维护一张映射表：那条映射是第二个真相源，
// 加了新模块忘了补就退化成机器名，而退化不会报错。
//
// 做成**可选**接口是为了让存量模块能一个个补：没实现的照旧显示机器名，
// 不会编译不过。scripts/check_component_bindings.py 会盯着谁还没补。
type Describer interface {
	// Title 界面上显示的名字，例如「Webhook 通知」。
	Title() string
	// Summary 一句话说明这个功能是干什么的。控制在 20 字以内，
	// 显示在列表的第二行，太长会把状态标签挤到下一行。
	Summary() string
}

// SchemaProvider 声明自己的配置结构，前端据此自动生成表单。
//
// 实现了本接口的组件，前端**不需要为它手写任何表单代码**。
type SchemaProvider interface {
	ConfigSchema() Schema
}

// RouteProvider 自己注册 HTTP 路由。
//
// 实现了本接口的组件，不再需要在 routers/ 下新增文件、也不需要在
// routers/enter.go 里手工挂载。注册表会把框架提供的路由组原样传进来，
// 组件自己决定挂在公开组还是受保护组（见 RouterGroups）。
type RouteProvider interface {
	Routes(g RouterGroups)
}

// ── 公共类型 ────────────────────────────────────────────────────────────────

// HealthStatus 健康状态枚举。
type HealthStatus string

const (
	// HealthUnknown 尚未初始化完成，或组件主动表示「不适用」。
	HealthUnknown HealthStatus = "unknown"
	// HealthHealthy 一切正常。
	HealthHealthy HealthStatus = "healthy"
	// HealthDegraded 还能用，但有部分能力受损（例如证书将过期、某个上游不可达）。
	HealthDegraded HealthStatus = "degraded"
	// HealthDown 不可用，需要用户处理。
	HealthDown HealthStatus = "down"
)

// Health 组件健康状态。
type Health struct {
	Status HealthStatus `json:"status"`
	// Message 给用户看的一句话，例如「证书还有 3 天过期」。健康时留空。
	Message string `json:"message,omitempty"`
	// Details 给排查用的结构化信息，会原样出现在接口响应里。
	// ⚠️ 不要放凭据、内网 IP 列表等敏感内容。
	Details map[string]any `json:"details,omitempty"`
	// CheckedAt 本次状态的产生时间。
	CheckedAt time.Time `json:"checkedAt"`
}

// Healthy 是构造健康状态的快捷方法。
func Healthy() Health {
	return Health{Status: HealthHealthy, CheckedAt: time.Now()}
}

// Degraded 构造「可用但有问题」的状态。
func Degraded(msg string) Health {
	return Health{Status: HealthDegraded, Message: msg, CheckedAt: time.Now()}
}

// Down 构造「不可用」的状态。
func Down(msg string) Health {
	return Health{Status: HealthDown, Message: msg, CheckedAt: time.Now()}
}

// Task 一个定时任务声明。
//
// Interval 与 Cron 二选一：Interval > 0 时按固定间隔跑，否则按 Cron 表达式跑。
type Task struct {
	// Name 任务名，同组件内唯一，用于日志与「上次执行结果」展示。
	Name string
	// Interval 固定间隔；为 0 时改用 Cron。
	Interval time.Duration
	// Cron 标准 5 段 Cron 表达式，例如 "0 3 * * *"。
	Cron string
	// Run 任务体。返回 error 时调度器会记日志并按策略重试。
	// ctx 会在组件 Stop 时被取消，任务体应尊重它。
	Run func(ctx context.Context) error
	// Singleton 为 true 时，上一次还没跑完就不再启动下一次（长任务建议开启）。
	Singleton bool
}
