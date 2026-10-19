package traffic

import (
	"testing"
)

// 这几条守的是「传输途中分段上报」的修复。
//
// 背景：原来只在请求/连接结束时记一次，于是大文件下载、SSH、看视频这类
// 长传输**在整段过程中统计都是零**，用户在路由器上看到上行在跑，
// 我们这边却是平的 —— 表现为"流量统计不准"。

// TestRecordPartialAddsBytesButNotRequests
// 途中上报只加字节，**绝不能加请求数**。
//
// 一次下载是一个请求。若按上报次数计请求，传一个 1GB 的文件会被记成
// 几十个请求，统计页上的"请求数"就没法看了。
func TestRecordPartialAddsBytesButNotRequests(t *testing.T) {
	resetAccum(t)

	RecordPartial(1, 0, 1000)
	RecordPartial(1, 0, 2000)

	if accum.requests != 0 {
		t.Errorf("途中上报不该记请求数，实际 %d", accum.requests)
	}
	if accum.out != 3000 {
		t.Errorf("字节应当累加到 3000，实际 %d", accum.out)
	}
}

// TestPartialPlusSettleCountsOnce 途中分段 + 结尾结算 = 不重不漏。
//
// 这是这次修复最容易出错的地方：结算时若传了完整的总字节而不是**剩余**，
// 流量就会凭空翻倍；若忘了结算，最后一截就丢了。
func TestPartialPlusSettleCountsOnce(t *testing.T) {
	resetAccum(t)

	// 模拟：总共 5000 字节，途中报了两次（1000 + 2000），剩 2000
	RecordPartial(1, 0, 1000)
	RecordPartial(1, 0, 2000)
	// 结算：只补剩余，并记这一次请求
	Record(1, 0, 2000, 200)

	if accum.out != 5000 {
		t.Errorf("总字节应当正好 5000（不重不漏），实际 %d", accum.out)
	}
	if accum.requests != 1 {
		t.Errorf("一次下载只能算 1 个请求，实际 %d", accum.requests)
	}
}

// TestRecordStunSettleCountsEvenWhenFullyReported
// 剩余字节为 0 时，连接数**仍要记**。
//
// 加了途中上报后，结算时的"剩余"完全可能是 0（刚好报完）。
// 若沿用 RecordStun 那个"没字节就 return"的短路，这条连接就不会被记进
// 连接数 —— 而且漏得毫无规律（取决于字节数是否卡在阈值整数倍上）。
func TestRecordStunSettleCountsEvenWhenFullyReported(t *testing.T) {
	resetAccum(t)

	// 途中把 1000 字节全报完了
	RecordStunPartial(2, 3, 0, 1000)
	// 结算：剩余 0
	RecordStunSettle(2, 3, 0, 0)

	if accum.requests != 1 {
		t.Errorf("剩余字节为 0 时也应当把这条连接记进去，实际连接数 %d", accum.requests)
	}
	if accum.out != 1000 {
		t.Errorf("字节应当是途中报的 1000，不能被重复加，实际 %d", accum.out)
	}
}

// TestRecordStunStillSkipsZero 既有约定：RecordStun 一次性记录时零字节不记。
//
// 这条防的是探测性空连接（连上就断）把计数刷上去。
// 有测试守着，所以加 Settle 时不能改 RecordStun 的行为，只能新增函数。
func TestRecordStunStillSkipsZero(t *testing.T) {
	resetAccum(t)

	RecordStun(9, 9, 0, 0)

	if accum.requests != 0 {
		t.Errorf("RecordStun 零字节仍应跳过，实际连接数 %d", accum.requests)
	}
	if len(accum.byTunnel) != 0 {
		t.Errorf("RecordStun 零字节不应建分组，实际 %v", accum.byTunnel)
	}
}

// resetAccum 把全局累加器清干净，并恢复测试前的状态。
//
// traffic 用的是包级单例累加器，测试之间会互相影响 ——
// 不重置的话这些断言全都不成立。
func resetAccum(t *testing.T) {
	t.Helper()
	accum.mu.Lock()
	defer accum.mu.Unlock()
	accum.minute = 0
	accum.in, accum.out = 0, 0
	accum.requests, accum.errors = 0, 0
	accum.bySite = map[uint]*siteAccum{}
	accum.byTunnel = map[string]*siteAccum{}
	t.Cleanup(func() {
		accum.mu.Lock()
		defer accum.mu.Unlock()
		accum.minute = 0
		accum.in, accum.out = 0, 0
		accum.requests, accum.errors = 0, 0
		accum.bySite = map[uint]*siteAccum{}
		accum.byTunnel = map[string]*siteAccum{}
	})
}
