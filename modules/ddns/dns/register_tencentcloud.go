package dns

import (
	"github.com/chenggegedeid/catflap/core/component"
	"github.com/chenggegedeid/catflap/core/provider"
	"github.com/chenggegedeid/catflap/modules/ddns/model"
)

func init() {
	provider.MustRegister(provider.Descriptor{
		Key:     string(model.DNSProviderTencentCloud),
		Name:    "腾讯云 DNSPod",
		Kind:    provider.KindDNS,
		Order:   30,
		DocsURL: "https://console.cloud.tencent.com/cam/capi",
		Credentials: []component.Field{
			{
				Key:      "secretId",
				Label:    "SecretId",
				Required: true,
				Help:     "在腾讯云访问管理 → API 密钥管理里创建，建议用子账号并只授予 DNSPod 相关权限。",
			},
			{
				Key:      "secretKey",
				Label:    "SecretKey",
				Type:     component.FieldPassword,
				Required: true,
				Secret:   true,
			},
		},
		Build: func(cred map[string]any) (any, error) {
			id, _ := cred["secretId"].(string)
			key, _ := cred["secretKey"].(string)
			return NewTencentCloud(id, key), nil
		},
	})
}
