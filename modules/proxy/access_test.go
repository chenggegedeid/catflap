package proxy

import (
	"net/http"
	"testing"

	"github.com/chenggegedeid/catflap/modules/proxy/model"
)

func reqFrom(addr string) *http.Request {
	r, _ := http.NewRequest("GET", "http://nas.example.com/", nil)
	r.RemoteAddr = addr
	return r
}

func siteWith(mode model.AccessMode, rules ...string) model.Site {
	return model.Site{
		Hosts:  []string{"nas.example.com"},
		Access: model.AccessControl{Mode: mode, Rules: rules},
	}
}

// 没配规则一律放行：勾了白名单却没填内容时把所有人（包括自己）关在外面，
// 比不设防还糟——那会让人以为配置生效了其实只是把自己锁在门外。
func TestAccessAllowAllWhenEmpty(t *testing.T) {
	cases := []model.Site{
		{},
		siteWith(model.AccessModeAllow),
		siteWith(model.AccessModeDeny),
	}
	for _, s := range cases {
		if !accessAllowed(s, reqFrom("203.0.113.9:51234")) {
			t.Fatalf("空规则应放行，got 拒绝 (mode=%q)", s.Access.Mode)
		}
	}
}

func TestAccessAllowList(t *testing.T) {
	s := siteWith(model.AccessModeAllow, "203.0.113.0/24", "198.51.100.7")

	if !accessAllowed(s, reqFrom("203.0.113.9:51234")) {
		t.Fatal("白名单网段内的 IP 应放行")
	}
	if !accessAllowed(s, reqFrom("198.51.100.7:443")) {
		t.Fatal("白名单里的单个 IP 应放行")
	}
	if accessAllowed(s, reqFrom("192.0.2.1:443")) {
		t.Fatal("不在白名单里的应拒绝")
	}
}

func TestAccessDenyList(t *testing.T) {
	s := siteWith(model.AccessModeDeny, "203.0.113.0/24")

	if accessAllowed(s, reqFrom("203.0.113.9:51234")) {
		t.Fatal("黑名单网段内的 IP 应拒绝")
	}
	if !accessAllowed(s, reqFrom("192.0.2.1:443")) {
		t.Fatal("不在黑名单里的应放行")
	}
}

// 规则里带空格、IPv6、CIDR 混着写都要能认
func TestAccessRulesAreTrimmedAndIPv6Works(t *testing.T) {
	s := siteWith(model.AccessModeAllow, " 2001:db8::/32 ", "\t192.0.2.10\n")

	if !accessAllowed(s, reqFrom("[2001:db8::1]:443")) {
		t.Fatal("IPv6 网段应匹配（且规则前后的空白要剥掉）")
	}
	if !accessAllowed(s, reqFrom("192.0.2.10:443")) {
		t.Fatal("带制表符/换行的规则应能匹配")
	}
	if accessAllowed(s, reqFrom("192.0.2.11:443")) {
		t.Fatal("未列出的地址应拒绝")
	}
}

// 来源地址解析不出来时：白名单拒绝（拿不准就挡），黑名单放行（只挡列出来的）
func TestAccessUnparsableClientIP(t *testing.T) {
	if accessAllowed(siteWith(model.AccessModeAllow, "192.0.2.0/24"), reqFrom("")) {
		t.Fatal("解析不出来源时白名单应拒绝")
	}
	if !accessAllowed(siteWith(model.AccessModeDeny, "192.0.2.0/24"), reqFrom("")) {
		t.Fatal("解析不出来源时黑名单应放行")
	}
}

// 伪造 X-Forwarded-For 不能骗过门禁——准入判断只认 TCP 对端地址
func TestAccessIgnoresForgedForwardedFor(t *testing.T) {
	s := siteWith(model.AccessModeAllow, "198.51.100.0/24")
	r := reqFrom("192.0.2.66:443")
	r.Header.Set("X-Forwarded-For", "198.51.100.7")

	if accessAllowed(s, r) {
		t.Fatal("伪造 X-Forwarded-For 骗过了白名单")
	}
}
