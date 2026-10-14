package wol

import (
	"os"
	"path"
	"time"

	"github.com/chenggegedeid/catflap/utils/utilsFile"

	"github.com/sirupsen/logrus"
)

// ReadConfig 读取 WOL 配置，不存在或为空时创建默认配置
func ReadConfig() (Config, error) {
	if fi, err := os.Stat(ConfigPath); os.IsNotExist(err) || (fi != nil && fi.Size() == 0) {
		return createConfig()
	}
	cfg, err := utilsFile.ReadJsonFile[Config](ConfigPath)
	if err != nil {
		logrus.Error("WOL Config 读取失败：", err)
		return cfg, err
	}
	if cfg.Targets == nil {
		// 反序列化出 nil 切片会让 JSON 里出现 null，界面上循环直接炸
		cfg.Targets = []Target{}
	}
	return cfg, nil
}

// createConfig 创建默认配置并写入磁盘
func createConfig() (Config, error) {
	now := time.Now()
	cfg := Config{
		Targets:   []Target{},
		CreatedAt: now,
		UpdatedAt: now,
	}
	if err := utilsFile.EnsurePrivateDir(path.Dir(ConfigPath)); err != nil {
		logrus.Error("创建 wol config 目录失败：", err)
		return cfg, err
	}
	if err := utilsFile.WriteJsonFile(ConfigPath, cfg); err != nil {
		logrus.Error("WOL Config 写入失败：", err)
		return cfg, err
	}
	return cfg, nil
}

// SaveConfig 把 WOL 配置写入磁盘（自带更新时间戳）
func SaveConfig(cfg Config) error {
	cfg.UpdatedAt = time.Now()
	if cfg.Targets == nil {
		cfg.Targets = []Target{}
	}
	if err := utilsFile.WriteJsonFile(ConfigPath, cfg); err != nil {
		logrus.Error("WOL Config 写入失败：", err)
		return err
	}
	return nil
}
