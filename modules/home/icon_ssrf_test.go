package home

import (
	"net"
	"testing"
)

// 图标抓取要挡住各种"内网地址"写法，否则它就是个内网探测器。
func TestIsPrivateIP(t *testing.T) {
	blocked := []string{
		"127.0.0.1",          // 回环
		"::1",                // v6 回环
		"10.0.0.1",           // RFC1918
		"172.16.5.5",         // RFC1918
		"192.168.1.1",        // RFC1918（家里路由器）
		"169.254.169.254",    // 云元数据服务，单独要挡
		"169.254.1.1",        // 链路本地
		"100.64.0.1",         // CGNAT
		"0.0.0.0",            // 未指定
		"fc00::1",            // v6 唯一本地
		"fe80::1",            // v6 链路本地
		"::ffff:192.168.1.1", // IPv4-mapped IPv6：要拆出来按 v4 判
	}
	for _, s := range blocked {
		if !isPrivateIP(net.ParseIP(s)) {
			t.Errorf("%s 应该被拒绝，实际放行了", s)
		}
	}

	allowed := []string{
		"8.8.8.8",
		"1.1.1.1",
		"203.0.113.10", // 文档示例段，公网行为
		"2606:4700::1111",
	}
	for _, s := range allowed {
		if isPrivateIP(net.ParseIP(s)) {
			t.Errorf("%s 是公网地址，不该被拒", s)
		}
	}
}

// nil 要当成"不安全的"处理：解析失败时不能默认放行
func TestIsPrivateIPNilIsBlocked(t *testing.T) {
	if !isPrivateIP(nil) {
		t.Fatal("nil 地址应视为不安全")
	}
}
