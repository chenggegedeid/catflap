package task

import (
	"os"
	"path"
	"time"

	"github.com/chenggegedeid/catflap/utils/utilsFile"

	"github.com/sirupsen/logrus"
)

// ReadConfig 读取计划任务配置，不存在或为空时创建默认配置
func ReadConfig() (Config, error) {
	if fi, err := os.Stat(ConfigPath); os.IsNotExist(err) || (fi != nil && fi.Size() == 0) {
		return createConfig()
	}
	cfg, err := utilsFile.ReadJsonFile[Config](ConfigPath)
	if err != nil {
		logrus.Error("Task Config 读取失败：", err)
		return cfg, err
	}
	if cfg.Tasks == nil {
		cfg.Tasks = []Task{}
	}
	return cfg, nil
}

func createConfig() (Config, error) {
	now := time.Now()
	cfg := Config{Tasks: []Task{}, CreatedAt: now, UpdatedAt: now}
	if err := utilsFile.EnsurePrivateDir(path.Dir(ConfigPath)); err != nil {
		return cfg, err
	}
	if err := utilsFile.WriteJsonFile(ConfigPath, cfg); err != nil {
		return cfg, err
	}
	return cfg, nil
}

// SaveConfig 写入磁盘（自带更新时间戳）
func SaveConfig(cfg Config) error {
	cfg.UpdatedAt = time.Now()
	if cfg.Tasks == nil {
		cfg.Tasks = []Task{}
	}
	return utilsFile.WriteJsonFile(ConfigPath, cfg)
}
