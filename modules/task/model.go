package task

import "time"

// ConfigPath 计划任务配置文件路径
const ConfigPath = "config/taskConfig.json"

// ScheduleKind 调度方式
type ScheduleKind string

const (
	// KindInterval 每隔 N 分钟跑一次
	KindInterval ScheduleKind = "interval"
	// KindDaily 每天在固定时刻跑一次
	KindDaily ScheduleKind = "daily"
)

// ActionKind 任务做什么
type ActionKind string

const (
	// ActionDDNS 立即同步一轮 DDNS 解析记录（复用 ddns 调度器的 Trigger）
	ActionDDNS ActionKind = "ddns"
	// ActionWOL 唤醒一台已登记的机器（复用 wol 模块）
	ActionWOL ActionKind = "wol"
)

// Task 一条计划任务
type Task struct {
	ID string `json:"id"`
	// Name 只是给人看的标签，不参与匹配
	Name string `json:"name"`
	// Enabled 关掉的任务不参与到期判断，但保留配置与上次执行结果
	Enabled bool         `json:"enabled"`
	Kind    ScheduleKind `json:"kind"`
	// IntervalMin 每隔多少分钟，仅在 Kind=interval 时有效
	IntervalMin int `json:"intervalMin"`
	// Hour/Minute 每天的时刻，仅在 Kind=daily 时有效
	Hour   int `json:"hour"`
	Minute int `json:"minute"`

	Action ActionKind `json:"action"`
	// Target 动作的对象：ActionWOL 时是机器 ID；ActionDDNS 时留空
	Target string `json:"target"`

	// 下面三个是运行结果，界面上展示，不参与配置语义
	LastRun    string `json:"lastRun,omitempty"`
	LastResult string `json:"lastResult,omitempty"`
	LastError  string `json:"lastError,omitempty"`

	CreatedAt time.Time `json:"createdAt"`
}

// Config 计划任务配置
type Config struct {
	Tasks     []Task    `json:"tasks"`
	CreatedAt time.Time `json:"createdAt"`
	UpdatedAt time.Time `json:"updatedAt"`
}
