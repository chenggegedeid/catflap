package audit

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"
)

// 这几条守的是「审计页打开慢」的修复，重点是**改了实现之后结果还得对**。
//
// TopSites 原来逐条扫明细，现在改读按小时聚合里的那份 Sites 计数。
// 换数据源最容易出的事故是：快了，但数不对。所以这里不测快慢（快慢在
// CI 上不稳定），只测**两种算法给的排名和计数必须一致**。

// seedDay 在临时目录里铺一天的记录，同时累加进按小时聚合。
//
// 必须两个都做：AddToDayStat 是实时路径（请求进来时调），
// ensureDayStat 没找到聚合时会从明细重算。这里直接调 AddToDayStat
// 模拟运行中的实例，也顺便验证了「明细 + 聚合」两边是一致的。
func seedDay(t *testing.T, day string, recs []Record) {
	t.Helper()
	dir := t.TempDir()
	oldAudit, oldIPStats := auditDir, ipStatsPath
	auditDir = dir
	ipStatsPath = filepath.Join(dir, "ipstats.json")
	t.Cleanup(func() {
		auditDir, ipStatsPath = oldAudit, oldIPStats
		dayStats = map[string]*DayStat{}
		dayCache = map[string]*dayCacheEntry{}
	})

	name := filepath.Join(dir, day+".jsonl")
	f, err := os.Create(name)
	if err != nil {
		t.Fatalf("建明细文件失败：%v", err)
	}
	defer f.Close()
	for _, r := range recs {
		line, _ := json.Marshal(r)
		f.Write(append(line, '\n'))
	}
	f.Close() // 先写完再累加，确保 mtime 稳定
	for _, r := range recs {
		AddToDayStat(r)
	}
}

// TestTopSitesCountsMatchRecords
// TopSites 改读聚合后，计数必须与明细里实际的站点请求数一致。
//
// 这是这次改动唯一的正确性风险：换了数据源，万一聚合里落的是别的口径
// （比如只算错误请求），界面上「被访问最多的站点」就会悄悄排错，
// 而页面看起来完全正常。
func TestTopSitesCountsMatchRecords(t *testing.T) {
	day := time.Now().Format("2006-01-02")
	base := time.Now().Add(-2 * time.Hour)

	// 三个站点，数量差得开，排序才有意义
	want := map[string]int{"a.example.com": 30, "b.example.com": 15, "c.example.com": 5}
	var recs []Record
	for site, n := range want {
		for i := 0; i < n; i++ {
			recs = append(recs, Record{
				Time: base.Add(time.Duration(i) * time.Second),
				IP:   "203.0.113.7",
				Host: site,
			})
		}
	}
	seedDay(t, day, recs)

	got := TopSites(24, 10)
	if len(got) != len(want) {
		t.Fatalf("期望 %d 个站点，实际 %d（%v）", len(want), len(got), got)
	}
	// 必须按请求数从多到少
	if got[0].Site != "a.example.com" || got[1].Site != "b.example.com" || got[2].Site != "c.example.com" {
		t.Errorf("排序不对，期望 a > b > c，实际 %v", got)
	}
	for _, g := range got {
		if g.Count != int64(want[g.Site]) {
			t.Errorf("站点 %s：期望 %d 次，实际 %d —— 聚合口径与明细对不上",
				g.Site, want[g.Site], g.Count)
		}
	}
}

// TestTopSitesFallsBackToRef 穿透记录没有 host，站点维度要落到 ref。
//
// 穿透（kind=tunnel）的请求整个没有 Host 字段，只有 Ref（形如 s1-2）。
// 聚合里如果只记 Host，穿透服务就会全被归到 "-" 一个桶里，
// 「被访问最多的站点」里根本看不到穿透服务。
func TestTopSitesFallsBackToRef(t *testing.T) {
	day := time.Now().Format("2006-01-02")
	recs := []Record{
		{Time: time.Now().Add(-time.Hour), IP: "198.51.100.3", Ref: "s1-2"},
		{Time: time.Now().Add(-time.Hour), IP: "198.51.100.3", Ref: "s1-2"},
		{Time: time.Now().Add(-time.Hour), IP: "198.51.100.3"}, // 连 ref 都没有
	}
	seedDay(t, day, recs)

	got := TopSites(24, 10)
	bySite := map[string]int64{}
	for _, g := range got {
		bySite[g.Site] = g.Count
	}
	if bySite["s1-2"] != 2 {
		t.Errorf("穿透服务的 ref 应当被算成站点，期望 s1-2 = 2，实际 %d（全部：%v）",
			bySite["s1-2"], bySite)
	}
	if bySite["-"] != 1 {
		t.Errorf("既无 host 也无 ref 的应归到 \"-\"，期望 1，实际 %d（全部：%v）",
			bySite["-"], bySite)
	}
}

// TestBuildOverviewCountsUniqueIPsOnce 唯一 IP 必须跨天去重。
//
// 唯一 IP 是用 map 去重的，而最近 24 小时可能跨两个文件。
// 如果哪天改成「每天各算一个集合再相加」，同一个 IP 会被数两次。
func TestBuildOverviewCountsUniqueIPsOnce(t *testing.T) {
	now := time.Now()
	today := now.Format("2006-01-02")
	yesterday := now.AddDate(0, 0, -1).Format("2006-01-02")

	// 同一个 IP 在两天都出现。
	//
	// 三个时间点都落在 24 小时窗口内（所以 Requests 应为 3），
	// 但第一个要跨到前一天 —— 那是唯一能触发「读两个文件」的情况，
	// 也是"唯一 IP 被数成两个"这个 bug 会现形的地方。
	//
	// 取 22 小时而不是 20：当前时刻刚过 20 点时，20 小时前仍在同一天，
	// 用例会退化成单文件，等于什么都没测。
	recs := []Record{
		{Time: now.Add(-22 * time.Hour), IP: "203.0.113.9"},
		{Time: now.Add(-2 * time.Hour), IP: "203.0.113.9"},
		{Time: now.Add(-time.Hour), IP: "203.0.113.9"},
	}

	dir := t.TempDir()
	oldAudit, oldIPStats := auditDir, ipStatsPath
	auditDir = dir
	ipStatsPath = filepath.Join(dir, "ipstats.json")
	t.Cleanup(func() {
		auditDir, ipStatsPath = oldAudit, oldIPStats
		dayStats = map[string]*DayStat{}
		dayCache = map[string]*dayCacheEntry{}
	})

	for _, day := range []string{today, yesterday} {
		f, err := os.Create(filepath.Join(dir, day+".jsonl"))
		if err != nil {
			t.Fatalf("建明细失败：%v", err)
		}
		_ = f.Close()
	}
	// 按每条记录**自己的日期**决定写进哪个文件 —— 这才是真实落盘的样子，
	// 也让这个用例在一天里的任何时刻跑都能正确构造出跨天场景
	write := func(day string, r Record) {
		f, err := os.OpenFile(filepath.Join(dir, day+".jsonl"), os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o644)
		if err != nil {
			t.Fatalf("写明细失败：%v", err)
		}
		defer f.Close()
		line, _ := json.Marshal(r)
		f.Write(append(line, '\n'))
	}
	for _, r := range recs {
		write(r.Time.Format("2006-01-02"), r)
	}

	ov := BuildOverview(24)
	if ov.UniqueIPs != 1 {
		t.Errorf("同一个 IP 跨两天只应算 1 个独立 IP，实际 %d（请求数 %d）",
			ov.UniqueIPs, ov.Requests)
	}
	if ov.Requests != 3 {
		t.Errorf("最近 24 小时应有 3 次请求（昨天那条已过期），实际 %d", ov.Requests)
	}
}

// TestDayCacheReflectsNewWrites 缓存不能把新写进去的记录挡在外面。
//
// 加了缓存之后最容易出的毛病是"刚被扫完去查审计，看到的还是旧的"。
// flush 写完会调 invalidateDayCache，这里验证那条路径真的通了。
func TestDayCacheReflectsNewWrites(t *testing.T) {
	day := time.Now().Format("2006-01-02")
	dir := t.TempDir()
	oldAudit, oldIPStats := auditDir, ipStatsPath
	auditDir = dir
	ipStatsPath = filepath.Join(dir, "ipstats.json")
	t.Cleanup(func() {
		auditDir, ipStatsPath = oldAudit, oldIPStats
		dayStats = map[string]*DayStat{}
		dayCache = map[string]*dayCacheEntry{}
	})

	name := filepath.Join(dir, day+".jsonl")
	appendRec := func(ip string) {
		f, err := os.OpenFile(name, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o644)
		if err != nil {
			t.Fatalf("写明细失败：%v", err)
		}
		defer f.Close()
		line, _ := json.Marshal(Record{Time: time.Now(), IP: ip})
		f.Write(append(line, '\n'))
		f.Close()
		invalidateDayCache(day) // flushRecords 里的同一动作
	}

	appendRec("203.0.113.20")
	if n := len(mustReadDay(t, day)); n != 1 {
		t.Fatalf("第一次读应得 1 条，实际 %d", n)
	}
	appendRec("203.0.113.21")
	if n := len(mustReadDay(t, day)); n != 2 {
		t.Errorf("新写入后应立刻读到 2 条，实际 %d —— 缓存把新记录挡住了", n)
	}
}

func mustReadDay(t *testing.T, day string) []Record {
	t.Helper()
	recs, err := ReadDay(day, "", 0)
	if err != nil {
		t.Fatalf("读 %s 失败：%v", day, err)
	}
	return recs
}
