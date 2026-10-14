package wol

import "time"

// ConfigPath WOL 模块配置文件路径
const ConfigPath = "config/wolConfig.json"

// Config WOL 配置
type Config struct {
	Targets   []Target  `json:"targets"`
	CreatedAt time.Time `json:"createdAt"`
	UpdatedAt time.Time `json:"updatedAt"`
}

// Target 一台可以被唤醒的机器。
//
// 为什么必须让用户填 Broadcast：唤醒包是靠**二层广播**送到目标网卡的，
// 目标机器此刻没有 IP、也不响应 ARP，单播地址根本送不到。
// 常见取值是子网的广播地址（192.168.1.255）或 255.255.255.255。
// 这个坑不写清楚的话，用户填了目标机器的 IP，现象是「点了没反应、也没报错」。
type Target struct {
	ID        string    `json:"id"`
	Name      string    `json:"name"`
	MAC       string    `json:"mac"`
	Broadcast string    `json:"broadcast"`
	Port      int       `json:"port"`
	Note      string    `json:"note,omitempty"`
	CreatedAt time.Time `json:"createdAt"`
}
