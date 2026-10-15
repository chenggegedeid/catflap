package traffic

import (
	"strconv"
	"sync"
	"sync/atomic"
	"time"
)

// ===================== 实时速率 =====================
//
// 「实时速率」不是从磁盘数据拼出来的 —— 那是滞后一分钟的均值。这里在内存里
// 维护一个按秒分桶的滚动窗口，前端每两秒拉一次就能看到真实瞬时速率。
//
// 为什么按秒分桶而不是记每次请求：一个被扫的站点一秒能有上百个请求，
// 每条都记进切片再求和，迟早把内存和 CPU 吃满。分桶后每个桶只有一个
// atomic 计数器，写入是常数代价。

const (
	// windowSeconds 滚动窗口长度。60 秒够看出趋势，又不会太长显得迟钝。
	windowSeconds = 60
	// rateRefreshHint 建议前端多久拉一次（秒）。给前端一个提示而不是写死。
	rateRefreshHint = 2
)

// bucket 一秒的计数
type bucket struct {
	sec      int64 // Unix 秒
	in       int64 // 入站字节（atomic）
	out      int64 // 出站字节（atomic）
	requests int64 // 请求数（atomic）
}

// window 按秒分的滚动窗口。
//
// 用「固定大小数组 + 按秒取模」而不是「队列」：
// 队列要加锁做增删，而取模定位既无锁又不会有 GC 压力。
// 数组槽位的时间戳对不上就当它已过期（重置后用）。
type window struct {
	buckets [windowSeconds]bucket
}

var rateWindow = &window{}

// Add 记一次请求的流量。热路径上调用，只做 atomic 加法。
func (w *window) Add(in, out int64, requests int64) {
	now := time.Now().Unix()
	b := &w.buckets[now%windowSeconds]

	// 槽位还是上一轮（60 秒前）的：重置。用 CAS 以免并发下双重置
	if atomic.LoadInt64(&b.sec) != now {
		if atomic.CompareAndSwapInt64(&b.sec, b.sec, now) {
			atomic.StoreInt64(&b.in, 0)
			atomic.StoreInt64(&b.out, 0)
			atomic.StoreInt64(&b.requests, 0)
		}
	}

	atomic.AddInt64(&b.in, in)
	atomic.AddInt64(&b.out, out)
	atomic.AddInt64(&b.requests, requests)
}

// Rates 近 windowSeconds 秒的速率快照。
//
// 只统计还有效的槽位（时间戳在窗口内的），返回：
//   - 平均速率（窗口内总量 / 有效秒数）
//   - 逐秒序列（给前端画曲线）
func (w *window) Rates() (inPerSec float64, outPerSec float64, series []SecondPoint) {
	now := time.Now().Unix()
	from := now - windowSeconds + 1

	var totalIn, totalOut, totalReq int64
	// 按时间顺序输出，前端直接画
	series = make([]SecondPoint, 0, windowSeconds)

	for sec := from; sec <= now; sec++ {
		b := &w.buckets[sec%windowSeconds]
		if atomic.LoadInt64(&b.sec) != sec {
			// 这一秒没有记录（服务刚起来，或那一秒确实没流量）
			series = append(series, SecondPoint{At: sec, In: 0, Out: 0})
			continue
		}
		bi := atomic.LoadInt64(&b.in)
		bo := atomic.LoadInt64(&b.out)
		br := atomic.LoadInt64(&b.requests)
		totalIn += bi
		totalOut += bo
		totalReq += br
		series = append(series, SecondPoint{At: sec, In: bi, Out: bo, Requests: br})
	}

	elapsed := float64(windowSeconds)
	if elapsed > 0 {
		inPerSec = float64(totalIn) / elapsed
		outPerSec = float64(totalOut) / elapsed
	}
	return inPerSec, outPerSec, series
}

// SecondPoint 逐秒的一个采样点
type SecondPoint struct {
	At       int64 `json:"at"` // Unix 秒
	In       int64 `json:"in"`
	Out      int64 `json:"out"`
	Requests int64 `json:"requests"`
}

// ===================== 每分钟聚合 =====================

// minuteAccum 当前这一分钟的累加器。每分钟落一次盘，中间只做内存加法。
type minuteAccum struct {
	mu       sync.Mutex
	minute   int64 // 这一分钟的时间戳（Unix 秒对齐到分钟）
	in       int64
	out      int64
	requests int64
	errors   int64 // 4xx/5xx 计数，用来识别扫描
	bySite   map[uint]*siteAccum
	// byTunnel 穿透服务的流量：键是 "s<设备ID>-<服务ID>"
	byTunnel map[string]*siteAccum
}

type siteAccum struct {
	In       int64
	Out      int64
	Requests int64
}

var accum = &minuteAccum{
	bySite:   map[uint]*siteAccum{},
	byTunnel: map[string]*siteAccum{},
}

// Record 记一次请求。这是热路径上的入口。
//
// 分两段：先做无锁的 atomic（实时速率），再加锁更新分钟累加器。
// 后者每秒最多被调用几百次（不是每请求都加锁 —— 见下面 flush 的说明），
// 实际开销可忽略。
func Record(siteID uint, bytesIn, bytesOut int64, status int) {
	// 入站：请求体；出站：响应体。CH 记成 1 个请求
	rateWindow.Add(bytesIn, bytesOut, 1)

	now := time.Now()
	thisMinute := now.Truncate(time.Minute).Unix()

	accum.mu.Lock()
	defer accum.mu.Unlock()

	// 跨分钟了：先把上一分钟落盘，再开新的
	if accum.minute != 0 && accum.minute != thisMinute {
		accum.flushLocked()
	}
	accum.minute = thisMinute

	accum.in += bytesIn
	accum.out += bytesOut
	accum.requests++
	if status >= 400 {
		accum.errors++
	}

	sa := accum.bySite[siteID]
	if sa == nil {
		sa = &siteAccum{}
		accum.bySite[siteID] = sa
	}
	sa.In += bytesIn
	sa.Out += bytesOut
	sa.Requests++
}

// Flush 把当前这一分钟落盘（定时器每分钟调一次，退出时也调一次）
func Flush() {
	accum.mu.Lock()
	defer accum.mu.Unlock()
	accum.flushLocked()
}

// flushLocked 落盘当前累加器并重置。调用方必须持锁。
func (a *minuteAccum) flushLocked() {
	if a.minute == 0 || a.requests == 0 {
		// 这一分钟没有任何请求：不写空记录（一天 1440 行里大部分是空的
		// 会白白占地方）
		a.minute = 0
		return
	}

	rec := MinuteRecord{
		Minute:   a.minute,
		In:       a.in,
		Out:      a.out,
		Requests: a.requests,
		Errors:   a.errors,
		BySite:   map[string]SiteTraffic{},
		ByTunnel: map[string]SiteTraffic{},
	}
	for id, sa := range a.bySite {
		rec.BySite[strconv.FormatUint(uint64(id), 10)] = SiteTraffic{
			In:       sa.In,
			Out:      sa.Out,
			Requests: sa.Requests,
		}
	}
	for key, sa := range a.byTunnel {
		rec.ByTunnel[key] = SiteTraffic{
			In:       sa.In,
			Out:      sa.Out,
			Requests: sa.Requests,
		}
	}

	// 累计值：加到内存里的 totals，落盘交给 appendRecord
	AddTotals(a.in, a.out, a.requests)

	if err := appendRecord(rec); err != nil {
		// 落盘失败不能把内存里的数据丢了 —— 但也不能无限留在内存里。
		// 这里的策略是记日志并继续（下一分钟会重新开始），因为
		// 磁盘满了/没权限是运维问题，不是这里能自己解决的。
		logWriteErr(err)
	}

	// 重置
	a.in, a.out, a.requests, a.errors = 0, 0, 0, 0
	a.bySite = map[uint]*siteAccum{}
	a.byTunnel = map[string]*siteAccum{}
	a.minute = 0
}

// CurrentRates 实时速率快照。给前端 2 秒一次地调的接口用 ——
// 只读内存里的滚动窗口，**不碰磁盘**，所以调多频都不怕。
func CurrentRates() (inPerSec, outPerSec float64, series []SecondPoint) {
	return rateWindow.Rates()
}

// TodayTotals 今日累计。仪表盘上那个「今日流量」用它。
//
// 从当天的文件读而不是从内存：进程可能今天重启过，内存里只有重启后的
// 那一段。读文件才能拿到完整的一天。这个接口调用频率低（页面加载时读一次），
// 读一次文件没关系。
func TodayTotals() DailyTotals {
	now := time.Now()
	start := now.Truncate(24 * time.Hour)

	recs, err := ReadRange(start, now)
	if err != nil {
		return DailyTotals{Date: now.Format("2006-01-02")}
	}

	day := DailyTotals{Date: now.Format("2006-01-02")}
	for _, r := range recs {
		day.In += r.In
		day.Out += r.Out
		day.Requests += r.Requests
		day.Errors += r.Errors
	}
	return day
}

// ===================== 穿透服务的流量 =====================
//
// 反向代理那边的采集在 proxy 层（每个 HTTP 请求结束记一次）。
// 穿透是**另一条出口**：外部直接连到洞上、由这里转发到内网，
// 根本不经过反代 —— 所以必须单独在这里采，否则那部分流量是漏的。
//
// 与反代的两个区别：
//   - 反代按"请求"记（有 URL、状态码）；穿透是字节流，只按"连接"记
//   - 穿透的连接可能很长（SSH、看视频），所以**连接结束才上报**，
//     中途不记（转发中途上报既拿不到最终值又要额外加锁）

// RecordStun 记一次穿透转发的流量。
//
// deviceID / serviceID 用于按服务归组；都是 0 时算到"未分组"，不影响累计值。
func RecordStun(deviceID, serviceID uint, bytesIn, bytesOut int64) {
	if bytesIn <= 0 && bytesOut <= 0 {
		return
	}
	// 实时速率：与反代共用同一个滚动窗口，仪表盘上看到的是两者之和
	rateWindow.Add(bytesIn, bytesOut, 1)

	accum.mu.Lock()
	defer accum.mu.Unlock()

	now := time.Now()
	thisMinute := now.Truncate(time.Minute).Unix()
	if accum.minute != 0 && accum.minute != thisMinute {
		accum.flushLocked()
	}
	accum.minute = thisMinute

	accum.in += bytesIn
	accum.out += bytesOut
	accum.requests++ // 一条连接算一次

	// 按"穿透服务"分组。键用 s<设备>-<服务>，和站点的纯数字 ID 区分开，
	// 前端显示时据此知道这是穿透服务而不是反代站点
	key := "s" + strconv.FormatUint(uint64(deviceID), 10) + "-" + strconv.FormatUint(uint64(serviceID), 10)
	sa := accum.byTunnel[key]
	if sa == nil {
		sa = &siteAccum{}
		accum.byTunnel[key] = sa
	}
	sa.In += bytesIn
	sa.Out += bytesOut
	sa.Requests++
}
