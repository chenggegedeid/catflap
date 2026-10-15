package auth

import (
	"net"
	"sync"
	"time"
)

const (
	// 单个来源：3 分钟内最多 13 次失败
	perIPWindow = 3 * time.Minute
	perIPMax    = 13

	// 全局总闸：3 分钟内最多 200 次失败。
	//
	// 这是**换 IP 的分布式爆破**的最后一道闸。不能太小 —— 它一旦触发
	// 会把所有人都挡住，包括管理员自己。
	globalWindow = 3 * time.Minute
	globalMax    = 200
)

// ===================== 按来源限速 =====================
//
// 为什么要按来源分桶而不是只用一个全局计数器：
//
// 全局计数的毛病是**管理员会把自己锁在外面**。管理页一旦暴露到公网被
// 持续爆破，13 次的名额被占满，你自己也登不进去 —— 攻击者用很小代价
// 就完成了 DoS。
//
// 分桶之后：攻击者消耗的是他自己那个 IP 的名额，管理员从另一个 IP
// 进来不受影响。全局只留一个宽松很多的总闸兜底分布式爆破。

type bucketState struct {
	failures []time.Time
	inFlight int
}

type loginFailureLimiter struct {
	mu       sync.Mutex
	perIP    map[string]*bucketState // 键是 clientKey：IP 或 IP+用户名
	global   bucketState
	ipWindow time.Duration
	ipLimit  int
	gWindow  time.Duration
	gLimit   int
}

type loginAttemptResult uint8

const (
	loginAttemptIgnored loginAttemptResult = iota
	loginAttemptFailed
	loginAttemptSucceeded
)

var globalLoginFailureLimiter = newLoginFailureLimiter()

func newLoginFailureLimiter() *loginFailureLimiter {
	return &loginFailureLimiter{
		perIP:    map[string]*bucketState{},
		ipWindow: perIPWindow,
		ipLimit:  perIPMax,
		gWindow:  globalWindow,
		gLimit:   globalMax,
	}
}

// begin 为本次登录预留名额。返回 false 表示已超限，调用方应直接拒绝。
//
// key 是来源标识（见 clientKey）。空 key 表示"无法判断来源"（比如内部调用），
// 此时只受全局总闸约束 —— 不能因为认不出来源就放行，也不能因此拒绝。
func (l *loginFailureLimiter) begin(key string, now time.Time) bool {
	l.mu.Lock()
	defer l.mu.Unlock()

	l.sweepLocked(now)

	if l.global.inFlight+len(l.global.failures) >= l.gLimit {
		return false
	}

	if key != "" {
		b := l.bucketLocked(key)
		if b.inFlight+len(b.failures) >= l.ipLimit {
			return false
		}
		b.inFlight++
	}
	l.global.inFlight++
	return true
}

// finish 结束预留。失败则记一笔，成功则把这个来源的失败记录清掉
// （登录成功说明这个来源是可信的，不该还留着旧账）。
func (l *loginFailureLimiter) finish(result loginAttemptResult, key string, now time.Time) {
	l.mu.Lock()
	defer l.mu.Unlock()

	if l.global.inFlight > 0 {
		l.global.inFlight--
	}
	if key != "" {
		if b, ok := l.perIP[key]; ok && b.inFlight > 0 {
			b.inFlight--
		}
	}

	switch result {
	case loginAttemptFailed:
		l.global.failures = append(l.global.failures, now)
		if key != "" {
			b := l.bucketLocked(key)
			b.failures = append(b.failures, now)
		}
	case loginAttemptSucceeded:
		if key != "" {
			if b, ok := l.perIP[key]; ok {
				b.failures = b.failures[:0]
			}
		}
	}
}

func (l *loginFailureLimiter) bucketLocked(key string) *bucketState {
	b := l.perIP[key]
	if b == nil {
		b = &bucketState{}
		l.perIP[key] = b
	}
	return b
}

// sweepLocked 清掉窗口外的失败记录，并顺手回收空的桶 ——
// 扫描器换 IP 会留下大量一次性桶，不回收这张表会一直涨。
func (l *loginFailureLimiter) sweepLocked(now time.Time) {
	cutoff := now.Add(-l.ipWindow)
	for k, b := range l.perIP {
		b.failures = keepAfter(b.failures, cutoff)
		if len(b.failures) == 0 && b.inFlight == 0 {
			delete(l.perIP, k)
		}
	}
	l.global.failures = keepAfter(l.global.failures, now.Add(-l.gWindow))
}

func keepAfter(ts []time.Time, cutoff time.Time) []time.Time {
	active := ts[:0]
	for _, t := range ts {
		if t.After(cutoff) {
			active = append(active, t)
		}
	}
	return active
}

// ===================== 来源标识 =====================

// clientKey 登录限速用的来源键。
//
// **只用 IP，不掺用户名**：掺进去的话，攻击者把用户名换成随机值就能
// 每次拿到一个全新的桶，限速形同虚设。
//
// IP 取自调用方传入的 remoteAddr（TCP 对端），**不读 X-Forwarded-For** ——
// 那个头攻击者能伪造，用它分桶等于把钥匙交出去（见 middleware/enter.go 里
// SetTrustedProxies(nil) 的说明）。
func clientKey(remoteAddr string) string {
	host := remoteAddr
	if h, _, err := net.SplitHostPort(remoteAddr); err == nil {
		host = h
	}
	ip := net.ParseIP(host)
	if ip == nil {
		// 认不出来就返回空：让调用方走"只受总闸约束"的分支，
		// 既不误放行也不误拒
		return ""
	}
	return ip.String()
}

// isExemptFromLimit 这个来源是否不限速。
//
// 本机（回环）和管理员配的白名单不限 —— 自己调试时输错密码不该被锁。
func isExemptFromLimit(key string) bool {
	if key == "" {
		return false
	}
	ip := net.ParseIP(key)
	return ip != nil && ip.IsLoopback()
}
