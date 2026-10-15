package traffic

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"sync"
	"time"

	"github.com/sirupsen/logrus"
)

// ===================== 落盘 =====================

const (
	// trafficDir 按天的明细放这儿
	trafficDir = "data/traffic"
	// summaryPath 累计值
	summaryPath = "data/traffic/summary.json"

	// maxFileBytes 单天文件上限。超过就停止写入并告警 ——
	// 被刷流量时最怕的不是流量本身，是把磁盘写满导致整个服务挂掉。
	maxFileBytes = 64 << 20 // 64 MB

	// keepDays 明细保留天数，更早的删掉（累计值不受影响）
	keepDays = 90
)

// MinuteRecord 一分钟的汇总
type MinuteRecord struct {
	Minute   int64                  `json:"minute"` // Unix 秒（对齐到分钟）
	In       int64                  `json:"in"`
	Out      int64                  `json:"out"`
	Requests int64                  `json:"requests"`
	Errors   int64                  `json:"errors"`
	BySite   map[string]SiteTraffic `json:"bySite,omitempty"`
	// ByTunnel 穿透服务的流量，键是 "s<设备ID>-<服务ID>"
	ByTunnel map[string]SiteTraffic `json:"byTunnel,omitempty"`
}

// SiteTraffic 单个站点的流量
type SiteTraffic struct {
	In       int64 `json:"in"`
	Out      int64 `json:"out"`
	Requests int64 `json:"requests"`
}

// Summary 累计值。**跨重启保留** —— 每次重启从零开始的话，"累计"就没意义了。
type Summary struct {
	TotalIn    int64 `json:"totalIn"`
	TotalOut   int64 `json:"totalOut"`
	TotalReqs  int64 `json:"totalRequests"`
	Since      int64 `json:"since"` // 开始统计的时间（Unix 秒）
	LastMinute int64 `json:"lastMinute"`
}

var (
	totalsMu sync.Mutex
	totals   Summary

	// writeBlocked 当天的文件超限后置为 true，停止写入并只记一次日志
	writeBlocked   bool
	writeBlockedOn string
)

// Init 读累计值、清理过期文件
func Init() error {
	if err := os.MkdirAll(trafficDir, 0o755); err != nil {
		return err
	}

	data, err := os.ReadFile(summaryPath)
	if err == nil {
		var s Summary
		if err := json.Unmarshal(data, &s); err == nil {
			totalsMu.Lock()
			totals = s
			totalsMu.Unlock()
			logrus.Infof("[traffic] 累计流量已接上：入 %s / 出 %s",
				HumanBytes(s.TotalIn), HumanBytes(s.TotalOut))
		}
	} else if os.IsNotExist(err) {
		totalsMu.Lock()
		totals.Since = time.Now().Unix()
		totalsMu.Unlock()
	}

	cleanOldFiles()
	return nil
}

// AddTotals 累加累计值（内存），落盘交给 SaveTotals
func AddTotals(in, out, requests int64) {
	totalsMu.Lock()
	defer totalsMu.Unlock()
	totals.TotalIn += in
	totals.TotalOut += out
	totals.TotalReqs += requests
	totals.LastMinute = time.Now().Unix()
}

// Totals 累计值快照
func Totals() Summary {
	totalsMu.Lock()
	defer totalsMu.Unlock()
	return totals
}

// SaveTotals 把累计值落盘。定时器每分钟和退出时各调一次。
//
// 先写临时文件再改名：直接覆盖时断电会留下半截 JSON，
// 下次启动解析失败 = 累计值全丢。
func SaveTotals() error {
	totalsMu.Lock()
	snapshot := totals
	totalsMu.Unlock()

	if err := os.MkdirAll(filepath.Dir(summaryPath), 0o755); err != nil {
		return err
	}
	data, err := json.MarshalIndent(snapshot, "", "  ")
	if err != nil {
		return err
	}
	tmp := summaryPath + ".tmp"
	if err := os.WriteFile(tmp, data, 0o644); err != nil {
		return err
	}
	return os.Rename(tmp, summaryPath)
}

// appendRecord 追加一行到当天的文件
func appendRecord(rec MinuteRecord) error {
	name := recordPath(time.Unix(rec.Minute, 0))

	// 单文件超限：停止写入。宁可丢统计也不能把磁盘写满 ——
	// 磁盘满了整个服务（包括前面那些正常的转发）都会挂。
	if fi, err := os.Stat(name); err == nil && fi.Size() > maxFileBytes {
		markBlocked(name)
		return nil
	}

	f, err := os.OpenFile(name, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o644)
	if err != nil {
		return err
	}
	defer f.Close()

	line, err := json.Marshal(rec)
	if err != nil {
		return err
	}
	line = append(line, '\n')
	if _, err := f.Write(line); err != nil {
		return err
	}
	return nil
}

func markBlocked(name string) {
	if writeBlocked && writeBlockedOn == name {
		return
	}
	writeBlocked = true
	writeBlockedOn = name
	logrus.Warnf("[traffic] %s 已达 %s 上限，暂停写入当天的流量明细（累计值不受影响）",
		name, HumanBytes(maxFileBytes))
}

func recordPath(t time.Time) string {
	return filepath.Join(trafficDir, t.Format("2006-01-02")+".jsonl")
}

// ReadRange 读某段时间的分钟记录（含首尾）
func ReadRange(from, to time.Time) ([]MinuteRecord, error) {
	var out []MinuteRecord

	fromMin := from.Truncate(time.Minute).Unix()
	toMin := to.Unix()

	// 按天遍历：跨天的范围要读多个文件
	for day := from.Truncate(24 * time.Hour); !day.After(to); day = day.Add(24 * time.Hour) {
		name := recordPath(day)
		recs, err := readFile(name)
		if err != nil {
			if os.IsNotExist(err) {
				continue // 那天没数据，正常
			}
			return nil, err
		}
		for _, r := range recs {
			if r.Minute >= fromMin && r.Minute <= toMin {
				out = append(out, r)
			}
		}
	}

	sort.Slice(out, func(i, j int) bool { return out[i].Minute < out[j].Minute })
	return out, nil
}

func readFile(name string) ([]MinuteRecord, error) {
	f, err := os.Open(name)
	if err != nil {
		return nil, err
	}
	defer f.Close()

	var out []MinuteRecord
	dec := json.NewDecoder(f)
	for {
		var r MinuteRecord
		if err := dec.Decode(&r); err != nil {
			if err.Error() == "EOF" {
				break
			}
			// 单行坏了不该让整天的数据读不出来：跳过继续。
			// （断电时最后一行可能是半截 JSON）
			continue
		}
		out = append(out, r)
	}
	return out, nil
}

// DailyTotals 按天汇总，给趋势图用
type DailyTotals struct {
	Date     string `json:"date"`
	In       int64  `json:"in"`
	Out      int64  `json:"out"`
	Requests int64  `json:"requests"`
	Errors   int64  `json:"errors"`
}

// AggregateByDay 把分钟记录汇总成按天的
func AggregateByDay(recs []MinuteRecord) []DailyTotals {
	byDay := map[string]*DailyTotals{}
	for _, r := range recs {
		day := time.Unix(r.Minute, 0).Format("2006-01-02")
		d := byDay[day]
		if d == nil {
			d = &DailyTotals{Date: day}
			byDay[day] = d
		}
		d.In += r.In
		d.Out += r.Out
		d.Requests += r.Requests
		d.Errors += r.Errors
	}

	out := make([]DailyTotals, 0, len(byDay))
	for _, d := range byDay {
		out = append(out, *d)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Date < out[j].Date })
	return out
}

// AggregateBySite 按站点汇总，给「服务流量 Top」用
func AggregateBySite(recs []MinuteRecord) map[string]SiteTraffic {
	out := map[string]SiteTraffic{}
	for _, r := range recs {
		for id, st := range r.BySite {
			cur := out[id]
			cur.In += st.In
			cur.Out += st.Out
			cur.Requests += st.Requests
			out[id] = cur
		}
	}
	return out
}

// AggregateByTunnel 按穿透服务汇总。
//
// 和站点分开返回：它们是两种东西（一个是反代站点、一个是穿透服务），
// 前端要分别显示，且键的格式也不同（"s1-2" vs "1"）。
func AggregateByTunnel(recs []MinuteRecord) map[string]SiteTraffic {
	out := map[string]SiteTraffic{}
	for _, r := range recs {
		for key, st := range r.ByTunnel {
			cur := out[key]
			cur.In += st.In
			cur.Out += st.Out
			cur.Requests += st.Requests
			out[key] = cur
		}
	}
	return out
}

// cleanOldFiles 删掉超过保留期的明细。累计值在 summary.json 里，不受影响。
func cleanOldFiles() {
	entries, err := os.ReadDir(trafficDir)
	if err != nil {
		return
	}
	cutoff := time.Now().AddDate(0, 0, -keepDays)
	for _, e := range entries {
		name := e.Name()
		if filepath.Ext(name) != ".jsonl" {
			continue
		}
		dateStr := name[:len(name)-len(".jsonl")]
		d, err := time.Parse("2006-01-02", dateStr)
		if err != nil {
			continue
		}
		if d.Before(cutoff) {
			_ = os.Remove(filepath.Join(trafficDir, name))
		}
	}
}

func logWriteErr(err error) {
	logrus.Warnf("[traffic] 写入失败（这一分钟的明细丢失，累计值仍在）：%v", err)
}

// HumanBytes 给人看的体积。日志和界面上都用它，别到处写一遍。
func HumanBytes(n int64) string {
	const unit = 1024
	if n < unit {
		return strconv.FormatInt(n, 10) + " B"
	}
	units := []string{"KB", "MB", "GB", "TB", "PB"}
	v := float64(n)
	i := -1
	for v >= unit && i < len(units)-1 {
		v /= unit
		i++
	}
	return fmt.Sprintf("%.1f %s", v, units[i])
}
