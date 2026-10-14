package wol

import (
	"fmt"
	"time"

	"github.com/chenggegedeid/catflap/core"

	"github.com/sirupsen/logrus"
)

// InitWOL 初始化 WOL 模块配置，并注册退出时持久化
func InitWOL() error {
	cfg, err := ReadConfig()
	if err != nil {
		return fmt.Errorf("读取 WOL 配置失败: %w", err)
	}
	Runtime.mu.Lock()
	Runtime.Config = cfg
	Runtime.mu.Unlock()

	core.OnShutdown(func() {
		Runtime.mu.RLock()
		snapshot := Runtime.Config
		Runtime.mu.RUnlock()
		snapshot.UpdatedAt = time.Now()
		if err := SaveConfig(snapshot); err != nil {
			logrus.Error("保存 WOL 配置失败：", err)
		}
	})
	return nil
}
