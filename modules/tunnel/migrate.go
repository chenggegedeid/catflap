package tunnel

import (
	"encoding/json"
	"os"

	"github.com/sirupsen/logrus"
)

// ===================== 从反代配置迁移 =====================
//
// Tunnel 原来住在 `modules/proxy` 里，配置存在 proxy.json 的 tunnel 字段上。
// 拆成独立模块后，直接读那个字段会有问题：proxy 的配置结构体不归我们管，
// 引它又会把两个模块缠在一起。
//
// 所以这里**按 JSON 直接读**：只要字段名对得上就够了（`tunnel` / `enabled` /
// `binaryPath` / `token` / `extraArgs`）。这样做还有个好处 —— proxy 那边将来
// 把字段删掉，这里的迁移代码也不会因为编译失败而拦住升级。
//
// 老配置**不删**：万一迁移有问题，用户手上还有原始文件。

// legacyProxyConfigPath 反代配置的位置。和 modules/proxy 里那个常量保持一致
// （重复一个字符串比 import 一个模块划算）。
const legacyProxyConfigPath = "config/proxy.json"

type legacyTunnelShape struct {
	Tunnel struct {
		Enabled    bool   `json:"enabled"`
		BinaryPath string `json:"binaryPath"`
		Token      string `json:"token"`
		ExtraArgs  string `json:"extraArgs"`
	} `json:"tunnel"`
}

// readLegacyTunnel 从反代配置里把隧道设置捞出来
func readLegacyTunnel() (Config, bool) {
	data, err := os.ReadFile(legacyProxyConfigPath)
	if err != nil {
		return Config{}, false
	}

	var legacy legacyTunnelShape
	if err := json.Unmarshal(data, &legacy); err != nil {
		return Config{}, false
	}

	// 什么都没配就不算迁移过（免得白白建一个空配置文件，
	// 让人以为"以前配过隧道"）
	if !legacy.Tunnel.Enabled && legacy.Tunnel.Token == "" && legacy.Tunnel.BinaryPath == "" {
		return Config{}, false
	}

	logrus.Infof("[tunnel] 已从反代配置迁移隧道设置（原字段保留未动）")
	return Config{
		Enabled:    legacy.Tunnel.Enabled,
		BinaryPath: legacy.Tunnel.BinaryPath,
		Token:      legacy.Tunnel.Token,
		ExtraArgs:  legacy.Tunnel.ExtraArgs,
	}, true
}
