package dns

import (
	"github.com/chenggegedeid/catflap/core/component"
	"github.com/chenggegedeid/catflap/core/provider"
	"github.com/chenggegedeid/catflap/modules/ddns/model"
)

// HE.net（dns.he.net）只提供动态 DNS 更新接口：记录要先在它的面板里建好，
// 并**给那条记录单独设一个更新密码**。这里是那个密码，不是账号密码。
//
// 只支持 A 记录，所以打上 dns-ipv4-only —— 前端据此不给它选 AAAA。
func init() {
	provider.MustRegister(provider.Descriptor{
		Key:  string(model.DNSProviderHE),
		Name: "HE.net（dns.he.net）",
		Kind: provider.KindDNS,
		Capabilities: []provider.Capability{
			provider.CapIPv4Only,
		},
		Order:   100,
		DocsURL: "https://dns.he.net/",
		Credentials: []component.Field{
			{
				Key:      "password",
				Label:    "更新密码",
				Type:     component.FieldPassword,
				Required: true,
				Secret:   true,
				Help: "在 dns.he.net 面板里先建好那条 A 记录，再给它单独设一个更新密码；" +
					"这里填的是那个密码，不是登录 HE.net 的账号密码。本程序只更新 IP，不会替你新建记录。",
			},
		},
		Build: func(cred map[string]any) (any, error) {
			p, _ := cred["password"].(string)
			return NewHE(p), nil
		},
	})
}
