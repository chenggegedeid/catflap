package notify

import (
	"time"

	"github.com/chenggegedeid/catflap/modules/webhook"
)

// ===================== 渠道：怎么发 =====================
//
// 渠道是**全局**的：你的 PushPlus token、你的群机器人、你的邮箱在整台机器上
// 只有一份，所有服务、规则都引用它，而不是各存一份。
//
// 这样改 token 只改一次，停用也只停一次 —— 全都立刻生效。

// ChannelType 渠道类型。决定 Config 里该有哪些键、body 长什么样。
type ChannelType string

const (
	// ChannelWebhook 通用 HTTP 回调（原有能力，URL 方法头体全自己填）
	ChannelWebhook ChannelType = "webhook"
	// ChannelDingTalk 钉钉群机器人
	ChannelDingTalk ChannelType = "dingtalk"
	// ChannelWeCom 企业微信群机器人
	ChannelWeCom ChannelType = "wecom"
	// ChannelFeishu 飞书群机器人
	ChannelFeishu ChannelType = "feishu"
	// ChannelBark iOS 推送
	ChannelBark ChannelType = "bark"
	// ChannelServerChan Server 酱
	ChannelServerChan ChannelType = "serverchan"
	// ChannelPushPlus PushPlus
	ChannelPushPlus ChannelType = "pushplus"
)

// Channel 一条推送通道
type Channel struct {
	ID   string      `json:"id"`
	Name string      `json:"name"` // 下拉里显示的就是它
	Type ChannelType `json:"type"`
	// Enabled 关掉 = 所有引用它的地方立刻停发（不用挨个去改）
	Enabled bool `json:"enabled"`
	// Config 按 Type 取不同的键。密钥类字段在接口层做掩码，不回显真值。
	Config map[string]string `json:"config"`

	// MigratedFrom 老配置迁移来的话记下出处（"stun:1:2"），便于回退时还原
	MigratedFrom string `json:"migratedFrom,omitempty"`

	CreatedAt time.Time `json:"createdAt"`
	UpdatedAt time.Time `json:"updatedAt"`
}

// WebhookConfig 渠道转成发送器要的配置。
//
// 通用 webhook 直接用它自己那套；其余类型都是「往固定 URL POST 一段固定 JSON」，
// 所以在这里拼出来交给同一个发送器 —— 不为每种渠道各写一个 HTTP 客户端。
func (c Channel) WebhookConfig(title, content string) webhook.WebhookConfig {
	switch c.Type {
	case ChannelWebhook:
		// 原样返回，#{xxx} 占位符由发送器替换
		return webhook.WebhookConfig{
			Enabled:             true,
			URL:                 c.Config["url"],
			Method:              orDefault(c.Config["method"], "POST"),
			Headers:             c.Config["headers"],
			Body:                c.Config["body"],
			DisableSuccessCheck: true,
		}

	case ChannelDingTalk, ChannelWeCom, ChannelFeishu:
		// 三家群机器人的 body 结构不同，但都是 POST JSON
		return webhook.WebhookConfig{
			Enabled:             true,
			URL:                 c.Config["url"],
			Method:              "POST",
			Headers:             "Content-Type: application/json",
			Body:                groupRobotBody(c.Type, title, content),
			DisableSuccessCheck: true,
		}

	case ChannelBark:
		// Bark：GET https://api.day.app/<key>/<title>/<content>
		return webhook.WebhookConfig{
			Enabled: true,
			URL:     joinURL(c.Config["server"], c.Config["key"], title, content),
			Method:  "GET",
		}

	case ChannelServerChan:
		// Server 酱：POST 表单
		return webhook.WebhookConfig{
			Enabled: true,
			URL:     "https://sctapi.ftqq.com/" + c.Config["key"] + ".send",
			Method:  "POST",
			Headers: "Content-Type: application/x-www-form-urlencoded",
			Body:    "title=" + urlEscape(title) + "&desp=" + urlEscape(content),
		}

	case ChannelPushPlus:
		return webhook.WebhookConfig{
			Enabled:             true,
			URL:                 "https://www.pushplus.plus/send",
			Method:              "POST",
			Headers:             "Content-Type: application/json",
			Body:                pushPlusBody(c.Config["token"], title, content),
			DisableSuccessCheck: true,
		}
	}

	// 未知类型：当成通用 webhook 处理，至少不会静默什么都不发
	return webhook.WebhookConfig{
		Enabled: true,
		URL:     c.Config["url"],
		Method:  orDefault(c.Config["method"], "POST"),
		Body:    c.Config["body"],
	}
}
