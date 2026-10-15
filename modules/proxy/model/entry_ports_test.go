package model

import (
	"reflect"
	"testing"
)

// 老配置的单端口要迁到数组里，且只迁一次
func TestMigratePorts(t *testing.T) {
	cfg := ProxyConfig{LegacyHTTPPort: 80, LegacyHTTPSPort: 443}
	cfg.MigratePorts()

	if !reflect.DeepEqual(cfg.HTTPPorts, []int{80}) {
		t.Errorf("HTTP 端口应迁成 [80]，实际 %v", cfg.HTTPPorts)
	}
	if !reflect.DeepEqual(cfg.HTTPSPorts, []int{443}) {
		t.Errorf("HTTPS 端口应迁成 [443]，实际 %v", cfg.HTTPSPorts)
	}
	// 老字段要清掉，不再写回文件
	if cfg.LegacyHTTPPort != 0 || cfg.LegacyHTTPSPort != 0 {
		t.Error("老字段应清零（避免文件里出现两份端口）")
	}

	// 幂等：再调一次不该动
	cfg.MigratePorts()
	if !reflect.DeepEqual(cfg.HTTPPorts, []int{80}) {
		t.Errorf("重复迁移不该改结果，实际 %v", cfg.HTTPPorts)
	}
}

// 已有数组时，老字段不该覆盖它（老字段是残留的脏数据）
func TestMigratePortsKeepsExisting(t *testing.T) {
	cfg := ProxyConfig{
		HTTPPorts:       []int{80, 8080},
		LegacyHTTPPort:  80, // 老的残留值
		LegacyHTTPSPort: 443,
	}
	cfg.MigratePorts()

	if !reflect.DeepEqual(cfg.HTTPPorts, []int{80, 8080}) {
		t.Errorf("已有数组时不该被老字段覆盖，实际 %v", cfg.HTTPPorts)
	}
	// HTTPS 数组为空，老字段可以补上
	if !reflect.DeepEqual(cfg.HTTPSPorts, []int{443}) {
		t.Errorf("HTTPS 数组为空时应用老字段补上，实际 %v", cfg.HTTPSPorts)
	}
}

// 端口列表要清洗：去重、排序、剔掉非法值
//
// 同一个端口出现两次会让 bind 失败（address already in use），
// 所以去重不是"锦上添花"而是必需。
func TestCleanPorts(t *testing.T) {
	cases := []struct {
		name string
		in   []int
		want []int
	}{
		{"去重", []int{80, 80, 443}, []int{80, 443}},
		{"排序", []int{8080, 80}, []int{80, 8080}},
		{"剔非法", []int{0, -1, 70000, 80}, []int{80}},
		{"全非法", []int{0, -5}, []int{}},
		{"空", nil, []int{}},
	}
	for _, c := range cases {
		got := cleanPorts(c.in)
		if !reflect.DeepEqual(got, c.want) {
			t.Errorf("%s：cleanPorts(%v) = %v, want %v", c.name, c.in, got, c.want)
		}
	}
}

// 多端口时列表要完整保留（这是这版的核心能力）
func TestPortListsKeepAll(t *testing.T) {
	cfg := ProxyConfig{
		HTTPPorts:  []int{80, 8080, 8081},
		HTTPSPorts: []int{443, 8443},
	}

	if got := cfg.HTTPPortList(); len(got) != 3 {
		t.Errorf("HTTP 应有 3 个端口，实际 %v", got)
	}
	if got := cfg.HTTPSPortList(); len(got) != 2 {
		t.Errorf("HTTPS 应有 2 个端口，实际 %v", got)
	}
	if !cfg.HasHTTPS() {
		t.Error("配了 HTTPS 端口时 HasHTTPS 应为 true（界面靠它决定默认协议）")
	}

	// 没配 HTTPS
	noTLS := ProxyConfig{HTTPPorts: []int{80}}
	if noTLS.HasHTTPS() {
		t.Error("没配 HTTPS 端口时 HasHTTPS 应为 false")
	}
}
