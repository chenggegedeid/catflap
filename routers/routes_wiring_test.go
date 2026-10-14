package routers

import (
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"

	"github.com/chenggegedeid/catflap/core/component"

	"github.com/gin-gonic/gin"
)

// fetchCode 发一个 GET 只看状态码。测试里只关心「这个路径还能不能访问」，
// 不关心响应体 —— 关掉的组件返回的正是与「路径不存在」一模一样的 404。
func fetchCode(t *testing.T, r *gin.Engine, path string) int {
	t.Helper()
	w := httptest.NewRecorder()
	r.ServeHTTP(w, httptest.NewRequest(http.MethodGet, path, nil))
	return w.Code
}

// redirectComponentState 把开关状态的重定向到临时目录。
//
// 不这么做的话，测试会把 config/components.json 写进**仓库目录**
// （测试的工作目录就是包目录），跑一次测试就在源码树里留下垃圾，
// 更糟的是它还会被下一次运行读到。
func redirectComponentState(t *testing.T) {
	t.Helper()
	old := component.StatePath
	component.StatePath = filepath.Join(t.TempDir(), "components.json")
	t.Cleanup(func() { component.StatePath = old })
}

// buildAPIEngine 复刻 routers.Run 里挂 API 的那几步，但不启服务器。
//
// 这里刻意不复用 Run：Run 会 ListenAndServe 把测试卡住。代价是这段构造逻辑
// 有两份，所以下面的断言要盯住「路由到底挂上了没有」这个结果，
// 而不是盯实现方式 —— 这样即使将来 Run 里的写法变了，测试依然有意义。
func buildAPIEngine(t *testing.T) *gin.Engine {
	t.Helper()
	gin.SetMode(gin.TestMode)
	r := gin.New()
	g := r.Group("api")
	// 测试里不挂 AuthMiddleware：我们只数路由，不发真实请求，
	// 挂上反而要求 auth 运行时已初始化。
	if _, err := component.MountRoutes(component.RouterGroups{
		Public:    g.Group(""),
		Protected: g.Group(""),
	}); err != nil {
		t.Fatalf("挂载组件路由失败：%v", err)
	}
	return r
}

// TestEveryRouteContributionHasComponent 路由贡献的名字必须真的有同名组件。
//
// 名字写错的表现是「整套接口 404、日志里毫无线索」，很难查。Run 里已经会让它
// 启动失败，这条测试的作用是把它挡在更前面 —— 不让一个必然起不来的版本
// 走到 push。
func TestEveryRouteContributionHasComponent(t *testing.T) {
	registered := map[string]bool{}
	for _, n := range component.Names() {
		registered[n] = true
	}
	for _, n := range component.ContributedRouteNames() {
		if !registered[n] {
			t.Errorf("路由贡献 %q 没有对应的组件注册（名字写错了？）", n)
		}
	}
}

// TestAllComponentsHaveRoutes 反向检查：每个组件都应当有接口。
//
// 不变量是「组件名 ↔ 路由贡献」一一对应。漏掉一条的表现是新功能装上之后
// 界面调接口 404，而且因为组件已经注册、启动日志里一切正常，非常难定位。
func TestAllComponentsHaveRoutes(t *testing.T) {
	contributed := map[string]bool{}
	for _, n := range component.ContributedRouteNames() {
		contributed[n] = true
	}
	for _, n := range component.Names() {
		if !contributed[n] {
			t.Errorf("组件 %q 没有任何路由贡献：要么忘了写 routers/<x>_routers.go 里的 init，要么该组件本来就无接口（那就把它从组件表里去掉）", n)
		}
	}
}

// TestMountedRoutesCoverEveryModule 每个模块的接口都要真的出现在路由表上。
//
// 上面的两条只检查「名字对得上」，检查不出「函数体里忘了调 XxxRouters」。
// 这条按前缀数一遍，能抓住整块 API 丢失。
func TestMountedRoutesCoverEveryModule(t *testing.T) {
	r := buildAPIEngine(t)

	got := make([]string, 0, len(r.Routes()))
	for _, route := range r.Routes() {
		got = append(got, route.Path)
	}
	hasPrefix := func(p string) bool {
		for _, g := range got {
			if strings.HasPrefix(g, p) {
				return true
			}
		}
		return false
	}

	want := []string{
		"/api/auth/",
		"/api/home/",
		"/api/webhook/",
		"/api/cert/",
		"/api/stun/",
		"/api/ddns/",
		"/api/proxy/",
		"/api/system/",
		"/api/version",
	}
	for _, p := range want {
		if !hasPrefix(p) {
			t.Errorf("路由表里找不到 %s* 的接口，可能这一块根本没有挂上去", p)
		}
	}
}

// TestDisabledComponentRoutesReturn404 关掉一个组件之后，它的接口要 404。
//
// 这是 A1「组件开关」这条语义的端到端证据：不是看 EnabledGuard 的单元测试
// 过了，而是看**真实路由表**上的接口是否真的拒绝服务。
func TestDisabledComponentRoutesReturn404(t *testing.T) {
	redirectComponentState(t)

	// 拿 webhook 做样本：它是最小的模块，关掉不影响别的接口。
	const target = "/api/webhook/templates"

	r := buildAPIEngine(t)
	if code := fetchCode(t, r, target); code == 404 {
		t.Fatalf("%s 在组件启用时就已经 404 了，测试前提不成立", target)
	}

	if err := component.SetEnabled("webhook", false); err != nil {
		t.Fatalf("关闭 webhook 组件失败：%v", err)
	}
	t.Cleanup(func() {
		if err := component.SetEnabled("webhook", true); err != nil {
			t.Logf("恢复 webhook 组件失败：%v", err)
		}
	})

	if code := fetchCode(t, r, target); code != 404 {
		t.Errorf("组件关闭后 %s 应当 404，实际 %d", target, code)
	}
	// 别的组件的接口不能被牵连
	if code := fetchCode(t, r, "/api/ddns/config"); code == 404 {
		t.Error("关掉 webhook 不该影响 ddns 的接口")
	}
}
