package dns

import (
	"github.com/chenggegedeid/catflap/core/component"
	"github.com/chenggegedeid/catflap/core/provider"
	"github.com/chenggegedeid/catflap/modules/ddns/model"
)

func init() {
	provider.MustRegister(provider.Descriptor{
		Key:     string(model.DNSProviderBaiduCloud),
		Name:    "百度云 DNS",
		Kind:    provider.KindDNS,
		Order:   40,
		DocsURL: "https://console.bce.baidu.com/iam/#/iam/accesslist",
		Credentials: []component.Field{
			{
				Key:      "accessKeyId",
				Label:    "AccessKey ID",
				Required: true,
				Help:     "在百度智能云 → 用户中心 → 安全认证里创建 Access Key。",
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
			return NewBaiduCloud(id, secret), nil
		},
	})
}
