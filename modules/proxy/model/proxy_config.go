package model

import (
	"sort"
	"strconv"
	"time"
)

// 默认入口端口，对应 nginx 的 listen 80 / listen 443 ssl，
// 也对应 GoDoxy 的 HTTP_ADDR / HTTPS_ADDR。
const (
	DefaultHTTPPort  = 80
	DefaultHTTPSPort = 443
)

// ProxyConfig 反向代理的配置。
//
// 它干的就是 nginx 的活：监听端口，按域名把请求转给内网服务。
// 这些端口怎么让外网访问得到（STUN 打洞 / 路由器端口映射 / 就在内网用）
// 是另一件事，本模块一概不管——要打洞就去 STUN 页加一条指向端口的服务。
//
// 端口模型抄 GoDoxy：有两个默认入口（HTTP / HTTPS），站点不特别指定
// 就挂在默认入口上；指定了 ListenPort 的站点单独起一个监听。
type ProxyConfig struct {
	// Enabled 总开关。关掉只是不监听，站点配置一条不动
	Enabled bool `json:"enabled"`

	// HTTPPorts 默认 HTTP 入口，**可以多个**；空 = 不开这个入口。
	//
	// 多个入口的用处：内网直接走 80、外网映射进来走 8080，两个端口上
	// 都要能访问同一批站点。以前只能在每条站点上填"独占端口"，
	// 结果同一个数字要填很多遍。
	HTTPPorts []int `json:"httpPorts"`
	// HTTPSPorts 默认 HTTPS 入口，可以多个；空 = 不开这个入口
	HTTPSPorts []int `json:"httpsPorts"`
	// CertID 默认 HTTPS 入口用哪张证书；0 = 按 SNI 自动匹配
	CertID uint `json:"certId"`

	// 下面两个是旧版的单端口字段，只用来读老配置。
	// 迁移成上面的数组后清空，之后不再写进文件。
	LegacyHTTPPort  int `json:"httpPort,omitempty"`
	LegacyHTTPSPort int `json:"httpsPort,omitempty"`

	// AccessLog 访问日志，默认关
	AccessLog bool `json:"accessLog"`

	// BaseDomain 主域名，如 example.com。
	//
	// 填了之后加站点只用写前缀：填 nas 就是 nas.example.com。
	// 家里绝大多数人只有一个域名，让他在每一条站点上重复抄一遍没有意义。
	BaseDomain string `json:"baseDomain"`

	Sites []Site `json:"sites"`

	// 下面两个是旧版的单端口字段，只用来读老配置。
	// ReadConfig 迁移完就清空，之后不再写进文件。
	LegacyPort int  `json:"listenPort,omitempty"`
	LegacyTLS  bool `json:"tls,omitempty"`

	CreatedAt time.Time `json:"createdAt"`
	UpdatedAt time.Time `json:"updatedAt"`
}

// Site 一个站点，对应 nginx 的一个 server 块。
//
// 端口和证书都能一站一配：不填就跟默认入口走，填了就自己占一个端口，
// 和 nginx 里每个 server 块可以写自己的 listen / ssl_certificate 一样。
type Site struct {
	ID uint `json:"id"`
	// Hosts 域名，分流依据。
	//
	// 对应 nginx 的 server_name a.com www.a.com——一个站点挂多个域名是常事
	// （裸域 + www、内外网各一个名字），没必要逼用户复制粘贴出好几条一样的站点。
	//
	// 留空 = 不按域名分流，这个端口上来的请求全收，也就是当端口转发使。
	// nginx 的 server_name 本来就可以不写，只是那种 server 块得自己占一个
	// listen 端口——这里同理，所以留空时 ListenPort 必填。
	Hosts []string `json:"hosts"`
	// PathPrefix 只接管这个路径下的请求；留空 = 整站
	PathPrefix string `json:"pathPrefix"`
	// StripPrefix 转发前剥掉前缀（后端需自己支持 base path）
	StripPrefix bool `json:"stripPrefix"`
	// Backend 内网后端，形如 192.168.1.20:8080
	Backend string `json:"backend"`
	// BackendHTTPS 内网服务本身是 HTTPS（自签也算）
	BackendHTTPS bool `json:"backendHttps"`

	// ListenPort 这个站点单独占一个端口；0 = 挂在默认入口上
	ListenPort int `json:"listenPort"`
	// HTTPS 这个站点要不要 HTTPS。
	//
	// 挂默认入口时：HTTP 入口一直有，勾上就额外挂到 HTTPS 入口。
	// 自己占端口时：决定这个端口是 https 还是 http。
	HTTPS bool `json:"https"`
	// CertID 指定证书；0 = 按 SNI 自动匹配（证书里的域名对得上就用哪张）
	CertID uint `json:"certId"`

	// Access 谁能访问这个站点。
	//
	// 把服务暴露到公网之后，光靠「域名没人知道」是不算防护的——域名会出现在
	// 证书透明日志里，扫描器也在扫整个网段。所以给站点加一道 IP 门禁。
	Access AccessControl `json:"access"`

	Enabled     bool   `json:"enabled"`
	Description string `json:"description"`

	// ShowOnHome 在导航主页上显示这个站点。
	//
	// 和穿透服务那个标记是同一回事，只是存在各自的模块里 ——
	// 主页那边统一收集，两边不用互相知道。
	ShowOnHome bool `json:"showOnHome,omitempty"`

	UpdatedAt time.Time `json:"updatedAt"`

	// LegacyHost 旧版的单域名字段，只用来读老配置。
	// ReadConfig 迁移完就清空，之后不再写进文件。
	LegacyHost string `json:"host,omitempty"`
}

// AccessMode 访问控制模式
type AccessMode string

const (
	// AccessModeAllowAll 谁都能访问（默认，不填就是它）
	AccessModeAllowAll AccessMode = ""
	// AccessModeAllow 白名单：只有列表里的 IP/网段能访问
	AccessModeAllow AccessMode = "allow"
	// AccessModeDeny 黑名单：列表里的 IP/网段被拒，其余放行
	AccessModeDeny AccessMode = "deny"
)

// AccessControl 站点级的 IP 门禁。
//
// Rules 里每一项是一个 IP 或一个 CIDR 网段，两种都收：
// 家里宽带的 IP 会变，写单个 IP 迟早失效，写网段更耐用。
type AccessControl struct {
	Mode  AccessMode `json:"mode"`
	Rules []string   `json:"rules"`
}

// PrimaryHost 第一个域名，用在错误提示和日志里
func (s Site) PrimaryHost() string {
	if len(s.Hosts) == 0 {
		return ""
	}
	return s.Hosts[0]
}

// Label 日志和报错里怎么称呼这个站点。
//
// 没填域名的站点（当端口转发使的那种）只能用端口指代，
// 不然日志里会出现一条前面空着的「 → 转发失败」。
func (s Site) Label() string {
	if h := s.PrimaryHost(); h != "" {
		return h
	}
	if s.ListenPort > 0 {
		return ":" + strconv.Itoa(s.ListenPort)
	}
	return "(未命名站点)"
}

// ===================== 入口端口 =====================

// MigratePorts 把老配置的单端口字段迁到数组里。
//
// **幂等**：只在数组为空且老字段有值时搬，搬完把老字段清零。
func (c *ProxyConfig) MigratePorts() {
	if len(c.HTTPPorts) == 0 && c.LegacyHTTPPort > 0 {
		c.HTTPPorts = []int{c.LegacyHTTPPort}
	}
	if len(c.HTTPSPorts) == 0 && c.LegacyHTTPSPort > 0 {
		c.HTTPSPorts = []int{c.LegacyHTTPSPort}
	}
	// 老字段不再写回
	c.LegacyHTTPPort = 0
	c.LegacyHTTPSPort = 0
}

// HTTPPortList 默认 HTTP 入口的端口列表（已去重、排序、剔掉非法值）。
//
// 用这个函数而不是直接读字段，是因为要保证拿到的永远是干净的列表 ——
// 配置文件可能被手改出 0、负数、重复值。
func (c ProxyConfig) HTTPPortList() []int { return cleanPorts(c.HTTPPorts) }

// HTTPSPortList 默认 HTTPS 入口的端口列表
func (c ProxyConfig) HTTPSPortList() []int { return cleanPorts(c.HTTPSPorts) }

// HasHTTPS 有没有开 HTTPS 入口。
// 界面上"有证书就默认 HTTPS"靠这个判断。
func (c ProxyConfig) HasHTTPS() bool { return len(c.HTTPSPortList()) > 0 }

// cleanPorts 剔掉非法值、去重、排序。
// 同一个端口出现两次会让 bind 失败（"address already in use"），必须去重。
func cleanPorts(in []int) []int {
	out := make([]int, 0, len(in))
	seen := map[int]bool{}
	for _, p := range in {
		if p <= 0 || p > 65535 || seen[p] {
			continue
		}
		seen[p] = true
		out = append(out, p)
	}
	// 排序保证每次算出来的顺序稳定，日志和页面上不会来回跳
	sort.Ints(out)
	return out
}
