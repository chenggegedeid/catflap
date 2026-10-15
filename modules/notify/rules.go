package notify

import "time"

// ===================== 事件 =====================

// EventType 会发生什么。
//
// 命名用「模块.动作」的形式：加新事件时不用改已有代码，前端也按前缀分组显示。
type EventType string

const (
	// EventCertExpiring 证书快到期了（提前 14 天，见 cert 模块）
	EventCertExpiring EventType = "cert.expiring"
	// EventCertRenewFailed 证书续期失败
	EventCertRenewFailed EventType = "cert.renewFailed"

	// EventDDNSSyncFailed DDNS 同步失败
	EventDDNSSyncFailed EventType = "ddns.syncFailed"
	// EventPublicIPChanged 公网 IP 变了
	EventPublicIPChanged EventType = "ddns.publicIPChanged"

	// EventStunDown 穿透服务掉线
	EventStunDown EventType = "stun.down"
	// EventStunUp 穿透服务恢复
	EventStunUp EventType = "stun.up"
	// EventStunPortChanged 穿透外部端口变了
	EventStunPortChanged EventType = "stun.portChanged"

	// EventLoginFailed 有人反复登录失败（可能在被猜密码）
	EventLoginFailed EventType = "auth.loginFailed"
)

// EventMeta 事件的静态描述，给前端画选择列表用
type EventMeta struct {
	Type  EventType `json:"type"`
	Group string    `json:"group"` // 分组：证书 / DDNS / 穿透 / 安全
	Title string    `json:"title"`
	// FailOnly 这类事件本身就只在异常时发生，规则里的"仅失败时"对它没意义
	FailOnly bool `json:"failOnly"`
}

// AllEvents 全部可订阅事件。前端直接渲染这个列表，加事件不用改前端。
func AllEvents() []EventMeta {
	return []EventMeta{
		{Type: EventCertExpiring, Group: "证书", Title: "证书即将过期"},
		{Type: EventCertRenewFailed, Group: "证书", Title: "证书续期失败", FailOnly: true},
		{Type: EventDDNSSyncFailed, Group: "DDNS", Title: "域名解析同步失败", FailOnly: true},
		{Type: EventPublicIPChanged, Group: "DDNS", Title: "公网 IP 发生变化"},
		{Type: EventStunDown, Group: "穿透", Title: "穿透服务掉线", FailOnly: true},
		{Type: EventStunUp, Group: "穿透", Title: "穿透服务恢复"},
		{Type: EventStunPortChanged, Group: "穿透", Title: "穿透外部端口变化"},
		{Type: EventLoginFailed, Group: "安全", Title: "登录失败次数过多", FailOnly: true},
	}
}

// Event 一次待发送的通知
type Event struct {
	Type EventType `json:"type"`
	// Title 一句话说清发生了什么，会作为推送标题
	Title string `json:"title"`
	// Content 详情，可多行
	Content string `json:"content"`
	// Fields 给通用 webhook 的 #{xxx} 占位符用
	Fields map[string]string `json:"fields"`
	// Key 去重键：同一个 Key 在冷却窗口内只发一次，防刷屏
	Key string    `json:"key"`
	At  time.Time `json:"at"`
}

// ===================== 规则与配置 =====================

// Rule 什么事件发到哪些渠道
type Rule struct {
	ID    string    `json:"id"`
	Name  string    `json:"name"`
	Event EventType `json:"event"`
	// ChannelIDs 引用的**全局渠道**。渠道改了这里不用动。
	ChannelIDs []string  `json:"channelIds"`
	Enabled    bool      `json:"enabled"`
	CreatedAt  time.Time `json:"createdAt"`
	UpdatedAt  time.Time `json:"updatedAt"`
}

// Config 通知中心的全部配置
type Config struct {
	Channels []Channel `json:"channels"`
	Rules    []Rule    `json:"rules"`

	// CooldownSeconds 同一个事件在这么长时间内只发一次。
	// 证书检查、DDNS 重试这类会高频触发，没有冷却会刷屏。
	// 0 = 用默认值（见 defaultCooldown）。
	CooldownSeconds int `json:"cooldownSeconds"`

	// LegacyWebhookFallback 回退开关：迁移后如果发现通知行为不对，
	// 打开它就让穿透服务回到"读自己那份 webhookconfig"的老路径。
	LegacyWebhookFallback bool `json:"legacyWebhookFallback"`

	CreatedAt time.Time `json:"createdAt"`
	UpdatedAt time.Time `json:"updatedAt"`
}

const defaultCooldown = 10 * time.Minute

func (c Config) cooldown() time.Duration {
	if c.CooldownSeconds <= 0 {
		return defaultCooldown
	}
	return time.Duration(c.CooldownSeconds) * time.Second
}

// rulesFor 这个事件该发给哪些渠道（只取启用的规则，且渠道本身也要启用）
func (c Config) rulesFor(ev EventType) []Channel {
	byID := make(map[string]Channel, len(c.Channels))
	for _, ch := range c.Channels {
		byID[ch.ID] = ch
	}

	var out []Channel
	seen := map[string]bool{}
	for _, r := range c.Rules {
		if !r.Enabled || r.Event != ev {
			continue
		}
		for _, id := range r.ChannelIDs {
			ch, ok := byID[id]
			if !ok || !ch.Enabled {
				// 渠道被删了 / 被停用：静默跳过而不是报错 ——
				// 停用是用户的明确意图，不该在日志里刷成异常
				continue
			}
			if seen[ch.ID] {
				continue
			}
			seen[ch.ID] = true
			out = append(out, ch)
		}
	}
	return out
}
