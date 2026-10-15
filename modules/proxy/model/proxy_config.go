package model

import (
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

	// HTTPPort 默认 HTTP 入口；0 = 不开这个入口
	HTTPPort int `json:"httpPort"`
	// HTTPSPort 默认 HTTPS 入口；0 = 不开这个入口
	HTTPSPort int `json:"httpsPort"`
	// CertID 默认 HTTPS 入口用哪张证书；0 = 按 SNI 自动匹配
	CertID uint `json:"certId"`

	// AccessLog 访问日志，默认关
	AccessLog bool `json:"accessLog"`

	// BaseDomain 主域名，如 example.com。
	//
	// 填了之后加站点只用写前缀：填 nas 就是 nas.example.com。
	// 家里绝大多数人只有一个域名，让他在每一条站点上重复抄一遍没有意义。
	BaseDomain string `json:"baseDomain"`

	// Tunnel Cloudflare Tunnel（cloudflared）托管。
	//
	// 没有 IPv4 公网（运营商 CGNAT）时，打洞出来的端口是 NAT 给的、URL 必须带
	// 端口；而 Tunnel 是 cloudflared 主动往外连 Cloudflare，入口在 CF 那边，
	// 于是访问地址既没有端口，也不需要打洞、不需要 DDNS、不需要在路由器上放行。
	//
	// 这里不内置 cloudflared（它是独立程序，体积不小且版本迭代快），而是托管
	// 用户自己装的：填好程序路径和 tunnel token，由 Catflap 负责拉起、看住、
	// 关掉，状态和日志都在面板上看。
	Tunnel TunnelConfig `json:"tunnel"`

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

	Enabled     bool      `json:"enabled"`
	Description string    `json:"description"`
	UpdatedAt   time.Time `json:"updatedAt"`

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

// TunnelConfig Cloudflare Tunnel 的配置
type TunnelConfig struct {
	// Enabled 用不用。关掉时如果正在跑会停掉进程。
	Enabled bool `json:"enabled"`
	// BinaryPath cloudflared 的路径。留空时按下面的默认名字在 PATH 里找。
	BinaryPath string `json:"binaryPath"`
	// Token tunnel 的 token（在 Cloudflare Zero Trust 后台创建隧道时给的一长串）。
	//
	// 它是这条隧道的全部凭证：拿它就能以你的名义起一条隧道。所以按 Secret
	// 处理——面板上回显成掩码，日志里也绝不打印。
	Token string `json:"token"`
	// ExtraArgs 额外参数，一般留空。比如想指定 --protocol quic。
	ExtraArgs string `json:"extraArgs"`
}

// TunnelState 隧道的运行状态（不落盘，运行时才有）
type TunnelState struct {
	Running bool   `json:"running"`
	Message string `json:"message"` // 最近的状态或失败原因
	// TokenSet 只表示"填没填"，不回显真实 token——那是凭证，不该在页面上飘
	TokenSet bool `json:"tokenSet"`
	// BinaryPath 实际在用的路径，方便确认找到的是不是你装的那个
	BinaryPath string `json:"binaryPath"`
	StartedAt  string `json:"startedAt"`
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
