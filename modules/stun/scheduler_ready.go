package stun

// SchedulerReady 调度器是否可以用了。
//
// Runtime.Scheduler 在 stun 组件初始化时才建（init_stun.go）。
// 在那之前（以及**初始化失败时**）它是 nil —— 而 HTTP 早就开始监听了，
// 用户在启动的头几秒里点穿透页就会撞上空指针：
// 每次重启日志里都有一两条「panic recovered: nil pointer dereference」，
// 堆栈指向 scheduler.go 的 Subscribe / StartService。
//
// 以前只有 /api/stun/status 判了空（那里最先被踩到），其它调用点没判。
// 统一从这里判：没就绪就别调方法，返回一句话让界面重试，而不是崩。
func SchedulerReady() bool {
	return Runtime.Scheduler != nil
}
