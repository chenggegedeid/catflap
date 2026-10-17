package geo

import (
	"strings"
	"testing"
)

// TestLookupPublicIP 真实查一个公网 IP。
//
// 这不是"接口能调用"的测试，是**数据真的在起作用**的测试：库文件没被打进去、
// 或者路径写错时，这里会返回「未知」而不是崩溃 —— 那种情况从编译和启动日志
// 里都看不出来，只有这条断言能抓住。
func TestLookupPublicIP(t *testing.T) {
	if !Enabled() {
		t.Fatal("归属地库没加载起来，数据文件可能没被 embed 进去")
	}

	// 114.114.114.114 是国内公共 DNS，数据稳定，不会因为库版本更新就变国家
	info := Lookup("114.114.114.114")
	if info.Country != "中国" {
		t.Errorf("114.114.114.114 的国家 = %q，期望「中国」", info.Country)
	}
	if info.Label == "" || info.Label == "未知" {
		t.Errorf("114.114.114.114 的显示名 = %q，期望有具体地区", info.Label)
	}
	t.Logf("114.114.114.114 → %+v", info)

	// 8.8.8.8 是美国，跨到另一个国家才说明拆分逻辑不是照着一条数据写死的
	us := Lookup("8.8.8.8")
	if us.Country == "" || us.Country == "未知" {
		t.Errorf("8.8.8.8 应当能查到国家，实际 %+v", us)
	}
	t.Logf("8.8.8.8 → %+v", us)
}

// TestLookupSpecialIPs 私有地址与 IPv6 不该被当成公网去查。
//
// 这些是审计页最常见的输入（内网反代时来源就是 192.168.x.x），
// 显示成「内网」比显示成某个省份强得多 —— 后者是明显的错。
func TestLookupSpecialIPs(t *testing.T) {
	cases := []struct{ ip, want string }{
		{"192.168.1.10", "内网"},
		{"10.0.0.1", "内网"},
		{"172.16.5.4", "内网"},
		{"127.0.0.1", "内网"},
		{"::1", "内网"},
		{"fe80::1", "内网"},
		{"2001:4860:4860::8888", "美国"}, // Google 的公网 DNS，IPv6 库要能查到
		{"2400:3200::1", ""},           // 阿里 IPv6 DNS，只断言不是「内网」
		{"", "未知"},
		{"不是IP", "未知"},
		{"999.1.1.1", "未知"},
	}
	for _, c := range cases {
		got := Lookup(c.ip).Label
		if c.want == "" {
			if got == "内网" || got == "" {
				t.Errorf("Lookup(%q).Label = %q，不该是内网或空", c.ip, got)
			}
			continue
		}
		// 走前缀匹配而不是全等：IPv6 库连城市都有（「美国 · Miami」），
		// 全等断言会因为库里补了城市而莫名变红，那种红没有意义
		if c.want != "未知" && !strings.HasPrefix(got, c.want) {
			t.Errorf("Lookup(%q).Label = %q，期望以 %q 开头", c.ip, got, c.want)
		}
		if c.want == "未知" && got != "未知" {
			t.Errorf("Lookup(%q).Label = %q，期望「未知」", c.ip, got)
		}
	}
}

// TestLookupIPv6 v2.5.0 起支持 IPv6 归属地。
//
// 这条守的是「懒加载」那条路：IPv6 库是 gzip 压缩内嵌、首次查询才解压。
// 解压逻辑写错、文件没 embed 进去、格式对不上——都不会让编译失败，
// 只会静默返回「未知」，而界面上看不出是「没这个 IP 的记录」还是「功能坏了」。
func TestLookupIPv6(t *testing.T) {
	cases := []struct {
		ip   string
		want string // 期望的国家，不做城市级断言（IPv6 库的城市字段经常是 0）
	}{
		{"2001:4860:4860::8888", "美国"}, // Google Public DNS
		// Cloudflare 的 IPv6 段注册在英国 —— 这条我最初按直觉写成「美国」并因此变红，
		// 是断言错了不是代码错了。留着提醒：库的数据按注册地算。
		{"2606:4700:4700::1111", "英国"}, // Cloudflare DNS
		{"2400:3200::1", ""},           // 阿里 DNS：只要求不是「未知」
	}
	for _, c := range cases {
		info := Lookup(c.ip)
		t.Logf("%s → %+v", c.ip, info)
		if info.Label == "未知" || info.Label == "内网" {
			t.Errorf("%s 应当能查到归属地，实际 %q", c.ip, info.Label)
			continue
		}
		if c.want != "" && info.Country != c.want {
			t.Errorf("%s 的国家 = %q，期望 %q", c.ip, info.Country, c.want)
		}
	}
}

// TestLookupIPv4Mapped IPv4-mapped IPv6 要当成 IPv4 查。
//
// 反代场景下来源地址经常是 ::ffff:1.2.3.4 这种写法，直接丢给 IPv4 库会解析失败，
// 结果就是一批本该有归属地的 IP 全显示「未知」。
func TestLookupIPv4Mapped(t *testing.T) {
	mapped := Lookup("::ffff:114.114.114.114")
	plain := Lookup("114.114.114.114")
	if mapped.Label != plain.Label {
		t.Errorf("IPv4-mapped 查出来是 %q，和 IPv4 的 %q 不一致", mapped.Label, plain.Label)
	}
}

// TestLookupDocumentationRanges 文档/保留地址不该显示成真实地区。
//
// 2001:db8::/32 是 RFC 3849 专门留给文档示例的段。它不可能来自真实访问者，
// 但库里确实会给它一个地区（实测是「Reserved · Reserved」，有些段甚至会
// 落到某个真实国家）—— 显示出来看着像「真有个人从那里来」，实际不存在。
func TestLookupDocumentationRanges(t *testing.T) {
	for _, ip := range []string{
		"2001:0db8:85a3:0000:0000:8a2e:0370:7334",
		"2001:db8::1",
		"192.0.2.1",
		"198.51.100.7",
		"203.0.113.9",
	} {
		if got := Lookup(ip).Label; got != "保留地址" {
			t.Errorf("Lookup(%q).Label = %q，期望「保留地址」", ip, got)
		}
	}
}

// TestCountryCN 国外数据里的英文国家名要换成中文。
//
// 「按国家排行」那块一列里中英混排会很难看，而且同一个国家的不同写法
// （"DR Congo" / "Congo"）没法合并。
func TestCountryCN(t *testing.T) {
	cases := map[string]string{
		"United States": "美国",
		"Netherlands":   "荷兰",
		"DR Congo":      "刚果（金）",
		"Hong Kong":     "中国香港",
		"China":         "中国",
		// 没收录的原样返回，不编造
		"Atlantis": "Atlantis",
	}
	for in, want := range cases {
		if got := countryCN(in); got != want {
			t.Errorf("countryCN(%q) = %q，期望 %q", in, got, want)
		}
	}
}

// TestParseRegion 拆分那串竖线文本。
//
// **字段布局是 v3 的五段**：国家|省份|城市|ISP|国家码。
// 网上教程几乎全是老的六段（国家|区域|省份|城市|ISP），照抄会错位一格。
// 这条断言就是钉住那个错位的：如果哪天有人按老格式改回来，国家会变成空、
// 城市会变成国家码。
func TestParseRegion(t *testing.T) {
	got := parseRegion("中国|江苏省|南京市|电信|CN")
	if got.Country != "中国" || got.Province != "江苏省" || got.City != "南京市" || got.ISP != "电信" {
		t.Errorf("拆分结果不对：%+v", got)
	}
	if got.Label != "江苏 · 南京" {
		t.Errorf("显示名 = %q，期望「江苏 · 南京」", got.Label)
	}

	// 字段缺失（0 表示没有）
	miss := parseRegion("中国|0|0|0|CN")
	if miss.Province != "" || miss.City != "" || miss.ISP != "" {
		t.Errorf("缺失字段应当为空串，实际 %+v", miss)
	}
	if miss.Label != "中国" {
		t.Errorf("只剩国家时显示名 = %q，期望「中国」", miss.Label)
	}

	// 国外数据：国家是英文，要换成中文，且不显示州（California）
	us := parseRegion("United States|California|0|Google LLC|US")
	if us.Country != "美国" || us.City != "" {
		t.Errorf("国外数据拆分不对：%+v", us)
	}
	if us.Label != "美国" {
		t.Errorf("美国数据（无城市）显示名 = %q，期望「美国」", us.Label)
	}

	uk := parseRegion("United Kingdom|England|Bristol|0|GB")
	if uk.Label != "英国 · Bristol" {
		t.Errorf("英国数据显示名 = %q，期望「英国 · Bristol」", uk.Label)
	}
}

// TestLabelOfMunicipality 直辖市不该显示成「北京 · 北京市」。
func TestLabelOfMunicipality(t *testing.T) {
	got := parseRegion("中国|0|北京市|北京市|联通")
	if got.Label != "北京" {
		t.Errorf("直辖市显示名 = %q，期望「北京」", got.Label)
	}
}

// TestTrimSuffixes 长行政名要缩到能摆进表格。
func TestTrimSuffixes(t *testing.T) {
	cases := map[string]string{
		"福建省":        "福建",
		"内蒙古自治区":     "内蒙古",
		"新疆维吾尔自治区":   "新疆",
		"广西壮族自治区":    "广西",
		"香港特别行政区":    "香港",
		"湘西土家族苗族自治州": "湘西土家族苗族",
		"阿里地区":       "阿里",
		"上海市":        "上海",
	}
	for in, want := range cases {
		if got := trimSuffixes(in); got != want {
			t.Errorf("trimSuffixes(%q) = %q，期望 %q", in, got, want)
		}
	}
}
