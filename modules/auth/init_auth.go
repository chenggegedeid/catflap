package auth

import "fmt"

// InitAuth 读配置进 Runtime。中间件依赖此配置，未就绪时零值 Initialized=false，
// 走 fail-closed（拒绝受保护请求 / 引导设置流程），安全。
func InitAuth() error {
	cfg, err := ReadConfig()
	if err != nil {
		return fmt.Errorf("读取 Auth 配置失败: %w", err)
	}
	Runtime.mu.Lock()
	Runtime.Config = cfg
	Runtime.mu.Unlock()
	// 阈值与白名单要在启动时就灌进限流器 —— 否则第一次登录用的还是常量，
	// 配置里的 5 次/白名单要等到下一次写配置才生效，那中间这段是裸的。
	ApplyConfig(cfg)
	return nil
}
