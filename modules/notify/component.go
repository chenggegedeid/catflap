package notify

import (
	"context"

	"github.com/chenggegedeid/catflap/core/component"
	"github.com/chenggegedeid/catflap/modules/stun"

	"github.com/sirupsen/logrus"
)

// Name 组件名
const Name = "notify"

type notifyComponent struct {
	component.LocalSwitch
}

func (notifyComponent) Name() string { return Name }

func (notifyComponent) Init(context.Context) error {
	if err := Init(); err != nil {
		return err
	}
	// 迁移放在 Init 之后：先把已有渠道读进来，再决定要不要从旧配置补
	MigrateFromServices()
	// 接管穿透服务的通知发送（依赖方向是这边 → stun，stun 不动）
	installStunHook()
	return nil
}

func init() {
	component.MustRegister(notifyComponent{
		LocalSwitch: component.NewLocalSwitch(Name),
	})
}

// MigrateFromServices 把穿透服务里内联的 webhook 配置迁成全局渠道。
//
// 只在「通知中心还没有任何渠道」时跑一次 —— 用户后来手动删光了渠道是他的
// 选择，不该下次启动又被旧配置灌回来。
//
// 迁移完**不删原字段**：万一迁移逻辑有问题，打开 LegacyWebhookFallback
// 就能退回旧行为。代价是同一份配置在磁盘上存了两份，但那点体积换"可回退"
// 是值的。
func MigrateFromServices() {
	cfg := Runtime.Snapshot()
	if len(cfg.Channels) > 0 {
		return // 已经有渠道了，迁移过或用户自己建过
	}

	sources := stun.LegacyWebhooks()
	if len(sources) == 0 {
		return
	}

	var legacy []LegacySource
	for _, s := range sources {
		legacy = append(legacy, LegacySource{
			DeviceID:    s.DeviceID,
			ServiceID:   s.ServiceID,
			ServiceName: s.ServiceName,
			Config:      s.Config,
		})
	}

	res := MigrateLegacy(legacy)
	if len(res.Channels) == 0 {
		return
	}

	// 给迁移出来的渠道建一条规则：穿透掉线 → 这些渠道。
	// 这是旧配置最常见（也几乎唯一）的用途，这样行为与升级前一致。
	var ids []string
	for _, ch := range res.Channels {
		ids = append(ids, ch.ID)
	}
	rule := Rule{
		ID:         "rule-stun-down",
		Name:       "穿透服务掉线",
		Event:      EventStunDown,
		ChannelIDs: ids,
		Enabled:    true,
	}

	if err := Runtime.Update(func(c *Config) error {
		c.Channels = res.Channels
		c.Rules = []Rule{rule}
		return nil
	}); err != nil {
		logrus.Warnf("[notify] 迁移旧 webhook 配置失败: %v", err)
		return
	}

	logrus.Infof("[notify] 已从穿透服务迁移 %d 个渠道（合并了 %d 份重复配置）",
		len(res.Channels), res.Merged)

	// 让穿透服务改用渠道引用
	if err := stun.ApplyChannelRefs(res.SourceChannel); err != nil {
		logrus.Warnf("[notify] 回写服务的渠道引用失败: %v", err)
	}
}

// IsMigrated 迁移是否已完成（面板上给个状态提示用）
func IsMigrated() bool {
	return len(Runtime.Snapshot().Channels) > 0
}
