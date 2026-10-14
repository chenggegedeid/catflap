package dns

import (
	"github.com/chenggegedeid/catflap/core/component"
	"github.com/chenggegedeid/catflap/core/provider"
	"github.com/chenggegedeid/catflap/modules/ddns/model"
)

// GoDaddy 走的是「按 域名+类型+主机名 覆盖」的 replace 接口，A/AAAA 都支持，
// 也允许把主机名写成 `*` 做通配。
func init() {
	provider.MustRegister(provider.Descriptor{
		Key:  string(model.DNSProviderGoDaddy),
		Name: "GoDaddy",
		Kind: provider.KindDNS,
		Capabilities: []provider.Capability{
			provider.CapWildcard,
		},
		Order:   80,
		DocsURL: "https://developer.godaddy.com/keys",
		Credentials: []component.Field{
			{
				Key:         "apiKey",
				Label:       "API Key",
				Required:    true,
				Placeholder: "GoDaddy 开发者后台生成的 Key",
			},
			{
				Key:         "apiSecret",
				Label:       "API Secret",
				Type:        component.FieldPassword,
				Required:    true,
				Secret:      true,
				Placeholder: "与 Key 配对的 Secret",
				Help:        "两者会拼成 sso-key <Key>:<Secret> 使用，缺一个都不行。",
			},
		},
		Build: func(cred map[string]any) (any, error) {
			k, _ := cred["apiKey"].(string)
			s, _ := cred["apiSecret"].(string)
			return NewGoDaddy(k, s), nil
		},
	})
}
