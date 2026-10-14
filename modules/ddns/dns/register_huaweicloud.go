package dns

import (
	"github.com/chenggegedeid/catflap/core/component"
	"github.com/chenggegedeid/catflap/core/provider"
	"github.com/chenggegedeid/catflap/modules/ddns/model"
)

func init() {
	provider.MustRegister(provider.Descriptor{
		Key:     string(model.DNSProviderHuaweiCloud),
		Name:    "华为云 DNS",
		Kind:    provider.KindDNS,
		Order:   50,
		DocsURL: "https://console.huaweicloud.com/iam/#/myCredential",
		Credentials: []component.Field{
			{
				Key:      "accessKeyId",
				Label:    "AccessKey ID",
				Required: true,
				Help:     "在华为云 → 我的凭证 → 访问密钥里创建，建议用子用户并只授予 DNS FullAccess。",
			},
			{
				Key:      "accessKeySecret",
				Label:    "AccessKey Secret",
				Type:     component.FieldPassword,
				Required: true,
				Secret:   true,
			},
		},
		Build: func(cred map[string]any) (any, error) {
			id, _ := cred["accessKeyId"].(string)
			secret, _ := cred["accessKeySecret"].(string)
			return NewHuaweicloud(id, secret), nil
		},
	})
}
