package proxy

import (
	"errors"
	"fmt"
	"net"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/chenggegedeid/catflap/core"
	device "github.com/chenggegedeid/catflap/modules/device"
	"github.com/chenggegedeid/catflap/modules/proxy/model"
	"github.com/chenggegedeid/catflap/utils/domain"

	"github.com/sirupsen/logrus"
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
	// 前端没显式传 deviceId 时（老数据 / 手填 backend），按 backend 的 IP 反查设备。
	// 存下来之后，改那台设备的 IP 也不会把这条站点判成"未归设备"。
	if site.DeviceID == 0 {
		site.DeviceID = deviceIDByIP(site.Backend)
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
				// **保留纯展示标记**：前端保存表单时不带 showOnHome
				// （那是列表上的快捷开关，不属于编辑表单），
				// 整体替换会把它冲成 false —— 表现是"点过主页，编辑一次就没了"。
				//
				// 这个坑实测踩到过：点上主页 → 编辑保存一次 → 标记丢失。
				// 原则是"表单管不到的字段，保存时不该被动"。
				site.ShowOnHome = cfg.Sites[i].ShowOnHome
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

	// Scheme 白名单：只认三个值，其它一律拒。
	//
	// 不校验的话，配置文件里出现一个不认识的 scheme，绑定时会走 default
	// 分支当 auto 处理 —— 看着"能用"，但用户以为自己设了某个协议，
	// 实际行为完全不同。这种"悄悄降级"比直接报错难查得多。
	switch out.Scheme {
	case model.SchemeAuto, model.SchemeHTTP, model.SchemeHTTPS:
		// ok
	default:
		return out, fmt.Errorf("协议只能是 http 或 https（留空表示自动），实际填的是 %q", out.Scheme)
	}
	// 选了 https 就一定要能 HTTPS，否则这条站点一个入口都挂不上
	if out.Scheme == model.SchemeHTTPS {
		out.HTTPS = true
	}

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
	case backendHitsAdminPort(out.Backend, adminPort()) && !allowExposeAdmin():
		// 后端指向 Catflap 自己的后台端口 = 把管理后台挂到公网。
		//
		// 这一条看着像"用户不会这么填"，但反而是很容易犯的错：想在外网访问
		// 管理页面时，最顺手的做法就是建个站点指向 127.0.0.1:<管理端口>，
		// 心想"反正还要登录"。结果是把登录页和 /api/auth/login 一起挂到
		// 公网，谁都能来爆破密码。
		//
		// **默认仍然拦**，但留了开关（见 allowExposeAdmin）：
		// 远程管理是正当需求，不能因为怕手滑就彻底堵死 ——
		// 那样只会逼着人绕过这道校验。开开关时会在下面打一条醒目的日志。
		return out, fmt.Errorf(
			"后端不能填 Catflap 自己的后台端口 %d——那等于把管理后台挂到公网上，任何人都能来试密码。"+
				"确实要远程管理的话，先在「系统设置」里打开「允许通过反代暴露管理后台」",
			adminPort())
	case backendHitsAdminPort(out.Backend, adminPort()):
		// 开关打开了：放行，但代价要说清楚，并且必须留下痕迹。
		//
		// 记 WARN 而不是 INFO：这条配置意味着登录页对该站点可见。
		// 事后排查「后台是不是被暴露过」时，这条日志是唯一的凭据。
		logrus.Warnf("[proxy] 站点（%s）的后端指向管理后台端口 %d —— 登录页已对它可见，"+
			"公网任何人都能来试密码。请确保审计页的「永不封禁」名单配置正确，"+
			"并且知道：登录失败达到阈值会自动封禁来源（含反向的 DoS 风险）。",
			strings.Join(out.Hosts, " / "), adminPort())
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

// allowExposeAdmin 是否允许把管理后台通过反代挂出去。
//
// **默认关**。这不是洁癖：挂出去意味着 /api/auth/login 对任何人可见，
// 猜中密码就完全接管。但远程管理是真实需求（要在外面加服务、调穿透），
// 一刀切堵死只会逼着人绕过这道校验 —— 所以留一个显式开关，
// 打开时校验会放行，但记一条 WARN 留下痕迹（见上面那条 case）。
func allowExposeAdmin() bool {
	return Runtime.Snapshot().ExposeAdmin
}

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

// ===================== 端口转发 =====================

// normalizeForward 校验并整理一条端口转发。
//
// 校验项里**最要紧的两条**和站点那边一致（v1.8 修的漏洞）：
//   - 监听端口不能是后台端口：否则反代会把 Catflap 自己的监听顶掉
//   - 目标不能指向后台端口：否则等于把管理后台挂到公网上，
//     谁都能在上面爆破密码（/api/auth/login 是公开的）
//
// 端口转发是新增功能，这两条要是从一开始就漏了，等于给那个漏洞新开一条路。
func normalizeForward(in model.Forward, entry model.ProxyConfig) (model.Forward, error) {
	out := in
	out.Target = strings.TrimSpace(out.Target)
	out.Description = strings.TrimSpace(out.Description)
	out.Protocol = strings.ToLower(strings.TrimSpace(out.Protocol))
	if out.Protocol == "" {
		out.Protocol = "tcp"
	}

	switch {
	case out.ListenPort <= 0:
		return out, errors.New("端口转发要填一个监听端口（它没有域名可分流，只能自己占一个）")
	case out.ListenPort > 65535:
		return out, errors.New("端口要填 1 到 65535 之间的数字")
	case out.ListenPort == adminPort():
		return out, fmt.Errorf("%d 是 Catflap 自己的后台端口，换一个", adminPort())
	case containsInt(entry.HTTPPortList(), out.ListenPort):
		return out, fmt.Errorf("端口 %d 已经是默认 HTTP 入口了，换一个——同一个端口不能 bind 两次", out.ListenPort)
	case containsInt(entry.HTTPSPortList(), out.ListenPort):
		return out, fmt.Errorf("端口 %d 已经是默认 HTTPS 入口了，换一个", out.ListenPort)
	case out.Target == "":
		return out, errors.New("目标地址必填，形如 192.168.1.20:80")
	case strings.ContainsAny(out.Target, " \t"):
		return out, errors.New("目标地址里有空格，请检查")
	case strings.HasPrefix(out.Target, "/"):
		return out, errors.New("目标地址要填主机和端口，形如 192.168.1.20:80")
	case backendHitsAdminPort(out.Target, adminPort()):
		return out, fmt.Errorf(
			"目标不能是 Catflap 自己的后台端口 %d——那等于把管理后台挂到公网上，任何人都能来试密码",
			adminPort())
	case out.Protocol != "tcp" && out.Protocol != "udp":
		return out, errors.New("协议只能填 tcp 或 udp")
	}

	// 两条转发不能占同一个端口
	for _, f := range entry.Forwards {
		if f.ID != out.ID && f.ListenPort == out.ListenPort {
			return out, fmt.Errorf("端口 %d 已经被另一条转发占了", out.ListenPort)
		}
	}
	return out, nil
}

// SaveForward 新增或修改一条端口转发（ID 为 0 表示新增）
func SaveForward(in model.Forward) (model.Forward, error) {
	if in.DeviceID == 0 {
		in.DeviceID = deviceIDByIP(in.Target)
	}
	var saved model.Forward
	err := Runtime.Update(func(cfg *model.ProxyConfig) error {
		f, err := normalizeForward(in, *cfg)
		if err != nil {
			return err
		}
		if f.ID == 0 {
			var max uint
			for _, x := range cfg.Forwards {
				if x.ID > max {
					max = x.ID
				}
			}
			f.ID = max + 1
			f.CreatedAt = time.Now()
			cfg.Forwards = append(cfg.Forwards, f)
		} else {
			found := false
			for i := range cfg.Forwards {
				if cfg.Forwards[i].ID == f.ID {
					// 保留不在表单里的字段（CreatedAt 是其一）。
					// 以后加新的纯展示标记时也要在这里保住 ——
					// 站点那边踩过"编辑一次就把标记冲没"的坑。
					f.CreatedAt = cfg.Forwards[i].CreatedAt
					cfg.Forwards[i] = f
					found = true
					break
				}
			}
			if !found {
				return errors.New("端口转发不存在")
			}
		}
		f.UpdatedAt = time.Now()
		saved = f
		return nil
	})
	if err != nil {
		return model.Forward{}, err
	}
	// Runtime.Update 落盘后会自动 Apply（重开监听），不用再手动调一次
	return saved, nil
}

// DeleteForward 删一条端口转发
func DeleteForward(id uint) error {
	return Runtime.Update(func(cfg *model.ProxyConfig) error {
		for i := range cfg.Forwards {
			if cfg.Forwards[i].ID != id {
				continue
			}
			cfg.Forwards = append(cfg.Forwards[:i], cfg.Forwards[i+1:]...)
			return nil
		}
		return errors.New("端口转发不存在")
	})
}

// ReorderSites 拖动排序后保存站点顺序。
//
// 和 device.Reorder 同样的策略：只认已有 ID，多余的忽略；
// 没在列表里的排后面。并发下前端给的顺序可能不全，不该因此报错。
func ReorderSites(ids []uint) error {
	return Runtime.Update(func(cfg *model.ProxyConfig) error {
		byID := map[uint]*model.Site{}
		for i := range cfg.Sites {
			byID[cfg.Sites[i].ID] = &cfg.Sites[i]
		}
		next := 1
		for _, id := range ids {
			s, ok := byID[id]
			if !ok {
				continue
			}
			s.SortOrder = next
			next++
			delete(byID, id)
		}
		rest := make([]*model.Site, 0, len(byID))
		for _, s := range byID {
			rest = append(rest, s)
		}
		sort.Slice(rest, func(i, j int) bool { return rest[i].ID < rest[j].ID })
		for _, s := range rest {
			s.SortOrder = next
			next++
		}
		return nil
	})
}

// SortSites 按 (SortOrder, ID) 排好返回，界面按这个顺序显示
func SortSites(in []model.Site) []model.Site {
	out := make([]model.Site, len(in))
	copy(out, in)
	sort.SliceStable(out, func(i, j int) bool {
		if out[i].SortOrder != out[j].SortOrder {
			return out[i].SortOrder < out[j].SortOrder
		}
		return out[i].ID < out[j].ID
	})
	return out
}

// SortForwards 端口转发按界面顺序排（sortOrder 小的在前）。
//
// 和 SortSites 同一套规则：拖动排序存在转发自己的 sortOrder 上，
// 接口返回时不排的话，拖完一刷新就回原顺序 —— 表现为「拖动没反应」。
func SortForwards(in []model.Forward) []model.Forward {
	out := make([]model.Forward, len(in))
	copy(out, in)
	sort.SliceStable(out, func(i, j int) bool {
		if out[i].SortOrder != out[j].SortOrder {
			return out[i].SortOrder < out[j].SortOrder
		}
		return out[i].ID < out[j].ID
	})
	return out
}

// deviceIDByIP 从 "ip:port" 里切出 IP，反查设备表的 ID；查不到返回 0（调用方按老逻辑：按 IP 字符串匹配）。
//
// **放在这里而不是 device 模块**：proxy 已经依赖 device（拿设备列表），不需要再绕一层。
// 返回值只用于"归属"，拿不到就按 IP 匹配 —— 不会因为设备表里没有这台机器就丢规则。
func deviceIDByIP(addr string) uint {
	ip := addr
	if i := strings.LastIndex(addr, ":"); i >= 0 {
		ip = addr[:i]
	}
	if ip == "" {
		return 0
	}
	if d := device.GetByIP(ip); d != nil {
		return d.DeviceID
	}
	return 0
}

// SyncIPsAfterDeviceChange 设备 IP 变了之后调用：把本模块里按旧 IP 写的
// 规则（Site.Backend / Forward.Target）里的那段 IP 换成新 IP。
//
// **由 device 模块通过回调调用**（device.SetIPChangedHook）—— 依赖方向是
// proxy → device，device 不能反向 import proxy，所以同步逻辑放在这边。
//
// 为什么必须同步：设备表里的 IP 是单一真相，而 Backend/Target 里存了一份
// "ip:port" 字符串。不同步的话，改完设备 IP 后这条规则的 IP 还是旧的
// → 前端按 deviceId 找得到设备，但按 IP 匹配不上 → 被判成"未归设备"。
//
// 只改 IP 段、保留端口：端口是服务自己的，跟设备无关。
func SyncIPsAfterDeviceChange(oldIP, newIP string) error {
	if oldIP == "" || oldIP == newIP {
		return nil
	}
	return Runtime.Update(func(cfg *model.ProxyConfig) error {
		for i := range cfg.Sites {
			if ip, port := splitAddr(cfg.Sites[i].Backend); ip == oldIP {
				cfg.Sites[i].Backend = joinAddr(newIP, port)
			}
		}
		for i := range cfg.Forwards {
			if ip, port := splitAddr(cfg.Forwards[i].Target); ip == oldIP {
				cfg.Forwards[i].Target = joinAddr(newIP, port)
			}
		}
		return nil
	})
}

// splitAddr 把 "ip:port" 拆成两段；没有冒号时 port 为空（只有 IP）。
func splitAddr(addr string) (ip string, port string) {
	i := strings.LastIndex(addr, ":")
	if i < 0 {
		return addr, ""
	}
	return addr[:i], addr[i+1:]
}

// BackfillDeviceIDs 把老数据里 deviceId 还是 0 的站点 / 转发回填上设备 ID。
//
// **什么时候需要**：deviceId 字段是后加的（之前站点按 Backend 里的 IP 字符串
// 关联设备），老配置读进来全是 0。不回填的话：
//   - 改设备 IP 时同步不了（找不到"这条规则属于哪台设备"）
//   - 界面按 deviceId 找不到 → 掉进"未归到设备"
//
// 启动时跑一次即可；之后每次保存都会由 SaveSite / SaveForward 维护。
// 返回填了几条（日志用），失败返回错误。
func BackfillDeviceIDs() (int, error) {
	n := 0
	err := Runtime.Update(func(cfg *model.ProxyConfig) error {
		// **空配置绝不写盘**：如果这时配置还没加载（调用时机错了），
		// Update 会把"空"当成新状态写进文件 —— 那是一次真实事故的原因
		// （用户的 4 条站点被写成空表）。宁可什么都不做，也不能覆盖掉真实数据。
		if len(cfg.Sites) == 0 && len(cfg.Forwards) == 0 {
			logrus.Warn("[proxy] BackfillDeviceIDs 跳过：配置为空（通常意味着调用时机太早，配置还没加载）")
			return nil
		}
		logrus.Infof("[proxy] BackfillDeviceIDs 开始：sites=%d forwards=%d", len(cfg.Sites), len(cfg.Forwards))
		for i := range cfg.Sites {
			// 老表单留下的脏数据：选了设备但端口没填时会被拼成 "192.168.9.8:"，
			// 末尾那个空冒号会让后端解析不出端口、转发失败。顺手在这里清掉。
			cfg.Sites[i].Backend = strings.TrimSuffix(cfg.Sites[i].Backend, ":")
			if cfg.Sites[i].DeviceID == 0 {
				id := deviceIDByIP(cfg.Sites[i].Backend)
				logrus.Infof("[proxy] 站点 #%d backend=%s → 匹配到设备 id=%d", cfg.Sites[i].ID, cfg.Sites[i].Backend, id)
				if id != 0 {
					cfg.Sites[i].DeviceID = id
					n++
				}
			}
		}
		for i := range cfg.Forwards {
			cfg.Forwards[i].Target = strings.TrimSuffix(cfg.Forwards[i].Target, ":")
			if cfg.Forwards[i].DeviceID == 0 {
				if id := deviceIDByIP(cfg.Forwards[i].Target); id != 0 {
					cfg.Forwards[i].DeviceID = id
					n++
				}
			}
		}
		return nil
	})
	return n, err
}

// joinAddr 把 IP 和端口拼回 "ip:port"。
// **端口为空就只给 IP**：老配置里 backend 可能只写了 IP（没有 :port），
// 那种情况拼成 "192.168.9.77:" 会多一个冒号，转发时解析不出端口。
func joinAddr(ip, port string) string {
	if port == "" {
		return ip
	}
	return ip + ":" + port
}

// SetErrorPage 保存错误页设置（外网访客看到的 404 / 502）。
//
// 校验放在这里而不是接口层：模式就两个值，写错了页面会整个变成空白，
// 宁可在保存时就挡住。
func SetErrorPage(in model.ErrorPage) error {
	mode := strings.TrimSpace(in.Mode)
	switch mode {
	case "", "simple":
		mode = "simple"
	case "custom":
	default:
		return fmt.Errorf("错误页模式只能是 simple 或 custom，收到的是 %q", in.Mode)
	}

	html := in.CustomHTML
	if mode == "custom" {
		if strings.TrimSpace(html) == "" {
			return errors.New("选了自定义就要填页面内容（HTML）")
		}
		if len(html) > model.MaxErrorPageLen {
			return fmt.Errorf("页面内容太长（%d 字节，上限 %d）", len(html), model.MaxErrorPageLen)
		}
	} else {
		// 切回默认时把内容清掉：留着的话下次切回自定义会突然冒出旧内容，
		// 而用户在界面上看到的是空框 —— 对不上。
		html = ""
	}

	return Runtime.Update(func(cfg *model.ProxyConfig) error {
		cfg.ErrorPage = model.ErrorPage{Mode: mode, CustomHTML: html}
		return nil
	})
}

// SetExposeAdmin 开关「允许把管理后台通过反代挂出去」。
//
// 打开时记一条 WARN：这条配置的后果是登录页对公网可见，
// 得在日志里留痕，事后才查得出「后台曾经被挂出去过」。
func SetExposeAdmin(enabled bool) error {
	if enabled {
		logrus.Warnf("[proxy] 已允许通过反代暴露管理后台（端口 %d）。登录页将对对应站点可见，"+
			"请确认审计页「永不封禁」名单已包含管理员自己的出口 IP", adminPort())
	}
	return Runtime.Update(func(cfg *model.ProxyConfig) error {
		cfg.ExposeAdmin = enabled
		return nil
	})
}
