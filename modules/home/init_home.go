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

	// 老布局（pagedOrder / scrollOrder）→ 流式网格（order / size）。
	//
	// **放在读完之后、任何业务逻辑之前**：后面所有代码都只认 Order/Size，
	// 迁移跑晚了就会读到半新半旧的数据。
	//
	// 要落盘，否则迁移只在内存里生效 —— 下次启动又迁一遍，
	// 而且用户每次看到的顺序还可能不一样（排序依据已被清掉）。
	if Migrate(&cfg) {
		Runtime.mu.Lock()
		Runtime.Config = cfg
		Runtime.mu.Unlock()
		if err := UpdateConfig(cfg); err != nil {
			// 迁移结果没写进去不算致命：内存里已经是新格式，这次能用。
			// 但下一次启动还会再迁一次，所以要明确记出来
			logrus.Errorf("[home] 布局迁移结果落盘失败（下次启动会重试）：%v", err)
		}
	}

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
