package notify

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"time"

	"github.com/sirupsen/logrus"
)

// ConfigPath 通知中心配置。放 config/ 下，和其它模块一致。
const ConfigPath = "config/notify.json"

// ReadConfig 读配置。文件不存在就是全新安装，给个空配置。
func ReadConfig() (Config, error) {
	var cfg Config

	data, err := os.ReadFile(ConfigPath)
	if err != nil {
		if os.IsNotExist(err) {
			return Config{CreatedAt: time.Now()}, nil
		}
		return cfg, err
	}
	if err := json.Unmarshal(data, &cfg); err != nil {
		return cfg, fmt.Errorf("解析 %s 失败: %w", ConfigPath, err)
	}
	if cfg.Channels == nil {
		cfg.Channels = []Channel{}
	}
	if cfg.Rules == nil {
		cfg.Rules = []Rule{}
	}
	return cfg, nil
}

// SaveConfig 落盘。先写临时文件再改名：直接覆盖时若中途断电，
// 会把配置留成半截 JSON，下次启动直接解析失败 —— 那等于把用户的渠道全丢了。
func SaveConfig(cfg Config) error {
	cfg.UpdatedAt = time.Now()
	cfg.Channels = nonNilChannels(cfg.Channels)
	cfg.Rules = nonNilRules(cfg.Rules)

	if err := os.MkdirAll(filepath.Dir(ConfigPath), 0o755); err != nil {
		return err
	}
	data, err := json.MarshalIndent(cfg, "", "  ")
	if err != nil {
		return err
	}
	tmp := ConfigPath + ".tmp"
	if err := os.WriteFile(tmp, data, 0o600); err != nil {
		return err
	}
	// 渠道配置里有 token，文件权限收紧到 0600
	return os.Rename(tmp, ConfigPath)
}

func nonNilChannels(in []Channel) []Channel {
	if in == nil {
		return []Channel{}
	}
	return in
}

func nonNilRules(in []Rule) []Rule {
	if in == nil {
		return []Rule{}
	}
	return in
}

// Update 在写锁内改配置并落盘；回调返回错误则不落盘
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

// FindChannel 按 ID 找渠道
func (r *runtime) FindChannel(id string) (Channel, bool) {
	cfg := r.Snapshot()
	for _, ch := range cfg.Channels {
		if ch.ID == id {
			return ch, true
		}
	}
	return Channel{}, false
}

// channelInUse 哪些服务/规则正在引用这个渠道。
//
// 删渠道前要问一句 —— 直接删掉会让引用它的服务静默不发通知，
// 用户以为"设过了"，出事才发现没收到。
func (r *runtime) ChannelUsedByRules(id string) []string {
	cfg := r.Snapshot()
	var names []string
	for _, rule := range cfg.Rules {
		for _, cid := range rule.ChannelIDs {
			if cid == id {
				names = append(names, rule.Name)
				break
			}
		}
	}
	sort.Strings(names)
	return names
}

// Init 读配置 → 迁移旧 webhook → 落盘
func Init() error {
	cfg, err := ReadConfig()
	if err != nil {
		return err
	}
	Runtime.SetConfig(cfg)
	logrus.Infof("[notify] 已加载 %d 个渠道、%d 条规则", len(cfg.Channels), len(cfg.Rules))
	return nil
}
