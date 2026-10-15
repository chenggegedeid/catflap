package core

import (
	"runtime"
	"testing"

	"github.com/sirupsen/logrus"
)

// 模块名从调用栈推，得对各种路径形态都成立 ——
// 推错了界面上按模块筛选就会漏日志，而且不会有任何报错。
func TestModuleFromCaller(t *testing.T) {
	cases := []struct {
		fn   string
		want string
	}{
		// 注意用的是**真实的 Caller.Function 形态**：包路径 + "." + 函数名，
		// 后面没有 .go（写成文件名就会测出一堆假通过）
		{"github.com/chenggegedeid/catflap/modules/stun.(*Scheduler).run", "stun"},
		{"github.com/chenggegedeid/catflap/modules/proxy.withAccessLog.func1", "proxy"},
		{"github.com/chenggegedeid/catflap/modules/ddns/dns.(*Cloudflare).SetRecord", "ddns"},
		// 模块内的子包也要归到模块名，不能是 model / dns 这种
		{"github.com/chenggegedeid/catflap/modules/stun/model.(*Config).Validate", "stun"},
		{"github.com/chenggegedeid/catflap/modules/traffic.Record", "traffic"},

		// api 层归一到模块名（stun_api → stun）
		{"github.com/chenggegedeid/catflap/api/stun_api.StunApi.StunServiceAddView", "stun"},
		{"github.com/chenggegedeid/catflap/api/notify_api.NotifyApi.NotifyConfigView", "notify"},
		{"github.com/chenggegedeid/catflap/api/ddns_api.DdnsApi.GetDdnsConfigView", "ddns"},

		// 框架层
		{"github.com/chenggegedeid/catflap/core/component.MustRegister", "core"},
		{"github.com/chenggegedeid/catflap/core.InitLogger", "core"},
		{"github.com/chenggegedeid/catflap/middleware.AuthMiddleware", "core"},

		// 路由
		{"github.com/chenggegedeid/catflap/routers.ProxyRouters", "router"},
	}

	for _, c := range cases {
		// logrus.Entry.Caller 就是 *runtime.Frame。**File 必须一起给** ——
		// logrus 的 HasCaller() 要求 Caller != nil **且** File != ""，
		// 只给 Function 会被判成"没有 caller"，函数直接短路返回 other
		// （这一条是调试时踩出来的：测试数据不完整会得到假的通过/失败）。
		entry := &logrus.Entry{
			Logger: logrus.New(),
			Caller: &runtime.Frame{Function: c.fn, File: "x.go", Line: 1},
		}

		got := ModuleFromCaller(entry)
		if got != c.want {
			t.Errorf("函数路径 %s → 期望 %s，实际 %s", c.fn, c.want, got)
		}
	}
}

// 没有 caller 时给 other，而不是空字符串（界面上会显示成一个空标签）
func TestModuleFromCallerWithoutCaller(t *testing.T) {
	entry := &logrus.Entry{}
	if got := ModuleFromCaller(entry); got != "other" {
		t.Fatalf("无 caller 时应为 other，实际 %q", got)
	}
	if got := ModuleFromCaller(nil); got != "other" {
		t.Fatalf("nil entry 时应为 other，实际 %q", got)
	}
}
