package webhook

import (
	"strings"
	"testing"
	"time"
)

// TestSendErrorMasksURLSecrets 发送失败时，错误信息里不能出现 URL 上的凭证。
//
// 为什么要有这条测试：渠道的 token / key 大多直接拼在 URL 查询串里
// （PushPlus 是 ?token=xxx，Server 酱是 ?sendkey=xxx）。
// Go 的 http 包在拨号失败时返回的错误是
//
//	Post "https://www.pushplus.plus/send?token=xxx": dial tcp: ...
//
// —— URL 原样带在消息里。而这个错误会被一路回给前端
// （通知页的「测试发送」、服务里的「发一条测试通知」都是
// res.FailWithMsg("发送失败："+err.Error())），
// 等于把渠道凭证显示在浏览器里，还会进网络面板和各种日志。
//
// 所以这里是**安全约束**而不只是文案好看：错误信息必须先把 URL 抹掉。
func TestSendErrorMasksURLSecrets(t *testing.T) {
	const secret = "SUPERSECRETTOKEN0123456789"

	cfg := WebhookConfig{
		URL:    "http://127.0.0.1:1/send?token=" + secret,
		Method: "POST",
		Body:   "hello",
		// 关掉重试等待，测试不该真的睡 7 秒
		DisableSuccessCheck: true,
	}

	// 端口 1 没有任何服务在听，必然立刻失败
	start := time.Now()
	_, err := Send(cfg, map[string]string{})
	if err == nil {
		t.Fatalf("期望发送失败（目标不可达），实际返回了 nil")
	}
	if time.Since(start) > 20*time.Second {
		t.Fatalf("发送耗时过长（重试退避没生效？）：%v", time.Since(start))
	}

	msg := err.Error()
	if strings.Contains(msg, secret) {
		t.Errorf("错误信息泄漏了 URL 上的凭证：\n  %s", msg)
	}
	// 键名要留着（用户得知道是哪个参数），但每个值都必须是 ***
	for _, m := range queryValueRe.FindAllStringSubmatch(msg, -1) {
		// m[0] 全文 / m[1] 分隔符 / m[2] 键名
		if m[0] != m[1]+m[2]+"=***" {
			t.Errorf("查询参数 %q 的值没被抹掉：\n  %s", m[2], msg)
		}
	}
	// 顺带确认消息还读得通：抹完不该把后面的正文也吃进去
	if !strings.Contains(msg, "dial tcp") {
		t.Errorf("抹凭证时把错误正文也一起吃掉了：\n  %s", msg)
	}
}
