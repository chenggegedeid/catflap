package notify

import (
	"strings"
	"time"

	"github.com/chenggegedeid/catflap/modules/stun"

	"github.com/sirupsen/logrus"
)

// ===================== 接管穿透服务的通知 =====================
//
// stun 不能反过来 import notify（依赖方向是这边过去），
// 所以由通知中心主动把发送器注册进去 —— 和 device 的 IP 变更 hook 同一套路。

// installStunHook 注册穿透通知的发送器
func installStunHook() {
	stun.SetNotifyHook(func(req stun.NotifyRequest) {
		// 正文：用户写了模板就用模板（渲染占位符），否则给一句默认的话
		content := renderTemplate(req.Template, req.Fields)
		if strings.TrimSpace(content) == "" {
			content = defaultTunnelContent(req.Fields)
		}

		for _, id := range req.ChannelIDs {
			ch, ok := Runtime.FindChannel(id)
			if !ok || !ch.Enabled {
				continue
			}
			// 走渠道直发（不走"什么事件匹配什么规则"那一层 —— 服务选的就是精确指向）
			sendToChannel(ch, Event{
				Type:    EventType("stun.success"),
				Title:   req.Title,
				Content: content,
				Fields:  req.Fields,
				Key:     req.Key + "|" + id,
				At:      time.Now(),
			})
		}
	})
	logrus.Debug("[notify] 已接管穿透服务通知")
}

// defaultTunnelContent 没写模板时的默认正文
//
// 排版照着"手机上扫一眼就知道连哪儿"来：
// 域名那行是真正要用的地址（配了域名时），下面再给 IP 兜底 ——
// 域名解析还没生效、或者在外面临时连的时候要用 IP。
// 时间用可读格式，不再丢一个 RFC3339（带 +08:00 尾巴）给用户看。
func defaultTunnelContent(fields map[string]string) string {
	// 标题已经是「服务名:端口」了（见 stun/scheduler.go），正文只放详情 ——
	// 不重复服务名，也不重复那个最显眼的端口号。
	lines := make([]string, 0, 6)

	if fields["domain"] != "" {
		lines = append(lines, "域名+端口："+fields["address"])
		lines = append(lines, "IP地址+端口："+fields["ip_addr"])
	} else {
		// 没配域名就只有 IP 一条，别写"域名+端口："然后留空
		lines = append(lines, "外部地址："+fields["address"])
	}
	if fields["device_name"] != "" {
		lines = append(lines, "设备："+fields["device_name"])
	}
	if fields["internal_port"] != "" {
		lines = append(lines, "内网端口："+fields["internal_port"])
	}
	if fields["time"] != "" {
		lines = append(lines, "时间："+fields["time"])
	}
	return strings.Join(lines, "\n")
}

// renderTemplate 渲染 #{xxx} 占位符。
// 和 webhook 那边的占位符写法保持一致，用户不用记两套。
func renderTemplate(tpl string, fields map[string]string) string {
	if strings.TrimSpace(tpl) == "" {
		return ""
	}
	out := tpl
	for k, v := range fields {
		out = strings.ReplaceAll(out, "#{"+k+"}", v)
	}
	return out
}
