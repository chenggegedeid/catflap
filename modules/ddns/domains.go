package ddns

import (
	"strings"

	"github.com/chenggegedeid/catflap/modules/ddns/model"
)

// ===================== 域名选项（给别的模块选用）=====================
//
// 反向代理的站点要填对外域名，而那些域名往往就是 DDNS 已经在维护的那些。
// 让用户从已配的 DDNS 记录里挑，比手打靠谱：不会打错、也知道它是 v4 还是 v6。
//
// **只在 api 层聚合**：proxy 不 import ddns（ddns 初始化要等 STUN 探测公网 IP，
// 反代的监听不该被它拖住 —— 和 home 模块刻意不写 dependsOn("stun") 同理）。

// DomainOption 一个已经配好的域名
type DomainOption struct {
	// Host 完整域名，站点里填的就是它
	Host string `json:"host"`
	// Full 完整域名（等于 Host，保留给将来带通配符时用）
	Full string `json:"full"`
	// Stack 这条解析走哪条栈：v4 / v6 / dual
	Stack string `json:"stack"`
	// Provider 服务商名字（界面上按它分组）；拿不到就是空
	Provider string `json:"provider"`
	// Label 记录自己的名字。用户可能给同一域名配了两条（v4 / v6），
	// 界面上要靠它区分
	Label string `json:"label"`
	// Enabled 这条记录是否在同步
	Enabled bool `json:"enabled"`
}

// ListDomains 返回所有已配的 DDNS 域名。
//
// 只读，不做任何网络请求 —— 只是把配置里的记录摊开。
// 拿不到（配置还没读）就返回空，调用方按"没有可选项"处理。
func ListDomains() []DomainOption {
	if Runtime == nil {
		return nil
	}

	// 服务商 id → 名字。界面上按服务商分组显示，比只给一个域名好找
	providerNames := map[uint]string{}
	for _, p := range Runtime.Config.Providers {
		providerNames[p.ID] = strings.TrimSpace(p.Name)
	}

	out := make([]DomainOption, 0, 8)
	seen := map[string]bool{}

	records := Runtime.Config.Records
	for _, r := range records {
		host := fullName(r.Domain, r.SubDomain)
		if host == "" {
			continue
		}
		// 同一个域名可能被配了两条（v4 一条、v6 一条），合并成双栈展示
		if seen[host] {
			continue
		}
		seen[host] = true

		out = append(out, DomainOption{
			Host:     host,
			Full:     host,
			Stack:    stackOf(r.RecordType),
			Provider: providerNames[r.ProviderID],
			Label:    strings.TrimSpace(r.Name),
			Enabled:  r.Enabled,
		})
	}

	return out
}

// fullName 子域名 + 主域名 拼成完整域名。
// 子域名填 @ 表示就是主域名本身。
func fullName(domain, sub string) string {
	domain = strings.TrimSpace(strings.ToLower(domain))
	sub = strings.TrimSpace(strings.ToLower(sub))
	if domain == "" {
		return ""
	}
	if sub == "" || sub == "@" {
		return domain
	}
	return sub + "." + domain
}

// stackOf 记录类型 → 界面上的栈标记
func stackOf(t model.DNSRecordType) string {
	switch t {
	case model.DNSRecordTypeA:
		return "v4"
	case model.DNSRecordTypeAAAA:
		return "v6"
	case model.DNSRecordTypeDual:
		return "dual"
	default:
		// 认不出来就按 v4，至少不会显示成一个空标签
		return "v4"
	}
}
