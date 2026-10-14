package component

import (
	"errors"
	"fmt"
	"sort"
	"strings"
	"sync"

	"github.com/gin-gonic/gin"
)

// ── 路由贡献表 ──────────────────────────────────────────────────────────────
//
// # 为什么需要这张表
//
// 组件契约承诺「新增一个功能不用改 routers/enter.go 的清单」。但 modules/<x>
// 自己没法挂路由：路由用到的 handler 定义在 api/<x>_api 里，而 api/<x>_api
// 又 import 了 modules/<x>；modules/<x> 再回头 import api 就是循环依赖。
//
// 所以「挂路由」这件事只能留在 routers 包里，只是不再集中写在一处：
// 每个 routers/<x>_routers.go 在自己的 init() 里 ContributeRoutes("<组件名>", fn)。
// 同一个包内所有文件的 init() 都会在包被加载时执行，enter.go 不需要列清单 ——
// 加一个功能 = 加一个 routers/<x>_routers.go，enter.go 一行都不用动。
//
// # 组件名必须与 Register 的名字一致
//
// 两件事都靠这个名字串起来：挂载顺序跟着组件启动顺序走，而「组件关掉 → 它的
// 接口 404」这条语义也才能作用到正确的路由上。名字写错不会报错、只会让接口
// 全部 404，所以 MountRoutes 遇到对不上的名字会直接 Fatal（见下）。

type routeContribution struct {
	name string
	fn   func(RouterGroups)
	seq  int
}

var (
	routesMu          sync.RWMutex
	routeContributors []routeContribution
	routeSeq          int
)

// ContributeRoutes 给指定名字的组件挂上路由。
//
// 只在 routers 包的 init() 里调用。同一个名字重复贡献会报错 ——
// 静默覆盖会让「到底哪份路由生效」变成玄学。
func ContributeRoutes(name string, fn func(RouterGroups)) error {
	name = strings.TrimSpace(name)
	if name == "" {
		return errors.New("路由贡献的组件名不能为空")
	}
	if fn == nil {
		return fmt.Errorf("组件 %q 的路由函数为 nil", name)
	}

	routesMu.Lock()
	defer routesMu.Unlock()
	for _, c := range routeContributors {
		if c.name == name {
			return fmt.Errorf("组件 %q 重复贡献路由", name)
		}
	}
	routeSeq++
	routeContributors = append(routeContributors, routeContribution{name: name, fn: fn, seq: routeSeq})
	return nil
}

// MustContributeRoutes 贡献失败直接 panic。在 routers 包的 init() 里用。
func MustContributeRoutes(name string, fn func(RouterGroups)) {
	if err := ContributeRoutes(name, fn); err != nil {
		panic(fmt.Sprintf("routers: %v", err))
	}
}

// ContributedRouteNames 已贡献路由的组件名，按贡献顺序。
func ContributedRouteNames() []string {
	routesMu.RLock()
	defer routesMu.RUnlock()
	out := make([]string, 0, len(routeContributors))
	for _, c := range routeContributors {
		out = append(out, c.name)
	}
	return out
}

func contributionOf(name string) (func(RouterGroups), bool) {
	routesMu.RLock()
	defer routesMu.RUnlock()
	for _, c := range routeContributors {
		if c.name == name {
			return c.fn, true
		}
	}
	return nil, false
}

// ── 挂载 ────────────────────────────────────────────────────────────────────

// MountRoutes 把各组件的路由挂到框架给的路由组上，返回实际挂载的组件名。
//
// 挂载顺序就是组件启动顺序；每个组件拿到的是**按自己名字加过开关闸门**的
// 路由组（组件实现了 Enablable 时）。这样组件被关掉之后，它的接口自然 404，
// 不需要各模块自己记得在 handler 里判断开关。
//
// 三种情况都算「该组件有路由」，可以混用：
//   - routers 包用 ContributeRoutes 贡献（file-backed，legacy 模块走这条）；
//   - 组件自己实现 RouteProvider（模块内自带路由的走这条）；
//   - 两者都写 —— 两份都会被调用，适合「模块内生路 + 包外补几条」。
//
// 名字对不上的贡献会**返回 error**（路由已经挂上的部分不回滚，因为没有副作用
// 可以回滚）。调用方应当把它当作致命错误：那种情况下的表现是「整套接口 404
// 但日志里什么都看不出来」，与其让人查半天，不如起不来。
// 这里不用 logrus.Fatal 直接退进程，是因为库不该替调用方决定生死 —— 该退的是
// main，不是包。
func (r *Registry) MountRoutes(g RouterGroups) ([]string, error) {
	var mounted []string

	names := r.Names()
	for _, name := range names {
		fn, hasFn := contributionOf(name)
		comp, _ := r.Get(name)
		rp, hasRP := comp.(RouteProvider)

		if !hasFn && !hasRP {
			continue
		}

		target := r.guardGroups(g, name)
		if hasFn {
			fn(target)
		}
		if hasRP {
			rp.Routes(target)
		}
		mounted = append(mounted, name)
	}

	if dangling := r.danglingContributions(names); len(dangling) > 0 {
		return mounted, fmt.Errorf(
			"以下组件贡献了路由但没有注册同名组件（多半是名字写错）：%v；已注册的组件有：%v",
			dangling, names)
	}

	return mounted, nil
}

func (r *Registry) danglingContributions(registered []string) []string {
	known := make(map[string]bool, len(registered))
	for _, n := range registered {
		known[n] = true
	}
	seen := map[string]bool{}
	var out []string
	for _, n := range ContributedRouteNames() {
		if !known[n] && !seen[n] {
			seen[n] = true
			out = append(out, n)
		}
	}
	sort.Strings(out)
	return out
}

// guardGroups 给路由组套上该组件的开关闸门；组件不可开关时原样返回。
func (r *Registry) guardGroups(g RouterGroups, name string) RouterGroups {
	comp, ok := r.Get(name)
	if !ok {
		return g
	}
	if _, enableable := comp.(Enablable); !enableable {
		return g
	}
	guard := r.EnabledGuard(name)
	out := RouterGroups{}
	if g.Public != nil {
		out.Public = g.Public.Group("", guard)
	}
	if g.Protected != nil {
		out.Protected = g.Protected.Group("", guard)
	}
	return out
}

// EnabledGuard 组件被关掉时让请求以 404 结束。
//
// 用 404 而不是 403/500：403 等于告诉对方「这里确实有个功能，只是不给你用」，
// 对未授权者是额外信息；404 与「这个路径本来就不存在」完全一致。
// 响应体也照抄 NoRoute 的 {"error":"not found"}，不给探测者留下区分依据。
//
// 判断读的是**本注册表**的状态，不是全局 Default —— 挂到哪个注册表上就听谁的，
// 否则测试里建的独立注册表会与全局状态互相干扰，更糟的是多套编排时会串台。
func (r *Registry) EnabledGuard(name string) gin.HandlerFunc {
	return func(c *gin.Context) {
		if !r.IsEnabled(name) {
			c.AbortWithStatusJSON(404, gin.H{"error": "not found"})
			return
		}
		c.Next()
	}
}

// EnabledGuard 用 Default 注册表的开关状态做闸门。
func EnabledGuard(name string) gin.HandlerFunc { return Default.EnabledGuard(name) }

// ── 包级快捷方法 ────────────────────────────────────────────────────────────

// MountRoutes 把 Default 注册表里的组件路由挂到给定的路由组上。
func MountRoutes(g RouterGroups) ([]string, error) {
	return Default.MountRoutes(g)
}
