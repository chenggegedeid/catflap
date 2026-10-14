package cert

import (
	"context"
	"fmt"
	"time"

	"github.com/chenggegedeid/catflap/core/component"
)

// Name 组件名。与 routers/cert_routers.go 里 ContributeRoutes 用的名字一致。
const Name = "cert"

// expiringWindow 提前多久算「将过期」。14 天：留得下通知用户、让自动续期
// 重试几轮的时间，又不会让一个还有一个月有效期的证书天天报黄。
const expiringWindow = 14 * 24 * time.Hour

// certComponent 把证书管理模块接到组件注册表上。
type certComponent struct {
	// LocalSwitch 提供 Enabled / SetEnabled，状态存在
	// config/components.json（见 core/component/state.go）。
	component.LocalSwitch
}

func (certComponent) Name() string { return Name }

// Init 读配置、装载磁盘上已有的证书、启动续期与热重载调度器。
func (certComponent) Init(context.Context) error { return InitCert() }

// Health 看两件事：有没有证书续期失败、有没有证书快到期了。
//
// 只读内存里的配置快照，不发网络请求 —— 健康检查会被界面轮询，
// 在里面探测 ACME 服务器会把面板拖慢，也会把对方打烦。
func (certComponent) Health() component.Health {
	cfg := Runtime.Snapshot()

	var failed, expiring, soonest int
	now := time.Now()
	for _, c := range cfg.Certificates {
		if !c.Enabled {
			continue
		}
		if c.LastError != "" {
			failed++
		}
		if c.NotAfter.IsZero() {
			// 还没签发过，或者读不出有效期；不是故障，不在这里报
			continue
		}
		if c.NotAfter.Before(now) {
			failed++
			continue
		}
		if now.Add(expiringWindow).After(c.NotAfter) {
			expiring++
			if d := int(c.NotAfter.Sub(now).Hours() / 24); soonest == 0 || d < soonest {
				soonest = d
			}
		}
	}

	switch {
	case failed > 0:
		return component.Down(fmt.Sprintf("有 %d 张证书异常或已过期", failed))
	case expiring > 0:
		return component.Degraded(fmt.Sprintf("有 %d 张证书将在 %d 天内过期", expiring, soonest))
	default:
		return component.Healthy()
	}
}

func init() {
	component.MustRegister(certComponent{
		LocalSwitch: component.NewLocalSwitch(Name),
	})
}
