package audit

import (
	"path/filepath"
	"testing"
	"time"
)

// 「可疑」的判据是安全功能的核心：判宽了把自己人封了，判严了扫描器漏过去。
func TestSuspiciousJudgement(t *testing.T) {
	cases := []struct {
		name string
		st   IPStat
		want bool
	}{
		{
			name: "扫描器：大量 404",
			st:   IPStat{Requests: 12847, Errors: 12801},
			want: true,
		},
		{
			name: "正常服务：偶尔 404",
			st:   IPStat{Requests: 5000, Errors: 120},
			want: false, // 占比 2.4%，正常
		},
		{
			name: "样本太少：自己点两下",
			st:   IPStat{Requests: 3, Errors: 3},
			want: false, // 100% 但只有 3 次，不说明问题
		},
		{
			name: "刚好过错误数门槛且占比高",
			st:   IPStat{Requests: 60, Errors: 50},
			want: true, // 50 次错误是门槛，占比 83%
		},
		{
			name: "错误够多但占比低：说明是正常服务被调错",
			st:   IPStat{Requests: 100000, Errors: 2000},
			want: false, // 2%
		},
		{
			name: "没有任何请求",
			st:   IPStat{Requests: 0, Errors: 0},
			want: false,
		},
	}

	for _, c := range cases {
		if got := c.st.Suspicious(); got != c.want {
			t.Errorf("%s：期望 %v，实际 %v（请求 %d 错误 %d）",
				c.name, c.want, got, c.st.Requests, c.st.Errors)
		}
	}
}

// 临时封禁到期后要失效
func TestBlockEntryExpiry(t *testing.T) {
	now := time.Now()

	permanent := BlockEntry{IP: "1.2.3.4"}
	if permanent.Expired(now) {
		t.Fatal("没设到期时间的应视为永久，不该过期")
	}

	past := BlockEntry{IP: "1.2.3.4", ExpiresAt: now.Add(-time.Minute)}
	if !past.Expired(now) {
		t.Fatal("已过期的应判为过期")
	}

	future := BlockEntry{IP: "1.2.3.4", ExpiresAt: now.Add(time.Hour)}
	if future.Expired(now) {
		t.Fatal("还没到期的不该判过期")
	}
}

// 路径去重且封顶：被扫的 IP 能刷出几万条不同路径，
// 不封顶的话 ipstats.json 会涨到几十 MB
func TestAddCappedDedupAndLimit(t *testing.T) {
	var list []string
	// 重复的只记一次
	list = addCapped(list, "/a", 3)
	list = addCapped(list, "/a", 3)
	if len(list) != 1 {
		t.Fatalf("重复项应去重，实际 %v", list)
	}

	// 超过上限不再加
	list = addCapped(list, "/b", 3)
	list = addCapped(list, "/c", 3)
	list = addCapped(list, "/d", 3)
	if len(list) != 3 {
		t.Fatalf("应封顶在 3，实际 %d：%v", len(list), list)
	}
	// 但已记的不能丢
	if list[0] != "/a" {
		t.Fatalf("已记的项不该被挤掉：%v", list)
	}

	// 空字符串不记
	list2 := addCapped(nil, "", 3)
	if len(list2) != 0 {
		t.Fatalf("空字符串不该记，实际 %v", list2)
	}
}

// IP / CIDR 解析：封禁要支持网段，也要拒绝乱七八糟的输入
func TestParseCIDROrIP(t *testing.T) {
	if parseCIDROrIP("192.168.1.10") == nil {
		t.Fatal("单个 IP 应该能解析")
	}
	if parseCIDROrIP("203.0.113.0/24") == nil {
		t.Fatal("CIDR 应该能解析")
	}
	if parseCIDROrIP("2001:db8::1") == nil {
		t.Fatal("IPv6 应该能解析")
	}
	// 这些不该被当成有效目标
	for _, bad := range []string{"", "  ", "example.com", "192.168.1.999", "not-an-ip"} {
		if parseCIDROrIP(bad) != nil {
			t.Errorf("%q 不该被解析成有效网段", bad)
		}
	}
}

// 网段匹配
func TestMatchAny(t *testing.T) {
	nets := parseNets([]string{"203.0.113.0/24", "192.168.1.10"})

	if !matchAny(nets, "203.0.113.99") {
		t.Fatal("网段内的地址应匹配")
	}
	if !matchAny(nets, "192.168.1.10") {
		t.Fatal("单个 IP 应精确匹配")
	}
	if matchAny(nets, "192.168.1.11") {
		t.Fatal("不在名单里的不该匹配")
	}
	if matchAny(nets, "not-an-ip") {
		t.Fatal("解析不了的输入不该匹配")
	}
}

// 手动封禁在前、自动在后 —— 自动的可能误伤，排后面便于复查
func TestSortBlocksManualFirst(t *testing.T) {
	now := time.Now()
	list := []BlockEntry{
		{IP: "auto-new", Auto: true, AddedAt: now},
		{IP: "manual-old", Auto: false, AddedAt: now.Add(-time.Hour)},
		{IP: "manual-new", Auto: false, AddedAt: now},
	}
	sortBlocks(list)

	if list[0].IP != "manual-new" || list[1].IP != "manual-old" {
		t.Fatalf("手动封禁应排前面且新的在前，实际 %v", []string{list[0].IP, list[1].IP, list[2].IP})
	}
	if list[2].IP != "auto-new" {
		t.Fatalf("自动封禁应排最后，实际 %v", list[2].IP)
	}
}

// ===================== 传输途中上报（来源流量） =====================

// TestAddTrafficProgressOnlyAddsBytes
// 途中上报只加字节，**不记请求数、不刷新 LastSeen**。
//
// 一次下载是一个请求。若按上报次数计请求，传一个大文件会把这个 IP 的
// 请求数放大几十倍，列表上"请求数"就没法看了。
func TestAddTrafficProgressOnlyAddsBytes(t *testing.T) {
	dir := t.TempDir()
	oldA, oldI := auditDir, ipStatsPath
	auditDir = dir
	ipStatsPath = filepath.Join(dir, "ipstats.json")
	t.Cleanup(func() {
		auditDir, ipStatsPath = oldA, oldI
		Runtime.ipStats = map[string]*IPStat{}
		dayStats = map[string]*DayStat{}
		dayCache = map[string]*dayCacheEntry{}
	})
	Runtime.ipStats = map[string]*IPStat{}
	setEnabledForTest(t, true)

	t0 := time.Now().Add(-time.Minute)
	AddTrafficProgress("203.0.113.5", 0, 1000)
	AddTrafficProgress("203.0.113.5", 0, 2000)

	st, ok := GetIPStat("203.0.113.5")
	if !ok {
		t.Fatal("途中上报应当建出 IPStat")
	}
	if st.Requests != 0 {
		t.Errorf("途中上报不该记请求数，实际 %d", st.Requests)
	}
	if st.BytesOut != 3000 {
		t.Errorf("字节应当累加到 3000，实际 %d", st.BytesOut)
	}
	if !st.LastSeen.Before(t0.Add(time.Second)) && st.LastSeen.After(time.Now()) {
		t.Errorf("LastSeen 不该被途中上报改成未来时间：%v", st.LastSeen)
	}
}

// TestRecordAccessProgressNoDoubleCount
// 途中分段 + 结算 = 来源 IP 的流量不重不漏。
//
// 这是这次修复最容易出错的地方：结算时若直接加完整字节，
// 来源 IP 的流量就会翻一倍。
func TestRecordAccessProgressNoDoubleCount(t *testing.T) {
	dir := t.TempDir()
	oldA, oldI := auditDir, ipStatsPath
	auditDir = dir
	ipStatsPath = filepath.Join(dir, "ipstats.json")
	t.Cleanup(func() {
		auditDir, ipStatsPath = oldA, oldI
		Runtime.ipStats = map[string]*IPStat{}
		Runtime.accum = nil
		dayStats = map[string]*DayStat{}
		dayCache = map[string]*dayCacheEntry{}
	})
	Runtime.ipStats = map[string]*IPStat{}
	Runtime.accum = nil
	setEnabledForTest(t, true)

	// 总共 5000 字节：途中报了 1000 + 2000，剩 2000
	AddTrafficProgress("198.51.100.9", 0, 1000)
	AddTrafficProgress("198.51.100.9", 0, 2000)
	RecordAccessProgress(Record{
		Time:     time.Now(),
		IP:       "198.51.100.9",
		BytesOut: 5000,
	}, 0, 3000)

	st, ok := GetIPStat("198.51.100.9")
	if !ok {
		t.Fatal("应当有 IPStat")
	}
	if st.BytesOut != 5000 {
		t.Errorf("来源流量应当正好 5000（不重不漏），实际 %d", st.BytesOut)
	}
	if st.Requests != 1 {
		t.Errorf("一次下载只能算 1 个请求，实际 %d", st.Requests)
	}
	// 明细里必须是完整值：一条记录 = 一次完整请求
	if len(Runtime.accum) != 1 || Runtime.accum[0].BytesOut != 5000 {
		t.Errorf("明细记录必须是完整字节 5000，实际 %v", Runtime.accum)
	}
}

// setEnabledForTest 打开/关闭审计总开关。
//
// 直接改 Runtime.Config 而不是走 Runtime.Update：后者会顺带落盘配置文件，
// 测试不需要那个副作用，也不该往临时目录外写东西。
func setEnabledForTest(t *testing.T, on bool) {
	t.Helper()
	Runtime.mu.Lock()
	Runtime.Config.Enabled = on
	Runtime.mu.Unlock()
}
