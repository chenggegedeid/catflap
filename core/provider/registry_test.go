package provider

import (
	"errors"
	"strings"
	"testing"

	"github.com/chenggegedeid/catflap/core/component"
)

func dnsDesc(key string, caps ...Capability) Descriptor {
	return Descriptor{
		Key:          key,
		Name:         strings.ToUpper(key),
		Kind:         KindDNS,
		Capabilities: caps,
		Credentials: []component.Field{
			{Key: "token", Label: "Token", Type: component.FieldPassword, Required: true, Secret: true},
		},
		Build: func(cred map[string]any) (any, error) {
			t, _ := cred["token"].(string)
			if t == "" {
				return nil, errors.New("token 为空")
			}
			return "client:" + t, nil
		},
	}
}

func TestRegisterAndListKeepsOrder(t *testing.T) {
	r := NewRegistry()
	for _, k := range []string{"cloudflare", "alidns", "tencent"} {
		if err := r.Register(dnsDesc(k, CapDNS01)); err != nil {
			t.Fatal(err)
		}
	}
	keys := r.Keys(KindDNS)
	if strings.Join(keys, ",") != "cloudflare,alidns,tencent" {
		t.Fatalf("应保留注册顺序，实际 %v", keys)
	}
	// SortedKeys 与外网表现（生成文档/测试）需要确定性
	if got := strings.Join(r.SortedKeys(KindDNS), ","); got != "alidns,cloudflare,tencent" {
		t.Fatalf("SortedKeys 应字典序，实际 %s", got)
	}
}

func TestRegisterValidatesDescriptor(t *testing.T) {
	r := NewRegistry()

	bad := []Descriptor{
		{Name: "x", Kind: KindDNS, Build: func(map[string]any) (any, error) { return 1, nil }},           // 缺 Key
		{Key: "X", Name: "x", Kind: KindDNS, Build: func(map[string]any) (any, error) { return 1, nil }}, // Key 必须小写
		{Key: "x", Kind: KindDNS, Build: func(map[string]any) (any, error) { return 1, nil }},            // 缺 Name
		{Key: "x", Name: "x", Build: func(map[string]any) (any, error) { return 1, nil }},                // 缺 Kind
		{Key: "x", Name: "x", Kind: KindDNS},                                                             // 缺 Build
		{Key: "x", Name: "x", Kind: KindDNS, Build: func(map[string]any) (any, error) { return 1, nil },
			Credentials: []component.Field{{Key: "a"}, {Key: "a"}}}, // 字段重复
		{Key: "x", Name: "x", Kind: KindDNS, Build: func(map[string]any) (any, error) { return 1, nil },
			Credentials: []component.Field{{Key: "a", Type: component.FieldSelect}}}, // 下拉无选项
	}
	for i, d := range bad {
		if err := r.Register(d); err == nil {
			t.Fatalf("第 %d 个非法描述符应被拒绝：%+v", i, d)
		}
	}

	if err := r.Register(dnsDesc("cloudflare")); err != nil {
		t.Fatal(err)
	}
	if err := r.Register(dnsDesc("cloudflare")); err == nil {
		t.Fatal("重复注册应报错")
	}
}

func TestCapable(t *testing.T) {
	r := NewRegistry()
	if err := r.Register(dnsDesc("full", CapDNS01, CapRedirect, CapSRV)); err != nil {
		t.Fatal(err)
	}
	if err := r.Register(dnsDesc("basic")); err != nil {
		t.Fatal(err)
	}

	if !r.Capable(KindDNS, "full", CapDNS01, CapSRV) {
		t.Fatal("full 应当同时具备 DNS-01 与 SRV")
	}
	if r.Capable(KindDNS, "full", CapDNS01, CapWildcard) {
		t.Fatal("full 不具备通配符能力，应为 false")
	}
	if r.Capable(KindDNS, "basic", CapDNS01) {
		t.Fatal("basic 不具备任何能力")
	}
	if r.Capable(KindDNS, "nope", CapDNS01) {
		t.Fatal("未知服务商应为 false")
	}
}

func TestBuildEnforcesValidation(t *testing.T) {
	r := NewRegistry()
	if err := r.Register(dnsDesc("cloudflare", CapDNS01)); err != nil {
		t.Fatal(err)
	}

	if _, err := r.Build(KindDNS, "nope", nil); err == nil {
		t.Fatal("未知服务商应当报错")
	}
	if _, err := r.Build(KindDNS, "cloudflare", nil); err == nil {
		t.Fatal("缺失必填凭据应当报错")
	}

	got, err := r.Build(KindDNS, "cloudflare", map[string]any{"token": "abc"})
	if err != nil {
		t.Fatal(err)
	}
	if got != "client:abc" {
		t.Fatalf("构造结果不对：%v", got)
	}
}

func TestBuildRejectsNilInstance(t *testing.T) {
	r := NewRegistry()
	if err := r.Register(Descriptor{
		Key: "x", Name: "X", Kind: KindDNS,
		Build: func(map[string]any) (any, error) { return nil, nil },
	}); err != nil {
		t.Fatal(err)
	}
	if _, err := r.Build(KindDNS, "x", nil); err == nil {
		t.Fatal("Build 返回 nil 实例应当报错，而不是把 nil 交给调用方")
	}
}

func TestListAllOnlyIncludesNonEmptyKinds(t *testing.T) {
	r := NewRegistry()
	if err := r.Register(dnsDesc("cloudflare")); err != nil {
		t.Fatal(err)
	}
	all := r.ListAll()
	if len(all[KindDNS]) != 1 {
		t.Fatalf("DNS 应有 1 项，实际 %d", len(all[KindDNS]))
	}
	if _, ok := all[KindNotify]; ok {
		t.Fatal("没有注册项的类别不应出现在结果里")
	}
}
