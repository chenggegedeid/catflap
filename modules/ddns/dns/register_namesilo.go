package dns

import (
	"github.com/chenggegedeid/catflap/core/component"
	"github.com/chenggegedeid/catflap/core/provider"
	"github.com/chenggegedeid/catflap/modules/ddns/model"
)

func init() {
	provider.MustRegister(provider.Descriptor{
		Key:   string(model.DNSProviderNameSilo),
		Name:  "NameSilo",
		Kind:  provider.KindDNS,
		Order: 70,
		Credentials: []component.Field{
			{
				Key:      "apiKey",
				Label:    "API Key",
				Type:     component.FieldPassword,
				Required: true,
				Secret:   true,
				Help:     "在 NameSilo → Account → API Manager 里生成；生成后还要在同一个页面把本机公网 IP 加进白名单。",
			},
		},
		Build: func(cred map[string]any) (any, error) {
			apiKey, _ := cred["apiKey"].(string)
			return NewNameSilo(apiKey), nil
		},
	})
}
