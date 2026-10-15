package dns

import (
	"github.com/chenggegedeid/catflap/core/component"
	"github.com/chenggegedeid/catflap/core/provider"
	"github.com/chenggegedeid/catflap/modules/ddns/model"
)

// Cloudflare 是能力最全的一家：解析记录、ACME DNS-01、边缘重定向规则、SRV
// 四样都支持，所以四个能力标记全打上。
//
// 它是唯一支持 DNS-01 的服务商（见 SupportsACMEDNS 的历史行为），
// 也是唯一支持重定向规则与 SRV 的 —— 这两条不是随手写的，是
// cloudflare_redirect.go / cloudflare_srv.go 里真有实现。
func init() {
	provider.MustRegister(provider.Descriptor{
		Key:  string(model.DNSProviderCloudflare),
		Name: "Cloudflare",
		Kind: provider.KindDNS,
		Capabilities: []provider.Capability{
			provider.CapDNS01,
			provider.CapRedirect,
			provider.CapSRV,
			provider.CapWildcard,
			provider.CapProxied,
		},
		Order:   10,
		DocsURL: "https://dash.cloudflare.com/profile/api-tokens",
		Credentials: []component.Field{
			{
				Key:         "apiToken",
				Label:       "API Token",
				Type:        component.FieldPassword,
				Required:    true,
				Secret:      true,
				Placeholder: "粘贴 Cloudflare API Token",
				Help: "在 Cloudflare 后台 → My Profile → API Tokens 创建。\n\n" +
					"只做 DDNS 同步和签发证书：Zone → DNS → Edit 就够了。\n" +
					"还要用重定向规则：另外勾 Zone → Rulesets → Edit —— 重定向走的是\n" +
					"规则集接口（rulesets），不是 DNS 接口，只给 DNS 权限会在配重定向时报鉴权失败。\n\n" +
					"可用范围建议只勾需要的那个域名（Include → Specific zone），别给整个账号。\n" +
					"若报 Authentication error（code 10000），多半是权限或可用范围没覆盖到这个域名。",
			},
		},
		Build: func(cred map[string]any) (any, error) {
			token, _ := cred["apiToken"].(string)
			return NewCloudflare(token), nil
		},
	})
}
