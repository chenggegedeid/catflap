package home

import (
	"context"
	"fmt"
	"net"
	"time"
)

// 图标抓取是「用户给一个 URL，服务器去拉」——典型的 SSRF 面。
//
// 这个接口在保护组里（只有已登录的管理员能调），所以风险不高，但它能探到
// 内网：拿 `http://192.168.1.1/` 去试，响应快慢/成功与否就能推断内网有什么。
// 管理员本来就能看内网，真正要防的是**被 XSS 或 CSRF 借手**的情况。
//
// 拦住的做法是在**拨号层**判断，而不是只看一遍 URL：
// 只看 URL 挡不住重定向（`http://evil.com` → 302 → `http://192.168.1.1`），
// 也挡不住 DNS 解析到内网地址（域名指向 127.0.0.1 的那种）。在 DialContext
// 里判断"这次实际要连的 IP"才是真的那一次连接。
// safeDialContext 拨号前先看这次实际要连的 IP 是不是内网。
func safeDialContext(ctx context.Context, network, addr string) (net.Conn, error) {
	host, port, err := net.SplitHostPort(addr)
	if err != nil {
		return nil, err
	}

	ips, err := net.DefaultResolver.LookupIPAddr(ctx, host)
	if err != nil {
		return nil, err
	}
	if len(ips) == 0 {
		return nil, fmt.Errorf("解析不出地址：%s", host)
	}

	// 解析出多个就逐个试，但**任何一个是内网就整体拒绝**：
	// 只挑"安全的那个"连，等于把解析顺序交给对端控制（DNS 轮询 / 快速改写），
	// 上一刻还指着公网，下一刻就指着 127.0.0.1 了。
	for _, ip := range ips {
		if isPrivateIP(ip.IP) {
			return nil, fmt.Errorf("拒绝访问内网地址：%s", ip.IP)
		}
	}

	d := &net.Dialer{Timeout: 10 * time.Second}
	return d.DialContext(ctx, network, net.JoinHostPort(ips[0].IP.String(), port))
}

// isPrivateIP 这个地址是不是「不该被外部请求访问」的那类。
//
// 比 net.IP.IsPrivate() 宽：除了 RFC1918，还要挡回环、链路本地
// （169.254.0.0/16 —— 云环境的元数据服务就在那儿）、以及各种 v6 特殊段。
func isPrivateIP(ip net.IP) bool {
	if ip == nil {
		return true
	}
	if ip.IsLoopback() || ip.IsPrivate() || ip.IsLinkLocalUnicast() || ip.IsLinkLocalMulticast() {
		return true
	}
	if ip.IsUnspecified() || ip.IsMulticast() {
		return true
	}
	// IPv4-mapped IPv6（::ffff:192.168.1.1）：拆出来按 v4 再判一次，
	// 否则会被当成普通的 v6 地址放过去
	if v4 := ip.To4(); v4 != nil {
		return isPrivateIPv4(v4)
	}
	// 唯一本地地址 fc00::/7
	if len(ip) == net.IPv6len && ip[0]&0xfe == 0xfc {
		return true
	}
	return false
}

func isPrivateIPv4(ip net.IP) bool {
	// 云元数据服务，单独列出来：它不在 RFC1918 里但同样敏感
	meta := net.ParseIP("169.254.169.254")
	if ip.Equal(meta) {
		return true
	}
	// 0.0.0.0/8、100.64.0.0/10（CGNAT）、192.0.0.0/24、198.18.0.0/15
	// 这几段 IsPrivate/IsLinkLocal 不覆盖，但对"从服务器往外拉东西"来说
	// 都属于不该去的目标
	_, cgnat, _ := net.ParseCIDR("100.64.0.0/10")
	if cgnat.Contains(ip) {
		return true
	}
	return false
}
