package ddns_api

import (
	"github.com/chenggegedeid/catflap/core/component"
	"github.com/chenggegedeid/catflap/core/provider"
	"github.com/chenggegedeid/catflap/modules/ddns/dns"
	"github.com/chenggegedeid/catflap/modules/ddns/model"
	"github.com/chenggegedeid/catflap/utils/res"

	"github.com/gin-gonic/gin"
)

// ProviderTypeView 一个**可选**的 DNS 服务商类型，含它需要的凭据字段。
//
// 注意与 ProviderView（get_ddns_config.go）的区别：
//   - ProviderView 是「用户已经配好的某一家」，带 ID 与是否已填凭据；
//   - ProviderTypeView 是「系统支持哪些家」，是添加表单的选项来源。
//
// 这个接口存在的意义就是让前端**不要再维护一张服务商字段表**：
// 以前新增一家服务商要在 Ddns.tsx 里手动加一项，漏了就出现
// 「后端能存但界面上选不到」；现在前端只渲染，字段与能力都由这里给。
type ProviderTypeView struct {
	Type  model.DNSProviderType `json:"type"`
	Label string                `json:"label"`
	// DocsURL 「去哪拿凭据」的官方地址，可空
	DocsURL string `json:"docsUrl,omitempty"`
	// Capabilities 能力标记，前端据此决定选项可用性
	// （例如 dns-ipv4-only 的服务商不给选 AAAA 记录、dns-proxied 才显示小云朵开关）
	Capabilities []provider.Capability `json:"capabilities"`
	// Fields 凭据字段声明，前端据此自动生成输入框。
	// 标了 secret 的字段后端只会回显 "***"，不会回传明文。
	Fields []component.Field `json:"fields"`
}

// DdnsProviderTypesView 列出系统支持的全部 DNS 服务商及其凭据字段
func (DdnsApi) DdnsProviderTypesView(c *gin.Context) {
	descs := dns.Descriptors()

	list := make([]ProviderTypeView, 0, len(descs))
	for _, d := range descs {
		list = append(list, ProviderTypeView{
			Type:         model.DNSProviderType(d.Key),
			Label:        d.Name,
			DocsURL:      d.DocsURL,
			Capabilities: d.Capabilities,
			Fields:       d.Credentials,
		})
	}
	res.OkWithData(list, c)
}
