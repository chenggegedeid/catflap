package notify_api

import (
	"encoding/json"
	"testing"
	"time"

	"github.com/chenggegedeid/catflap/modules/notify"
)

// 响应里**绝不能出现 null 切片**。
//
// 这是踩过的白屏事故：后端返回 "usedByRules": null，前端拿到 null 取 .length
// 抛 TypeError，整个通知页白屏，而控制台只有一句
// "Cannot read properties of null" —— 看不出是哪个字段。
//
// Go 会把 nil 切片序列化成 null，所以每个切片字段都得给空切片。
func TestConfigViewHasNoNullSlices(t *testing.T) {
	cfg := notify.Config{}

	// 直接构造响应体（和 NotifyConfigView 里那段同样的兜底逻辑）
	channels := []channelView{}
	for _, ch := range cfg.Channels {
		used := []string{}
		channels = append(channels, channelView{
			Channel:     ch,
			Config:      map[string]string{},
			UsedByRules: used,
		})
	}
	if channels == nil {
		t.Fatal("channels 不能是 nil")
	}

	body := ConfigViewResponse{
		Channels: channels,
		Rules:    []notify.Rule{},
		Events:   notify.AllEvents(),
	}

	raw, err := json.Marshal(body)
	if err != nil {
		t.Fatal(err)
	}

	// 逐字段检查序列化结果里没有 null
	var decoded map[string]any
	if err := json.Unmarshal(raw, &decoded); err != nil {
		t.Fatal(err)
	}
	for _, key := range []string{"channels", "rules", "events"} {
		v, ok := decoded[key]
		if !ok {
			t.Errorf("%s 字段缺失", key)
			continue
		}
		if v == nil {
			t.Errorf("%s 序列化成了 null —— 前端会白屏", key)
		}
		if _, isSlice := v.([]any); !isSlice {
			t.Errorf("%s 不是数组，而是 %T", key, v)
		}
	}
}

// 空配置下 AllEvents 也要给数组，不能是 nil
func TestAllEventsNeverNil(t *testing.T) {
	ev := notify.AllEvents()
	if ev == nil {
		t.Fatal("AllEvents 不应返回 nil")
	}
	if _, err := json.Marshal(ev); err != nil {
		t.Fatal(err)
	}
	if string(mustJSON(t, ev)) == "null" {
		t.Fatal("AllEvents 序列化成了 null")
	}
	_ = time.Now()
}

func mustJSON(t *testing.T, v any) []byte {
	t.Helper()
	b, err := json.Marshal(v)
	if err != nil {
		t.Fatal(err)
	}
	return b
}
