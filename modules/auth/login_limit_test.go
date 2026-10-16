package auth

import (
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

// 和实际出厂值一致（默认 5 次），这样测的就是真正会上线的行为
const testLimit = 5

func testLimiter() *loginFailureLimiter {
	l := newLoginFailureLimiter()
	l.ipLimit = testLimit
	return l
}

// 没有失败记录时，连续成功的校验不该消耗失败额度
func TestLoginFailureLimiterOnlyCountsFailures(t *testing.T) {
	limiter := testLimiter()
	now := time.Date(2026, 8, 1, 12, 0, 0, 0, time.UTC)
	const key = "203.0.113.7"

	for i := 0; i < testLimit*2; i++ {
		if !limiter.begin(key, now) {
			t.Fatalf("successful attempt %d should be allowed", i+1)
		}
		limiter.finish(loginAttemptSucceeded, key, now)
	}

	for i := 0; i < testLimit; i++ {
		if !limiter.begin(key, now) {
			t.Fatalf("failed attempt %d should be allowed", i+1)
		}
		limiter.finish(loginAttemptFailed, key, now)
	}
	if limiter.begin(key, now) {
		t.Fatal("attempt above the failure limit should be rejected")
	}

	// 窗口过了**也不该放行**：以前是「3 分钟 13 次」的软限，撑过窗口又能接着试，
	// 爆破者只要放慢节奏（每分钟 4 次）就能无限试下去。现在是封禁到点才放。
	if limiter.begin(key, now.Add(limiter.ipWindow)) {
		t.Fatal("should still be blocked after the failure window expires")
	}
	// 封禁到期才放行
	if !limiter.begin(key, now.Add(limiter.blockFor+time.Second)) {
		t.Fatal("should be allowed once the block expires")
	}
	limiter.finish(loginAttemptIgnored, key, now.Add(limiter.blockFor+time.Second))
}

// 达到阈值那一次，finish 要返回 true —— 调用方靠它决定是否写进审计的封禁列表
func TestLoginFailureLimiterReportsBlockAtThreshold(t *testing.T) {
	limiter := testLimiter()
	now := time.Date(2026, 8, 1, 12, 0, 0, 0, time.UTC)
	const key = "203.0.113.7"

	for i := 0; i < testLimit-1; i++ {
		limiter.begin(key, now)
		if limiter.finish(loginAttemptFailed, key, now) {
			t.Fatalf("第 %d 次失败就报封禁了，太早", i+1)
		}
	}
	limiter.begin(key, now)
	if !limiter.finish(loginAttemptFailed, key, now) {
		t.Fatalf("第 %d 次失败应当触发封禁并上报", testLimit)
	}
}

// 成功之后失败计数清零，重新数
func TestLoginFailureLimiterResetsAfterSuccess(t *testing.T) {
	limiter := testLimiter()
	now := time.Date(2026, 8, 1, 12, 0, 0, 0, time.UTC)
	const key = "203.0.113.7"

	for i := 0; i < testLimit-1; i++ {
		limiter.begin(key, now)
		limiter.finish(loginAttemptFailed, key, now)
	}
	limiter.begin(key, now)
	limiter.finish(loginAttemptSucceeded, key, now)

	// 成功清了旧账，还能再错 testLimit-1 次而不被封
	for i := 0; i < testLimit-1; i++ {
		if !limiter.begin(key, now) {
			t.Fatalf("success 之后的第 %d 次失败不该被拒", i+1)
		}
		limiter.finish(loginAttemptFailed, key, now)
	}
	if !limiter.begin(key, now) {
		t.Fatal("成功清零后，额度应当恢复")
	}
}

// 登录成功会把这个来源的失败记录清掉
func TestLoginFailureLimiterClearsFailuresAfterSuccess(t *testing.T) {
	limiter := testLimiter()
	now := time.Date(2026, 8, 1, 12, 0, 0, 0, time.UTC)
	const key = "203.0.113.7"

	for i := 0; i < testLimit-1; i++ {
		if !limiter.begin(key, now) {
			t.Fatalf("failed attempt %d should be allowed", i+1)
		}
		limiter.finish(loginAttemptFailed, key, now)
	}
	if !limiter.begin(key, now) {
		t.Fatal("successful attempt should be allowed")
	}
	limiter.finish(loginAttemptSucceeded, key, now)

	for i := 0; i < testLimit; i++ {
		if !limiter.begin(key, now) {
			t.Fatalf("failed attempt %d should be allowed after success", i+1)
		}
		limiter.finish(loginAttemptFailed, key, now)
	}
	if limiter.begin(key, now) {
		t.Fatal("attempt above the reset failure limit should be rejected")
	}
}

// **这条是分桶的核心**：一个 IP 把额度用光，不该影响别的 IP。
//
// 全局计数时做不到这点 —— 管理页被爆破会把管理员自己也挡在外面，
// 那等于攻击者拿很小的代价完成了 DoS。
func TestLoginFailureLimiterIsPerIP(t *testing.T) {
	limiter := testLimiter()
	now := time.Date(2026, 8, 1, 12, 0, 0, 0, time.UTC)
	const attacker = "203.0.113.7"
	const admin = "198.51.100.9"

	// 攻击者把额度耗光
	for i := 0; i < testLimit; i++ {
		if !limiter.begin(attacker, now) {
			t.Fatalf("attacker attempt %d should be allowed", i+1)
		}
		limiter.finish(loginAttemptFailed, attacker, now)
	}
	if limiter.begin(attacker, now) {
		t.Fatal("attacker above limit should be rejected")
	}

	// 管理员从另一个 IP 进来，必须还能登
	if !limiter.begin(admin, now) {
		t.Fatal("admin from a different IP must still be allowed")
	}
	limiter.finish(loginAttemptSucceeded, admin, now)
}

// 每个 IP 的额度是独立的，不是共享同一份
func TestLoginFailureLimiterBucketsAreIndependent(t *testing.T) {
	limiter := testLimiter()
	now := time.Date(2026, 8, 1, 12, 0, 0, 0, time.UTC)

	// 两个 IP 各用掉一半额度，加起来超过上限，但都不该被拒
	for i := 0; i < testLimit-1; i++ {
		for _, k := range []string{"203.0.113.7", "198.51.100.9"} {
			if !limiter.begin(k, now) {
				t.Fatalf("%s attempt %d should be allowed", k, i+1)
			}
			limiter.finish(loginAttemptFailed, k, now)
		}
	}
	for _, k := range []string{"203.0.113.7", "198.51.100.9"} {
		if !limiter.begin(k, now) {
			t.Fatalf("%s should still be allowed", k)
		}
		limiter.finish(loginAttemptIgnored, k, now)
	}
}

// 认不出来源（空 key）时只受全局总闸约束：既不放行也不拒绝
func TestLoginFailureLimiterEmptyKeyUsesGlobalOnly(t *testing.T) {
	limiter := testLimiter()
	now := time.Date(2026, 8, 1, 12, 0, 0, 0, time.UTC)

	// 空 key 不受单 IP 桶约束：用掉 testLimit 次失败，比单 IP 上限还多也没事
	for i := 0; i < testLimit; i++ {
		if !limiter.begin("", now) {
			t.Fatalf("empty-key attempt %d should be allowed (global only)", i+1)
		}
		limiter.finish(loginAttemptFailed, "", now)
	}
	// 把全局总闸压到刚好等于已失败的次数，下一次就该被拒
	limiter.gLimit = len(limiter.global.failures)
	if limiter.begin("", now) {
		t.Fatal("should be rejected once the global ceiling is hit")
	}
}

// 并发预留不能超发
func TestLoginFailureLimiterReservesConcurrentAttempts(t *testing.T) {
	limiter := testLimiter()
	now := time.Date(2026, 8, 1, 12, 0, 0, 0, time.UTC)
	const key = "203.0.113.7"

	var allowed atomic.Int32
	var wg sync.WaitGroup
	for i := 0; i < 100; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			if limiter.begin(key, now) {
				allowed.Add(1)
			}
		}()
	}
	wg.Wait()

	if got := int(allowed.Load()); got != testLimit {
		t.Fatalf("reserved %d concurrent attempts, want %d", got, testLimit)
	}
}

// 空桶要被回收：扫描器换 IP 会留下大量一次性桶，不回收这张表一直涨
func TestLoginFailureLimiterReclaimsEmptyBuckets(t *testing.T) {
	limiter := testLimiter()
	now := time.Date(2026, 8, 1, 12, 0, 0, 0, time.UTC)

	for i := 0; i < 20; i++ {
		k := "203.0.113." + string(rune('0'+i%10))
		limiter.begin(k, now)
		limiter.finish(loginAttemptIgnored, k, now)
	}
	// 上面的都是"忽略"结果，窗口一过就该被清掉
	limiter.sweepLocked(now.Add(limiter.ipWindow + time.Second))
	if len(limiter.perIP) != 0 {
		t.Fatalf("empty buckets should be reclaimed, %d left", len(limiter.perIP))
	}
}

// ===== 来源标识 =====

func TestClientKey(t *testing.T) {
	cases := []struct{ in, want string }{
		{"203.0.113.7:54321", "203.0.113.7"},
		{"[2001:db8::1]:443", "2001:db8::1"},
		{"203.0.113.7", "203.0.113.7"},
		{"", ""},          // 认不出来 → 空（走全局总闸）
		{"garbage:x", ""}, // 不是合法 IP → 空
	}
	for _, c := range cases {
		if got := clientKey(c.in); got != c.want {
			t.Errorf("clientKey(%q) = %q, want %q", c.in, got, c.want)
		}
	}
}

func TestIsExemptFromLimit(t *testing.T) {
	if !isExemptFromLimit("127.0.0.1") {
		t.Fatal("loopback should be exempt")
	}
	if !isExemptFromLimit("::1") {
		t.Fatal("IPv6 loopback should be exempt")
	}
	if isExemptFromLimit("203.0.113.7") {
		t.Fatal("public IP should not be exempt")
	}
	if isExemptFromLimit("") {
		t.Fatal("empty key should not be exempt")
	}
}
