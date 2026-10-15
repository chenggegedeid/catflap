package notify

import (
	"sync"
	"time"

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
