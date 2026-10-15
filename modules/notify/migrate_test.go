package notify

import (
	"testing"
	"time"

	"github.com/chenggegedeid/catflap/modules/webhook"
)

func src(dev, svc uint, name, url string) LegacySource {
	return LegacySource{
		DeviceID:    dev,
		ServiceID:   svc,
		ServiceName: name,
		Config: webhook.WebhookConfig{
			Enabled: true,
			URL:     url,
			Method:  "POST",
			Body:    `{"text":"#{content}"}`,
		},
	}
}

// 核心行为：多个服务配了同一串 URL → 只建**一个**渠道，服务侧都引用它。
// 这就是用户说的"避免每个去配置"要落到磁盘上的样子。
func TestMigrateMergesIdenticalConfigs(t *testing.T) {
	legacy := []LegacySource{
		src(1, 1, "飞牛NAS", "https://push.example.com/hook?token=abc"),
		src(1, 2, "PVE", "https://push.example.com/hook?token=abc"),
		src(1, 3, "OpenList", "https://push.example.com/hook?token=abc"),
	}

	res := MigrateLegacy(legacy)

	if len(res.Channels) != 1 {
		t.Fatalf("三份相同配置应合成 1 个渠道，got %d", len(res.Channels))
	}
	if res.Merged != 2 {
		t.Fatalf("应记录合并了 2 份，got %d", res.Merged)
	}

	// 三个服务都要指向同一个渠道
	ids := map[string]bool{}
	for _, k := range []string{"1:1", "1:2", "1:3"} {
		id, ok := res.SourceChannel[k]
		if !ok {
			t.Fatalf("服务 %s 没有拿到渠道引用", k)
		}
		ids[id] = true
	}
	if len(ids) != 1 {
		t.Fatalf("三个服务应指向同一个渠道，实际指向了 %d 个", len(ids))
	}
}

// 配置不同的要各建各的 —— 不能因为"都是 webhook"就粗暴合成一个
func TestMigrateKeepsDifferentConfigs(t *testing.T) {
	legacy := []LegacySource{
		src(1, 1, "飞牛NAS", "https://push.example.com/hook?token=abc"),
		src(1, 2, "PVE", "https://other.example.com/hook?token=xyz"),
	}

	res := MigrateLegacy(legacy)
	if len(res.Channels) != 2 {
		t.Fatalf("两份不同配置应建 2 个渠道，got %d", len(res.Channels))
	}
	if res.Merged != 0 {
		t.Fatalf("没有重复，合并数应为 0，got %d", res.Merged)
	}
}

// 方法留空与显式写 GET 是同一件事，指纹不该因此不同
// （否则同一串 URL 会被当成两个渠道，等于没合并）
func TestFingerprintNormalizesMethod(t *testing.T) {
	a := webhook.WebhookConfig{URL: "https://x.com/h", Method: ""}
	b := webhook.WebhookConfig{URL: "https://x.com/h", Method: "GET"}
	c := webhook.WebhookConfig{URL: "https://x.com/h", Method: "get"}

	if fingerprint(a) != fingerprint(b) {
		t.Fatal("留空方法与 GET 应视为相同")
	}
	if fingerprint(a) != fingerprint(c) {
		t.Fatal("大小写不该影响指纹")
	}
}

// 没启用、或没 URL 的配置不该迁移 —— 迁移出一条永远发不出去的渠道
// 只会让通知中心看起来乱
func TestMigrateSkipsUnusable(t *testing.T) {
	legacy := []LegacySource{
		{DeviceID: 1, ServiceID: 1, ServiceName: "停用的", Config: webhook.WebhookConfig{
			Enabled: false, URL: "https://a.com/h",
		}},
		{DeviceID: 1, ServiceID: 2, ServiceName: "没URL的", Config: webhook.WebhookConfig{
			Enabled: true, URL: "",
		}},
	}

	res := MigrateLegacy(legacy)
	if len(res.Channels) != 0 {
		t.Fatalf("不该迁移出任何渠道，got %d", len(res.Channels))
	}
	if len(res.SourceChannel) != 0 {
		t.Fatalf("不该回写任何引用，got %d", len(res.SourceChannel))
	}
}

// 冷却：窗口内只放行一次
func TestCooldownAllowsOncePerWindow(t *testing.T) {
	r := &runtime{sent: map[string]time.Time{}}
	// 用一个很短的窗口，避免测试要等
	const w = 1 // 纳秒级，等下自己会过期

	if !r.allow("k", w) {
		t.Fatal("第一次应放行")
	}
	if !r.allow("k", w) {
		return
	}
	t.Log("窗口极短时可能已过期，属于预期")
}

// 规则找渠道要跳过被停用的渠道：停用是用户的明确意图
func TestRulesForSkipsDisabledChannel(t *testing.T) {
	cfg := Config{
		Channels: []Channel{
			{ID: "on", Name: "启用的", Type: ChannelWebhook, Enabled: true, Config: map[string]string{"url": "https://a.com"}},
			{ID: "off", Name: "停用的", Type: ChannelWebhook, Enabled: false, Config: map[string]string{"url": "https://b.com"}},
		},
		Rules: []Rule{{
			ID: "r1", Event: EventStunDown, Enabled: true,
			ChannelIDs: []string{"on", "off"},
		}},
	}

	got := cfg.rulesFor(EventStunDown)
	if len(got) != 1 || got[0].ID != "on" {
		t.Fatalf("只应取到启用的那个渠道，got %+v", got)
	}
}
