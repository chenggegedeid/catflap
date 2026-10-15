package notify

import (
	"encoding/json"
	"net/url"
	"strings"
)

func orDefault(v, def string) string {
	if strings.TrimSpace(v) == "" {
		return def
	}
	return v
}

func urlEscape(s string) string { return url.QueryEscape(s) }

// joinURL Bark 的地址是拼出来的，各段都要转义 —— 标题里带空格或中文时
// 不转义会把 URL 拆坏。
func joinURL(server, key, title, content string) string {
	if strings.TrimSpace(server) == "" {
		server = "https://api.day.app"
	}
	return strings.TrimRight(server, "/") + "/" +
		url.PathEscape(key) + "/" +
		url.PathEscape(title) + "/" +
		url.PathEscape(content)
}

// groupRobotBody 三家群机器人的消息体。
//
// 它们都是"POST 一段固定结构的 JSON"，但键名各不相同：
//
//	钉钉 {"msgtype":"text","text":{"content":...}}
//	企业微信 {"msgtype":"text","text":{"content":...}}   ← 和钉钉同构
//	飞书 {"msg_type":"text","content":{"text":...}}      ← 注意是下划线
//
// 拼错键名对方会回 200 但什么都不显示，最难查。
func groupRobotBody(t ChannelType, title, content string) string {
	text := title
	if content != "" {
		text = title + "\n" + content
	}

	var payload any
	if t == ChannelFeishu {
		payload = map[string]any{
			"msg_type": "text",
			"content":  map[string]string{"text": text},
		}
	} else {
		payload = map[string]any{
			"msgtype": "text",
			"text":    map[string]string{"content": text},
		}
	}

	b, err := json.Marshal(payload)
	if err != nil {
		// 上面都是 map[string]string，不会失败；兜底给个能发出去的
		return `{"msgtype":"text","text":{"content":"Catflap 通知"}}`
	}
	return string(b)
}

func pushPlusBody(token, title, content string) string {
	b, err := json.Marshal(map[string]string{
		"token":    token,
		"title":    title,
		"content":  content,
		"template": "txt",
	})
	if err != nil {
		return `{}`
	}
	return string(b)
}
