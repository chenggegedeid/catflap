package proxy

import (
	"strings"
	"testing"

	"github.com/chenggegedeid/catflap/modules/proxy/model"
)

// 端口转发必须挡住"把管理后台暴露出去"这条路径。
//
// 这是 v1.8 修过的漏洞（站点后端指向后台端口 → 管理页挂到公网 → 任何人
// 都能爆破 /api/auth/login）。端口转发是新功能，如果它没做同样的限制，
// 等于给那个漏洞新开一条路 —— 所以这两条是从一开始就钉住的。
func TestForwardRejectsAdminPort(t *testing.T) {
	admin := adminPort()
	entry := model.ProxyConfig{HTTPPorts: []int{80}, HTTPSPorts: []int{443}}

	cases := []struct {
		name string
		f    model.Forward
	}{
		{"监听端口是后台端口", model.Forward{ListenPort: admin, Target: "192.168.1.20:80"}},
		{"目标指向后台端口", model.Forward{ListenPort: 8081, Target: "127.0.0.1:3791"}},
		{"目标用 localhost 写法", model.Forward{ListenPort: 8081, Target: "localhost:3791"}},
		{"监听端口撞默认入口", model.Forward{ListenPort: 80, Target: "192.168.1.20:80"}},
		{"监听端口撞 HTTPS 入口", model.Forward{ListenPort: 443, Target: "192.168.1.20:80"}},
		{"没填监听端口", model.Forward{Target: "192.168.1.20:80"}},
		{"没填目标", model.Forward{ListenPort: 8081}},
		{"目标带空格", model.Forward{ListenPort: 8081, Target: "192.168.1.20: 80"}},
		{"目标是路径", model.Forward{ListenPort: 8081, Target: "/var/run/x.sock"}},
		{"协议瞎填", model.Forward{ListenPort: 8081, Target: "192.168.1.20:80", Protocol: "sctp"}},
	}
	for _, c := range cases {
		// 目标里的 3791 要换成真实后台端口，否则上面写死的用例在
		// CATFLAP_PORT 不是 3791 的环境里测不到点上
		f := c.f
		f.Target = strings.ReplaceAll(f.Target, ":3791", ":"+itoa(admin))
		if _, err := normalizeForward(f, entry); err == nil {
			t.Errorf("%s 应该被拒，实际通过了", c.name)
		}
	}
}

// 正常配置要能过（别把校验写得太严）
func TestForwardAcceptsNormal(t *testing.T) {
	entry := model.ProxyConfig{HTTPPorts: []int{80}}
	f, err := normalizeForward(model.Forward{
		ListenPort: 8081,
		Target:     "192.168.1.20:80",
	}, entry)
	if err != nil {
		t.Fatalf("正常配置不该被拒：%v", err)
	}
	if f.Protocol != "tcp" {
		t.Errorf("协议留空应默认成 tcp，实际 %q", f.Protocol)
	}

	// 大写 UDP 要归一成小写
	f2, err := normalizeForward(model.Forward{ListenPort: 8082, Target: "192.168.1.20:53", Protocol: "UDP"}, entry)
	if err != nil {
		t.Fatalf("UDP 不该被拒：%v", err)
	}
	if f2.Protocol != "udp" {
		t.Errorf("协议应归一成小写 udp，实际 %q", f2.Protocol)
	}
}

// 两条转发不能占同一个端口（否则 bind 有一个必然失败）
func TestForwardRejectsDuplicatePort(t *testing.T) {
	entry := model.ProxyConfig{
		Forwards: []model.Forward{{ID: 1, ListenPort: 8081, Target: "192.168.1.20:80"}},
	}
	// 另一条转发想占 8081
	if _, err := normalizeForward(model.Forward{ID: 2, ListenPort: 8081, Target: "192.168.1.21:80"}, entry); err == nil {
		t.Error("端口重复应被拒")
	}
	// 改自己（同 ID 同端口）不该被拒
	if _, err := normalizeForward(model.Forward{ID: 1, ListenPort: 8081, Target: "192.168.1.20:80"}, entry); err != nil {
		t.Errorf("改自己不该因为端口重复被拒：%v", err)
	}
}

// Scheme 必须是白名单里的值
func TestSchemeWhitelist(t *testing.T) {
	for _, ok := range []string{model.SchemeAuto, model.SchemeHTTP, model.SchemeHTTPS} {
		s, err := normalizeSite(model.Site{
			Hosts:   []string{"a.com"},
			Backend: "192.168.1.20:80",
			Scheme:  ok,
		}, "")
		if err != nil {
			t.Errorf("scheme %q 应该合法，实际被拒：%v", ok, err)
		}
		if ok == model.SchemeHTTPS && !s.HTTPS {
			t.Error("选了 https 时 HTTPS 应自动置为 true（否则这个站点一个入口都挂不上）")
		}
	}

	if _, err := normalizeSite(model.Site{
		Hosts:   []string{"a.com"},
		Backend: "192.168.1.20:80",
		Scheme:  "ftp",
	}, ""); err == nil {
		t.Error("白名单外的 scheme 应被拒（否则会悄悄降级成 auto，行为和用户想的不一样）")
	}
}

func itoa(n int) string {
	if n == 0 {
		return "0"
	}
	var b []byte
	for n > 0 {
		b = append([]byte{byte('0' + n%10)}, b...)
		n /= 10
	}
	return string(b)
}
