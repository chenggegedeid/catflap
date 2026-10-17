package component

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"sync/atomic"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
)

// ── 测试用的假组件 ──────────────────────────────────────────────────────────

// switchableComponent 实现全部可选接口，用来验证「实现了就参与对应环节」。
type switchableComponent struct {
	name    string
	initErr error
	enabled bool
	health  Health
	stopped bool
}

func (d *switchableComponent) Name() string               { return d.name }
func (d *switchableComponent) Init(context.Context) error { return d.initErr }
func (d *switchableComponent) Enabled() bool              { return d.enabled }
func (d *switchableComponent) SetEnabled(v bool) error    { d.enabled = v; return nil }
func (d *switchableComponent) Health() Health             { return d.health }
func (d *switchableComponent) Stop(context.Context) error { d.stopped = true; return nil }

// plainComponent 什么都不实现，用来验证「可选能力按需实现，注册表不要求空实现」。
type plainComponent struct{ name string }

func (p plainComponent) Name() string               { return p.name }
func (p plainComponent) Init(context.Context) error { return nil }

// routeDemoComponent 自己实现 RouteProvider，验证「模块内生路」也能挂上。
type routeDemoComponent struct{ plainComponent }

func (r routeDemoComponent) Routes(g RouterGroups) {
	g.Protected.GET("demo/inside", func(c *gin.Context) {
		c.String(http.StatusOK, "inside")
	})
}

// ── 测试隔离 ────────────────────────────────────────────────────────────────

// resetRouteContributions 清空包级的路由贡献表。
//
// 贡献表是包级全局的（路由最终挂在哪个 engine 上由 MountRoutes 的实参决定，
// 与注册表无关，所以没有「每个注册表一份」的版本）。测试之间必须手动隔离，
// 否则前一个测试留下的「名字写错的贡献」会让后一个测试的挂载一并报错。
func resetRouteContributions() {
	routesMu.Lock()
	routeContributors = nil
	routeSeq = 0
	routesMu.Unlock()
}

func isolateRoutes(t *testing.T) {
	t.Helper()
	resetRouteContributions()
	t.Cleanup(resetRouteContributions)
}

// ── 路由贡献与挂载 ──────────────────────────────────────────────────────────

func TestContributeRoutesRejectsBadInput(t *testing.T) {
	isolateRoutes(t)

	if err := ContributeRoutes("dup-demo", func(RouterGroups) {}); err != nil {
		t.Fatalf("第一次贡献应当成功：%v", err)
	}
	if err := ContributeRoutes("dup-demo", func(RouterGroups) {}); err == nil {
		t.Error("同名重复贡献应当报错")
	}
	if err := ContributeRoutes("  ", func(RouterGroups) {}); err == nil {
		t.Error("空名字应当报错")
	}
	if err := ContributeRoutes("nil-demo", nil); err == nil {
		t.Error("nil 路由函数应当报错")
	}
}

// TestMountRoutesAndEnableGuard 是组件开关的语义承诺所在：
// 开着能服务，关掉之后接口 404，重新打开又恢复。
func TestMountRoutesAndEnableGuard(t *testing.T) {
	gin.SetMode(gin.TestMode)
	isolateRoutes(t)

	reg := NewRegistry()
	demo := &switchableComponent{name: "guard-demo", enabled: true}
	if err := reg.Register(demo); err != nil {
		t.Fatal(err)
	}
	if err := ContributeRoutes("guard-demo", func(g RouterGroups) {
		g.Protected.GET("guard-demo/ping", func(c *gin.Context) {
			c.String(http.StatusOK, "pong")
		})
	}); err != nil {
		t.Fatal(err)
	}

	r := gin.New()
	g := r.Group("api")
	mounted, err := reg.MountRoutes(RouterGroups{Public: g.Group(""), Protected: g.Group("")})
	if err != nil {
		t.Fatalf("挂载失败：%v", err)
	}
	if len(mounted) != 1 || mounted[0] != "guard-demo" {
		t.Fatalf("挂载结果 = %v，期望 [guard-demo]", mounted)
	}

	if code := doGet(r, "/api/guard-demo/ping"); code != http.StatusOK {
		t.Fatalf("组件启用时应当 200，实际 %d", code)
	}

	demo.enabled = false
	if code := doGet(r, "/api/guard-demo/ping"); code != http.StatusNotFound {
		t.Fatalf("组件关闭时应当 404，实际 %d", code)
	}

	demo.enabled = true
	if code := doGet(r, "/api/guard-demo/ping"); code != http.StatusOK {
		t.Fatalf("组件重新启用后应当恢复 200，实际 %d", code)
	}
}

// TestMountRoutesSupportsRouteProvider 模块自己实现 RouteProvider 也要能挂上，
// 不必走 routers 包的贡献表。
func TestMountRoutesSupportsRouteProvider(t *testing.T) {
	gin.SetMode(gin.TestMode)
	isolateRoutes(t)

	reg := NewRegistry()
	if err := reg.Register(routeDemoComponent{plainComponent{name: "provider-demo"}}); err != nil {
		t.Fatal(err)
	}

	r := gin.New()
	g := r.Group("api")
	if _, err := reg.MountRoutes(RouterGroups{Public: g.Group(""), Protected: g.Group("")}); err != nil {
		t.Fatal(err)
	}
	if code := doGet(r, "/api/demo/inside"); code != http.StatusOK {
		t.Fatalf("RouteProvider 的路由没挂上，实际 %d", code)
	}
}

// TestMountRoutesReportsDanglingContribution 名字写错必须报出来，不能静默。
func TestMountRoutesReportsDanglingContribution(t *testing.T) {
	isolateRoutes(t)

	reg := NewRegistry()
	if err := reg.Register(plainComponent{name: "real"}); err != nil {
		t.Fatal(err)
	}
	if err := ContributeRoutes("typo-name", func(RouterGroups) {}); err != nil {
		t.Fatal(err)
	}

	if _, err := reg.MountRoutes(RouterGroups{}); err == nil {
		t.Fatal("贡献了路由但没有同名组件，应当返回错误")
	}
}

// TestGuardGroupsOnlyForEnableable 不能开关的组件不该被套上闸门。
func TestGuardGroupsOnlyForEnableable(t *testing.T) {
	gin.SetMode(gin.TestMode)
	reg := NewRegistry()
	if err := reg.Register(plainComponent{name: "plain"}); err != nil {
		t.Fatal(err)
	}
	g := gin.New().Group("")
	out := reg.guardGroups(RouterGroups{Public: g, Protected: g}, "plain")
	if out.Public != g {
		t.Error("未实现 Enablable 的组件不应被套开关闸门")
	}
}

// ── 依赖编排 ────────────────────────────────────────────────────────────────

func TestInitAllRunsDependenciesFirst(t *testing.T) {
	reg := NewRegistry()
	var order []string
	mk := func(name string) Component { return initRecorder{name: name, sink: &order} }

	if err := reg.Register(mk("c"), "b"); err != nil {
		t.Fatal(err)
	}
	if err := reg.Register(mk("b"), "a"); err != nil {
		t.Fatal(err)
	}
	if err := reg.Register(mk("a")); err != nil {
		t.Fatal(err)
	}

	if err := reg.InitAll(context.Background()); err != nil {
		t.Fatal(err)
	}
	want := []string{"a", "b", "c"}
	for i := range want {
		if order[i] != want[i] {
			t.Fatalf("初始化顺序 = %v，期望 %v", order, want)
		}
	}
}

// TestInitAllRunsSameLayerConcurrently 同一层的组件必须并发起，否则
// stun 那几秒的网络探测会把后面每个模块都拖住（改造前的 app.go 就是并发的，
// 不能因为上了注册表反而变慢）。
func TestInitAllRunsSameLayerConcurrently(t *testing.T) {
	reg := NewRegistry()

	const n = 4
	gate := make(chan struct{})
	for i := 0; i < n; i++ {
		if err := reg.Register(blockingComponent{name: string(rune('a' + i)), gate: gate}); err != nil {
			t.Fatal(err)
		}
	}

	done := make(chan error, 1)
	go func() { done <- reg.InitAll(context.Background()) }()

	// 等全部进入 Init 之后才放行。若初始化是串行的，第一个会一直卡住，
	// 计数永远到不了 n，这里就会超时。
	ticker := time.NewTicker(10 * time.Millisecond)
	defer ticker.Stop()
	deadline := time.After(5 * time.Second)
	for {
		select {
		case <-deadline:
			t.Fatal("同层组件没有并发初始化（等不到全部进入 Init）")
		case <-ticker.C:
			if blockingEntered.Load() >= int32(n) {
				close(gate)
				if err := <-done; err != nil {
					t.Fatal(err)
				}
				return
			}
		}
	}
}

// ── 开关状态落盘 ────────────────────────────────────────────────────────────

func isolateSwitchState(t *testing.T) {
	t.Helper()
	oldPath := StatePath
	oldState := switches

	StatePath = filepath.Join(t.TempDir(), "components.json")
	switches = &switchState{}

	t.Cleanup(func() {
		StatePath = oldPath
		switches = oldState
	})
}

func TestSwitchStatePersists(t *testing.T) {
	isolateSwitchState(t)

	if err := RecordEnabled("webhook", false); err != nil {
		t.Fatal(err)
	}
	if switches.isEnabled("webhook") {
		t.Error("关掉之后应当是关闭状态")
	}
	if !switches.isEnabled("audit") {
		t.Error("没碰过的组件应当默认启用（audit）")
	}

	// 模拟进程重启：丢掉内存状态，从文件恢复
	switches = &switchState{}
	if switches.isEnabled("webhook") {
		t.Error("状态应当从文件恢复为关闭")
	}

	// 再打开并确认落盘内容里也没了
	if err := RecordEnabled("webhook", true); err != nil {
		t.Fatal(err)
	}
	switches = &switchState{}
	if !switches.isEnabled("webhook") {
		t.Error("重新打开后应当恢复为启用")
	}

	data, err := os.ReadFile(StatePath)
	if err != nil {
		t.Fatal(err)
	}
	var f stateFile
	if err := json.Unmarshal(data, &f); err != nil {
		t.Fatalf("落盘内容不是合法 JSON：%v\n%s", err, data)
	}
	if len(f.Disabled) != 0 {
		t.Errorf("重新打开后 disabled 应为空，实际 %v", f.Disabled)
	}
}

// TestSwitchStateStoresDisabledNotEnabled 存的是「被关掉的」而不是「开着的」：
// 新增的组件默认就该是开的，存白名单会让新功能在老用户那儿默认不生效。
func TestSwitchStateStoresDisabledNotEnabled(t *testing.T) {
	isolateSwitchState(t)

	if err := RecordEnabled("stun", false); err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(StatePath)
	if err != nil {
		t.Fatal(err)
	}
	var f stateFile
	if err := json.Unmarshal(data, &f); err != nil {
		t.Fatal(err)
	}
	if len(f.Disabled) != 1 || f.Disabled[0] != "stun" {
		t.Errorf("落盘的禁用列表 = %v，期望 [stun]", f.Disabled)
	}
}

// TestSwitchStateToleratesBrokenFile 文件坏掉不该让功能全不可用。
func TestSwitchStateToleratesBrokenFile(t *testing.T) {
	isolateSwitchState(t)

	if err := os.WriteFile(StatePath, []byte("{ 这不是 json"), 0600); err != nil {
		t.Fatal(err)
	}
	if !switches.isEnabled("webhook") {
		t.Error("状态文件损坏时应按全部启用处理")
	}
}

// TestRegistrySetEnabledValidates 开关必须经过注册表，避免写下没人认的名字。
func TestRegistrySetEnabledValidates(t *testing.T) {
	isolateSwitchState(t)

	reg := NewRegistry()
	demo := &switchableComponent{name: "sw", enabled: true}
	if err := reg.Register(demo); err != nil {
		t.Fatal(err)
	}
	// plainComponent 没实现 Enablable，可以注册但不该能开关
	if err := reg.Register(plainComponent{name: "noswitch"}); err != nil {
		t.Fatal(err)
	}

	if err := reg.SetEnabled("sw", false); err != nil {
		t.Fatalf("可开关组件应当允许关闭：%v", err)
	}
	if demo.enabled {
		t.Error("关闭没有生效")
	}
	if err := reg.SetEnabled("nope", false); err == nil {
		t.Error("不存在的组件应当报错")
	}
	if err := reg.SetEnabled("noswitch", false); err == nil {
		t.Error("不支持开关的组件应当报错")
	}
	if !reg.IsEnabled("noswitch") {
		t.Error("不支持开关的组件应当永远视为启用")
	}
	if !reg.IsEnabled("nope") {
		t.Error("未注册的组件应当视为启用")
	}
}

// TestDescriptorsMarkUnswitchableAsEnabled 不可开关的组件 Enabled 必须是 true。
//
// 这条守的是一个**接口语义**而不是内部实现：Descriptor.Enabled 只有 true/false
// 两个值，没有第三种「不适用」。消费者（功能管理页、侧栏过滤）拿它直接判断
// 「是不是被关了」，所以不支持开关的组件必须落在 true —— 停在零值 false 会把
// auth / home / system 误报成已关闭。活体验收（M3）时真出现过这个问题。
func TestDescriptorsMarkUnswitchableAsEnabled(t *testing.T) {
	isolateSwitchState(t)

	reg := NewRegistry()
	if err := reg.Register(plainComponent{name: "always-on"}); err != nil {
		t.Fatal(err)
	}
	if err := reg.Register(&switchableComponent{name: "can-off", enabled: false}); err != nil {
		t.Fatal(err)
	}

	got := map[string]Descriptor{}
	for _, d := range reg.Descriptors() {
		got[d.Name] = d
	}

	if !got["always-on"].Enabled {
		t.Error("不可开关的组件 Enabled 应为 true：false 会被界面当成「已被关闭」")
	}
	if got["always-on"].Enableable {
		t.Error("plainComponent 不该被认成可开关")
	}
	if got["can-off"].Enabled {
		t.Error("可开关且已关闭的组件 Enabled 应为 false")
	}
	if !got["can-off"].Enableable {
		t.Error("实现了 Enablable 的组件应当标记为可开关")
	}
}

// ── 辅助 ────────────────────────────────────────────────────────────────────

func doGet(r *gin.Engine, path string) int {
	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, path, nil)
	r.ServeHTTP(w, req)
	return w.Code
}

type initRecorder struct {
	name string
	sink *[]string
}

func (i initRecorder) Name() string { return i.name }
func (i initRecorder) Init(context.Context) error {
	*i.sink = append(*i.sink, i.name)
	return nil
}

var blockingEntered atomic.Int32

type blockingComponent struct {
	name string
	gate chan struct{}
}

func (b blockingComponent) Name() string { return b.name }
func (b blockingComponent) Init(context.Context) error {
	blockingEntered.Add(1)
	<-b.gate
	return nil
}
