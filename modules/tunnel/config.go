package tunnel

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"time"
)

// Name 组件名
const Name = "tunnel"

// ConfigPath 隧道配置。独立成模块后就自己存一份，
// 不再挂在反代配置里 —— 它和反代的监听没有任何关系。
const ConfigPath = "config/tunnel.json"

// Config Cloudflare Tunnel 配置
type Config struct {
	Enabled    bool   `json:"enabled"`
	BinaryPath string `json:"binaryPath"`
	// Token tunnel 的 token。它是这条隧道的全部凭证，按 Secret 处理：
	// 面板回显掩码、日志不打印、掩码提交不覆盖真值。
	Token     string `json:"token"`
	ExtraArgs string `json:"extraArgs"`

	CreatedAt time.Time `json:"createdAt"`
	UpdatedAt time.Time `json:"updatedAt"`
}

// State 运行状态（不落盘）
type State struct {
	Running bool   `json:"running"`
	Message string `json:"message"`
	// TokenSet 只表示"填没填"，绝不回显真实 token
	TokenSet   bool   `json:"tokenSet"`
	BinaryPath string `json:"binaryPath"`
	StartedAt  string `json:"startedAt"`
}

// Runtime 隧道模块的运行时
var Runtime = &runtime{}

type runtime struct {
	mu     sync.RWMutex
	Config Config
}

func (r *runtime) Snapshot() Config {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return r.Config
}

func (r *runtime) SetConfig(cfg Config) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.Config = cfg
}

// Update 改配置并落盘；回调报错则不落盘
func (r *runtime) Update(mutator func(cfg *Config) error) error {
	r.mu.Lock()
	if err := mutator(&r.Config); err != nil {
		r.mu.Unlock()
		return err
	}
	snapshot := r.Config
	r.mu.Unlock()
	return SaveConfig(snapshot)
}

// ReadConfig 读配置。文件不存在 = 没配过，给个空配置。
//
// 也负责**从旧的 proxy.tunnel 迁移**：Tunnel 原先住在反代配置里，
// 升级上来的用户那儿有一份。搬完就写新文件，老的留着不动（不破坏回退）。
func ReadConfig() (Config, error) {
	data, err := os.ReadFile(ConfigPath)
	if err == nil {
		var cfg Config
		if err := json.Unmarshal(data, &cfg); err != nil {
			return cfg, fmt.Errorf("解析 %s 失败: %w", ConfigPath, err)
		}
		return cfg, nil
	}
	if !os.IsNotExist(err) {
		return Config{}, err
	}

	// 新位置没有：看看老位置
	if migrated, ok := readLegacyTunnel(); ok {
		// 落一份到新位置，下次就不用再迁
		if err := SaveConfig(migrated); err != nil {
			return migrated, nil // 迁移失败也能用，只是下次还走老路径
		}
		return migrated, nil
	}

	return Config{CreatedAt: time.Now()}, nil
}

// SaveConfig 落盘。先写临时文件再改名：直接覆盖时断电会留下半截 JSON，
// 下次启动解析失败 —— 那等于把 token 弄丢了。
func SaveConfig(cfg Config) error {
	cfg.UpdatedAt = time.Now()
	if err := os.MkdirAll(filepath.Dir(ConfigPath), 0o755); err != nil {
		return err
	}
	data, err := json.MarshalIndent(cfg, "", "  ")
	if err != nil {
		return err
	}
	tmp := ConfigPath + ".tmp"
	// 里面有 tunnel token，权限收紧
	if err := os.WriteFile(tmp, data, 0o600); err != nil {
		return err
	}
	return os.Rename(tmp, ConfigPath)
}
