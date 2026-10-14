package home

import (
	"fmt"
	"os"

	"github.com/sirupsen/logrus"
)

// InitHome 读配置 + 订阅「服务被删除」事件
//
// 注意注释里过去写的「在 stun 初始化之后调用」已经不成立：现在门户页通过
// 事件总线订阅读 stun 的消息，不要求发布方先起来 —— 两个模块在同一层里并发
// 启动，谁先都行。保留这条注释是为了说明这次改动的语义变化。
func InitHome() error {
	cfg, err := ReadConfig()
	if err != nil {
		return fmt.Errorf("读取 Home 配置失败: %w", err)
	}
	Runtime.mu.Lock()
	Runtime.Config = cfg
	Runtime.mu.Unlock()

	// 图标目录
	if err := os.MkdirAll("data/icon", 0755); err != nil {
		logrus.Warn("创建 data/icon 失败：", err)
	}
	// 自定义壁纸目录
	if err := os.MkdirAll("data/wallpaper", 0755); err != nil {
		logrus.Warn("创建 data/wallpaper 失败：", err)
	}

	registerStunHooks()
	return nil
}
