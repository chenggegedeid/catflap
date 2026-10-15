package proxy

import (
	"errors"
	"fmt"
	"net"
	"strconv"
	"strings"
	"time"

	"github.com/chenggegedeid/catflap/core"
	"github.com/chenggegedeid/catflap/modules/proxy/model"
	"github.com/chenggegedeid/catflap/utils/domain"
)

// 本文件是 api 层改配置的唯一入口。
//
// 校验刻意放在模块里而不是 api 层：站点表的不变量（域名必填、同域名同前缀
// 不能重复）是转发行为的一部分，让每个调用方各写一份迟早会写歪。

// ==== 站点 ====

// SaveSite 新增或更新一个站点，ID 为 0 表示新增
func SaveSite(in model.Site) (model.Site, error) {
	site, err := normalizeSite(in, Runtime.Snapshot().BaseDomain)
	if err != nil {
		return model.Site{}, err
	}

	var saved model.Site
	err = Runtime.Update(func(cfg *model.ProxyConfig) error {
		if dup, host, ok := conflictSite(cfg.Sites, site); ok {
			return fmt.Errorf("%s 上的%s已经配过了（后端 %s），换一个路径，或者去改那一条",
				host, prefixLabel(dup.PathPrefix), dup.Backend)
		}
		if err := checkPortProtocol(cfg, site); err != nil {
			return err
		}
		site.UpdatedAt = time.Now()

		if site.ID == 0 {
			site.ID = nextSiteID(cfg)
			cfg.Sites = append(cfg.Sites, site)
			saved = site
			return nil
		}
		for i := range cfg.Sites {
			if cfg.Sites[i].ID == site.ID {
				cfg.Sites[i] = site
				saved = site
				return nil
			}
		}
		return errors.New("站点不存在")
	})
	if err != nil {
		return model.Site{}, err
	}
	return saved, nil
}

// DeleteSite 删除一个站点
func DeleteSite(id uint) error {
	return Runtime.Update(func(cfg *model.ProxyConfig) error {
		for i := range cfg.Sites {
			if cfg.Sites[i].ID == id {
				cfg.Sites = append(cfg.Sites[:i], cfg.Sites[i+1:]...)
				return nil
			}
		}
		return errors.New("站点不存在")
	})
}

func normalizeSite(in model.Site, base string) (model.Site, error) {
	out := in
	baseDomain := domain.Normalize(base)

	hosts := make([]string, 0, len(out.Hosts))
	for _, raw := range out.Hosts {
		h := domain.Normalize(raw)
		if h == "" {
			continue
		}
		// 先挑毛病再补主域名：写成 nas:8080 的话补完会变成 nas:8080.example.com，
		// 报错时把那个怪东西原样甩给用户只会更难懂
		if strings.ContainsAny(h, "/:") {
			return out, fmt.Errorf("域名 %s 只填域名本身，不要带协议、端口或路径", h)
		}
		hosts = append(hosts, expandHost(h, baseDomain))
	}
	out.Hosts = NormalizeHosts(hosts)

	out.Backend = strings.TrimSpace(out.Backend)
	out.PathPrefix = normalizePrefix(out.PathPrefix)
	out.Description = strings.TrimSpace(out.Description)

	switch {
	// 域名留空 = 当端口转发使，那这个端口上只能有它一条，所以端口必填。
	// nginx 里不写 server_name 的 server 块同理，得自己占一个 listen。
	case len(out.Hosts) == 0 && out.ListenPort == 0:
		return out, errors.New("要么填域名，要么给它填一个自己的端口——挂在默认入口上的站点靠域名分流，没域名分不开")
	case out.Backend == "":
		return out, errors.New("后端地址必填，形如 192.168.1.20:8080")
	case strings.ContainsAny(out.Backend, " \t"):
		return out, errors.New("后端地址里有空格，请检查")
	case strings.HasPrefix(out.Backend, "/"):
		return out, errors.New("后端地址要填主机和端口，形如 192.168.1.20:8080")
	case backendHitsAdminPort(out.Backend, adminPort()):
		// 后端指向 Catflap 自己的后台端口 = 把管理后台挂到公网。
		//
		// 这一条看着像"用户不会这么填"，但反而是很容易犯的错：想在外网访问
		// 管理页面时，最顺手的做法就是建个站点指向 127.0.0.1:<管理端口>，
		// 心想"反正还要登录"。结果是把登录页和 /api/auth/login 一起挂到
		// 公网，谁都能来爆破密码，猜中就完全接管。
		//
		// 要在外网管 Catflap，正确做法是让它监听的地址本身可达（或走
		// 你自己的 VPN），而不是通过本程序的反代转发给自己。
		return out, fmt.Errorf(
			"后端不能填 Catflap 自己的后台端口 %d——那等于把管理后台挂到公网上，任何人都能来试密码",
			adminPort())
	}

	if out.ListenPort != 0 {
		if out.ListenPort < 0 || out.ListenPort > 65535 {
			return out, errors.New("端口要填 1 到 65535 之间的数字")
		}
		if p := adminPort(); out.ListenPort == p {
			return out, fmt.Errorf("%d 是 Catflap 自己的后台端口，换一个", p)
		}
	}
	if !out.HTTPS {
		// 没开 HTTPS 就没有绑定证书一说，和 STUN 那边同一个取舍
		out.CertID = 0
	}
	return out, nil
}

// checkPortProtocol 同一个端口不能既是 http 又是 https。
//
// 一个监听只有一个 tls.Config，两边都想要的话必然有一边打不开——
// 与其让用户对着一个打不开的站点猜，不如在保存的时候就说清楚。
// 证书不同倒无所谓：那是 SNI 该干的事，planBindings 会自动退回自动匹配。
func checkPortProtocol(cfg *model.ProxyConfig, site model.Site) error {
	port := site.ListenPort
	if port == 0 || !site.Enabled {
		return nil
	}
	if containsInt(cfg.HTTPPortList(), port) && site.WantsHTTPS() {
		return fmt.Errorf("端口 %d 是默认 HTTP 入口，想让这个站点走 HTTPS 就别单独指定端口，或者换一个端口", port)
	}
	if containsInt(cfg.HTTPSPortList(), port) && !site.WantsHTTPS() {
		return fmt.Errorf("端口 %d 是默认 HTTPS 入口，这个站点没开 HTTPS——换一个端口，或者把 HTTPS 打开", port)
	}

	for _, s := range cfg.Sites {
		if s.ID == site.ID || !s.Enabled || s.ListenPort != port {
			continue
		}
		if s.HTTPS != site.HTTPS {
			return fmt.Errorf("端口 %d 上的站点 %s 是 %s，同一个端口不能一半 HTTP 一半 HTTPS",
				port, s.Label(), schemeLabel(s.HTTPS))
		}
	}
	return nil
}

func schemeLabel(https bool) string {
	if https {
		return "HTTPS"
	}
	return "HTTP"
}

// expandHost 用主域名把前缀补成完整域名：配了 example.com 之后，nas → nas.example.com。
//
// 判据是「有没有点」：带点的当成用户已经写全了，原样保留——
// 他完全可能同时代理另一个域名下的服务，不该被主域名绑死。
func expandHost(host, base string) string {
	if host == "" || base == "" || strings.Contains(host, ".") {
		return host
	}
	if host == "*" {
		return "*." + base
	}
	return host + "." + base
}

// conflictSite 同一个域名下同一个路径前缀只能有一条。
//
// 一个站点能挂好几个域名，所以撞车的判据是「域名集合有交集」。
// 返回值里带上撞的是哪个域名：只说「和某某站点冲突」的话，
// 挂了五个域名的那条站点还得让用户自己去比对。
//
// 没填域名的站点比不了域名，改按端口比：那种站点是所在端口的兜底，
// 一个端口上同一个路径有两条兜底的话，谁生效全看排序，等于抛硬币。
func conflictSite(sites []model.Site, site model.Site) (model.Site, string, bool) {
	if len(site.Hosts) == 0 {
		for _, s := range sites {
			if s.ID == site.ID || normalizePrefix(s.PathPrefix) != site.PathPrefix {
				continue
			}
			if len(NormalizeHosts(s.Hosts)) > 0 || s.ListenPort != site.ListenPort {
				continue
			}
			return s, site.Label(), true
		}
		return model.Site{}, "", false
	}

	want := make(map[string]bool, len(site.Hosts))
	for _, h := range site.Hosts {
		want[h] = true
	}
	for _, s := range sites {
		if s.ID == site.ID || normalizePrefix(s.PathPrefix) != site.PathPrefix {
			continue
		}
		for _, h := range NormalizeHosts(s.Hosts) {
			if want[h] {
				return s, h, true
			}
		}
	}
	return model.Site{}, "", false
}

func prefixLabel(prefix string) string {
	if prefix == "" {
		return "整站转发"
	}
	return "路径 " + prefix
}

// ==== 默认入口 ====

// adminPort Catflap 后台自己占的端口，反代不能抢。
//
// 跟监听端口走同一个来源（core.ListenPort）：端口改成环境变量指定的值时，
// 这里也得跟着变，否则「反代不能抢后台端口」这条保护就会保护错端口。
func adminPort() int { return core.ListenPort() }

// EntrySettings 两个默认入口：HTTP 若干端口，HTTPS 若干端口。
//
// 对应 nginx 的 listen 80 / listen 443 ssl，也对应 GoDoxy 的
// HTTP_ADDR / HTTPS_ADDR。站点不单独指定端口就挂在默认入口上。
//
// 端口改成**列表**：内网走 80、外网映射走 8080 这种需求，
// 以前得在每条站点上各填一遍"独占端口"，现在在入口加一个就行。
type EntrySettings struct {
	Enabled bool
	// HTTPPorts HTTP 入口端口，可以多个；空 = 不开这个入口
	HTTPPorts []int
	// HTTPSPorts HTTPS 入口端口，可以多个；空 = 不开这个入口
	HTTPSPorts []int
	CertID     uint
	BaseDomain string
}

// SetEntry 保存默认入口设置并立刻生效。
// 端口占用之类的失败会原样返回——配置已经存下了，用户改个端口再存一次即可。
func SetEntry(in EntrySettings) error {
	if err := checkEntryPorts("HTTP 端口", in.HTTPPorts); err != nil {
		return err
	}
	if err := checkEntryPorts("HTTPS 端口", in.HTTPSPorts); err != nil {
		return err
	}
	if in.Enabled && len(in.HTTPPorts) == 0 && len(in.HTTPSPorts) == 0 {
		return errors.New("HTTP 和 HTTPS 端口至少要填一个，否则没有入口")
	}
	// 同一个端口不能既是 HTTP 入口又是 HTTPS 入口：
	// 那会让两个监听抢同一个端口，bind 必有一个失败
	for _, p := range in.HTTPPorts {
		if containsInt(in.HTTPSPorts, p) {
			return fmt.Errorf("端口 %d 不能同时作为 HTTP 和 HTTPS 入口", p)
		}
	}

	base := domain.Normalize(in.BaseDomain)
	if strings.ContainsAny(base, "/:*") {
		return errors.New("主域名只填域名本身，形如 example.com")
	}

	return Runtime.Update(func(cfg *model.ProxyConfig) error {
		cfg.Enabled = in.Enabled
		cfg.BaseDomain = base
		cfg.HTTPPorts = in.HTTPPorts
		cfg.HTTPSPorts = in.HTTPSPorts
		cfg.CertID = in.CertID
		if len(in.HTTPSPorts) == 0 {
			cfg.CertID = 0
		}
		return nil
	})
}

// checkEntryPorts 校验一组入口端口。空列表 = 不开这个入口，是合法的。
func checkEntryPorts(label string, ports []int) error {
	admin := adminPort()
	seen := map[int]bool{}
	for _, port := range ports {
		switch {
		case port <= 0 || port > 65535:
			return fmt.Errorf("%s要填 1 到 65535 之间的数字", label)
		case port == admin:
			return fmt.Errorf("%d 是 Catflap 自己的后台端口，换一个", admin)
		case seen[port]:
			// 同一个入口里填两个一样的端口，bind 会失败
			return fmt.Errorf("%s里端口 %d 填了两次", label, port)
		}
		seen[port] = true
	}
	return nil
}

// SetAccessLog 单独开关访问日志，不碰监听
func SetAccessLog(on bool) error {
	return Runtime.Update(func(cfg *model.ProxyConfig) error {
		cfg.AccessLog = on
		return nil
	})
}

// backendHitsAdminPort 后端地址是不是指向 Catflap 自己的后台端口。
//
// 为什么要专门判：后端填 127.0.0.1:<管理端口> 会把管理后台转发到公网，
// 任何人都能在上面爆破密码（详见 normalizeSite 里那条错误的说明）。
//
// 判据是"主机名解析后是本机的任一地址 **且** 端口等于管理端口"：
//   - 只判字符串等于 "127.0.0.1:3791" 挡不住 localhost、::1、本机内网 IP
//   - 只判"是本机地址"会误伤正常场景（服务确实跑在本机，比如 127.0.0.1:8080）
//
// 所以两个条件都要满足才拦。
func backendHitsAdminPort(backend string, admin int) bool {
	if admin <= 0 {
		return false
	}

	host, portStr, err := net.SplitHostPort(strings.TrimSpace(backend))
	if err != nil {
		return false
	}
	port, err := strconv.Atoi(portStr)
	if err != nil || port != admin {
		return false
	}

	return isLocalHost(host)
}

// isLocalHost 这个主机名是不是指向本机。
// 覆盖 127.0.0.0/8、::1、localhost、以及本机所有网卡上的地址。
func isLocalHost(host string) bool {
	host = strings.Trim(host, "[]") // IPv6 字面量可能带方括号
	if host == "" {
		return false
	}

	// 字面量直接判，不用走解析
	if ip := net.ParseIP(host); ip != nil {
		if ip.IsLoopback() {
			return true
		}
		// 本机网卡上的地址（含内网 IP），也算"指向这台机器"
		return hasLocalIP(ip)
	}

	// 主机名（如 localhost）：解析后判。查不到就当不是 ——
	// 挡不住就挡不住，不能因为解析失败就误判放行或误拒正常配置
	ips, err := net.LookupIP(host)
	if err != nil {
		return false
	}
	for _, ip := range ips {
		if ip.IsLoopback() || hasLocalIP(ip) {
			return true
		}
	}
	return false
}

// hasLocalIP 这个地址是不是本机某个网卡上的地址
func hasLocalIP(ip net.IP) bool {
	addrs, err := net.InterfaceAddrs()
	if err != nil {
		return false
	}
	for _, a := range addrs {
		ipnet, ok := a.(*net.IPNet)
		if !ok {
			continue
		}
		if ipnet.IP.Equal(ip) {
			return true
		}
	}
	return false
}
