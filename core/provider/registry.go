package provider

import (
	"fmt"
	"sort"
	"sync"

	"github.com/sirupsen/logrus"
)

// Registry 服务商注册表。
type Registry struct {
	mu    sync.RWMutex
	kinds map[Kind]map[string]Descriptor
	seq   map[Kind][]string // 每个类别内的注册顺序，保证界面列表顺序稳定
}

// NewRegistry 建一个独立的注册表（测试用）。
func NewRegistry() *Registry {
	return &Registry{
		kinds: make(map[Kind]map[string]Descriptor),
		seq:   make(map[Kind][]string),
	}
}

// Default 全局默认注册表。各服务商在自己的 register_*.go 里注册到这里。
var Default = NewRegistry()

// Register 注册一个服务商。同类别同 Key 重复注册会报错。
func (r *Registry) Register(d Descriptor) error {
	if err := d.validate(); err != nil {
		return err
	}

	r.mu.Lock()
	defer r.mu.Unlock()

	if r.kinds[d.Kind] == nil {
		r.kinds[d.Kind] = make(map[string]Descriptor)
	}
	if _, dup := r.kinds[d.Kind][d.Key]; dup {
		return fmt.Errorf("%s 服务商 %q 重复注册", d.Kind, d.Key)
	}

	r.kinds[d.Kind][d.Key] = d
	r.seq[d.Kind] = append(r.seq[d.Kind], d.Key)
	return nil
}

// MustRegister 注册失败直接 panic。在 init() 里用：
// Key 写错、Build 忘写这类问题应该在启动瞬间暴露，而不是等用户点了保存才发现。
func (r *Registry) MustRegister(d Descriptor) {
	if err := r.Register(d); err != nil {
		panic(fmt.Sprintf("core/provider: %v", err))
	}
}

// Get 按类别与 Key 取服务商。
func (r *Registry) Get(kind Kind, key string) (Descriptor, bool) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	d, ok := r.kinds[kind][key]
	return d, ok
}

// List 列出某类别下的全部服务商，按 Order 升序；Order 相同则按注册先后。
func (r *Registry) List(kind Kind) []Descriptor {
	r.mu.RLock()
	defer r.mu.RUnlock()

	keys := r.seq[kind]
	out := make([]Descriptor, 0, len(keys))
	for _, k := range keys {
		out = append(out, r.kinds[kind][k])
	}
	// SliceStable：Order 全为 0（都没指定）时，顺序就是注册顺序，与老行为一致
	sort.SliceStable(out, func(i, j int) bool { return out[i].Order < out[j].Order })
	return out
}

// Exists 该类别下是否注册了指定的 Key。
func (r *Registry) Exists(kind Kind, key string) bool {
	_, ok := r.Get(kind, key)
	return ok
}

// ListAll 列出全部类别的服务商，供界面一次拉完。
func (r *Registry) ListAll() map[Kind][]Descriptor {
	out := make(map[Kind][]Descriptor, len(AllKinds()))
	for _, k := range AllKinds() {
		if l := r.List(k); len(l) > 0 {
			out[k] = l
		}
	}
	return out
}

// ExistsDefault 判断 Default 注册表里是否存在该服务商。
func ExistsDefault(kind Kind, key string) bool { return Default.Exists(kind, key) }

// Keys 列出某类别下的全部 Key（有序）。
func (r *Registry) Keys(kind Kind) []string {
	r.mu.RLock()
	defer r.mu.RUnlock()
	out := make([]string, len(r.seq[kind]))
	copy(out, r.seq[kind])
	return out
}

// Capable 判断某服务商是否同时具备给定的全部能力。
//
// 用法：判断能不能给这个域名签通配符证书 ——
//
//	if !provider.Capable(provider.KindDNS, p.Type, provider.CapDNS01) {
//	    return errors.New("该服务商不支持 DNS-01，请换一家或改用 HTTP-01")
//	}
func (r *Registry) Capable(kind Kind, key string, caps ...Capability) bool {
	d, ok := r.Get(kind, key)
	if !ok {
		return false
	}
	for _, c := range caps {
		if !d.Has(c) {
			return false
		}
	}
	return true
}

// Build 构造服务商实例。
//
// 这是老代码里 BuildClient / BuildACMEClient / BuildRedirectClient 的统一入口，
// 调用方拿到 any 之后自行断言成本模块需要的接口。
func (r *Registry) Build(kind Kind, key string, cred map[string]any) (any, error) {
	d, ok := r.Get(kind, key)
	if !ok {
		return nil, fmt.Errorf("未知的 %s 服务商：%s", kind, key)
	}
	if err := d.Validate(cred); err != nil {
		return nil, fmt.Errorf("%s 的凭据不完整：%w", d.Name, err)
	}
	inst, err := d.Build(cred)
	if err != nil {
		return nil, fmt.Errorf("%s 初始化失败：%w", d.Name, err)
	}
	if inst == nil {
		return nil, fmt.Errorf("%s 初始化失败：Builder 返回了空实例", d.Name)
	}
	return inst, nil
}

// SortedKeys 返回按 Key 字典序排的列表，用于需要确定性输出的场合（例如生成文档、测试）。
func (r *Registry) SortedKeys(kind Kind) []string {
	keys := r.Keys(kind)
	sort.Strings(keys)
	return keys
}

// ── 包级快捷方法（操作 Default 注册表）────────────────────────────────────

// Register 把服务商注册到 Default 注册表。
func Register(d Descriptor) error { return Default.Register(d) }

// MustRegister 注册到 Default 注册表，失败 panic。在 init() 里用。
func MustRegister(d Descriptor) { Default.MustRegister(d) }

// Get 从 Default 注册表取服务商。
func Get(kind Kind, key string) (Descriptor, bool) { return Default.Get(kind, key) }

// List 列出 Default 注册表里某类别的服务商。
func List(kind Kind) []Descriptor { return Default.List(kind) }

// ListAll 列出 Default 注册表里全部类别的服务商。
func ListAll() map[Kind][]Descriptor { return Default.ListAll() }

// Capable 判断 Default 注册表里的服务商是否具备给定能力。
func Capable(kind Kind, key string, caps ...Capability) bool {
	return Default.Capable(kind, key, caps...)
}

// Build 用 Default 注册表构造服务商实例。
func Build(kind Kind, key string, cred map[string]any) (any, error) {
	return Default.Build(kind, key, cred)
}

// LogSummary 启动时把注册情况写进日志。
//
// 迁移期特别有用：能一眼看出「哪几家还在走老 switch、哪几家已经注册进注册表」，
// 等所有服务商都迁完之后，可以把 BuildClient 的 switch 删掉。
func LogSummary() {
	all := ListAll()
	if len(all) == 0 {
		logrus.Info("服务商注册表：暂无注册项")
		return
	}
	for _, k := range AllKinds() {
		list := all[k]
		if len(list) == 0 {
			continue
		}
		keys := make([]string, 0, len(list))
		for _, d := range list {
			keys = append(keys, d.Key)
		}
		logrus.Infof("服务商注册表 %s（%d）: %v", k, len(keys), keys)
	}
}
