# core — 框架层

这里是**不依赖任何业务模块**的复用型能力。业务逻辑一律放 `modules/`，
`core/` 只放「所有模块都会用到、且不需要知道业务细节」的东西。

```
core/
├─ component/   组件契约 + 注册表 + 依赖拓扑排序   ← 加业务功能走这里
├─ provider/    服务商注册表（DNS / 通知 / 存储）  ← 加服务商走这里
├─ bus/         事件总线                          ← 模块间通信走这里
├─ enter.go     日志、关闭信号等基础设施（上游原有）
└─ logrus.go    日志初始化（上游原有）
```

## 为什么要有这三层

引入之前，**每加一个功能要手改四处硬编码列表**：

| 位置 | 改什么 |
|---|---|
| `app.go` | `startModulesInBackground()` 里加一行 `initModule("Xxx", ...)` |
| `routers/enter.go` | 加一行 `XxxRouters(g)` 或 `XxxRouters(protected)` |
| `api/enter.go` | 在 `Api` 结构体里加一个字段 |
| `web/admin/src/App.tsx` + `nav.ts` | 加 import、加路由分支、加导航项 |

漏任何一处，功能就是「半隐身」的 —— 后端跑着但界面上找不到，或者界面能点但接口 404。

现在改成：**注册 + 声明依赖**，注册表统一编排。

## 三层各自解决什么

### `component` —— 加业务功能

一个模块实现 `Component`（`Name()` + `Init()`），在 `init()` 里注册即可。
其余能力按需通过**可选接口**实现，不实现就完全跳过，不用写空方法：

| 可选接口 | 实现后得到什么 |
|---|---|
| `Stopper` | 退出时被逆序调用，可清理资源 |
| `Enablable` | 界面上出现开关，用户能启用/禁用 |
| `HealthChecker` | 状态页与健康检查会读它 |
| `Schedulable` | 定时任务被调度器接管（v1.1） |
| `SchemaProvider` | **前端自动生成配置表单**，不用写任何表单代码 |
| `RouteProvider` | 自己注册 HTTP 路由，不用改 `routers/` |

启动顺序由 `Register(c, "dep1", "dep2")` 声明，注册表做**拓扑排序**。
上游那些写在注释里的隐式顺序（例如「Cert 排在 STUN 前面，洞口终结 TLS 时要用它的证书」）
从此变成代码里能校验的约束 —— 有环会直接 panic 报出环上的组件名。

### `provider` —— 加服务商

老写法是 `modules/ddns/dns/dns_build_provider.go` 里的一个大 `switch`，
加一家 DNS 商要改三处后端 + 一处前端。

现在：**新增一家 = 一个文件**，把凭据字段声明在 `Descriptor.Credentials` 里，
前端按声明自动出表单，脱敏、校验、能力判断都由框架统一处理。

同一个机制复用到通知渠道与存储后端 —— 因为它们的问题结构完全一样。

### `bus` —— 模块之间通信

上游用「发布方自己维护一组全局回调」的做法（见 `modules/stun/hooks.go`），
问题在于：发布方被迫知道「有多少种别人关心我」，而且接线要在 `app.go` 里手写。

总线之后，发布方只管 `Publish`，订阅方只管 `Subscribe`，**谁都不需要认识对方**。
主题集中在 `core/bus/topics.go` 里声明成常量 —— 这样「改了发布方忘了改订阅方」
可以用类型检查提前发现，而不是等运行时发现事件没人收。

## 设计边界（有意不做的）

- **不加消息队列 / 持久化 / 重试**：总线是通知机制，不是任务系统。
  需要可靠执行的任务用 `component.Task`。
- **不用 Go 的 `plugin` 包**：ABI 与 toolchain 强绑定、Windows/macOS 不支持、
  与 GPL 的动态链接许可存在争议、且崩溃不隔离。外部插件走独立进程 + stdio JSON-RPC（v2.0）。
- **`core` 不 import `modules`**：依赖方向永远是 `modules → core`。
  `provider.Build` 返回 `any` 而不是某个具体接口，就是为了守住这条线。

## 迁移进度

| 模块 | 已接入组件注册表 | 备注 |
|---|---|---|
| `webhook` | ☐ | 计划第一个接入（最小） |
| `home` | ☐ | |
| `stun` | ☐ | 最后一个（最大，且有最多 hook） |
| 其余模块 | ☐ | 沿用上游的 `initModule(...)` 方式 |

迁移策略是**加适配层，不重写模块**：每个模块只新增一个 60~120 行的 `component.go`，
模块内部零改动。每接一个跑一次 `docs/design/` 里的路线图与回归清单。

服务商注册表同理：老的 `switch` 继续能跑，**新增一律走注册表**，全部迁完之后再删 `switch`。
