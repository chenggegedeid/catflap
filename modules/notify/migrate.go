package notify

import (
	"crypto/sha1"
	"encoding/hex"
	"fmt"
	"sort"
	"strings"

	"github.com/chenggegedeid/catflap/modules/webhook"
)

// ===================== 从旧的 per-service webhook 迁移 =====================
//
// 旧的形态是：每个穿透服务自己存一份完整的 WebhookConfig（URL / 方法 / 头 / 体）。
// 于是同一个群机器人的地址，在 5 个服务里存了 5 份 —— 换一次要改 5 处。
//
// 迁移的核心动作就是**把这些重复的合成一条全局渠道**，服务侧改成引用。
// 这不是"顺手去重"，是把用户已经承受过的重复一次性清掉。

// LegacySource 一处旧配置的出处
type LegacySource struct {
	DeviceID  uint
	ServiceID uint
	// ServiceName 用来给迁移出来的渠道取个能认出来的名字
	ServiceName string
	Config      webhook.WebhookConfig
}

// Migrated 迁移结果
type Migrated struct {
	// Channels 新渠道（已含去重合并）
	Channels []Channel
	// SourceChannel 每个出处该引用哪个渠道 ID：key 是 "deviceID:serviceID"
	SourceChannel map[string]string
	// Merged 合并了几份重复配置（0 表示没重复，每份都独立）
	Merged int
}

// fingerprint 两份 webhook 配置算不算"同一个渠道"。
//
// 比的是**真正决定"发到哪、发什么"的那几个字段**：URL / 方法 / 头 / 体 / 代理。
// 用 sha1 而不是直接拼字符串：用户可能填了几百行自定义头，拼出来太长。
func fingerprint(c webhook.WebhookConfig) string {
	// 方法留空默认 GET，比较时要先归一，否则"留空"和"写了GET"会被当成两个
	method := strings.ToUpper(strings.TrimSpace(c.Method))
	if method == "" {
		method = "GET"
	}
	raw := strings.Join([]string{
		strings.TrimSpace(c.URL),
		method,
		strings.TrimSpace(c.Headers),
		strings.TrimSpace(c.Body),
		strings.TrimSpace(c.Proxy),
	}, "\x00")

	sum := sha1.Sum([]byte(raw))
	return hex.EncodeToString(sum[:])
}

// MigrateLegacy 把各处旧 webhook 配置转成全局渠道 + 一条规则。
//
// 幂等性由调用方保证（只在配置里没有任何渠道时才跑）：这里不判断
// "是不是已经迁移过"，那属于配置加载的事。
func MigrateLegacy(sources []LegacySource) Migrated {
	out := Migrated{SourceChannel: map[string]string{}}
	if len(sources) == 0 {
		return out
	}

	byPrint := map[string]string{} // 指纹 → 渠道 ID
	var channels []Channel

	for _, src := range sources {
		c := src.Config
		// 没启用、或连 URL 都没有的：跳过。迁移出一条永远发不出去的渠道
		// 只会让通知中心看起来乱。
		if !c.Enabled || strings.TrimSpace(c.URL) == "" {
			continue
		}

		fp := fingerprint(c)
		if id, ok := byPrint[fp]; ok {
			// 已有同样配置的渠道：直接引用它
			out.SourceChannel[key(src.DeviceID, src.ServiceID)] = id
			out.Merged++
			continue
		}

		id := "ch-" + fp[:8]
		ch := Channel{
			ID:           id,
			Name:         channelName(src),
			Type:         ChannelWebhook,
			Enabled:      true,
			MigratedFrom: "stun:" + key(src.DeviceID, src.ServiceID),
			Config: map[string]string{
				"url":     c.URL,
				"method":  c.Method,
				"headers": c.Headers,
				"body":    c.Body,
			},
		}
		channels = append(channels, ch)
		byPrint[fp] = id
		out.SourceChannel[key(src.DeviceID, src.ServiceID)] = id
	}

	// 渠道按名字排一下，界面上顺序稳定（map 遍历是随机的）
	sort.Slice(channels, func(i, j int) bool { return channels[i].Name < channels[j].Name })
	out.Channels = channels
	return out
}

// channelName 给迁移出来的渠道取名。
//
// 只有一份的时候用服务名（"飞牛NAS 的通知"），用户一眼知道是谁的；
// 被多个服务共用时不敢挂某一个服务的名字（会误导），用地址的域名部分。
func channelName(src LegacySource) string {
	name := strings.TrimSpace(src.ServiceName)
	if name == "" {
		return "迁移的通知"
	}
	return name + " 的通知"
}

func key(deviceID, serviceID uint) string {
	return fmt.Sprintf("%d:%d", deviceID, serviceID)
}
