package traffic_api

import (
	"strconv"
	"time"

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
	// BySite 站点流量，键是站点 ID 的字符串形式
	BySite map[string]traffic.SiteTraffic `json:"bySite"`
}

// TrafficTrendView 近 N 天趋势（默认 7 天，最多 90）
func (TrafficApi) TrafficTrendView(c *gin.Context) {
	days := 7
	if v := c.Query("days"); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n > 0 && n <= 90 {
			days = n
		}
	}

	to := time.Now()
	from := to.AddDate(0, 0, -days+1).Truncate(24 * time.Hour)

	recs, err := traffic.ReadRange(from, to)
	if err != nil {
		res.FailWithMsg("读取流量记录失败："+err.Error(), c)
		return
	}

	res.OkWithData(TrendView{
		Days:   traffic.AggregateByDay(recs),
		BySite: traffic.AggregateBySite(recs),
	}, c)
}
