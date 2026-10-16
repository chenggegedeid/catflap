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

	// 老配置的站点/转发里 deviceId 还是 0（那是后加的字段，以前按 IP 字符串关联）。
	// **放在这里**：配置刚读完、数据一定在 —— 改设备 IP 时才知道该同步哪几条规则。
	// （早些时候放在启动钩子里，结果读到未加载的空配置，退出时又把空表写回文件，
	//   那次把用户的 4 条站点清空了。）
	if n, err := BackfillDeviceIDs(); err != nil {
		logrus.Warnf("[proxy] 回填站点/转发的 deviceId 失败：%v", err)
	} else if n > 0 {
		logrus.Infof("[proxy] 已为 %d 条站点/转发补上设备归属", n)
	}

	core.OnShutdown(func() {
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

	if n := len(Runtime.CurrentRouter().Hosts()); n > 0 {
		logrus.Infof("[proxy] 已加载 %d 个站点", n)
	}
	return nil
}
