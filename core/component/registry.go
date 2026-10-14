package component

import (
	"context"
	"errors"
	"fmt"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/sirupsen/logrus"
)

// RouterGroups 注册表交给组件的路由组。
//
// Public   对应 /api 下无需登录的接口；
// Protected 对应 /api 下经过 AuthMiddleware 的接口。
//
// 组件按需要挂到对应组上；**前端资源、门户跳转等非 /api 路由不在这里**，
// 需要那种路由请单独说明（目前刻意不开放，避免组件绕过统一的路径约定）。
type RouterGroups struct {
	Public    *gin.RouterGroup
	Protected *gin.RouterGroup
}

// DefaultInitTimeout 单个组件 Init 的默认超时。
//
// 设计取舍：超时到点会记 error 并把该组件标记为失败，但不 panic ——
// 一个模块起不来不应该让整个面板打不开。
const DefaultInitTimeout = 2 * time.Minute

type entry struct {
	comp Component
	deps []string
	seq  int // 注册顺序，用于让同层组件按稳定顺序启动
}

// Registry 组件注册表。
//
// 一般不需要自己 new：包级函数操作的就是 Default 注册表，
// 各模块在自己的 init() 里调用 component.MustRegister 即可。
// 需要隔离（例如测试）时用 NewRegistry 建独立实例。
type Registry struct {
	mu      sync.RWMutex
	entries map[string]*entry
	seq     int

	// InitTimeout 单个组件 Init 的超时，零值用 DefaultInitTimeout。
	InitTimeout time.Duration

	// started 已成功 Init 的组件，按启动顺序记录，StopAll 按逆序停止。
	started []Component

	// failed 初始化失败的组件名，供依赖它们的组件判断是否要跳过。
	failed map[string]bool
}

// NewRegistry 建一个独立的注册表（测试用，或需要多套编排时用）。
func NewRegistry() *Registry {
	return &Registry{
		entries: make(map[string]*entry),
		failed:  make(map[string]bool),
	}
}

// Default 全局默认注册表。模块通过包级 Register/MustRegister 注册到这里。
var Default = NewRegistry()

// ── 注册 ────────────────────────────────────────────────────────────────────

// Register 注册一个组件并声明它依赖的其它组件名。
//
// 依赖只影响**启动顺序**，不构成访问控制：被依赖的组件初始化失败时，
// 依赖方会被跳过（而不是拿到半初始化的状态）。
//
// 重名会返回错误 —— 静默覆盖会让「到底哪个实现了」变成玄学。
func (r *Registry) Register(c Component, dependsOn ...string) error {
	if c == nil {
		return errors.New("注册的组件为 nil")
	}
	name := strings.TrimSpace(c.Name())
	if name == "" {
		return errors.New("组件 Name() 不能为空")
	}

	r.mu.Lock()
	defer r.mu.Unlock()

	if _, dup := r.entries[name]; dup {
		return fmt.Errorf("组件 %q 重复注册", name)
	}

	// 去重依赖，顺便挡掉「依赖自己」
	deps := make([]string, 0, len(dependsOn))
	seen := make(map[string]bool, len(dependsOn))
	for _, d := range dependsOn {
		d = strings.TrimSpace(d)
		if d == "" {
			continue
		}
		if d == name {
			return fmt.Errorf("组件 %q 不能依赖自己", name)
		}
		if seen[d] {
			continue
		}
		seen[d] = true
		deps = append(deps, d)
	}

	r.seq++
	r.entries[name] = &entry{comp: c, deps: deps, seq: r.seq}
	return nil
}

// MustRegister 注册失败直接 panic。
//
// 只在包的 init() 里用：重名或依赖写错是**开发期的错误**，
// 应该让程序在启动瞬间就崩掉，而不是带着错误的编排跑起来。
func (r *Registry) MustRegister(c Component, dependsOn ...string) {
	if err := r.Register(c, dependsOn...); err != nil {
		panic(fmt.Sprintf("core/component: %v", err))
	}
}

// ── 查询 ────────────────────────────────────────────────────────────────────

// Get 按名字取组件。
func (r *Registry) Get(name string) (Component, bool) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	e, ok := r.entries[name]
	if !ok {
		return nil, false
	}
	return e.comp, true
}

// Names 返回**启动顺序**下的全部组件名。
//
// 排序失败（例如存在环）时退回按注册顺序返回，并把错误写到日志 ——
// 这个方法是给界面和日志用的，不应该因为编排有问题就整个不可用。
func (r *Registry) Names() []string {
	ordered, err := r.resolve()
	if err != nil {
		logrus.Errorf("core/component: 解析组件顺序失败：%v", err)
		return r.namesBySeq()
	}
	names := make([]string, 0, len(ordered))
	for _, e := range ordered {
		names = append(names, e.comp.Name())
	}
	return names
}

func (r *Registry) namesBySeq() []string {
	r.mu.RLock()
	defer r.mu.RUnlock()
	es := make([]*entry, 0, len(r.entries))
	for _, e := range r.entries {
		es = append(es, e)
	}
	sort.Slice(es, func(i, j int) bool { return es[i].seq < es[j].seq })
	names := make([]string, 0, len(es))
	for _, e := range es {
		names = append(names, e.comp.Name())
	}
	return names
}

// Healths 汇总所有实现了 HealthChecker 的组件的状态。
// 没实现 HealthChecker 的组件不会出现在结果里（用 component.List 可以看出差别）。
func (r *Registry) Healths() map[string]Health {
	out := map[string]Health{}
	for _, c := range r.Components() {
		if hc, ok := c.(HealthChecker); ok {
			out[c.Name()] = hc.Health()
		}
	}
	return out
}

// Components 返回**启动顺序**下的全部组件实例。
func (r *Registry) Components() []Component {
	ordered, err := r.resolve()
	if err != nil {
		logrus.Errorf("core/component: 解析组件顺序失败：%v", err)
		return nil
	}
	out := make([]Component, 0, len(ordered))
	for _, e := range ordered {
		out = append(out, e.comp)
	}
	return out
}

// Descriptors 汇总所有实现了 SchemaProvider / Enablable / Schedulable 的组件信息，
// 供界面渲染「功能开关」页与自动生成配置表单。
func (r *Registry) Descriptors() []Descriptor {
	var out []Descriptor
	for _, c := range r.Components() {
		d := Descriptor{Name: c.Name()}
		if sp, ok := c.(SchemaProvider); ok {
			s := sp.ConfigSchema()
			d.Schema = &s
		}
		if en, ok := c.(Enablable); ok {
			d.Enableable = true
			d.Enabled = en.Enabled()
		} else {
			// 不可开关的组件 = 永远开着。不写这句就停在零值 false，
			// 而接口消费者分不出「真的被关了」与「不支持开关」，
			// 会把 auth / home / system 当作已关闭列出来。
			d.Enabled = true
		}
		if sc, ok := c.(Schedulable); ok {
			for _, t := range sc.Tasks() {
				d.Tasks = append(d.Tasks, t.Name)
			}
		}
		if _, ok := c.(HealthChecker); ok {
			d.HealthAware = true
		}
		out = append(out, d)
	}
	return out
}

// ── 启动与停止 ──────────────────────────────────────────────────────────────

// InitAll 按依赖顺序初始化所有组件。
//
// 行为约定：
//   - **分层并发**：同一层（互相之间没有依赖）的组件并发初始化，有依赖的严格
//     后发。之所以不是纯串行：stun 的 Init 要探测出口网卡、挑 STUN 服务器，
//     是几秒级的网络操作，串行会让它后面每个模块都白等它 —— 改造前 app.go
//     就是并发起的，不能因为上了注册表反而变慢。之所以不是全并发：
//     「cert 必须先于 stun」这类约束得靠依赖声明说话，不能靠运气。
//   - 某个组件失败时**不中断**其它组件，但会跳过所有依赖它的组件；
//   - 结束时返回聚合 error（nil 表示全部成功），逐个失败的原因都在日志里。
func (r *Registry) InitAll(ctx context.Context) error {
	ordered, layers, err := r.resolveLayers()
	if err != nil {
		return err
	}
	_ = ordered

	timeout := r.InitTimeout
	if timeout <= 0 {
		timeout = DefaultInitTimeout
	}

	r.mu.Lock()
	r.started = r.started[:0]
	r.failed = make(map[string]bool)
	r.mu.Unlock()

	var (
		errsMu sync.Mutex
		errs   []error
	)

	for _, layer := range layers {
		// 层内并发。每个 goroutine 只写自己的下标，不需要锁。
		started := make([]Component, len(layer))
		ok := make([]bool, len(layer))
		var wg sync.WaitGroup

		for i, e := range layer {
			name := e.comp.Name()

			if skipped, dep := r.blockedBy(name, e.deps); skipped {
				logrus.Warnf("组件 %s 跳过初始化：依赖的 %s 未就绪", name, dep)
				r.markFailed(name)
				continue
			}

			wg.Add(1)
			go func(i int, e *entry) {
				defer wg.Done()
				name := e.comp.Name()

				cctx, cancel := context.WithTimeout(ctx, timeout)
				defer cancel()

				logrus.Infof("组件 %s 开始初始化", name)
				initErr := e.comp.Init(cctx)
				if initErr != nil {
					logrus.Errorf("组件 %s 初始化失败：%v", name, initErr)
					errsMu.Lock()
					errs = append(errs, fmt.Errorf("%s: %w", name, initErr))
					errsMu.Unlock()
					r.markFailed(name)
					return
				}
				logrus.Infof("组件 %s 初始化完成", name)
				started[i] = e.comp
				ok[i] = true
			}(i, e)
		}

		wg.Wait()

		// 按注册顺序登记，让 StopAll 的逆序是确定的
		r.mu.Lock()
		for i := range layer {
			if ok[i] {
				r.started = append(r.started, started[i])
			}
		}
		r.mu.Unlock()
	}

	return errors.Join(errs...)
}

// StopAll 逆序停止已启动的组件，尽量停完再汇总错误。
func (r *Registry) StopAll(ctx context.Context) error {
	r.mu.Lock()
	started := make([]Component, len(r.started))
	copy(started, r.started)
	r.mu.Unlock()

	var errs []error
	for i := len(started) - 1; i >= 0; i-- {
		c := started[i]
		s, ok := c.(Stopper)
		if !ok {
			continue
		}
		if err := s.Stop(ctx); err != nil {
			logrus.Errorf("组件 %s 停止失败：%v", c.Name(), err)
			errs = append(errs, fmt.Errorf("%s: %w", c.Name(), err))
		}
	}
	return errors.Join(errs...)
}

func (r *Registry) markFailed(name string) {
	r.mu.Lock()
	r.failed[name] = true
	r.mu.Unlock()
}

// blockedBy 判断该组件的依赖里是否有失败的，返回第一个失败的依赖名。
func (r *Registry) blockedBy(name string, deps []string) (bool, string) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	for _, d := range deps {
		if r.failed[d] {
			return true, d
		}
	}
	return false, ""
}

// ── 拓扑排序 ────────────────────────────────────────────────────────────────

// resolve 返回按依赖排序后的组件列表。
//
// 同层按注册顺序排列（用 seq 做稳定基准），保证每次启动顺序一致 ——
// 随机顺序会让「偶尔起不来」这种问题变得无法复现。
func (r *Registry) resolve() ([]*entry, error) {
	ordered, _, err := r.resolveLayers()
	return ordered, err
}

// resolveLayers 同时给出拓扑序与分层结果。
//
// 分层用于「同层并发、跨层串行」的初始化（见 InitAll）。分层的定义：
// 同一层里的组件互相之间没有依赖，任何一层都必须等前面所有层完成。
func (r *Registry) resolveLayers() ([]*entry, [][]*entry, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	// 1) 依赖必须存在
	for name, e := range r.entries {
		for _, d := range e.deps {
			if _, ok := r.entries[d]; !ok {
				return nil, nil, fmt.Errorf("组件 %q 依赖 %q，但 %q 没有注册", name, d, d)
			}
		}
	}

	// 2) 入度与邻接表：dep → name
	indeg := make(map[string]int, len(r.entries))
	children := make(map[string][]string, len(r.entries))
	for name := range r.entries {
		indeg[name] = 0
	}
	for name, e := range r.entries {
		for _, d := range e.deps {
			indeg[name]++
			children[d] = append(children[d], name)
		}
	}

	// 3) 拓扑排序（Kahn），就绪队列按注册顺序取
	ready := make([]string, 0, len(r.entries))
	for name, deg := range indeg {
		if deg == 0 {
			ready = append(ready, name)
		}
	}
	sortBySeq(ready, r.entries)

	var layers [][]*entry
	out := make([]*entry, 0, len(r.entries))
	for len(ready) > 0 {
		// 本层确定下来：ready 里这些互相没有依赖
		layer := make([]*entry, 0, len(ready))
		for _, name := range ready {
			layer = append(layer, r.entries[name])
			out = append(out, r.entries[name])
		}
		layers = append(layers, layer)

		next := make([]string, 0, len(ready))
		for _, name := range ready {
			for _, child := range children[name] {
				indeg[child]--
				if indeg[child] == 0 {
					next = append(next, child)
				}
			}
		}
		sortBySeq(next, r.entries)
		ready = next
	}

	// 4) 有环：把还在环上的组件名报出来，方便直接改代码
	if len(out) != len(r.entries) {
		var stuck []string
		for name, deg := range indeg {
			if deg > 0 {
				stuck = append(stuck, name)
			}
		}
		sortBySeq(stuck, r.entries)
		return nil, nil, fmt.Errorf("组件依赖存在环，涉及：%s", strings.Join(stuck, " → "))
	}

	return out, layers, nil
}

func sortBySeq(names []string, entries map[string]*entry) {
	sort.SliceStable(names, func(i, j int) bool {
		return entries[names[i]].seq < entries[names[j]].seq
	})
}

// SetEnabled 开关一个组件。
//
// 走注册表而不是直接改状态文件，是为了多一点校验：组件必须存在、且真的实现了
// Enablable。否则用户对着界面点开关，后端只会默默写下一个谁都不认的名字。
func (r *Registry) SetEnabled(name string, on bool) error {
	c, ok := r.Get(name)
	if !ok {
		return fmt.Errorf("组件 %q 不存在", name)
	}
	en, ok := c.(Enablable)
	if !ok {
		return fmt.Errorf("组件 %q 不支持开关", name)
	}
	return en.SetEnabled(on)
}

// IsEnabled 组件当前是否启用。
//
// 未注册、或未实现 Enablable 的组件一律返回 true —— 不能关的东西就是开着的，
// 调用方（路由闸门、界面）不需要为这两种情况各写一个分支。
//
// ⚠️ 组件的 Enablable 实现里**不能**调这个函数（也不能调包级的 IsEnabled）：
// 它会回过头调用组件的 Enabled()，两边互相调用会直接把栈跑爆。
// 组件应当内嵌 component.LocalSwitch，见 state.go。
func (r *Registry) IsEnabled(name string) bool {
	c, ok := r.Get(name)
	if !ok {
		return true
	}
	en, ok := c.(Enablable)
	if !ok {
		return true
	}
	return en.Enabled()
}

// ── 包级快捷方法（操作 Default 注册表）─────────────────────────────────────

// Register 把组件注册到 Default 注册表。
func Register(c Component, dependsOn ...string) error {
	return Default.Register(c, dependsOn...)
}

// MustRegister 注册到 Default 注册表，失败 panic。在 init() 里用。
func MustRegister(c Component, dependsOn ...string) {
	Default.MustRegister(c, dependsOn...)
}

// InitAll 初始化 Default 注册表里的全部组件。
func InitAll(ctx context.Context) error {
	return Default.InitAll(ctx)
}

// StopAll 停止 Default 注册表里的全部组件。
func StopAll(ctx context.Context) error {
	return Default.StopAll(ctx)
}

// Get 从 Default 注册表按名字取组件。
func Get(name string) (Component, bool) {
	return Default.Get(name)
}

// Names 返回 Default 注册表里的组件名（启动顺序）。
func Names() []string {
	return Default.Names()
}

// Descriptors 返回 Default 注册表里全部组件的元信息（启动顺序）。
func Descriptors() []Descriptor {
	return Default.Descriptors()
}

// Healths 返回 Default 注册表里全部组件的健康状态（只有实现了 HealthChecker 的会出现）。
func Healths() map[string]Health {
	return Default.Healths()
}

// SetEnabled 开关 Default 注册表里的组件。
func SetEnabled(name string, on bool) error {
	return Default.SetEnabled(name, on)
}

// IsEnabled 判断 Default 注册表里的组件当前是否启用。
func IsEnabled(name string) bool {
	return Default.IsEnabled(name)
}
