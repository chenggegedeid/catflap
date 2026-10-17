package audit

import (
	"encoding/json"
	"os"
	"path/filepath"
	"sort"
	"time"

	"github.com/chenggegedeid/catflap/utils/utilsFile"

	"github.com/sirupsen/logrus"
)

// 按小时/按天的聚合。
//
// # 为什么要单独存一份
//
// 明细（data/access/<日期>.jsonl）能读到想要的任何数字，但它按天切、一天可能
// 几十 MB —— 审计页一打开就要「最近 24 小时的趋势 + 指标卡」，每次都去扫一遍
// 文件，等于每次刷新都读几十 MB 磁盘。
//
// 这份聚合是**派生数据**：几百字节一天，随手就算出来了。
// 它丢失（被删、格式变了）不影响正确性，读的时候发现缺哪天就补算哪天。
//
// # 为什么不直接用 ipstats.json
//
// ipstats.json 是「按 IP 的累计值」，只活在内存里、重启就从头攒，回答不了
// 「昨天下午三点有多少请求」。两者维度不同，不能合并。

// DayStat 某一天的聚合。时间桶用小时（0–23）。
type DayStat struct {
	Day string `json:"day"`
	// Hours 每小时一个桶，索引即小时
	Hours [24]HourStat `json:"hours"`
	// Sites 这一天各站点/服务的请求数（按 host 或穿透 ref）
	//
	// 只有整天粒度：站点分布是「谁被访问得多」，不需要按小时看，
	// 而按小时再存一份会让这个文件大好几倍。
	Sites map[string]int64 `json:"sites"`
	// UpdatedAt 最后一次累加的时间，用来判断这份缓存是不是还新鲜
	UpdatedAt time.Time `json:"updatedAt"`
}

// HourStat 一个小时内的数字。
type HourStat struct {
	Requests int64 `json:"requests"`
	Errors   int64 `json:"errors"`
	Blocked  int64 `json:"blocked"`
	// DurSum / DurCount 用来算平均耗时。存累加和而不是存平均值 ——
	// 平均值不能相加，两个小时的均值再平均是错的
	DurSum   int64 `json:"durSum"`
	DurCount int64 `json:"durCount"`
	BytesIn  int64 `json:"bytesIn"`
	BytesOut int64 `json:"bytesOut"`
	// Kinds 这一小时里各类攻击的次数。
	//
	// 放在小时桶里而不是整天存一份：趋势和「最近 N 小时攻击类型」都要按小时
	// 截取，整天粒度只能算「今天」，那在早上八点打开面板时几乎什么也看不到。
	Kinds map[AttackKind]int64 `json:"kinds"`
}

const dayStatPath = "data/access/daystats.json"

// dayStatLock 保护下面的内存缓存。
var dayStats = map[string]*DayStat{}

// AddToDayStat 把一条记录累加进当天的聚合。
//
// 在 RecordAccess 里调，和 updateIPStats 同一处：记录进来就同时更新两个维度，
// 分开算迟早会出现「明细有、趋势没有」这种对不上的情况。
func AddToDayStat(r Record) {
	if r.Time.IsZero() {
		r.Time = time.Now()
	}
	day := r.Time.Format("2006-01-02")

	Runtime.mu.Lock()
	st := dayStats[day]
	if st == nil {
		st = newDayStat(day)
		dayStats[day] = st
	}
	h := r.Time.Hour()
	b := &st.Hours[h]
	b.Requests++
	if r.isError() {
		b.Errors++
	}
	if r.Status == 403 || r.Status == 429 {
		b.Blocked++
	}
	if r.DurationMs > 0 {
		b.DurSum += r.DurationMs
		b.DurCount++
	}
	b.BytesIn += r.BytesIn
	b.BytesOut += r.BytesOut

	if b.Kinds == nil {
		b.Kinds = map[AttackKind]int64{}
	}
	b.Kinds[ClassifyAttack(r)]++

	// 站点维度：优先用 host（反代），穿透没有 host 就用 ref
	site := r.Host
	if site == "" {
		site = r.Ref
	}
	if site == "" {
		site = "-"
	}
	if st.Sites == nil {
		st.Sites = map[string]int64{}
	}
	st.Sites[site]++
	st.UpdatedAt = time.Now()
	Runtime.mu.Unlock()
}

func newDayStat(day string) *DayStat {
	st := &DayStat{
		Day:   day,
		Sites: map[string]int64{},
	}
	for i := range st.Hours {
		st.Hours[i].Kinds = map[AttackKind]int64{}
	}
	return st
}

// LoadDayStats 启动时读回来。
//
// 读不到、解析失败都只是「从头攒」，绝不阻止审计功能起来 ——
// 这份数据是派生出来的，丢了可以重算，起不来才是事故。
func LoadDayStats() {
	data, err := os.ReadFile(dayStatPath)
	if err != nil {
		if !os.IsNotExist(err) {
			logrus.Warnf("[audit] 读取按天统计失败，将从明细重算：%v", err)
		}
		return
	}
	var loaded map[string]*DayStat
	if err := json.Unmarshal(data, &loaded); err != nil {
		logrus.Warnf("[audit] 按天统计解析失败，将从明细重算：%v", err)
		return
	}

	Runtime.mu.Lock()
	defer Runtime.mu.Unlock()
	for day, st := range loaded {
		if st == nil {
			continue
		}
		// 老版本写的缓存里没有 Kinds（字段是后加的），读回来要补空表 ——
		// 不补的话下一条记录进来会往 nil map 里写，直接 panic
		for i := range st.Hours {
			if st.Hours[i].Kinds == nil {
				st.Hours[i].Kinds = map[AttackKind]int64{}
			}
		}
		if st.Sites == nil {
			st.Sites = map[string]int64{}
		}
		dayStats[day] = st
	}
}

// SaveDayStats 落盘。由每分钟的 flush 一起调。
func SaveDayStats() {
	Runtime.mu.RLock()
	snapshot := make(map[string]*DayStat, len(dayStats))
	for k, v := range dayStats {
		snapshot[k] = v
	}
	Runtime.mu.RUnlock()

	if err := utilsFile.EnsurePrivateDir(filepath.Dir(dayStatPath)); err != nil {
		logrus.Warnf("[audit] 创建统计目录失败：%v", err)
		return
	}
	// 原子写：先临时文件再改名。写到一半被杀时保留的是上一份完整的
	if err := utilsFile.WriteJsonFile(dayStatPath, snapshot); err != nil {
		logrus.Warnf("[audit] 保存按天统计失败：%v", err)
	}
}

// ensureDayStat 保证某天的聚合在内存里：没有就从明细算一遍。
//
// 这条是「派生数据可以随时重建」的落点。缺的情况有三种：新装、
// 缓存文件被删、升级上来的老版本第一次跑（那时候还没有这个缓存文件）。
func ensureDayStat(day string) *DayStat {
	Runtime.mu.RLock()
	st, ok := dayStats[day]
	Runtime.mu.RUnlock()
	if ok {
		return st
	}

	// 从明细重算。只读这一个文件，不碰别的天
	recs, err := ReadDay(day, "", 0)
	if err != nil && !os.IsNotExist(err) {
		logrus.Warnf("[audit] 重算 %s 的统计失败：%v", day, err)
	}

	computed := newDayStat(day)
	for _, r := range recs {
		if r.Time.IsZero() {
			continue
		}
		b := &computed.Hours[r.Time.Hour()]
		b.Requests++
		if r.isError() {
			b.Errors++
		}
		if r.Status == 403 || r.Status == 429 {
			b.Blocked++
		}
		if r.DurationMs > 0 {
			b.DurSum += r.DurationMs
			b.DurCount++
		}
		b.BytesIn += r.BytesIn
		b.BytesOut += r.BytesOut
		b.Kinds[ClassifyAttack(r)]++
		site := r.Host
		if site == "" {
			site = r.Ref
		}
		if site == "" {
			site = "-"
		}
		computed.Sites[site]++
	}

	Runtime.mu.Lock()
	// 再查一次：重算期间可能有新记录进来，别把它刚累加的那份覆盖掉
	if cur, ok := dayStats[day]; ok {
		Runtime.mu.Unlock()
		return cur
	}
	dayStats[day] = computed
	Runtime.mu.Unlock()
	return computed
}

// HourPoint 趋势图上的一点。
type HourPoint struct {
	// Label 显示用的时间，例如「14:00」或「10-16 14:00」
	Label    string `json:"label"`
	Time     string `json:"time"`
	Requests int64  `json:"requests"`
	Errors   int64  `json:"errors"`
	Blocked  int64  `json:"blocked"`
}

// Trends 最近 hours 小时的趋势，按时间正序（左旧右新）。
//
// 跨天要能拼起来：只取「今天」的小时桶会让早上八点打开面板时左边全是空的。
func Trends(hours int) []HourPoint {
	if hours <= 0 {
		hours = 24
	}
	now := time.Now()
	start := now.Add(-time.Duration(hours-1) * time.Hour).Truncate(time.Hour)

	out := make([]HourPoint, 0, hours)
	for i := 0; i < hours; i++ {
		t := start.Add(time.Duration(i) * time.Hour)
		day := t.Format("2006-01-02")

		point := HourPoint{
			Time: t.Format("2006-01-02T15:04"),
		}
		// 跨天时标上日期，否则 23:00 和 00:00 并排看不出分界
		if t.Format("2006-01-02") == now.Format("2006-01-02") {
			point.Label = t.Format("15:04")
		} else {
			point.Label = t.Format("01-02 15:04")
		}

		if st := ensureDayStat(day); st != nil {
			b := st.Hours[t.Hour()]
			point.Requests = b.Requests
			point.Errors = b.Errors
			point.Blocked = b.Blocked
		}
		out = append(out, point)
	}
	return out
}

// Overview 指标卡那一排数字。
type Overview struct {
	Requests  int64 `json:"requests"`
	Errors    int64 `json:"errors"`
	Blocked   int64 `json:"blocked"`
	UniqueIPs int64 `json:"uniqueIPs"`
	AttackIPs int64 `json:"attackIPs"`
	AvgDurMs  int64 `json:"avgDurMs"`
	BytesIn   int64 `json:"bytesIn"`
	BytesOut  int64 `json:"bytesOut"`
	Hours     int   `json:"hours"`
	// ErrorRatio 错误率，0–1。放后端算，免得前端在好几个地方各算一遍算错
	ErrorRatio float64 `json:"errorRatio"`
}

// BuildOverview 最近 hours 小时的指标。
//
// 唯一 IP 数用**明细重算**而不是把每小时桶里的计数加起来：
// 同一小时里同一个 IP 会重复出现，逐小时相加会得到「IP 次」而不是「个 IP」。
// 明细本身已经是按天分文件，最近 24 小时最多读两个文件。
func BuildOverview(hours int) Overview {
	if hours <= 0 {
		hours = 24
	}
	now := time.Now()
	since := now.Add(-time.Duration(hours) * time.Hour)

	ov := Overview{Hours: hours}
	seen := map[string]struct{}{}
	attacked := map[string]struct{}{}

	// 只读跨度内的那几天
	daySet := map[string]bool{}
	for d := since.Truncate(time.Hour); !d.After(now); d = d.Add(time.Hour) {
		daySet[d.Format("2006-01-02")] = true
	}
	days := make([]string, 0, len(daySet))
	for d := range daySet {
		days = append(days, d)
	}
	sort.Strings(days)

	var durSum, durCount int64
	for _, day := range days {
		recs, err := ReadDay(day, "", 0)
		if err != nil {
			continue
		}
		for _, r := range recs {
			if r.Time.Before(since) || r.Time.After(now) {
				continue
			}
			ov.Requests++
			if r.isError() {
				ov.Errors++
			}
			if r.Status == 403 || r.Status == 429 {
				ov.Blocked++
			}
			ov.BytesIn += r.BytesIn
			ov.BytesOut += r.BytesOut
			if r.DurationMs > 0 {
				durSum += r.DurationMs
				durCount++
			}
			if r.IP != "" {
				seen[r.IP] = struct{}{}
				// 「攻击 IP」的判据和列表里的可疑判定保持一致：
				// 4xx/5xx 出现过的 IP。这里不追求精确，追求「能一眼看出有几个在打我」
				if r.isError() {
					attacked[r.IP] = struct{}{}
				}
			}
		}
	}

	ov.UniqueIPs = int64(len(seen))
	ov.AttackIPs = int64(len(attacked))
	if durCount > 0 {
		ov.AvgDurMs = durSum / durCount
	}
	if ov.Requests > 0 {
		ov.ErrorRatio = float64(ov.Errors) / float64(ov.Requests)
	}
	return ov
}

// KindCount 攻击类型的一项。
type KindCount struct {
	Kind  AttackKind `json:"kind"`
	Label string     `json:"label"`
	Count int64      `json:"count"`
}

// AttackKinds 最近 hours 小时里各类攻击的次数，按次数从多到少。
//
// 走的是按小时的缓存，不读明细：明细一天几十 MB，而这只是页面上的一块小统计。
// 缓存缺了会自动从明细重算（ensureDayStat），所以不会因此少算。
func AttackKinds(hours int) []KindCount {
	if hours <= 0 {
		hours = 24
	}
	now := time.Now()
	start := now.Add(-time.Duration(hours-1) * time.Hour).Truncate(time.Hour)

	totals := map[AttackKind]int64{}
	for t := start; !t.After(now); t = t.Add(time.Hour) {
		st := ensureDayStat(t.Format("2006-01-02"))
		if st == nil {
			continue
		}
		for k, v := range st.Hours[t.Hour()].Kinds {
			totals[k] += v
		}
	}

	out := make([]KindCount, 0, len(totals))
	for k, v := range totals {
		out = append(out, KindCount{Kind: k, Label: k.Label(), Count: v})
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].Count != out[j].Count {
			return out[i].Count > out[j].Count
		}
		return out[i].Label < out[j].Label
	})
	return out
}

// SiteCount 一个站点的请求数。
type SiteCount struct {
	Site  string `json:"site"`
	Count int64  `json:"count"`
}

// TopSites 最近 hours 小时请求最多的站点。
func TopSites(hours, limit int) []SiteCount {
	if hours <= 0 {
		hours = 24
	}
	now := time.Now()
	since := now.Add(-time.Duration(hours) * time.Hour)

	totals := map[string]int64{}
	for d := since.Truncate(time.Hour); !d.After(now); d = d.Add(time.Hour) {
		recs, err := ReadDay(d.Format("2006-01-02"), "", 0)
		if err != nil {
			continue
		}
		for _, r := range recs {
			if r.Time.Before(since) || r.Time.After(now) {
				continue
			}
			site := r.Host
			if site == "" {
				site = r.Ref
			}
			if site == "" {
				site = "-"
			}
			totals[site]++
		}
	}

	out := make([]SiteCount, 0, len(totals))
	for s, c := range totals {
		out = append(out, SiteCount{Site: s, Count: c})
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].Count != out[j].Count {
			return out[i].Count > out[j].Count
		}
		return out[i].Site < out[j].Site
	})
	if limit > 0 && len(out) > limit {
		out = out[:limit]
	}
	return out
}
