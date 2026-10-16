package traffic_api

import (
	"strconv"
	"strings"
	"time"

	"github.com/chenggegedeid/catflap/modules/device"
	"github.com/chenggegedeid/catflap/modules/proxy"
	"github.com/chenggegedeid/catflap/modules/stun"
	"github.com/chenggegedeid/catflap/modules/traffic"
	"github.com/chenggegedeid/catflap/utils/res"

	"github.com/gin-gonic/gin"
)

// TrafficApi 流量统计
type TrafficApi struct{}

// RealtimeView 实时速率：给仪表盘那个 2 秒刷新的区域用
type RealtimeView struct {
	InPerSec  float64 `json:"inPerSec"`
	OutPerSec float64 `json:"outPerSec"`
	// Series 逐秒序列，前端直接画曲线
	Series []traffic.SecondPoint `json:"series"`
	// Totals 累计值顺带带上，省一次请求
	Totals traffic.Summary `json:"totals"`
	// Today 今日累计
	Today traffic.DailyTotals `json:"today"`
}

// TrafficRealtimeView 实时速率 + 累计值
//
// 这个接口会被前端每 2 秒调一次，所以只读内存、不碰磁盘。
func (TrafficApi) TrafficRealtimeView(c *gin.Context) {
	in, out, series := traffic.CurrentRates()
	totals := traffic.Totals()

	res.OkWithData(RealtimeView{
		InPerSec:  in,
		OutPerSec: out,
		Series:    series,
		Totals:    totals,
		Today:     traffic.TodayTotals(),
	}, c)
}

// TrendView 趋势：按天汇总
type TrendView struct {
	Days []traffic.DailyTotals `json:"days"`
	// BySite 反代站点流量，键是站点 ID 的字符串形式
	BySite map[string]traffic.SiteTraffic `json:"bySite"`
	// ByTunnel 穿透服务流量，键是 "s<设备ID>-<服务ID>"
	ByTunnel map[string]traffic.SiteTraffic `json:"byTunnel"`
	// Names 服务名映射，键与上面两个的键一致，值是「设备名 · 服务名」。
	//
	// 后端顺手解析好：前端不用为了显示名字再拉一次站点/设备配置，
	// 也少一处"前后端各自解析 ID"的重复。
	// 解析不到（比如站点已删）就没有对应项，前端回落到显示编号。
	Names map[string]string `json:"names,omitempty"`
	// PerServiceDay 每个服务按天的流量序列（key 同 BySite/ByTunnel）。
	// 站点和穿透混在一张 map 里 —— 前端按 names 里的键展示就行。
	PerServiceDay map[string][]traffic.ServiceDayPoint `json:"perServiceDay,omitempty"`
	// Series 均匀时间序列（速率折线图用）。bucket 由范围自动决定：
	// ≤48 小时按小时，更大按天。速率 = 流量 / BucketSeconds。
	Bucket        string                `json:"bucket,omitempty"`
	BucketSeconds int64                 `json:"bucketSeconds,omitempty"`
	Hours         int                   `json:"hours,omitempty"`
	Series        []traffic.SeriesPoint `json:"series,omitempty"`
}

// TrafficTrendView 近 N 天趋势（默认 7 天，最多 90）
func (TrafficApi) TrafficTrendView(c *gin.Context) {
	// hours 优先：范围按小时算（近 1 小时 / 近 24 小时这类）。
	// 没传就退回旧的 days 参数（兼容 + 近 7/30 天仍然好用）。
	hours := 0
	if v := c.Query("hours"); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n > 0 && n <= 24*90 {
			hours = n
		}
	}
	days := 0
	if hours == 0 {
		days = 7
		if v := c.Query("days"); v != "" {
			if n, err := strconv.Atoi(v); err == nil && n > 0 && n <= 90 {
				days = n
			}
		}
		hours = days * 24
	}

	to := time.Now()
	from := to.Add(-time.Duration(hours) * time.Hour)

	// **自动聚合**：范围小按小时，范围大按天 —— 30 天按小时看就是一坨密不透风的毛刺，
	// 1 小时按天看就一个点。和飞牛的"自动聚合"同一个思路。
	bucket := time.Hour
	bucketName := "hour"
	if hours > 48 {
		bucket = 24 * time.Hour
		bucketName = "day"
	}

	recs, err := traffic.ReadRange(from.Truncate(bucket), to)
	if err != nil {
		res.FailWithMsg("读取流量记录失败："+err.Error(), c)
		return
	}

	res.OkWithData(TrendView{
		Bucket:        bucketName,
		BucketSeconds: int64(bucket.Seconds()),
		Hours:         hours,
		Series:        traffic.AggregateSeries(recs, bucket),
		Days:          traffic.AggregateByDay(recs),
		BySite:        traffic.AggregateBySite(recs),
		ByTunnel:      traffic.AggregateByTunnel(recs),
		Names:         buildServiceNames(),
		// 每个服务**按天**的流量序列 —— "点开某个服务看它最近每天用了多少"
		PerServiceDay: traffic.AggregateByServiceDay(recs),
	}, c)
}

// buildServiceNames 把流量统计里的那些键解析成人能看的名字。
//
// 键长得像 "3"（站点 ID）或 "s1-2"（穿透：设备 1 的服务 2），
// 光看编号不知道是谁在跑流量。这里拼成「设备名 · 服务名」：
//
//	"3"    → "群晖 NAS · 照片库"
//	"s1-2" → "群晖 NAS · SSH"
//
// **在 api 层做聚合**：traffic 模块不该认识站点和穿透服务，
// proxy 和 stun 也不该互相依赖。这层和"域名从 DDNS 选"是同一个套路。
//
// 站点名优先用域名（用户更认域名），没有就用备注；
// 设备名从公共设备表按 IP 查（站点靠后端 IP、穿透靠设备 ID）。
func buildServiceNames() map[string]string {
	out := map[string]string{}

	// 设备：ID → 名字，以及 IP → 名字（站点靠 IP 归属）
	devName := map[uint]string{}
	devByIP := map[string]string{}
	for _, d := range device.List() {
		devName[d.DeviceID] = d.Name
		devByIP[d.IP] = d.Name
	}

	// 反代站点：键是站点 ID
	proxyCfg := proxy.Runtime.Snapshot()
	for _, s := range proxyCfg.Sites {
		svc := s.Description
		if svc == "" && len(s.Hosts) > 0 {
			svc = s.Hosts[0]
		}
		if svc == "" {
			continue
		}
		ip := hostOf(s.Backend)
		if dn, ok := devByIP[ip]; ok {
			svc = dn + " · " + svc
		}
		out[strconv.FormatUint(uint64(s.ID), 10)] = svc
	}

	// 穿透服务：键是 "s<设备ID>-<服务ID>"
	stunCfg := stun.Runtime.Config
	for _, svc := range stunCfg.Services {
		name := svc.Name
		if name == "" {
			continue
		}
		if dn, ok := devName[svc.DeviceID]; ok {
			name = dn + " · " + name
		}
		out["s"+strconv.FormatUint(uint64(svc.DeviceID), 10)+"-"+
			strconv.FormatUint(uint64(svc.ID), 10)] = name
	}

	return out
}

// hostOf 取 "192.168.1.20:5000" 里的主机部分
func hostOf(addr string) string {
	addr = strings.TrimSpace(addr)
	if i := strings.LastIndex(addr, ":"); i > 0 {
		return addr[:i]
	}
	return addr
}
