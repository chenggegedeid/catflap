// Package provider 是「服务商」的注册表。
//
// 这里的「服务商」指同一类能力的可替换实现：DNS 服务商、通知渠道、存储后端等。
//
// # 它替代了什么
//
// 引入本包之前，加一家 DNS 服务商要改**三处后端 + 一处前端**：
//
//	modules/ddns/dns/dns_build_provider.go  BuildClient()     的 switch 加一个 case
//	                                 同文件  BuildACMEClient() 由 BuildClient 派生，但要确认新商支持
//	                                 同文件  SupportsACMEDNS() 的 switch 再加一个 case
//	web/admin/src/pages/Ddns.tsx            前端表单加一组输入框
//
// 漏掉任何一处，用户就会遇到「能选但保存不了」或「能保存但签不出证书」。
//
// # 换成本包之后
//
// 新增一家服务商 = **新增一个文件**，前端的表单由 Descriptor.Credentials 自动生成：
//
//	// modules/ddns/dns/register_cloudflare.go
//	package dns
//
//	import "github.com/chenggegedeid/catflap/core/provider"
//
//	func init() {
//	    provider.MustRegister(provider.Descriptor{
//	        Key:          "cloudflare",
//	        Name:         "Cloudflare",
//	        Kind:         provider.KindDNS,
//	        Capabilities: []provider.Capability{provider.CapDNS01, provider.CapRedirect, provider.CapSRV},
//	        DocsURL:      "https://dash.cloudflare.com/profile/api-tokens",
//	        Credentials: []component.Field{
//	            {
//	                Key: "apiToken", Label: "API Token", Type: component.FieldPassword,
//	                Required: true, Secret: true,
//	                Help: "在 Cloudflare 后台创建，需要 Zone:DNS:Edit 权限",
//	            },
//	        },
//	        Build: func(cred map[string]any) (any, error) {
//	            token, _ := cred["apiToken"].(string)
//	            if token == "" {
//	                return nil, errors.New("API Token 不能为空")
//	            }
//	            return NewCloudflare(token), nil
//	        },
//	    })
//	}
//
// 迁移期两套机制并存：老的 switch 继续能跑，**新增一律走注册表**。
package provider

import (
	"errors"
	"fmt"
	"strings"

	"github.com/chenggegedeid/catflap/core/component"
)

// Kind 提供者的类别。
type Kind string

const (
	// KindDNS DNS 服务商（DDNS 解析记录、ACME DNS-01、重定向规则、SRV 记录）。
	KindDNS Kind = "dns"
	// KindNotify 通知渠道（Webhook、邮件、各类机器人）。
	KindNotify Kind = "notify"
	// KindStorage 存储后端（配置与数据的落盘位置）。
	KindStorage Kind = "storage"
)

// AllKinds 全部类别，供界面遍历。
func AllKinds() []Kind { return []Kind{KindDNS, KindNotify, KindStorage} }

// Capability 能力标记。同一类别的不同服务商能力并不相同
// （例如有的 DNS 商支持写 SRV 记录、有的不支持），用它来表达，
// 而不是让调用方去 type assert 到具体类型。
type Capability string

const (
	// CapDNS01 支持 ACME DNS-01 质询，可用于签通配符证书。
	CapDNS01 Capability = "dns-01"
	// CapRedirect 支持写重定向规则（固定入口用）。
	CapRedirect Capability = "dns-redirect"
	// CapSRV 支持写 SRV 记录（Minecraft 入口用）。
	CapSRV Capability = "dns-srv"
	// CapWildcard 支持通配符记录。
	CapWildcard Capability = "dns-wildcard"
	// CapTestable 提供「测试连接」能力。
	CapTestable Capability = "testable"
	// CapProxied 支持「小云朵」代理开关（记录走服务商边缘，而非直连源站）。
	CapProxied Capability = "dns-proxied"
	// CapIPv4Only 只支持 A 记录，不能写 AAAA。
	//
	// 单独标出来是因为它会**改变界面的默认选择**：这类服务商下选 AAAA
	// 会在保存时被服务商拒掉，不如在界面上就不给选。
	CapIPv4Only Capability = "dns-ipv4-only"
)

// Descriptor 一个服务商的完整描述。
type Descriptor struct {
	// Key 唯一键，小写，与配置文件里存的历史取值保持一致（例如 "cloudflare"）。
	// ⚠️ 一旦发布就不要改 —— 用户配置里存的是它。
	Key string `json:"key"`
	// Name 界面上显示的名字。
	Name string `json:"name"`
	// Kind 类别。
	Kind Kind `json:"kind"`
	// Capabilities 能力列表。
	Capabilities []Capability `json:"capabilities,omitempty"`
	// Credentials 需要的凭据字段，声明式。前端据此自动生成表单。
	Credentials []component.Field `json:"credentials,omitempty"`
	// DocsURL 「去哪拿这些凭据」的官方文档地址，界面上给个链接。
	DocsURL string `json:"docsUrl,omitempty"`
	// Order 界面列表里的排序权重，小的在前；留 0 按注册先后排。
	//
	// 为什么需要它：注册发生在各文件的 init() 里，而**同一个包内多个文件的
	// init 顺序不受语言规范保证**（实践中按文件名序，但别赌）。想把「Cloudflare
	// 排第一个」这种产品决策写死，就得有个显式字段，而不是靠文件名前缀。
	Order int `json:"order,omitempty"`
	// Build 用凭据构造出可用实例。
	//
	// 返回 any 而不是某个具体接口，是为了**避免 core 反向依赖 modules**：
	// 具体类型必须满足调用方的接口（例如 dns.DNSProvider），
	// 由调用方做一次类型断言；不满足时应当返回 error 而不是 nil + 不报错。
	Build func(cred map[string]any) (any, error) `json:"-"`
}

// Has 是否具备某项能力。
func (d Descriptor) Has(cap Capability) bool {
	for _, c := range d.Capabilities {
		if c == cap {
			return true
		}
	}
	return false
}

// Schema 把凭据声明包成通用的配置 Schema，供前端表单与脱敏复用。
func (d Descriptor) Schema() component.Schema {
	return component.Schema{
		Title:       d.Name,
		Description: "凭据仅保存在本机的配置目录中，不会外发。",
		Fields:      d.Credentials,
	}
}

// Validate 校验凭据是否满足声明（必填、类型）。
//
// 只做不联网就能判断的检查。真要确认凭据对不对，得由提供者自己实现
// CapTestable + 一个测试接口去连一次服务商。
func (d Descriptor) Validate(cred map[string]any) error {
	return d.Schema().Validate(cred)
}

// RedactCredential 返回脱敏后的凭据副本，用于日志、导出与回显。
func (d Descriptor) RedactCredential(cred map[string]any) map[string]any {
	return d.Schema().Redact(cred)
}

// validate 检查 Descriptor 自身是否合法。注册时调用，把错误挡在启动阶段。
func (d Descriptor) validate() error {
	if strings.TrimSpace(d.Key) == "" {
		return errors.New("Key 不能为空")
	}
	if d.Key != strings.ToLower(d.Key) {
		return fmt.Errorf("Key %q 必须是小写（它会写进用户配置文件）", d.Key)
	}
	if strings.TrimSpace(d.Name) == "" {
		return fmt.Errorf("服务商 %q 的 Name 不能为空", d.Key)
	}
	if d.Kind == "" {
		return fmt.Errorf("服务商 %q 的 Kind 不能为空", d.Key)
	}
	if d.Build == nil {
		return fmt.Errorf("服务商 %q 的 Build 不能为 nil", d.Key)
	}

	seen := make(map[string]bool, len(d.Credentials))
	for _, f := range d.Credentials {
		if strings.TrimSpace(f.Key) == "" {
			return fmt.Errorf("服务商 %q 有未命名的凭据字段", d.Key)
		}
		if seen[f.Key] {
			return fmt.Errorf("服务商 %q 的凭据字段 %q 重复", d.Key, f.Key)
		}
		seen[f.Key] = true

		if f.TypeOrDefault() == component.FieldSelect && len(f.Options) == 0 {
			return fmt.Errorf("服务商 %q 的字段 %q 是下拉框但没给 Options", d.Key, f.Key)
		}
	}
	return nil
}
