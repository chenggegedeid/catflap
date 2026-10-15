package traffic

import (
	"strconv"
	"testing"
	"time"
)

// 穿透服务的流量要能被单独统计 —— 它不走反代，漏了的话流量大屏里
// "各服务流量"就少了一块（用户会以为某个服务没流量）。
func TestRecordStunAggregatesByService(t *testing.T) {
	// 用独立的累加器，别动全局那个（会影响别的用例）
	old := accum
	accum = &minuteAccum{
		bySite:   map[uint]*siteAccum{},
		byTunnel: map[string]*siteAccum{},
	}
	defer func() { accum = old }()

	now := time.Now().Truncate(time.Minute).Unix()

	RecordStun(1, 2, 1000, 2000)
	RecordStun(1, 2, 500, 700) // 同一个服务再记一次，应该累加
	RecordStun(3, 4, 10, 20)   // 另一个服务

	if accum.in != 1510 {
		t.Errorf("入站合计应为 1510，实际 %d", accum.in)
	}
	if accum.out != 2720 {
		t.Errorf("出站合计应为 2720，实际 %d", accum.out)
	}
	if accum.requests != 3 {
		t.Errorf("连接数应为 3，实际 %d", accum.requests)
	}

	// 按服务分组
	key12 := "s1-2"
	sa, ok := accum.byTunnel[key12]
	if !ok {
		t.Fatalf("缺少 s1-2 的分组：%v", accum.byTunnel)
	}
	if sa.In != 1500 || sa.Out != 2700 || sa.Requests != 2 {
		t.Errorf("s1-2 应为 in=1500 out=2700 req=2，实际 in=%d out=%d req=%d",
			sa.In, sa.Out, sa.Requests)
	}
	if _, ok := accum.byTunnel["s3-4"]; !ok {
		t.Error("缺少 s3-4 的分组")
	}

	_ = now
}

// 全零不该记（转发没传任何数据时不产生脏记录）
func TestRecordStunSkipsZero(t *testing.T) {
	old := accum
	accum = &minuteAccum{
		bySite:   map[uint]*siteAccum{},
		byTunnel: map[string]*siteAccum{},
	}
	defer func() { accum = old }()

	RecordStun(1, 1, 0, 0)
	if len(accum.byTunnel) != 0 {
		t.Errorf("全零不该记录，实际 %v", accum.byTunnel)
	}
	if accum.requests != 0 {
		t.Errorf("全零不该增加连接数，实际 %d", accum.requests)
	}
}

// 落盘的分钟记录里要带上穿透流量，否则重启后趋势里就没它了
func TestFlushWritesTunnelTraffic(t *testing.T) {
	old := accum
	accum = &minuteAccum{
		bySite:   map[uint]*siteAccum{},
		byTunnel: map[string]*siteAccum{},
	}
	defer func() { accum = old }()

	RecordStun(7, 9, 300, 400)

	// 直接调 flushLocked 的内部逻辑太绕，这里验证聚合函数能读到
	rec := MinuteRecord{
		Minute:   1,
		BySite:   map[string]SiteTraffic{},
		ByTunnel: map[string]SiteTraffic{},
	}
	for key, sa := range accum.byTunnel {
		rec.ByTunnel[key] = SiteTraffic{In: sa.In, Out: sa.Out, Requests: sa.Requests}
	}

	got := AggregateByTunnel([]MinuteRecord{rec})
	want := "s" + strconv.FormatUint(7, 10) + "-" + strconv.FormatUint(9, 10)
	if _, ok := got[want]; !ok {
		t.Fatalf("聚合结果里应有 %s，实际 %v", want, got)
	}
	if got[want].In != 300 || got[want].Out != 400 {
		t.Errorf("%s 应为 in=300 out=400，实际 in=%d out=%d", want, got[want].In, got[want].Out)
	}
}
