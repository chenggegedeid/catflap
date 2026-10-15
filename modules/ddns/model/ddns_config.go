package model

import "time"

// DDSN提供商类型
type DNSProviderType string

// DNSProviderType 服务商类型。
//
// 这里只保留**类型常量**（配置里存的是这些字符串，属于持久化契约，不能动）。
// 至于「有哪些服务商可用、各自要哪些凭据字段、支持哪些能力」，
// 已经全部搬到 core/provider 注册表，见 modules/ddns/dns/register_*.go ——
// 以前这里那张 ProviderCredentialKeys 表是手维护的第二个真相，加一家服务商
// 要改两处，两处一旦不一致就会出现「注册了但校验不认」。
const (
	DNSProviderCloudflare   DNSProviderType = "cloudflare"
	DNSProviderAlidns       DNSProviderType = "alidns"
	DNSProviderTencentCloud DNSProviderType = "tencentcloud"
	DNSProviderBaiduCloud   DNSProviderType = "baiducloud"
	DNSProviderHuaweiCloud  DNSProviderType = "huaweicloud"
	DNSProviderNameCheap    DNSProviderType = "namecheap"
	DNSProviderNameSilo     DNSProviderType = "namesilo"
	// 下面三家是 v1.1 新增的：注册表化之后，加一家 = 加一个 register_*.go，
	// 这里补常量只是为了让代码里不出现裸字符串。
	DNSProviderGoDaddy DNSProviderType = "godaddy"
	DNSProviderPorkbun DNSProviderType = "porkbun"
	DNSProviderHE      DNSProviderType = "he"
	DNSProviderDuckDNS DNSProviderType = "duckdns"
)

// 定义每个 DNS 记录类型
type DNSRecordType string

const (
	DNSRecordTypeA    DNSRecordType = "A"
	DNSRecordTypeAAAA DNSRecordType = "AAAA"

	// DNSRecordTypeDual 一条记录同时管 A 和 AAAA（双栈）。
	//
	// 没有 IPv4 公网、只有 IPv6 的环境里，往往还需要保留 IPv4 那条
	// （给没有 IPv6 的访问者用），于是同一个名字要两条记录。以前只能建两条
	// DDNS 记录，名字、来源、TTL 都得重复填一遍，还容易改了一处忘了另一处。
	// 双栈类型让一条记录同时同步两者，同步时分别按各自的协议族取 IP。
	DNSRecordTypeDual DNSRecordType = "A+AAAA"
)

// Families 这条记录实际要写哪些记录类型。
//
// 双栈展开成 A 和 AAAA 两条，其余就是自己。
func (t DNSRecordType) Families() []DNSRecordType {
	if t == DNSRecordTypeDual {
		return []DNSRecordType{DNSRecordTypeA, DNSRecordTypeAAAA}
	}
	return []DNSRecordType{t}
}

// 定义DDNS 记录状态
type DDNSRecordStatus string

const (
	// 等待执行同步
	DDNSRecordStatusPending DDNSRecordStatus = "pending"
	// 同步成功
	DDNSRecordStatusSuccess DDNSRecordStatus = "success"
	// 同步失败
	DDNSRecordStatusFailed DDNSRecordStatus = "failed"
	// 跳过同步
	DDNSRecordStatusSkipped DDNSRecordStatus = "skipped"
)

type IPSourceType string

const (
	IPSourceSTUN      IPSourceType = "stun"      // 复用 stun.Runtime.Network（PublicIP/LocalIP）
	IPSourceWeb       IPSourceType = "web"       // HTTP GET 某网站，如 https://ip.sb
	IPSourceDNS       IPSourceType = "dns"       // 解析某域名，保持一致
	IPSourceInterface IPSourceType = "interface" // 读本地网卡
	IPSourceCustom    IPSourceType = "custom"    // 固定指着填好的那个 IP，不去探测
)

// DDNS 配置
type DDNSConfig struct {
	IntervalSec int `json:"intervalSec"` // 全局默认同步时间300

	Providers []DDNSProvider `json:"providers"` // DNS 服务商 配置
	Records   []DDNSRecord   `json:"records"`   // DDNS 记录
	CreatedAt time.Time      `json:"createdAt"`
	UpdatedAt time.Time      `json:"updatedAt"`
}

// DNS 服务商
type DDNSProvider struct {
	ID         uint                   `json:"id"`
	Name       string                 `json:"name"` // 自定义名字
	Type       DNSProviderType        `json:"type"` // 服务商类型
	Credential map[string]interface{} `json:"credential"`

	CreatedAt time.Time `json:"createdAt"`
	UpdatedAt time.Time `json:"updatedAt"`
}

// DDNS 记录
type DDNSRecord struct {
	Enabled      bool          `json:"enabled"`
	ID           uint          `json:"id"`
	ProviderID   uint          `json:"providerId"`  // 服务商id
	Name         string        `json:"name"`        // DDNS 记录名字
	Domain       string        `json:"domain"`      // 主域名
	SubDomain    string        `json:"subDomain"`   // 子域名
	RecordType   DNSRecordType `json:"recordType"`  // 记录类型
	IPSourceType IPSourceType  `json:"ipSource"`    // ip来源
	IPSourceArg  string        `json:"ipSourceArg"` // web 的 URL / dns 的域名,stun/interface 留空
	TTL          int           `json:"ttl"`         // ttl
	Proxied      bool          `json:"proxied"`     // 小云朵代理

	// AutoCreated 这条记录是 Catflap 自己替某个服务的落地域名补的，不是用户加的。
	//
	// 只影响一件事：那个服务被删掉时，这条记录跟着删。用户自己加的绝不能动——
	// 他可能拿同一个域名干别的。用户在界面上改过之后也不再算自动的（见 UpdateRecord）。
	AutoCreated bool `json:"autoCreated"`

	LastIP      string           `json:"lastIP"`      // 上次成功同步的 IP
	LastStatus  DDNSRecordStatus `json:"lastStatus"`  // 上次同步状态,如 success/failed/skipped
	LastMessage string           `json:"lastMessage"` // 上次同步结果或失败原因
	LastCheckAt time.Time        `json:"lastCheckAt"` // 每次扫到就更新,用于 due 判断
	LastSyncAt  time.Time        `json:"lastSyncAt"`  // 上次同步时间

	CreatedAt time.Time `json:"createdAt"`
	UpdatedAt time.Time `json:"updatedAt"`
}
