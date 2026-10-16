package notify

import (
	"strings"
	"sync"
	"time"

	"github.com/chenggegedeid/catflap/modules/stun"
	"github.com/chenggegedeid/catflap/modules/webhook"

	"github.com/sirupsen/logrus"
)

// ===================== 运行时 =====================

// Runtime 通知中心的运行时。配置读写与 webhook 模块同一套思路：
// 一把锁护着配置，落盘在 WithLock 里做，外部只通过函数拿快照。
var Runtime = &runtime{
	sent: make(map[string]time.Time),
}

type runtime struct {
	mu     sync.RWMutex
	Config Config

	// sent 冷却记录：去重键 → 上次发送时间
	sentMu sync.Mutex
	sent   map[string]time.Time
}

// Snapshot 拿一份配置副本
func (r *runtime) Snapshot() Config {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return r.Config
}

// WithLock 改配置（改完由调用方落盘）
func (r *runtime) WithLock(fn func(cfg *Config) error) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	return fn(&r.Config)
}

// SetConfig 整体替换（读配置时用）
func (r *runtime) SetConfig(cfg Config) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.Config = cfg
}

// Notify 发一条通知：找出该发的渠道，逐个发。
//
// 这一步是**异步**的：调用方（业务模块）在事件发生时不该等网络请求。
// 也因此这里的失败只记日志，不往上抛 —— 通知失败不该影响业务本身。
func Notify(ev Event) {
	if ev.At.IsZero() {
		ev.At = time.Now()
	}
	if ev.Key == "" {
		// 没给去重键就用「事件类型 + 标题」，至少同类不重复刷
		ev.Key = string(ev.Type) + "|" + ev.Title
	}

	cfg := Runtime.Snapshot()

	// 冷却：同一个 Key 在窗口内只发一次
	if !Runtime.allow(ev.Key, cfg.cooldown()) {
		logrus.Debugf("[notify] %s 在冷却窗口内，跳过", ev.Key)
		return
	}

	targets := cfg.rulesFor(ev.Type)
	if len(targets) == 0 {
		logrus.Debugf("[notify] %s 没有匹配的渠道", ev.Type)
		return
	}

	for _, ch := range targets {
		go sendTo(ch, ev)
	}
}

// allow 冷却判断。放行才记录时间，被拒的不刷新时间戳
// （否则高频事件会一直把自己顶在窗口外，等于永不发送）。
func (r *runtime) allow(key string, window time.Duration) bool {
	r.sentMu.Lock()
	defer r.sentMu.Unlock()

	now := time.Now()
	if last, ok := r.sent[key]; ok && now.Sub(last) < window {
		return false
	}
	r.sent[key] = now

	// 顺手清理过期记录，免得这张表无限长
	for k, t := range r.sent {
		if now.Sub(t) > window*2 {
			delete(r.sent, k)
		}
	}
	return true
}

// sendToChannel 直接发给某一条渠道（不走规则匹配）。
//
// 服务上"选了哪几个渠道"是明确的指向，不该被规则再筛一遍 ——
// 规则是"什么事件发到哪"的全局配置，和这里不是一个语义。
func sendToChannel(ch Channel, ev Event) {
	sendTo(ch, ev)
}

func sendTo(ch Channel, ev Event) {
	title := ev.Title
	if title == "" {
		title = string(ev.Type)
	}
	cfg := ch.WebhookConfig(title, ev.Content)

	fields := ev.Fields
	if fields == nil {
		fields = map[string]string{}
	}
	// 补两个通用字段，用户写自定义 body 时能用上
	if _, ok := fields["title"]; !ok {
		fields["title"] = title
	}
	if _, ok := fields["content"]; !ok {
		fields["content"] = ev.Content
	}
	if _, ok := fields["time"]; !ok {
		fields["time"] = ev.At.Format("2006-01-02 15:04:05")
	}

	if _, err := webhook.Send(cfg, fields); err != nil {
		logrus.Warnf("[notify] 渠道「%s」发送失败: %v", ch.Name, err)
		return
	}
	logrus.Infof("[notify] 渠道「%s」已发送: %s", ch.Name, title)
}

// TestChannel 给「测试发送」按钮用：同步发一条，把错误直接返回给界面。
//
// 这里刻意同步 —— 用户点了按钮就该立刻知道通不通，异步的话只能看日志。
func TestChannel(ch Channel) error {
	cfg := ch.WebhookConfig("Catflap 测试通知", "如果你看到这条消息，说明这个渠道配置正确。")
	_, err := webhook.Send(cfg, map[string]string{
		"title":   "Catflap 测试通知",
		"content": "如果你看到这条消息，说明这个渠道配置正确。",
		"time":    time.Now().Format("2006-01-02 15:04:05"),
	})
	return err
}

// ScrubChannelSecrets 抹掉消息里出现过的渠道配置值。
//
// 导出给 api 层用：通知页的「测试发送」和服务的「发一条测试通知」
// 都要把发送失败的原因显示给用户，而那些错误里可能带着渠道凭证。
func ScrubChannelSecrets(msg string, ch Channel) string {
	return scrubChannelSecrets(msg, ch)
}

// scrubChannelSecrets 把渠道配置里出现过的敏感值从消息里抹掉。
//
// 这是 maskURLError（webhook 包）之外的第二道闸：那边只认 URL 查询串，
// 而凭证也可能走别的路子回到错误里 —— 比如服务端返回体被拼进错误、
// 或者某些渠道把 key 放在 path 而不是 query。
// 凡是这个渠道自己配过的值，一律不许出现在回给界面的错误里。
func scrubChannelSecrets(msg string, ch Channel) string {
	for k, v := range ch.Config {
		// 太短的值拿来全词替换会误伤正文（比如 "1"、"cn" 这种）
		if len(v) < 8 {
			continue
		}
		if strings.Contains(msg, v) {
			msg = strings.ReplaceAll(msg, v, "***("+k+")")
		}
	}
	return msg
}

// joinHostPort 拼 host:port；port 为空就只给 host。
// 测试发送时外部端口可能还没有（服务刚配、还没打洞成功）。
func joinHostPort(host, port string) string {
	if host == "" {
		return ""
	}
	if port == "" {
		return host
	}
	return host + ":" + port
}

// ServiceTestResult 一次「按服务的配置试发」的结果。
//
// 逐渠道给结果，而不是一个总的成功/失败：三个渠道里两个通一个不通时，
// 只回一句"发送失败"用户根本不知道该去改哪个。
type ServiceTestResult struct {
	ChannelID   string `json:"channelId"`
	ChannelName string `json:"channelName"`
	OK          bool   `json:"ok"`
	Error       string `json:"error,omitempty"`
}

// TestServiceNotify 用**服务自己的配置**（渠道 + 模板）试发一条。
//
// 用户在服务里选好渠道、写好模板，最想知道的就是"这样配到底能不能收到"。
// 走的是和真实事件完全相同的渲染与发送路径，所以测出来通就是真通。
func TestServiceNotify(channelIDs []string, template string, fields map[string]string) []ServiceTestResult {
	if fields == nil {
		fields = map[string]string{}
	}
	// 试发也要有像样的内容：真实事件里这些字段都有，测试时不补的话
	// 模板里的 #{address} 会渲染成空，用户会以为模板写错了。
	now := time.Now()
	if fields["updated_at"] == "" {
		fields["updated_at"] = now.Format(time.RFC3339)
	}
	if fields["time"] == "" {
		fields["time"] = now.Format("2006-01-02 15:04:05")
	}
	if fields["address"] == "" {
		fields["address"] = "（测试）"
	}
	if fields["service_name"] == "" {
		fields["service_name"] = "测试服务"
	}
	// 试发也要拼出像样的地址，否则模板里的 #{address} 是空的，
	// 用户会以为模板写错了。规则跟真实事件一致：域名优先，回落公网 IP。
	// 公网 IP 前端拿不到（那是运行时探测出来的），这里直接取当前的。
	if fields["external_ip"] == "" {
		fields["external_ip"] = stun.Runtime.Network.PublicIP
	}
	if host := fields["domain"]; host != "" {
		fields["ip_addr"] = joinHostPort(fields["external_ip"], fields["external_port"])
		if p := fields["external_port"]; p != "" {
			host = host + ":" + p
		}
		fields["address"] = host
	} else if fields["external_ip"] != "" {
		fields["address"] = joinHostPort(fields["external_ip"], fields["external_port"])
		fields["ip_addr"] = fields["address"]
	}

	content := renderTemplate(template, fields)
	if strings.TrimSpace(content) == "" {
		content = defaultTunnelContent(fields)
	}
	// 标题照真实通知的样子来（服务名:端口），这样用户能预览到"手机锁屏上
	// 看到的是什么"；加个（测试）后缀，免得过后翻记录时分不清哪条是真的。
	title := "Catflap 通知测试"
	if fields["title"] != "" {
		title = fields["title"]
	} else if fields["service_name"] != "" {
		title = fields["service_name"]
		if fields["external_port"] != "" {
			title = title + ":" + fields["external_port"]
		}
		title = title + "（测试）"
	}

	out := make([]ServiceTestResult, 0, len(channelIDs))
	for _, id := range channelIDs {
		ch, ok := Runtime.FindChannel(id)
		if !ok {
			out = append(out, ServiceTestResult{ChannelID: id, ChannelName: id, Error: "渠道不存在（可能已被删除）"})
			continue
		}
		if !ch.Enabled {
			out = append(out, ServiceTestResult{ChannelID: id, ChannelName: ch.Name, Error: "渠道已停用"})
			continue
		}
		cfg := ch.WebhookConfig(title, content)
		if _, err := webhook.Send(cfg, fields); err != nil {
			// 两道闸：webhook 层抹 URL 查询串，这里再抹渠道自己配过的任何值
			out = append(out, ServiceTestResult{
				ChannelID: id, ChannelName: ch.Name,
				Error: scrubChannelSecrets(err.Error(), ch),
			})
			continue
		}
		out = append(out, ServiceTestResult{ChannelID: id, ChannelName: ch.Name, OK: true})
	}
	return out
}
