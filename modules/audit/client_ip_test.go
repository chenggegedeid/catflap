package audit

import (
	"net/http"
	"testing"
)

// 本文件守 ResolveClientIP —— 它是封禁、日志、审计三处的共同判据。
//
// 最要紧的一条是「对端不可信时，伪造的 X-Forwarded-For 必须完全无效」：
// 这是整个 IP 门禁的地基，地基塌了后面全白搭。

func TestResolveClientIP(t *testing.T) {
	// 测试期间直接改 Runtime.trustedNets：它是包内字段，
	// 走 rebuildIndexes 绕一圈反而看不清到底测的是哪一条路径。
	setTrusted := func(list ...string) {
		t.Helper()
		Runtime.mu.Lock()
		defer Runtime.mu.Unlock()
		Runtime.trustedNets = parseNets(list)
	}
	setTrusted() // 用例之间不留状态：默认清空

	cases := []struct {
		name    string
		trusted []string
		remote  string
		xff     string
		cfip    string
		want    string
	}{
		{
			name:   "没配可信反代，直接连：返回对端地址",
			remote: "203.0.113.7:51234",
			want:   "203.0.113.7",
		},
		{
			name:   "没配可信反代，但客户端伪造了 XFF：伪造无效，仍返回对端",
			remote: "203.0.113.7:51234",
			xff:    "1.2.3.4",
			want:   "203.0.113.7",
		},
		{
			name:    "配了可信反代，但来访者对端不是代理：XFF 仍不可信",
			trusted: []string{"198.51.100.0/24"},
			remote:  "203.0.113.7:51234",
			xff:     "1.2.3.4",
			want:    "203.0.113.7",
		},
		{
			name:    "对端是可信代理：取 XFF 里的值",
			trusted: []string{"198.51.100.0/24"},
			remote:  "198.51.100.10:443",
			xff:     "203.0.113.7",
			want:    "203.0.113.7",
		},
		{
			name:    "客户端伪造 XFF，CF 在末尾追加真实来源：从右往左取到真的那个",
			trusted: []string{"198.51.100.0/24"},
			remote:  "198.51.100.10:443",
			xff:     "9.9.9.9, 203.0.113.7",
			want:    "203.0.113.7",
		},
		{
			name:    "多层代理：跳过可信的那一跳，取再往左一个",
			trusted: []string{"198.51.100.0/24", "10.0.0.1"},
			remote:  "198.51.100.10:443",
			xff:     "203.0.113.7, 10.0.0.1",
			want:    "203.0.113.7",
		},
		{
			name:    "XFF 整条链都可信：退到 CF-Connecting-IP",
			trusted: []string{"198.51.100.0/24"},
			remote:  "198.51.100.10:443",
			xff:     "198.51.100.77",
			cfip:    "203.0.113.7",
			want:    "203.0.113.7",
		},
		{
			name:    "XFF 为空但对端可信：用 CF-Connecting-IP",
			trusted: []string{"198.51.100.0/24"},
			remote:  "198.51.100.10:443",
			cfip:    "203.0.113.7",
			want:    "203.0.113.7",
		},
		{
			name:    "XFF 里是不合法 IP（垃圾）：跳过它，退回对端",
			trusted: []string{"198.51.100.0/24"},
			remote:  "198.51.100.10:443",
			xff:     "not-an-ip",
			want:    "198.51.100.10",
		},
		{
			name:    "XFF 里带端口：规整成裸 IP",
			trusted: []string{"198.51.100.0/24"},
			remote:  "198.51.100.10:443",
			xff:     "203.0.113.7:51234",
			want:    "203.0.113.7",
		},
		{
			name:   "IPv6 对端带方括号：去掉方括号",
			remote: "[2001:db8::1]:51234",
			want:   "2001:db8::1",
		},
		{
			name:    "IPv6 走可信代理：正确取出来源",
			trusted: []string{"2001:db8:aaaa::/48"},
			remote:  "[2001:db8:aaaa::1]:443",
			xff:     "2001:db8:bbbb::9",
			want:    "2001:db8:bbbb::9",
		},
		{
			name:    "网段写法的可信代理也认（/24 里的任意地址）",
			trusted: []string{"198.51.100.0/24"},
			remote:  "198.51.100.200:443",
			xff:     "203.0.113.7",
			want:    "203.0.113.7",
		},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			setTrusted(c.trusted...)

			req, err := http.NewRequest(http.MethodGet, "http://example.com/", nil)
			if err != nil {
				t.Fatalf("构造请求失败: %v", err)
			}
			req.RemoteAddr = c.remote
			if c.xff != "" {
				req.Header.Set("X-Forwarded-For", c.xff)
			}
			if c.cfip != "" {
				req.Header.Set("CF-Connecting-IP", c.cfip)
			}

			if got := ResolveClientIP(req); got != c.want {
				t.Errorf("来源 IP = %q，期望 %q", got, c.want)
			}
		})
	}

	setTrusted() // 还原，别影响同包里其它测试
}

// TestResolveClientIPNilRequest 空请求不能 panic。
//
// 反代的 ErrorHandler 那条路径上，请求对象有可能还没走完初始化就进来了。
func TestResolveClientIPNilRequest(t *testing.T) {
	if got := ResolveClientIP(nil); got != "" {
		t.Errorf("空请求应返回空串，实际 %q", got)
	}
}
