package component

// Descriptor 一个组件对外暴露的元信息，供两个地方用：
//
//   - 界面的「功能管理」页：列出所有组件、显示是否能开关、当前是否启用；
//   - 配置表单：Schema 非空时，前端**不需要为该组件写任何表单代码**。
//
// 它由注册表从组件实例上探测可选接口**自动拼出来**，组件不需要自己实现 Descriptor。
type Descriptor struct {
	// Name 组件名。
	Name string `json:"name"`

	// Schema 配置结构；组件未实现 SchemaProvider 时为 nil。
	Schema *Schema `json:"schema,omitempty"`

	// Enableable 该组件是否支持在界面上开关（即是否实现了 Enablable）。
	Enableable bool `json:"enableable"`

	// Enabled 当前是否启用。不可开关的组件恒为 true —— 它们开不了也关不掉，
	// 语义上就是「永远开着」。
	//
	// 这里不能用零值 false 充当「无意义」：接口的消费者（功能管理页、
	// 侧栏过滤）分不出「真的被关了」和「不支持开关」，会把 auth / home /
	// system 误报成已关闭。这条是活体验收时真踩到的。
	Enabled bool `json:"enabled"`

	// Tasks 该组件声明的定时任务名（实现了 Schedulable 时非空）。
	Tasks []string `json:"tasks,omitempty"`

	// HealthAware 该组件是否提供健康状态（即是否实现了 HealthChecker）。
	HealthAware bool `json:"healthAware"`
}
