package ddns

import (
	"fmt"
	"github.com/chenggegedeid/catflap/core"

	"github.com/sirupsen/logrus"
	"golang.org/x/sync/errgroup"
)

func DDNSInit() error {
	var err error

	// 读取 STUN 配置文件
	Runtime.Config, err = ReadConfig()
	if err != nil {
		return fmt.Errorf("读取 DDNS 配置失败: %w", err)
	}

	// 监听退出保存配置文件
	core.OnShutdown(func() {
		if err := SaveConfig(Runtime.Snapshot()); err != nil {
			logrus.Error("保存 DDNS 配置失败：", err)
		}
	})

	// 并发初始化
	var g errgroup.Group

	// 1. 初始化WebIpSource
	g.Go(func() error {
		initDefaultIPSources()
		return nil
	})

	// 等待基础运行时准备完成
	if err := g.Wait(); err != nil {
		return fmt.Errorf("初始化 DDNS 运行时失败: %w", err)
	}

	// 先订阅再启动调度器：调度器起来就会立刻扫一轮记录，此时 STUN 若已有公网
	// IP，我们希望「IP 变化」那条通知能赶在这之前就被接住。
	subscribePublicIPChanged()

	Runtime.StartScheduler()

	return nil
}
