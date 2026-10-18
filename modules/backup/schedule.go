package backup

import (
	"os"
	"time"

	"github.com/chenggegedeid/catflap/utils/utilsFile"
)

/**
 * 定时备份的配置。
 *
 * 存自己的一份配置文件，不塞进别的模块 —— 备份是横切功能，
 * 借别人的配置文件放字段会让"备份设置"散落两处。
 */
const ConfigPath = "config/backupConfig.json"

type ScheduleConfig struct {
	// Enabled 是否开启定时备份
	Enabled bool `json:"enabled"`
	// IntervalHours 每隔几小时做一次。0 或负数按 24 处理
	IntervalHours int `json:"intervalHours"`
	// KeepCount 保留最近几份。老的超了就删。0 或负数按 7 处理
	KeepCount int `json:"keepCount"`
	// Modules 要备份哪些模块；空 = 全部
	Modules []string `json:"modules"`
	// LastRunAt 上次执行时间，只作展示
	LastRunAt time.Time `json:"lastRunAt"`
	// LastResult 上次结果（成功写文件名，失败写错误）。只作展示
	LastResult string `json:"lastResult"`
}

// 默认值：每天一次、留最近 7 份
func defaultSchedule() ScheduleConfig {
	return ScheduleConfig{
		Enabled:       false,
		IntervalHours: 24,
		KeepCount:     7,
	}
}

/**
 * LoadSchedule 读定时备份配置。
 *
 * 文件不存在（全新安装）不算错误 —— 返回一份默认值，界面上就是"未开启"。
 */
func LoadSchedule() (ScheduleConfig, error) {
	if fi, err := os.Stat(ConfigPath); os.IsNotExist(err) || (fi != nil && fi.Size() == 0) {
		return defaultSchedule(), nil
	}
	cfg, err := utilsFile.ReadJsonFile[ScheduleConfig](ConfigPath)
	if err != nil {
		return defaultSchedule(), err
	}
	// 历史配置里这两个字段可能是 0，兜个底，免得算出 0 小时的间隔
	if cfg.IntervalHours <= 0 {
		cfg.IntervalHours = 24
	}
	if cfg.KeepCount <= 0 {
		cfg.KeepCount = 7
	}
	return cfg, nil
}

// SaveSchedule 存定时备份配置
func SaveSchedule(cfg ScheduleConfig) error {
	if err := utilsFile.EnsurePrivateDir("config"); err != nil {
		return err
	}
	if cfg.IntervalHours <= 0 {
		cfg.IntervalHours = 24
	}
	if cfg.KeepCount <= 0 {
		cfg.KeepCount = 7
	}
	return utilsFile.WriteJsonFile(ConfigPath, cfg)
}
