package auth

import (
	"crypto/rand"
	"encoding/hex"
	"os"
	"time"

	"github.com/chenggegedeid/catflap/utils/utilsFile"

	"github.com/sirupsen/logrus"
)

const ConfigPath = "config/authConfig.json"

// token 有效期：
//
// 默认 24 小时。面板保存着 DNS 服务商 Token 这类明文凭据，还能把内网服务
// 暴露到公网，token 泄漏的代价很高；7 天的窗口太长，所以默认收紧到一天。
//
// 勾了登录页的「记住我」才给 7 天 —— 手机、平板这类天天要用的设备用它换方便，
// 风险由用户自己权衡。
const (
	defaultTokenTTLHours  = 24
	rememberTokenTTLHours = 24 * 7
)

// tokenTTL 按「是否勾了记住我」挑有效期；配置里的 TokenTTLHours 是默认值，<=0 时用兜底常量
func tokenTTL(configuredHours int, remember bool) time.Duration {
	if remember {
		return time.Duration(rememberTokenTTLHours) * time.Hour
	}
	if configuredHours <= 0 {
		configuredHours = defaultTokenTTLHours
	}
	return time.Duration(configuredHours) * time.Hour
}

// ReadConfig 读取鉴权配置，文件不存在则用默认值创建（含随机 JwtSecret）
func ReadConfig() (Config, error) {
	if fi, err := os.Stat(ConfigPath); os.IsNotExist(err) || (fi != nil && fi.Size() == 0) {
		return createConfig()
	}
	cfg, err := utilsFile.ReadJsonFile[Config](ConfigPath)
	if err != nil {
		logrus.Error("Auth Config 读取失败：", err)
		return cfg, err
	}
	// 兜底：老配置或被清空的 secret，补一个
	if cfg.JwtSecret == "" {
		cfg.JwtSecret = randomSecret()
	}
	if cfg.TokenTTLHours <= 0 {
		cfg.TokenTTLHours = defaultTokenTTLHours
	}
	return cfg, nil
}

// createConfig 首次启动写入默认值：未初始化 + 随机 JwtSecret
func createConfig() (Config, error) {
	cfg := Config{
		Initialized:   false,
		JwtSecret:     randomSecret(),
		TokenTTLHours: defaultTokenTTLHours,
		UpdatedAt:     time.Now().Format(time.RFC3339),
	}
	if err := utilsFile.EnsurePrivateDir("config"); err != nil {
		logrus.Error("创建 config 目录失败：", err)
		return cfg, err
	}
	if err := utilsFile.WriteJsonFile(ConfigPath, cfg); err != nil {
		logrus.Error("Auth Config 写入失败：", err)
		return cfg, err
	}
	return cfg, nil
}

// UpdateConfig 写回配置文件
func UpdateConfig(cfg Config) error {
	cfg.UpdatedAt = time.Now().Format(time.RFC3339)
	if err := utilsFile.WriteJsonFile(ConfigPath, cfg); err != nil {
		logrus.Error("Auth Config 写入失败：", err)
		return err
	}
	return nil
}

// randomSecret 生成 32 字节的 hex 随机密钥
func randomSecret() string {
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		// crypto/rand 失败极罕见，用时间兜底避免空 secret
		return hex.EncodeToString([]byte(time.Now().Format(time.RFC3339Nano)))
	}
	return hex.EncodeToString(b)
}
