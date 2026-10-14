package dns

import (
	"reflect"
	"testing"

	"github.com/chenggegedeid/catflap/modules/ddns/model"
)

// 本文件是 A3 改造的**对照测试**：把改造前 dns_build_provider.go 里那张
// 硬编码 switch 的行为原样抄一份放在这里，再断言注册表给出的结果一致。
//
// 为什么值得单独写：拆掉 switch 换成注册表，最怕的不是编译不过，
// 而是**悄悄少了一家**或**某家少了一个必填字段**。那种问题编译器和
// 现有测试都发现不了，只会在用户点保存时报「暂不支持的服务商类型」。

// legacyCredentialKeys 改造前 model.ProviderCredentialKeys 的内容，一字未改。
var legacyCredentialKeys = map[model.DNSProviderType][]string{
	model.DNSProviderCloudflare:   {"apiToken"},
	model.DNSProviderAlidns:       {"accessKeyId", "accessKeySecret"},
	model.DNSProviderTencentCloud: {"secretId", "secretKey"},
	model.DNSProviderBaiduCloud:   {"accessKeyId", "accessKeySecret"},
	model.DNSProviderHuaweiCloud:  {"accessKeyId", "accessKeySecret"},
	model.DNSProviderNameCheap:    {"password"},
	model.DNSProviderNameSilo:     {"apiKey"},
}

// legacyACME / legacyRedirect 改造前 SupportsACMEDNS / SupportsRedirectRule 的行为：
// 只有 Cloudflare 返回 true。
var legacyOnlyCloudflare = map[model.DNSProviderType]bool{
	model.DNSProviderCloudflare:   true,
	model.DNSProviderAlidns:       false,
	model.DNSProviderTencentCloud: false,
	model.DNSProviderBaiduCloud:   false,
	model.DNSProviderHuaweiCloud:  false,
	model.DNSProviderNameCheap:    false,
	model.DNSProviderNameSilo:     false,
}

// allTypes 全部 7 家，顺序与配置里历史的展示顺序一致。
var allTypes = []model.DNSProviderType{
	model.DNSProviderCloudflare,
	model.DNSProviderAlidns,
	model.DNSProviderTencentCloud,
	model.DNSProviderBaiduCloud,
	model.DNSProviderHuaweiCloud,
	model.DNSProviderNameCheap,
	model.DNSProviderNameSilo,
}

func TestRegistryCoversAllLegacyProviders(t *testing.T) {
	for _, tp := range allTypes {
		if !IsSupported(tp) {
			t.Errorf("%s 未注册：改造前 switch 是支持它的", tp)
		}
	}

	// 反向：注册表里不该多出 switch 时代没有的类型（多出来会让界面出现
	// 一个选了必然报错的选项）
	if got, want := len(Descriptors()), len(allTypes); got != want {
		t.Errorf("注册了 %d 家，期望 %d 家", got, want)
	}
}

func TestCredentialKeysMatchLegacy(t *testing.T) {
	for _, tp := range allTypes {
		want := legacyCredentialKeys[tp]
		got := CredentialKeys(tp)
		if !reflect.DeepEqual(got, want) {
			t.Errorf("%s 的凭据字段 = %v，期望 %v", tp, got, want)
		}
	}
}

func TestCapabilitiesMatchLegacy(t *testing.T) {
	for _, tp := range allTypes {
		if got, want := SupportsACMEDNS(tp), legacyOnlyCloudflare[tp]; got != want {
			t.Errorf("SupportsACMEDNS(%s) = %v，期望 %v", tp, got, want)
		}
		if got, want := SupportsRedirectRule(tp), legacyOnlyCloudflare[tp]; got != want {
			t.Errorf("SupportsRedirectRule(%s) = %v，期望 %v", tp, got, want)
		}
	}
}

// TestBuildAllProviders 用各家的真实字段名填一组假凭据，确认能构造出实例。
//
// 假凭据不会联网，构造器只做赋值，所以这一步能验证「字段名对得上」——
// 这正是改注册表最容易写错的地方（Build 里取错 key 会静默拿到空字符串）。
func TestBuildAllProviders(t *testing.T) {
	for _, tp := range allTypes {
		cred := map[string]any{}
		for _, k := range CredentialKeys(tp) {
			cred[k] = "fake-" + k
		}
		p := model.DDNSProvider{Type: tp, Credential: cred}

		c, err := BuildDNSProvider(p)
		if err != nil {
			t.Errorf("%s 构造失败：%v", tp, err)
			continue
		}
		if c == nil {
			t.Errorf("%s 构造出了空实例", tp)
		}
	}
}

// TestBuildRejectsEmptyCredential 空凭据必须被注册表挡下来。
//
// 改造前 BuildClient 对缺字段是「拿了空字符串继续跑」，要等到真去写记录
// 才由服务商返回 401；现在在构造阶段就报错，错误信息也更明确。
func TestBuildRejectsEmptyCredential(t *testing.T) {
	for _, tp := range allTypes {
		p := model.DDNSProvider{Type: tp, Credential: map[string]any{}}
		if _, err := BuildDNSProvider(p); err == nil {
			t.Errorf("%s 凭据全空却构造成功了，期望报错", tp)
		}
	}
}

func TestBuildUnknownProviderIsNil(t *testing.T) {
	p := model.DDNSProvider{Type: "not-a-real-provider", Credential: map[string]any{}}
	if _, err := BuildDNSProvider(p); err == nil {
		t.Error("未注册的服务商却构造成功了")
	}
	// 兼容函数：老调用点按 nil 判断
	if c := BuildClient(p); c != nil {
		t.Error("BuildClient 对未知服务商应返回 nil")
	}
	if IsSupported("not-a-real-provider") {
		t.Error("IsSupported 对未知服务商应返回 false")
	}
	if CredentialKeys("not-a-real-provider") != nil {
		t.Error("未知服务商不应有凭据字段")
	}
}

// TestDescriptorsOrderIsStable 界面上的顺序不能随 init 的文件顺序漂移。
func TestDescriptorsOrderIsStable(t *testing.T) {
	want := []string{
		"cloudflare", "alidns", "tencentcloud", "baiducloud",
		"huaweicloud", "namecheap", "namesilo",
	}
	var got []string
	for _, d := range Descriptors() {
		got = append(got, d.Key)
	}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("服务商顺序 = %v，期望 %v", got, want)
	}
}

// TestSchemaRedactsSecrets 表单结构里凡是标了 Secret 的字段，回显时必须脱敏。
//
// 这条直接关系到 A3 的一个副产品：表单由描述符生成之后，
// 「哪些字段是密码」这件事也由描述符说了算，脱敏必须跟着同一份声明走。
func TestSchemaRedactsSecrets(t *testing.T) {
	for _, tp := range allTypes {
		schema, ok := SchemaFor(tp)
		if !ok {
			t.Fatalf("%s 没有表单结构", tp)
		}
		values := map[string]any{}
		for _, f := range schema.Fields {
			values[f.Key] = "super-secret-value"
		}

		red := schema.Redact(values)
		for _, f := range schema.Fields {
			if !f.Secret {
				continue
			}
			if red[f.Key] != "***" {
				t.Errorf("%s 的字段 %s 未脱敏，得到 %v", tp, f.Key, red[f.Key])
			}
		}
	}
}
