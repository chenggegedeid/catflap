package task

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/chenggegedeid/catflap/core/component"
	"github.com/chenggegedeid/catflap/modules/wol"
)

// TestMain 把组件开关状态指到临时目录。
//
// component.StatePath 默认是 config/components.json，是相对当前工作目录的。
// 测试的工作目录是包目录（modules/task），直接写会在源码树里留下 config/，
// 更糟的是会把开发者本机「关掉了哪些功能」改掉。
//
// 必须在 m.Run 之前设置：switchState.ensureLoaded 是懒加载且只加载一次，
// 等某个用例先碰过开关再改路径就来不及了。
//
// 另外：RunDue → RunNow 会 SaveConfig，而 task.ConfigPath 是常量指不过来，
// 那份文件照旧落在 modules/task/config/ 下 —— 已加进 .gitignore，
// 处理方式与 modules/stun/test/config/ 一致。
func TestMain(m *testing.M) {
	dir, err := os.MkdirTemp("", "catflap-task-switch-*")
	if err != nil {
		panic(err)
	}
	component.StatePath = filepath.Join(dir, "components.json")
	code := m.Run()
	_ = os.RemoveAll(dir)
	os.Exit(code)
}

// TestRunDueSkipsWhenTaskComponentDisabled 关掉「计划任务」之后，定时任务一个都不许跑。
//
// 这是「关掉功能就真的不干活」这条语义的端到端证据。光看接口 404 说明不了
// 问题：调度循环是常驻 goroutine，开关翻位不会把它停掉 —— 曾经
// modules/task/component.go 的注释还写着「关掉会停掉调度循环」，那是错的。
func TestRunDueSkipsWhenTaskComponentDisabled(t *testing.T) {
	// 先确认前提：组件开着的时候，这条任务是会跑的。
	// 少了这一步，后面「关掉之后没跑」可能只是因为任务压根不到期。
	setTasks(t, Task{
		ID: "t1", Name: "唤醒", Enabled: true,
		Kind: KindInterval, IntervalMin: 30, Action: ActionWOL, Target: "不存在的机器",
	})

	RunDue(time.Now())
	if _, ran := Runtime.lastRun["t1"]; !ran {
		t.Fatal("前提不成立：组件启用时任务就该执行，但 lastRun 没记录")
	}

	// 关掉组件，清掉记录再来一次
	if err := component.SetEnabled(Name, false); err != nil {
		t.Fatalf("关闭 task 组件失败：%v", err)
	}
	t.Cleanup(func() {
		if err := component.SetEnabled(Name, true); err != nil {
			t.Logf("恢复 task 组件失败：%v", err)
		}
	})

	Runtime.mu.Lock()
	delete(Runtime.lastRun, "t1")
	Runtime.mu.Unlock()

	RunDue(time.Now())

	Runtime.mu.RLock()
	_, ran := Runtime.lastRun["t1"]
	Runtime.mu.RUnlock()
	if ran {
		t.Error("组件已关闭，RunDue 不该执行任何任务")
	}
}

// TestExecuteSkipsWhenTargetComponentDisabled 关掉 WOL 之后，「定时唤醒」不该再发魔术包。
//
// 计划任务自己开着、但它依赖的那个组件关了 —— 这种组合最容易漏：
// 用户关掉「网络唤醒」，界面上任务列表还挂着，后台照发不误。
func TestExecuteSkipsWhenTargetComponentDisabled(t *testing.T) {
	task := Task{
		ID: "t1", Name: "唤醒", Enabled: true,
		Kind: KindInterval, IntervalMin: 30, Action: ActionWOL, Target: "不存在的机器",
	}

	// 前提：开着的时候，错误来自「找不到目标机器」，说明确实走到了发包的门口
	if err := execute(task); err == nil || !strings.Contains(err.Error(), "不存在") {
		t.Fatalf("前提不成立：组件启用时应报「机器不存在」，实际 %v", err)
	}

	if err := component.SetEnabled(wol.Name, false); err != nil {
		t.Fatalf("关闭 wol 组件失败：%v", err)
	}
	t.Cleanup(func() {
		if err := component.SetEnabled(wol.Name, true); err != nil {
			t.Logf("恢复 wol 组件失败：%v", err)
		}
	})

	err := execute(task)
	if err == nil {
		t.Fatal("wol 已关闭，execute 应该报错跳过")
	}
	if !strings.Contains(err.Error(), "已关闭") {
		t.Errorf("错误信息应该说明是功能关闭导致的跳过，实际：%v", err)
	}
}

// TestActionComponent 动作与组件的对应关系。
//
// 加新动作时这里会红，逼你想清楚「它依赖哪个组件」——
// 漏登记的话，关掉那个组件之后这个动作还会照跑。
func TestActionComponent(t *testing.T) {
	cases := map[ActionKind]string{
		ActionDDNS: "ddns",
		ActionWOL:  "wol",
	}
	for action, want := range cases {
		if got := actionComponent(action); got != want {
			t.Errorf("actionComponent(%q) = %q，期望 %q", action, got, want)
		}
	}
	if got := actionComponent(ActionKind("不存在的动作")); got != "" {
		t.Errorf("未登记的动作应返回空串（表示不限制），实际 %q", got)
	}
}

// setTasks 装一批任务进运行期，并在用例结束时清掉。
func setTasks(t *testing.T, tasks ...Task) {
	t.Helper()
	Runtime.mu.Lock()
	Runtime.Config.Tasks = tasks
	Runtime.lastRun = map[string]time.Time{}
	Runtime.running = map[string]bool{}
	Runtime.mu.Unlock()

	t.Cleanup(func() {
		Runtime.mu.Lock()
		Runtime.Config.Tasks = nil
		Runtime.lastRun = map[string]time.Time{}
		Runtime.running = map[string]bool{}
		Runtime.mu.Unlock()
	})
}
