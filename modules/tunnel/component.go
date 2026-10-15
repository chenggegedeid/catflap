package tunnel

import (
	"context"

	"github.com/chenggegedeid/catflap/core"
	"github.com/chenggegedeid/catflap/core/component"

	"github.com/sirupsen/logrus"
)

type tunnelComponent struct {
	component.LocalSwitch
}

func (tunnelComponent) Name() string { return Name }

func (tunnelComponent) Init(context.Context) error {
	cfg, err := ReadConfig()
	if err != nil {
		return err
	}
	Runtime.SetConfig(cfg)

	// 退出时带走 cloudflared。它是我们拉起来的子进程，不收拾会留下孤儿
	// 占着隧道 —— 下次启动再拉一条，CF 那边就看到两条同名隧道在抢。
	core.OnShutdown(func() {
		Stop()
	})

	// 配置说开就拉起来。放在监听之外：隧道是独立的一条对外通路，
	// 起不来也不该影响别的。
	if cfg.Enabled {
		if err := Start(); err != nil {
			logrus.Warnf("[tunnel] 未能启动: %v", err)
		}
	}
	return nil
}

func init() {
	component.MustRegister(tunnelComponent{
		LocalSwitch: component.NewLocalSwitch(Name),
	})
}
