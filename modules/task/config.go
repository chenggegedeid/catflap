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
//
// 自己保证目录存在，不依赖「调用方之前已经 ReadConfig 过一次」：
// ReadConfig 缺文件时会 createConfig 顺手建目录，于是这个隐含前提在生产路径上
// 成立，但它是一条看不见的耦合 —— 换个调用顺序（或测试直接装一份配置）就会
// 变成一句「保存失败」的日志，然后静默丢掉执行结果。
func SaveConfig(cfg Config) error {
	cfg.UpdatedAt = time.Now()
	if cfg.Tasks == nil {
		cfg.Tasks = []Task{}
	}
	if err := utilsFile.EnsurePrivateDir(path.Dir(ConfigPath)); err != nil {
		return err
	}
	return utilsFile.WriteJsonFile(ConfigPath, cfg)
}
