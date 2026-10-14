package dns

import (
	"github.com/chenggegedeid/catflap/core/component"
	"github.com/chenggegedeid/catflap/core/provider"
	"github.com/chenggegedeid/catflap/modules/ddns/model"
)

// NameCheap 用的是它那套「Dynamic DNS Password」，不是账号密码。
// 而且它只提供 A 记录（IPv4）接口，所以在界面上要禁掉 AAAA 选项 ——
// 这个差异以前是前端硬编码的 ipv4Only 布尔，现在由 CapIPv4Only 表达。
func init() {
	provider.MustRegister(provider.Descriptor{
		Key:          string(model.DNSProviderNameCheap),
		Name:         "NameCheap",
		Kind:         provider.KindDNS,
		Order:        60,
		Capabilities: []provider.Capability{provider.CapIPv4Only},
		Credentials: []component.Field{
			{
				Key:      "password",
				Label:    "Dynamic DNS Password",
				Type:     component.FieldPassword,
				Required: true,
				Secret:   true,
				Help:     "不是登录密码：去 NameCheap → Domain List → 该域名 → Advanced DNS → Dynamic DNS 里开启并复制。",
			},
		},
		Build: func(cred map[string]any) (any, error) {
			password, _ := cred["password"].(string)
			return NewNameCheap(password), nil
		},
	})
}
