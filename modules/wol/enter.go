package wol

import "sync"

// WOLRuntime 保存 WOL 模块的运行期配置
type WOLRuntime struct {
	mu     sync.RWMutex
	Config Config
}

// Runtime WOL 模块全局运行时
var Runtime = &WOLRuntime{}

// ListTargets 返回当前配置里的一（份副本）。
func ListTargets() []Target {
	Runtime.mu.RLock()
	defer Runtime.mu.RUnlock()
	out := make([]Target, len(Runtime.Config.Targets))
	copy(out, Runtime.Config.Targets)
	return out
}

// SetConfig 用一份新配置替换运行期的（外面已经写盘了，这里只换内存）
func SetConfig(cfg Config) {
	Runtime.mu.Lock()
	Runtime.Config = cfg
	Runtime.mu.Unlock()
}

// FindTarget 按 ID 找一台机器
func FindTarget(id string) (Target, bool) {
	Runtime.mu.RLock()
	defer Runtime.mu.RUnlock()
	for _, t := range Runtime.Config.Targets {
		if t.ID == id {
			return t, true
		}
	}
	return Target{}, false
}
