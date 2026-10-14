package dns

import (
	"strings"

	"github.com/chenggegedeid/catflap/core/component"
	"github.com/chenggegedeid/catflap/core/provider"
	"github.com/chenggegedeid/catflap/modules/ddns/model"

	"github.com/sirupsen/logrus"
)

// 本文件是「服务商能力」的统一出口，全部由 core/provider 注册表驱动。
//
// # 为什么要改成注册表
//
// 以前这里是四个 switch：BuildClient / SupportsACMEDNS / SupportsRedirectRule
// / BuildSRVClient。加一家服务商要同时改**后端三处 + 前端一张表**，
// 漏任何一处就会出现「能选但保存不了」「能保存但签不出证书」这类半隐身故障。
//
// 现在：新增一家 = 新增一个 register_<名字>.go，能力与凭据字段都写在描述符里，
// 前端表单由 Credentials 自动生成。本文件一行都不用动。
//
// # 类型断言为什么还留着
//
// Descriptor.Build 返回 any（core 不能反向依赖 modules），所以具体是不是
// DNSProvider / ACMEDNSProvider 只能在这里断言一次。断言失败时返回 nil，
// 与改造前的行为一致：调用方据此提示「该服务商暂不支持 XX」。
//
// 值得注意的是**能力标记与接口实现是两件事**：
//   - 能力标记（Capabilities）是给界面看的，决定选项能不能点；
//   - 接口实现是运行时的真相，决定调用会不会成功。
//
// 新增服务商时两者都要对：只写标记不实现接口，界面会放行然后运行时报错。
// 因此下面的 SupportsXxx 一律「先看标记，再看断言」，两者都满足才算支持。

// BuildDNSProvider 用注册表构造服务商实例。
//
// 与老的 BuildClient 的区别：返回 error，能把「类型没注册」「凭据缺字段」
// 这些具体原因带出去。新代码应当用这个。
func BuildDNSProvider(p model.DDNSProvider) (DNSProvider, error) {
	kind := string(p.Type)

	// 配置里存的是字符串，历史值必须能和注册表的 Key 对上。
	// 多余的空格（手工改过配置文件）不该导致「未知服务商」，先规整再查。
	kind = strings.TrimSpace(strings.ToLower(kind))

	inst, err := provider.Build(provider.KindDNS, kind, p.Credential)
	if err != nil {
		return nil, err
	}
	client, ok := inst.(DNSProvider)
	if !ok {
		return nil, errProviderCapabilityMiss(kind, "解析记录")
	}
	return client, nil
}

// BuildClient 兼容旧调用点：失败时返回 nil。
//
// 不要在新代码里用 —— nil 会把「为什么失败」丢掉。保留它只是因为
// ddns 的调度器、stun 的入口同步等十来处调用点目前都按 nil 判断。
func BuildClient(p model.DDNSProvider) DNSProvider {
	c, err := BuildDNSProvider(p)
	if err != nil {
		logrus.Warnf("构建 DNS 服务商 %s 失败：%v", p.Type, err)
		return nil
	}
	return c
}

// BuildACMEClient 构建支持 ACME DNS-01 的客户端。
// 服务商未实现 ACMEDNSProvider 时返回 nil，调用方据此提示用户改用其他方式。
func BuildACMEClient(p model.DDNSProvider) ACMEDNSProvider {
	c := BuildClient(p)
	if c == nil {
		return nil
	}
	acmeClient, ok := c.(ACMEDNSProvider)
	if !ok {
		return nil
	}
	return acmeClient
}

// SupportsACMEDNS 该服务商类型是否支持 DNS-01（给前端做能力展示用）
func SupportsACMEDNS(t model.DNSProviderType) bool {
	return provider.Capable(provider.KindDNS, string(t), provider.CapDNS01)
}

// BuildRedirectClient 构建支持入口重定向规则同步的客户端。
// 服务商未实现 RedirectRuleProvider 时返回 nil，调用方据此提示用户换服务商。
func BuildRedirectClient(p model.DDNSProvider) RedirectRuleProvider {
	c := BuildClient(p)
	if c == nil {
		return nil
	}
	redirectClient, ok := c.(RedirectRuleProvider)
	if !ok {
		return nil
	}
	return redirectClient
}

// SupportsRedirectRule 该服务商类型是否支持入口重定向（给前端做能力展示用）
func SupportsRedirectRule(t model.DNSProviderType) bool {
	return provider.Capable(provider.KindDNS, string(t), provider.CapRedirect)
}

// BuildSRVClient 构建支持 SRV 记录的客户端；服务商没实现时返回 nil
func BuildSRVClient(p model.DDNSProvider) SRVRecordProvider {
	c := BuildClient(p)
	if c == nil {
		return nil
	}
	srvClient, ok := c.(SRVRecordProvider)
	if !ok {
		return nil
	}
	return srvClient
}

// SupportsSRV 该服务商类型是否支持 SRV 记录（给前端做能力展示用）
func SupportsSRV(t model.DNSProviderType) bool {
	return provider.Capable(provider.KindDNS, string(t), provider.CapSRV)
}

// ── 注册表查询（供 ddns 模块与接口层使用）────────────────────────────────

// IsSupported 该服务商类型是否已注册。
func IsSupported(t model.DNSProviderType) bool {
	return provider.ExistsDefault(provider.KindDNS, string(t))
}

// CredentialKeys 该服务商类型需要的凭据字段名，顺序与界面显示一致。
//
// 这是校验的**唯一依据**：以前靠 model.ProviderCredentialKeys 这张手维护的
// 表，与注册表并存就会出现「注册了但校验不认」的裂缝。
func CredentialKeys(t model.DNSProviderType) []string {
	d, ok := provider.Get(provider.KindDNS, string(t))
	if !ok {
		return nil
	}
	keys := make([]string, 0, len(d.Credentials))
	for _, f := range d.Credentials {
		keys = append(keys, f.Key)
	}
	return keys
}

// Descriptors 列出全部已注册的 DNS 服务商，供界面渲染「选一家」与自动表单。
func Descriptors() []provider.Descriptor {
	return provider.List(provider.KindDNS)
}

// SchemaFor 取某家服务商的凭据表单结构。
func SchemaFor(t model.DNSProviderType) (component.Schema, bool) {
	d, ok := provider.Get(provider.KindDNS, string(t))
	if !ok {
		return component.Schema{}, false
	}
	return d.Schema(), true
}

func errProviderCapabilityMiss(key, what string) error {
	return &capabilityError{key: key, what: what}
}

type capabilityError struct {
	key  string
	what string
}

func (e *capabilityError) Error() string {
	return "服务商 " + e.key + " 未实现「" + e.what + "」能力"
}
