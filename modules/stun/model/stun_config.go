package model

import (
	"github.com/chenggegedeid/catflap/modules/webhook"
	"time"
)

type Config struct {
	// 基础网络信息

	BestSTUN  string    `json:"bestStun"`  // 最快的STUN服务器
	CreatedAt time.Time `json:"createdAt"` // 配置创建时间
	UpdatedAt time.Time `json:"updatedAt"` // 最后更新时间

	// Devices 设备列表。
	//
	// **设备的"主人"现在是 modules/device**（config/devices.json）——
	// 穿透、反代、端口转发共用那一份。这里只保留字段用于读老配置、
	// 迁移完之后清空，不再写回。新代码请用 device.List()。
	//
	// 想拿"设备 + 它上面的穿透服务"用 stun 的 DevicesWithServices()。
	Devices []Device `json:"devices,omitempty"`

	// Services 穿透服务，**带 DeviceID 关联到设备**。
	//
	// 以前服务嵌在 Device.Services 里；设备搬走之后没地方放了，
	// 拆成独立列表（这是拆分的全部原因）。
	Services []Service `json:"services"`

	StunServerList []string `json:"stunServerList"` // stun服务器列表

	Network NetworkConfig `json:"network"` // 打洞走哪张网卡、STUN 域名找谁解析
}

// NetworkConfig STUN 模块的出口网卡和 DNS。零值就是默认：自动挑网卡、内置 DNS，
// 老配置文件没有这一段，升级后行为不变。
//
// 只管 STUN 模块（打洞、查公网 IP、UPnP、NAT 探测）。DDNS、证书、Webhook
// 走系统网络，开着代理也无所谓。
type NetworkConfig struct {
	IfaceMode string `json:"ifaceMode"` // "" 自动 / "system" 跟随系统 / "custom" 指定网卡
	Iface     string `json:"iface"`     // IfaceMode 为 custom 时的网卡名

	DNSMode string   `json:"dnsMode"` // "" 内置 / "system" 跟随系统 / "custom" 自定义
	DNS     []string `json:"dns"`     // DNSMode 为 custom 时的服务器，"ip" 或 "ip:port"
}

const (
	ModeAuto   = ""
	ModeSystem = "system"
	ModeCustom = "custom"
)

// Device 一台设备。
//
// **新代码请用 modules/device 里的 Device** —— 设备的唯一一份在那边
// （穿透、反代、端口转发共用）。这个类型现在只用来读老配置文件。
type Device struct {
	DeviceID uint   `json:"id"`   // id
	Name     string `json:"name"` // "本机" / "群晖NAS" / "树莓派"
	IP       string `json:"ip"`   // 设备ip

	// Services 老配置里嵌在设备下的服务。
	//
	// **只用于迁移**：读老配置时把它搬到 Config.Services（见 stun 的迁移），
	// 之后一律为空，不再写回文件。
	Services []Service `json:"services,omitempty"`

	CreatedAt time.Time `json:"createdAt"`
	UpdatedAt time.Time `json:"updatedAt"`
}

// Service 单个服务配置
//
// 服务不再嵌在设备里（设备归 modules/device 管），靠 DeviceID 关联过去。
type Service struct {
	// DeviceID 这个服务在哪台设备上。和设备表的 DeviceID 对应。
	DeviceID uint `json:"deviceId"`

	// SortOrder 界面上的排列顺序，小的在前。只影响显示，不影响转发。
	// 0 表示没排过，按 (SortOrder, ID) 排 —— 老配置升级后顺序不变。
	SortOrder      int    `json:"sortOrder,omitempty"`
	ID             uint   `json:"id"` // 服务唯一标识符（设备内唯一）
	StartupSuccess bool   `json:"-"`
	Name           string `json:"name"`         // 服务名称,如 "SSH" / "Web管理" / "照片库"
	InternalPort   uint16 `json:"internalPort"` // 内网端口,如 22
	Protocol       string `json:"protocol"`     // 传输协议 "TCP"/"UDP" (默认 TCP)
	Https          bool   `json:"https"`        // 仅影响链接展示成 http:// 还是 https://,不改变转发行为

	// 对外访问配置
	Domain       string `json:"domain"`       // 该服务对外域名,如 fw.example.com;留空回落公网 IP
	TLSTerminate bool   `json:"tlsTerminate"` // 由 Catflap 在洞口终结 TLS(仅 TCP 有效)
	CertID       uint   `json:"certId"`       // 绑定的证书 ID,0 表示按域名自动匹配

	// BackendHTTPS Catflap 拨内网时用 tls.Dial,等价于 nginx 的 proxy_pass https://。
	//
	// 它描述的不是「内网是不是 HTTPS」,而是「Catflap 要不要自己去和内网握手」,
	// 只在 TLSTerminate 为真时才成立——那时洞口解了密,必须重新建一条连接进内网。
	// TLSTerminate 为假时洞是纯字节管道,握手是浏览器和内网服务之间的事,
	// 此时置真就成了双层 TLS,浏览器侧报 ERR_SSL_PROTOCOL_ERROR。
	//
	// 也不能复用上面的 Https:那个字段一直只管展示,老用户为了让首页链接显示成
	// https:// 就会勾它,而服务本身多半是明文。拿它去决定怎么拨号,
	// 等于让所有勾过的老配置在升级后直接转发失败。
	BackendHTTPS bool `json:"backendHttps"`

	// UPnP 相关配置
	UseUPnP bool `json:"useUpnp"` // 是否启用 UPnP 自动端口映射 (默认 true)
	// UPnPMappedPort 已废弃：表单曾经让人填，但从来没有代码拿它建映射，也没有代码写回真实值。
	// 只为老配置文件能原样读进来保留，新代码别再用。
	UPnPMappedPort uint16 `json:"upnpMappedPort"`

	// PunchLocalPort 打洞用的本机端口。0 = 每次随机分配（老行为）。
	//
	// 为什么要有这个：NAT 的外部端口是**跟着本机端口**分配的。本机端口每次随机，
	// 外部端口就每次都变 —— 面板上那个"外部端口"会一直在跳，用户拿到的地址
	// 过一会儿就失效，配好的域名+端口也就废了。
	// （实测同一条服务：3103 → 3555 → 3304 → 3220，一分钟内变四次。）
	//
	// 钉住本机端口之后，只要 NAT 是锥型（对同一本机端口给出同一外部端口），
	// 外部端口就稳定下来，即使运营商是 CGNAT、即使不用 UPnP 也能长期可用 ——
	// 这正是同网络下别的产品能做到"域名+固定端口"的原因：它们不用 UPnP，
	// 靠的就是固定本机端口 + 持续保活。
	//
	// 填了不代表一定成功：端口被别的程序占了会绑定失败，此时自动回退到随机，
	// 并在日志里说清楚，不会让服务起不来。
	PunchLocalPort uint16 `json:"punchLocalPort"`

	Enabled     bool   `json:"enabled"`     // 服务是否启用 (默认 true)
	Description string `json:"description"` // 服务描述信息 (可选)

	WebHookConfig webhook.WebhookConfig `json:"webhookconfig"` // 旧的内联 webhook，已被通知中心取代（保留做回退）
	Redirect      RedirectConfig        `json:"redirect"`      // 固定域名跟着外部端口走
	MCEntry       MCEntryConfig         `json:"mcEntry"`       // MC Java 版联机：SRV 记录跟着外部端口走

	// ChannelIDs 这个服务出事时发到哪些通知渠道。
	//
	// 存的是**全局渠道的 ID**，不再把 URL/密钥内联在这里 —— 同一个群机器人
	// 地址不该在 5 个服务里存 5 份。改一次渠道，所有引用它的服务立刻生效。
	//
	// 空 = 这个服务不发通知。旧的 WebHookConfig 字段保留：迁移万一有问题，
	// 打开通知中心的 LegacyWebhookFallback 就能退回读它。
	ChannelIDs []string `json:"channelIds,omitempty"`

	UpdatedAt time.Time `json:"updatedAt"` // 最后更新时间
}

// MCEntryConfig 让朋友在 MC 里只填一个域名就能连上，不用管端口。
//
// MC Java 版连服务器时先查 _minecraft._tcp.<域名> 的 SRV 记录，里面写着真正的端口。
// 打洞的外部端口会变，这条记录就跟着改。基岩版不认 SRV，用不上这个。
//
// 零值就是关闭，老配置没有这一段不受影响。
type MCEntryConfig struct {
	Enabled    bool   `json:"enabled"`
	ProviderID uint   `json:"providerId"` // DDNS 里的服务商，目前只有 Cloudflare 能写 SRV
	Host       string `json:"host"`       // 朋友在 MC 里填的那个域名，如 mc.example.com
	ZoneDomain string `json:"zoneDomain"` // 主域名；留空按 Host 的后两段取

	// Target SRV 指向的主机。SRV 只能写域名不能写 IP，这个域名自己得解析到公网 IP。
	//   - 留空：指向 Host 自己，Host 在 DDNS 里没有记录就自动补一条；
	//   - 填了：指向这个已经解析好的域名（一般是 DDNS 里现成的那条），不再补记录。
	Target string `json:"target"`
}

// RedirectConfig 让一个固定的域名，始终指向这个服务当前的外网地址。
//
// 它替代的是「Cloudflare 重定向规则」那个 webhook 模板。用那个模板得自己去
// Cloudflare 后台抄三个 ID（zone / ruleset / rule）贴进 URL，还要手写整段 JSON，
// 而且在后台重建一次规则，rule ID 就变了，webhook 会静默失效。
//
// 这里三个 ID 一个都不用填：zone 用域名查，规则集走固定的 phase 入口，
// 自己那条规则按 description 认领。所以只剩下面三个字段要填。
type RedirectConfig struct {
	Enabled    bool   `json:"enabled"`
	ProviderID uint   `json:"providerId"` // 用 DDNS 里已经配好的服务商，token 不用再贴一遍
	EntryHost  string `json:"entryHost"`  // 外面访问用的名字，如 catflap.example.com
	ZoneDomain string `json:"zoneDomain"` // 主域名；留空按 EntryHost 的后两段取
}
