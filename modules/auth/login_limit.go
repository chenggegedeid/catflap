package auth

import (
	"net"
	"sync"
	"time"

	"github.com/chenggegedeid/catflap/modules/audit"
	"github.com/sirupsen/logrus"
)

// 本文件分两层，别混：
//
//   - 上层的 begin / finish 是**纯粹的**计数与封禁逻辑，不碰任何全局模块，
//     所以单元测试可以独立跑（一个用例不会污染另一个）。
//   - 下面的 AllowlistContains / LoginBlockRemaining / UnblockIP 是给接口层
//     用的便利函数，它们需要读审计模块的封禁表与白名单 —— 那是唯一
//     「登录限速」和「异常访问拉黑」的交汇点。
//
// 之所以让两个功能共用审计那份封禁列表：只有一处能管。用户封了这里
// 漏了那里，或者界面上看不见自动封的来源，都是同一类事故。

// 默认阈值（配置里没填时用这些）
const (
	defaultLoginFailMax      = 5
	defaultLoginBlockMinutes = 60
)

const (
	// 单个来源：窗口内允许的失败次数，达到即**封禁**（不是"稍后重试"的软限）
	perIPWindow = 3 * time.Minute
	perIPMax    = defaultLoginFailMax

	// 触发后封多久
	blockFor = defaultLoginBlockMinutes * time.Minute

	// 全局总闸：3 分钟内最多 200 次失败。
	//
	// 这是**换 IP 的分布式爆破**的最后一道闸。不能太小 —— 它一旦触发
	// 会把所有人都挡住，包括管理员自己（白名单来源豁免，见 isExempt）。
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

	// blocked 被封的来源 → 解封时间。
	//
	// **注意**：真正的封禁记录已经统一写到审计模块（`audit.BlockAuto`），
	// 和「异常访问自动拉黑」共用一份列表 —— 只有一处能管、审计页上看得见。
	//
	// 这张表只是**兜底**：审计模块还没初始化完（启动早期）时用它顶一下，
	// 免得登录限速在这段空窗期形同虚设。审计可用时以审计为准。
	blocked   map[string]time.Time
	blockFor  time.Duration
	allowlist []net.IPNet // 已废弃，保留字段只为兼容旧调用；豁免改用审计的 Whitelist
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
		perIP:     map[string]*bucketState{},
		ipWindow:  perIPWindow,
		ipLimit:   perIPMax,
		gWindow:   globalWindow,
		gLimit:    globalMax,
		blocked:   map[string]time.Time{},
		blockFor:  blockFor,
		allowlist: nil,
	}
}

// ApplyConfig 把配置里的阈值灌进限流器。
//
// 改密码 / 改设置之后要重新调一次，否则新值要等下次重启才生效。
//
// **白名单不在这里**：豁免名单用的是审计模块的 Whitelist，见 isExemptFromLimit。
func ApplyConfig(cfg Config) {
	limit := cfg.LoginFailMax
	if limit <= 0 {
		limit = defaultLoginFailMax
	}
	minutes := cfg.LoginBlockMinutes
	if minutes <= 0 {
		minutes = defaultLoginBlockMinutes
	}

	l := globalLoginFailureLimiter
	l.mu.Lock()
	defer l.mu.Unlock()
	l.ipLimit = limit
	l.blockFor = time.Duration(minutes) * time.Minute
}

// isBlocked 这个来源是不是正在被封（只看本进程内的记录）。
//
// **审计里那份由调用方（Login）先查**：限流器不直接依赖 audit，
// 否则单元测试会碰全局状态，一个用例污染另一个。职责分开之后，
// 这里只管"数次数、到阈值就封"，持久化交给调用方。
func (l *loginFailureLimiter) isBlocked(key string, now time.Time) bool {
	if key == "" {
		return false
	}
	until, ok := l.blocked[key]
	if !ok {
		return false
	}
	if !now.Before(until) {
		delete(l.blocked, key) // 到点了，放出来
		return false
	}
	return true
}

// Unblock 手动解封（白名单来源可用，作为"把自己锁在外面"的救援手段）
func (l *loginFailureLimiter) Unblock(key string) {
	l.mu.Lock()
	defer l.mu.Unlock()
	delete(l.blocked, key)
	if b := l.perIP[key]; b != nil {
		b.failures = b.failures[:0]
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

	// 白名单先看：它连全局总闸都豁免。
	// 理由见 Config.AdminAllowlist 的注释 —— 不豁免的话，别人一直爆破
	// 会把管理员自己也挡在总闸外面。
	if l.isExemptLocked(key) {
		return true
	}

	// 已被封：直接拒绝，且不消耗任何配额（否则封禁会被"尝试"磨掉）
	if l.isBlocked(key, now) {
		return false
	}

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
//
// 返回 true 表示**这一次正好达到阈值、刚把这个来源封掉** ——
// 调用方据此把封禁写进审计模块（持久 + 和异常访问共用一份列表）。
// 限流器自己不写，是为了保持纯粹、可单测。
func (l *loginFailureLimiter) finish(result loginAttemptResult, key string, now time.Time) bool {
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

	var failuresAtBlock int
	switch result {
	case loginAttemptFailed:
		l.global.failures = append(l.global.failures, now)
		if key != "" && !l.isExemptLocked(key) {
			b := l.bucketLocked(key)
			b.failures = append(b.failures, now)
			// 达到阈值就封掉，而不是等到窗口自然过去。
			//
			// 以前是「3 分钟 13 次」的软限：撑过 3 分钟又能接着试。
			// 爆破者只要放慢节奏（比如每分钟 4 次）就能无限试下去。
			// 封禁之后，继续试也不会让解封时间提前。
			if len(b.failures) >= l.ipLimit {
				// 进程内先封：立刻生效，不等调用方去写审计。
				// 调用方（Login）拿到 true 之后会把这条写进审计模块持久化。
				l.blocked[key] = now.Add(l.blockFor)
				failuresAtBlock = len(b.failures)
				b.failures = b.failures[:0]
				logrus.Warnf("[auth] 来源 %s 登录失败达到 %d 次，封禁至 %s",
					key, l.ipLimit, now.Add(l.blockFor).Format("15:04:05"))
			}
		}
	case loginAttemptSucceeded:
		if key != "" {
			if b, ok := l.perIP[key]; ok {
				b.failures = b.failures[:0]
			}
		}
	}
	return failuresAtBlock > 0
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
// 三类豁免：本机回环（自己调试时不该被锁）、以及配置里的白名单。
// 白名单是安全设施而非便利设施 —— 详见 Config.AdminAllowlist 的说明。
func isExemptFromLimit(key string) bool {
	l := globalLoginFailureLimiter
	l.mu.Lock()
	defer l.mu.Unlock()
	return l.isExemptLocked(key)
}

// isExemptLocked 调用方必须已持有锁
func (l *loginFailureLimiter) isExemptLocked(key string) bool {
	if key == "" {
		return false
	}
	ip := net.ParseIP(key)
	if ip == nil {
		return false
	}
	if ip.IsLoopback() {
		return true
	}
	// 白名单用的是审计模块那一份 —— 两处各配各的，用户永远搞不清该填哪个。
	return audit.IsWhitelisted(ip.String())
}

// AllowlistContains 这个 IP 在不在白名单里（接口层判断"能不能解封别人"用）
func AllowlistContains(ipStr string) bool {
	return isExemptFromLimit(ipStr)
}

// LoginBlockRemaining 这个来源还要被封多久。
//
// 给接口层生成「还要等 X 分钟」的准确提示 ——
// 一句"请稍后再试"会让人反复刷新，而反复刷新在我们这边看起来和爆破没区别。
func LoginBlockRemaining(remoteAddr string) time.Duration {
	key := clientKey(remoteAddr)
	if key == "" {
		return 0
	}
	// 优先看审计那份（真正的记录在那里）
	if entry, ok := audit.IsBlocked(key); ok && !entry.ExpiresAt.IsZero() {
		left := time.Until(entry.ExpiresAt)
		if left > 0 {
			return left
		}
		return 0
	}
	l := globalLoginFailureLimiter
	l.mu.Lock()
	defer l.mu.Unlock()
	until, ok := l.blocked[key]
	if !ok {
		return 0
	}
	left := time.Until(until)
	if left <= 0 {
		return 0
	}
	return left
}

// UnblockIP 解封一个来源（救援用）。**删掉审计里的记录**才是真的解封，
// 本地兜底表只是保险。
func UnblockIP(ipStr string) {
	// 审计的 Unblock 会校验白名单；这里是管理员主动救援，直接走 Runtime 删除
	_ = audit.Unblock(ipStr)
	globalLoginFailureLimiter.Unblock(ipStr)
}
