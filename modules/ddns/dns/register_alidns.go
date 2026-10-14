package dns

import (
	"github.com/chenggegedeid/catflap/core/component"
	"github.com/chenggegedeid/catflap/core/provider"
	"github.com/chenggegedeid/catflap/modules/ddns/model"
)

func init() {
	provider.MustRegister(provider.Descriptor{
		Key:     string(model.DNSProviderAlidns),
		Name:    "阿里云 DNS",
		Kind:    provider.KindDNS,
		Order:   20,
		DocsURL: "https://ram.console.aliyun.com/manage/ak",
		Credentials: []component.Field{
			{
				Key:      "accessKeyId",
				Label:    "AccessKey ID",
				Required: true,
				Help:     "在阿里云 RAM 控制台创建 AccessKey；建议用子账号并只授予 AliyunDNSFullAccess。",
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
			return NewAlidns(id, secret), nil
		},
	})
}
