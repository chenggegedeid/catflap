package proxy_api

import (
	"net"

	"github.com/chenggegedeid/catflap/core/lanscan"
	"github.com/chenggegedeid/catflap/modules/ddns"
	"github.com/chenggegedeid/catflap/utils/res"

	"github.com/gin-gonic/gin"
)

// ===================== 站点表单的可选项 =====================
//
// 加/改站点时要选的东西散在几个模块里：局域网网段在 core/lanscan、
// 已配置的域名在 ddns。**在 api 层把它们合起来**，业务模块之间不加依赖边。
//
// 为什么不让 proxy 直接 import ddns：ddns 初始化要等 STUN 探测公网 IP
// （几秒），而反代的监听不该被它拖住。聚合放这里，依赖方向是
// api 层 → 各个模块，模块之间互不认识。

// SiteOptionsResponse 加站点时可选的那些东西
type SiteOptionsResponse struct {
	// Domains 已配的 DDNS 域名，可以拿来当站点域名
	Domains []ddns.DomainOption `json:"domains"`
	// Subnets 本机局域网网段，扫描时选
	Subnets []lanscan.LanSubnet `json:"subnets"`
	// LocalIPs 本机地址。想让站点指向本机时不用查网卡
	LocalIPs []string `json:"localIPs"`
}

// ProxySiteOptionsView 站点表单要的全部可选项
func (ProxyApi) ProxySiteOptionsView(c *gin.Context) {
	domains := ddns.ListDomains()
	if domains == nil {
		domains = []ddns.DomainOption{}
	}
	subnets := lanscan.ListLanSubnets()
	if subnets == nil {
		subnets = []lanscan.LanSubnet{}
	}

	res.OkWithData(SiteOptionsResponse{
		Domains:  domains,
		Subnets:  subnets,
		LocalIPs: localIPs(),
	}, c)
}

// localIPs 本机的 IPv4 地址。用 lanscan 的网段信息就够了 ——
// 它已经扫过网卡，不用再重复一遍。
func localIPs() []string {
	subnets := lanscan.ListLanSubnets()
	out := make([]string, 0, len(subnets))
	seen := map[string]bool{}
	for _, s := range subnets {
		ip := s.LocalIP
		if ip == "" || seen[ip] {
			continue
		}
		seen[ip] = true
		out = append(out, ip)
	}
	// 回环地址总在：跑在本机的服务填它最省事
	if net.ParseIP("127.0.0.1") != nil {
		out = append(out, "127.0.0.1")
	}
	return out
}
