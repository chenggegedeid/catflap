// Package geo 把来源 IP 换成「国家·省·市」。
//
// # 为什么内嵌数据库
//
// 归属地是审计页的辅助信息，不该为它引入任何运维负担：
//   - **不联网**：走第三方在线 API 等于把用户的每一个访问者 IP 发给别人，
//     而且审计页一刷新就是几百次查询，先被限流的就是自己。
//   - **不落文件**：运行目录里多一个 11MB 的文件，就得回答「被删了怎么办」
//     「升级时怎么替换」「群晖权限怎么配」。内嵌进二进制，这些全都不存在。
//
// 代价是二进制大了 11MB（xdb 的 IPv4 库）。用 ip2region 而不是 MaxMind
// GeoLite2 就是为这个：后者要 120MB，且需要注册账号拿 license key。
//
// # 为什么不支持 IPv6
//
// v2.5.0 之前确实不支持：IPv6 的库是 37MB，直接内嵌会让二进制从 26MB 涨到
// 63MB，为了一列辅助信息太重了。
//
// 现在支持了，做法是**存压缩版**（gzip 后 7.5MB）：
//   - 编译进二进制的是 .gz，不是解析后的表；
//   - 首次用到 IPv6 归属地时才解压到内存（几个 GB 的机器也就一秒左右，之后常驻）；
//   - IPv4 仍是未压缩直接映射，最常见的那种查询一秒都不耽搁。
//
// 代价从「+37MB 二进制」降到「+7.2MB 二进制」和「+37MB 内存（首次查 IPv6 后）」——
// 后者只在真的有用 IPv6 来访问时才会发生。
//
// 许可证：ip2region 是 Apache-2.0 OR MIT 双许可，数据文件随二进制分发没有问题。
package geo

import (
	"bytes"
	"compress/gzip"
	_ "embed"
	"io"
	"net/netip"
	"strings"
	"sync"

	"github.com/lionsoul2014/ip2region/binding/golang/xdb"
	"github.com/sirupsen/logrus"
)

// xdb 数据文件。随二进制一起分发，见包注释。
//
//go:embed ip2region_v4.xdb
var xdbV4 []byte

// IPv6 的库太大，存 gzip。见包注释里那段说明。
//
//go:embed ip2region_v6.xdb.gz
var xdbV6Gz []byte

// Info 一个 IP 的归属信息。
//
// 三个字段分开而不是拼成一个字符串：界面上要做「按国家排行」这种聚合，
// 拼成一句"中国|0|福建省|福州市|电信"之后还得再拆回来，拆错了没人发现。
type Info struct {
	// Country 国家，例如「中国」。查不到为空
	Country string `json:"country"`
	// Province 省，例如「福建省」。直辖市是「北京市」这种
	Province string `json:"province"`
	// City 市，例如「福州市」
	City string `json:"city"`
	// ISP 运营商，例如「电信」。ip2region 不保证每个人都有
	ISP string `json:"isp"`
	// Label 一句话显示用，例如「福建 · 福州」。
	// 让别人自己拼会拼出「福建省福州市」这种比行宽还长的东西（日本是「日本 · 东京都」）。
	Label string `json:"label"`
}

var (
	// v4 的库 11MB，启动即加载（最常见的查询，不该有延迟）
	v4Searcher *xdb.Searcher
	v4Once     sync.Once
	v4Err      error

	// v6 的库解压后 37MB，**首次查到 IPv6 才加载**。
	// 大多数家用环境里 IPv6 访问是零，为了它每次启动都解压 37MB 不值当。
	v6Mu       sync.Mutex
	v6Searcher *xdb.Searcher
	v6Loaded   bool
	v6Err      error

	// cache 查询结果缓存。
	//
	// 审计页每次刷新都会查几百个 IP，而扫描器喜欢用固定几个 IP 猛打 ——
	// 不缓存的话同一批 IP 每刷新一次都要重新走一遍二分查找。
	// 用简单的 map + 容量上限而不是引第三方 LRU：这里只有几百个键，
	// 满了就整个清掉，下一次重建的成本比维护链表低。
	cacheMu  sync.RWMutex
	cache    = map[string]Info{}
	cacheMax = 4096
)

// initV4 首次使用时加载 IPv4 库。
//
// 懒加载而不是 init()：包级 init 的时机不可控，而这里要打日志、要处理
// 「数据坏了」的情况 —— 那种事应该发生在一个明确的位置。
func initV4() {
	v4Once.Do(func() {
		// NewWithBuffer 让整个库常驻内存，查询即一次二分查找。
		// 没有向量索引也能用，只是少一层加速；对审计页这点量级够了，
		// 也就不额外造一个索引文件。
		v4Searcher, v4Err = xdb.NewWithBuffer(xdb.IPv4, xdbV4)
		if v4Err != nil {
			logrus.Warnf("[audit] IPv4 归属地库加载失败：%v", v4Err)
		}
	})
}

// loadV6 解压并加载 IPv6 库。只在第一次查 IPv6 时调用。
func loadV6() *xdb.Searcher {
	v6Mu.Lock()
	defer v6Mu.Unlock()
	if v6Loaded {
		return v6Searcher
	}
	v6Loaded = true

	zr, err := gzip.NewReader(bytes.NewReader(xdbV6Gz))
	if err != nil {
		v6Err = err
		logrus.Warnf("[audit] IPv6 归属地库解压失败：%v", err)
		return nil
	}
	defer zr.Close()

	raw, err := io.ReadAll(zr)
	if err != nil {
		v6Err = err
		logrus.Warnf("[audit] IPv6 归属地库读取失败：%v", err)
		return nil
	}

	s, err := xdb.NewWithBuffer(xdb.IPv6, raw)
	if err != nil {
		v6Err = err
		logrus.Warnf("[audit] IPv6 归属地库解析失败：%v", err)
		return nil
	}
	v6Searcher = s
	logrus.Infof("[audit] IPv6 归属地库已加载（%d MB）", len(raw)/(1<<20))
	return s
}

// Enabled 归属地功能是否可用（IPv4 库加载成功）。
//
// 界面据此决定要不要显示「归属地」这一列 —— 库坏了就别摆一列空白。
// 只判 IPv4 库：它是启动时就加载的，IPv6 是懒加载，拿它判断会把「还没人
// 用 IPv6 访问过」误判成「功能不可用」。
func Enabled() bool {
	initV4()
	return v4Err == nil
}

// Lookup 查一个 IP 的归属。
//
// 永远返回一个可显示的东西，不返回 error：
//   - 私有地址 / 环回 → 「内网」（这是最该说清楚的一种，比「未知」有用得多）
//   - IPv6 → 查 IPv6 库；库坏了或查不到才是「未知」
//   - 解析不了的 → 「未知」
//
// 让调用方为「查归属失败」写分支是不划算的：审计页上这就是一列文字，
// 查不到不影响任何其他功能。
func Lookup(ip string) Info {
	if ip == "" {
		return Info{Label: "未知"}
	}

	addr, err := netip.ParseAddr(strings.TrimSpace(ip))
	if err != nil {
		return Info{Label: "未知"}
	}
	// IPv4-mapped IPv6（::ffff:1.2.3.4）当成 IPv4 查，这种写法在反代头里很常见
	addr = addr.Unmap()

	if addr.IsLoopback() || addr.IsPrivate() || addr.IsLinkLocalUnicast() ||
		addr.IsLinkLocalMulticast() || addr.IsUnspecified() ||
		addr.IsMulticast() {
		return Info{Label: "内网"}
	}

	// 文档与保留地址段（2001:db8::/32 是 RFC 3849 的文档示例段，
	// 192.0.2.0/24、198.51.100.0/24、203.0.113.0/24 是 RFC 5737 的）。
	//
	// 不拦的话它们会被查成「Reserved · Reserved」或者别的国家的地址 ——
	// 前者是英文噪音，后者更糟：看着像真有个人从那里来访问，实际不存在。
	if isDocumentation(addr) {
		return Info{Label: "保留地址"}
	}

	key := addr.String()
	cacheMu.RLock()
	if v, ok := cache[key]; ok {
		cacheMu.RUnlock()
		return v
	}
	cacheMu.RUnlock()

	info := query(addr)

	cacheMu.Lock()
	if len(cache) >= cacheMax {
		// 满了整体清空：这里不是热点数据，没必要维护淘汰顺序
		cache = map[string]Info{}
	}
	cache[key] = info
	cacheMu.Unlock()

	return info
}

// query 真正去库里查。
func query(addr netip.Addr) Info {
	var (
		raw string
		err error
	)
	if addr.Is4() {
		if !Enabled() {
			return Info{Label: "未知"}
		}
		raw, err = v4Searcher.Search(addr.String())
	} else {
		s := loadV6()
		if s == nil {
			// 解压/解析失败已经打过日志了，这里不再刷屏
			return Info{Label: "未知"}
		}
		raw, err = s.Search(addr.String())
	}
	if err != nil {
		return Info{Label: "未知"}
	}
	return parseRegion(raw)
}

// isDocumentation 是不是文档/保留地址段。
//
// 这些段里的地址**不可能**来自真实访问者（RFC 5737 / RFC 3849 专门留出来写
// 文档用的），所以被记进审计多半是测试数据或者伪造的 X-Forwarded-For。
// 显示成真实归属地会误导用户。
func isDocumentation(addr netip.Addr) bool {
	for _, p := range docPrefixes {
		if p.Contains(addr) {
			return true
		}
	}
	return false
}

var docPrefixes = func() []netip.Prefix {
	raw := []string{
		"192.0.2.0/24", "198.51.100.0/24", "203.0.113.0/24", // RFC 5737
		"2001:db8::/32", // RFC 3849
		"100::/64",      // RFC 6666 丢弃前缀
		"2002::/16",     // 6to4 中继（不是真实来源）
	}
	out := make([]netip.Prefix, 0, len(raw))
	for _, s := range raw {
		if p, err := netip.ParsePrefix(s); err == nil {
			out = append(out, p)
		}
	}
	return out
}()

// parseRegion 把 ip2region 那串竖线分隔的文本拆成结构。
//
// # 字段布局是 v3 的，和网上多数教程不一样
//
// 老版本（v1/v2）是六段：国家|区域|省份|城市|ISP，国内数据的第二段固定是 "0"。
// v3（本仓库 embed 的这份）变成五段：
//
//	国家|省份|城市|ISP|国家码
//
// 网上能搜到的示例几乎全是老格式，照着写会把国家码当成城市 —— 表现是
// 英国 IP 显示成「布里斯托 · GB」这种一眼假的东西，而且不报错。
//
// 实测样本（v3.18.0）：
//
//	中国|江苏省|南京市|0|CN
//	United States|California|0|Google LLC|US
//	Netherlands|South Holland|0|0|NL
func parseRegion(raw string) Info {
	parts := strings.Split(raw, "|")
	field := func(i int) string {
		if i < 0 || i >= len(parts) {
			return ""
		}
		v := strings.TrimSpace(parts[i])
		if v == "0" {
			return ""
		}
		return v
	}

	info := Info{
		Country:  countryCN(field(0)),
		Province: field(1),
		City:     field(2),
		ISP:      field(3),
	}
	info.Label = labelOf(info)
	return info
}

// countryCN 把英文国家名换成中文。
//
// 库里国内数据用中文、国外数据用英文（「中国」对「United States」），
// 「按国家排行」那块要是照原样显示，一列里中英混排会很难看，
// 也没法把同一个国家的不同写法合并到一起。
//
// 常见的那几十个手写就够了，不引一份几百项的国家表：这份数据里真正会出现的
// 就是这些 —— 攻击流量的来源相对集中。查不到就原样显示，不编造。
var countryCN = func() func(string) string {
	m := map[string]string{
		"China": "中国", "Hong Kong": "中国香港", "Macao": "中国澳门", "Taiwan": "中国台湾",
		"United States": "美国", "Japan": "日本", "South Korea": "韩国", "Korea": "韩国",
		"Singapore": "新加坡", "Netherlands": "荷兰", "Germany": "德国", "France": "法国",
		"United Kingdom": "英国", "Russia": "俄罗斯", "India": "印度", "Brazil": "巴西",
		"Canada": "加拿大", "Australia": "澳大利亚", "Vietnam": "越南", "Thailand": "泰国",
		"Indonesia": "印度尼西亚", "Malaysia": "马来西亚", "Philippines": "菲律宾",
		"Italy": "意大利", "Spain": "西班牙", "Sweden": "瑞典", "Switzerland": "瑞士",
		"Poland": "波兰", "Ukraine": "乌克兰", "Turkey": "土耳其", "Türkiye": "土耳其",
		"Iran": "伊朗", "Israel": "以色列", "United Arab Emirates": "阿联酋",
		"Saudi Arabia": "沙特阿拉伯", "Pakistan": "巴基斯坦", "Bangladesh": "孟加拉国",
		"South Africa": "南非", "Egypt": "埃及", "Nigeria": "尼日利亚",
		"Mexico": "墨西哥", "Argentina": "阿根廷", "Chile": "智利", "Colombia": "哥伦比亚",
		"Romania": "罗马尼亚", "Bulgaria": "保加利亚", "Czechia": "捷克", "Czech Republic": "捷克",
		"Austria": "奥地利", "Belgium": "比利时", "Denmark": "丹麦", "Finland": "芬兰",
		"Norway": "挪威", "Ireland": "爱尔兰", "Portugal": "葡萄牙", "Greece": "希腊",
		"Hungary": "匈牙利", "Lithuania": "立陶宛", "Latvia": "拉脱维亚", "Estonia": "爱沙尼亚",
		"New Zealand": "新西兰", "Kazakhstan": "哈萨克斯坦", "Mongolia": "蒙古",
		// 攻击流量常见的来源，一直补到「查不到的才原样显示」
		"DR Congo": "刚果（金）", "Congo": "刚果（布）", "Angola": "安哥拉",
		"Kenya": "肯尼亚", "Morocco": "摩洛哥", "Tunisia": "突尼斯", "Algeria": "阿尔及利亚",
		"Ethiopia": "埃塞俄比亚", "Ghana": "加纳", "Tanzania": "坦桑尼亚",
		"Peru": "秘鲁", "Venezuela": "委内瑞拉", "Ecuador": "厄瓜多尔", "Bolivia": "玻利维亚",
		"Paraguay": "巴拉圭", "Uruguay": "乌拉圭", "Costa Rica": "哥斯达黎加",
		"Panama": "巴拿马", "Guatemala": "危地马拉", "Dominican Republic": "多米尼加",
		"Croatia": "克罗地亚", "Serbia": "塞尔维亚", "Slovenia": "斯洛文尼亚",
		"Slovakia": "斯洛伐克", "Bosnia and Herzegovina": "波黑", "Albania": "阿尔巴尼亚",
		"Macedonia": "北马其顿", "North Macedonia": "北马其顿", "Moldova": "摩尔多瓦",
		"Belarus": "白俄罗斯", "Georgia": "格鲁吉亚", "Armenia": "亚美尼亚",
		"Azerbaijan": "阿塞拜疆", "Uzbekistan": "乌兹别克斯坦", "Kyrgyzstan": "吉尔吉斯斯坦",
		"Tajikistan": "塔吉克斯坦", "Turkmenistan": "土库曼斯坦", "Afghanistan": "阿富汗",
		"Iraq": "伊拉克", "Syria": "叙利亚", "Lebanon": "黎巴嫩", "Jordan": "约旦",
		"Kuwait": "科威特", "Qatar": "卡塔尔", "Oman": "阿曼", "Bahrain": "巴林",
		"Yemen": "也门", "Sri Lanka": "斯里兰卡", "Nepal": "尼泊尔", "Myanmar": "缅甸",
		"Cambodia": "柬埔寨", "Laos": "老挝", "Brunei": "文莱",
		"Taiwan, China": "中国台湾", "Hong Kong, China": "中国香港", "Macao, China": "中国澳门",
	}
	return func(s string) string {
		if cn, ok := m[s]; ok {
			return cn
		}
		return s
	}
}()

// labelOf 拼那个给界面看的一句话。
//
// 国内和国外分开处理，因为两边的数据粒度不一样：
//   - 国内：省和市都有，显示「江苏 · 南京」
//   - 国外：「California」「South Holland」这种是州/省，中文读者没有概念，
//     显示出来只是噪音。所以国外只给「国家 · 城市」，没城市就只给国家。
//
// 统一成一套规则的结果是英国 IP 显示成「布里斯托 · England」，不如「英国 · 布里斯托」。
func labelOf(i Info) string {
	if i.Country == "" && i.Province == "" && i.City == "" {
		return "未知"
	}

	domestic := i.Country == "中国" || i.Country == "中国香港" ||
		i.Country == "中国澳门" || i.Country == "中国台湾"

	if domestic {
		prov, city := i.Province, i.City
		// 直辖市：省和市是同一个名字，只留一个
		if prov != "" && city != "" && trimSuffixes(city) == trimSuffixes(prov) {
			return trimSuffixes(city)
		}
		switch {
		case prov != "" && city != "":
			return trimSuffixes(prov) + " · " + trimSuffixes(city)
		case prov != "":
			return trimSuffixes(prov)
		case city != "":
			return trimSuffixes(city)
		}
		return i.Country
	}

	// 国外：国家优先，能到城市就带上城市
	if i.Country == "" {
		return trimSuffixes(i.City)
	}
	if i.City != "" {
		return i.Country + " · " + i.City
	}
	return i.Country
}

// trimSuffixes 去掉行政区划后缀，让「福建省」变成「福建」。
//
// 「内蒙古自治区」「新疆维吾尔自治区」这类长名一并处理：
// 表格里一列就 6 个字宽，留全称会把后面的数字全挤走。
func trimSuffixes(s string) string {
	for _, suf := range []string{"维吾尔自治区", "壮族自治区", "回族自治区", "特别行政区", "自治区", "自治州", "地区", "省", "市"} {
		if strings.HasSuffix(s, suf) && len(s) > len(suf) {
			return strings.TrimSuffix(s, suf)
		}
	}
	return s
}
