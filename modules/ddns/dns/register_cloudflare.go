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
				Help: "在 Cloudflare 后台 → My Profile → API Tokens 创建，权限需要 Zone → DNS → Edit；" +
					"签证书与写重定向规则都走同一个 Token。",
			},
		},
		Build: func(cred map[string]any) (any, error) {
			token, _ := cred["apiToken"].(string)
			return NewCloudflare(token), nil
		},
	})
}
