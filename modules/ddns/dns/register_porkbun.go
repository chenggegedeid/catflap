package dns

import (
	"github.com/chenggegedeid/catflap/core/component"
	"github.com/chenggegedeid/catflap/core/provider"
	"github.com/chenggegedeid/catflap/modules/ddns/model"
)

// Porkbun 的 API 要求 TTL 至少 600 秒，实现里已经兜底；A/AAAA 都支持。
// 它也有通配记录的支持，但**不支持**在本程序里做 ACME DNS-01（没实现 TXT 写入）。
func init() {
	provider.MustRegister(provider.Descriptor{
		Key:  string(model.DNSProviderPorkbun),
		Name: "Porkbun",
		Kind: provider.KindDNS,
		Capabilities: []provider.Capability{
			provider.CapWildcard,
		},
		Order:   90,
		DocsURL: "https://porkbun.com/account/api",
		Credentials: []component.Field{
			{
				Key:      "apiKey",
				Label:    "API Key",
				Required: true,
			},
			{
				Key:      "secretApiKey",
				Label:    "Secret API Key",
				Type:     component.FieldPassword,
				Required: true,
				Secret:   true,
				Help:     "两个 Key 都在 Porkbun 账号的 API 页面生成，Secret 只显示一次。",
			},
		},
		Build: func(cred map[string]any) (any, error) {
			k, _ := cred["apiKey"].(string)
			s, _ := cred["secretApiKey"].(string)
			return NewPorkbun(k, s), nil
		},
	})
}
