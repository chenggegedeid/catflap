package dns

import (
	"github.com/chenggegedeid/catflap/core/component"
	"github.com/chenggegedeid/catflap/core/provider"
	"github.com/chenggegedeid/catflap/modules/ddns/model"
)

// DuckDNS 只要一个 token，接口也只管更新 IP。
// 只支持 IPv4，所以打上 dns-ipv4-only —— 前端据此不给它选 AAAA。
func init() {
	provider.MustRegister(provider.Descriptor{
		Key:  string(model.DNSProviderDuckDNS),
		Name: "DuckDNS",
		Kind: provider.KindDNS,
		Capabilities: []provider.Capability{
			provider.CapIPv4Only,
		},
		Order:   110,
		DocsURL: "https://www.duckdns.org/",
		Credentials: []component.Field{
			{
				Key:      "token",
				Label:    "Token",
				Type:     component.FieldPassword,
				Required: true,
				Secret:   true,
				Help: "登录 duckdns.org 后首页就能看到。子域名那一栏填 xxxx.duckdns.org 里的 xxxx，" +
					"主域名填 duckdns.org。",
			},
		},
		Build: func(cred map[string]any) (any, error) {
			t, _ := cred["token"].(string)
			return NewDuckDNS(t), nil
		},
	})
}
