package proxy

import (
	"fmt"

	"github.com/chenggegedeid/catflap/core"

	"github.com/sirupsen/logrus"
)

// InitProxy 读配置 → 建站点表 → 开监听
func InitProxy() error {
	cfg, err := ReadConfig()
	if err != nil {
		return fmt.Errorf("读取反向代理配置失败: %w", err)
	}

	Runtime.mu.Lock()
	Runtime.Config = cfg
	Runtime.Router = NewRouter(cfg.Sites)
	Runtime.mu.Unlock()

	core.OnShutdown(func() {
		// 隧道是 Catflap 拉起来的子进程，退出时必须带走，不然会留下一个孤儿
		// 进程占着隧道（下次启动再拉一条，CF 那边就看到两条同名隧道在抢）
		StopTunnel()
		Runtime.pool.StopAll()
		if err := SaveConfig(Runtime.Snapshot()); err != nil {
			logrus.Error("保存反向代理配置失败：", err)
		}
	})

	// 监听起不来不算模块初始化失败：站点配置都还在，用户到页面上
	// 换个端口就能救回来。原因已经记进状态里，页面上看得到。
	if err := Runtime.pool.Apply(cfg); err != nil {
		logrus.Error("[proxy] ", err)
	}

	// 隧道放在监听之后：它是独立的一条对外通路，起不来也不该挡住本地反代
	SyncTunnel()

	if n := len(Runtime.CurrentRouter().Hosts()); n > 0 {
		logrus.Infof("[proxy] 已加载 %d 个站点", n)
	}
	return nil
}
