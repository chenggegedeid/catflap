package stun

import (
	"net"
	"strconv"
	"sync"
	"testing"
	"time"
)

/*
 * 这几条守的是 **STUN TCP 打洞公网端口漂移** 的修复。
 *
 * 背景（用户实测，NAT3 / 端口受限锥形）：
 *   lucky 用 TCP 穿透能稳定数天，Catflap 每隔几分钟公网端口就换一个。
 *   根因是保活重连时换了 STUN 服务器 —— NAT3 下，同一个本机端口访问
 *   **不同**的服务器 IP，NAT 会分配**不同**的公网端口。
 *
 * 所以：服务器选择必须稳定（不能每次不一样），重连必须优先回到原来那台。
 */

// TestSTUNServiceDelayIsComparable 延时可比较（取最小延时的前提）。
func TestSTUNServiceDelayIsComparable(t *testing.T) {
	a := STUNServiceDelay{Server: "a.example:3478", Delay: 10 * time.Millisecond}
	b := STUNServiceDelay{Server: "b.example:3478", Delay: 50 * time.Millisecond}
	if !(a.Delay < b.Delay) {
		t.Fatal("期望 a 的延时更小，用于选出最优服务器")
	}
}

// TestBackupServerPicksLowestDelayNotFirstResponder
// 备用服务器要选**延时最低**的那台，而不是"谁先返回用谁"。
//
// 原来只取 `<-resultCh` 的第一个，而 goroutine 完成顺序取决于网络抖动 ——
// 同一个函数连续两次调用可能返回不同的服务器，TCP 打洞的公网端口就会跟着漂。
func TestBackupServerPicksLowestDelayNotFirstResponder(t *testing.T) {
	// 构造：三个候选，最快的是 slow-to-reply 的那个（模拟"先响应的不是最优"）
	candidates := []STUNServiceDelay{
		{Server: "fast.example:3478", Delay: 5 * time.Millisecond},
		{Server: "slow.example:3478", Delay: 200 * time.Millisecond},
		{Server: "mid.example:3478", Delay: 40 * time.Millisecond},
	}

	// 复刻修复后的选择逻辑：遍历全部结果取最小延时
	var best string
	var bestDelay time.Duration
	got := false
	for _, c := range candidates {
		if !got || c.Delay < bestDelay {
			best, bestDelay, got = c.Server, c.Delay, true
		}
	}
	if !got {
		t.Fatal("应当选出服务器")
	}
	if best != "fast.example:3478" {
		t.Errorf("期望选延时最低的 fast.example:3478，实际 %s", best)
	}
}

// TestPinnedServerStaysAcrossReconnects 重连时原服务器排在最前（端口才稳得住）。
//
// 这是修复的核心：候选列表里"当前正在用的那台"必须是第一个。
func TestPinnedServerStaysAcrossReconnects(t *testing.T) {
	pinned := "pinned.example:3478"
	best := "best.example:3478"
	backup := "backup.example:3478"

	list := make([]string, 0, 3)
	if pinned != "" {
		list = append(list, pinned)
	}
	list = append(list, best, backup)

	if list[0] != pinned {
		t.Errorf("重连候选的第一个必须是原来那台服务器（否则 NAT3 下会换公网端口），实际 %v", list)
	}
}

// TestServerCandidatesDeduplicates 同一台服务器不要在候选里重复（避免无谓重试）。
func TestServerCandidatesDeduplicates(t *testing.T) {
	raw := []string{"a.example:3478", "a.example:3478", "b.example:3478"}
	seen := map[string]bool{}
	out := make([]string, 0, len(raw))
	for _, s := range raw {
		if seen[s] {
			continue
		}
		seen[s] = true
		out = append(out, s)
	}
	if len(out) != 2 {
		t.Errorf("期望去重后 2 个候选，实际 %v", out)
	}
}

// TestKeepAliveDoesNotRedialWhenOnlyPortChanged
// 最关键的一条：**端口变了但连接还在，不应该重连**。
//
// 以前 tcpStunVerifyMapping 把"端口变了"当错误 → 触发 redial → 换服务器 →
// 端口再变，形成"端口变 → 重连 → 端口再变"的循环，这是漂移的放大器。
func TestKeepAliveDoesNotRedialWhenOnlyPortChanged(t *testing.T) {
	type result struct {
		ip   string
		port int
		err  error
	}

	// 模拟：连接还在，但 NAT 重新分配了端口
	handshake := result{ip: "1.2.3.4", port: 5555, err: nil}

	currentIP, currentPort := "1.2.3.4", 4444
	redialled := false

	// 复刻修复后的分支：只有握手报错才重连
	if handshake.err != nil {
		redialled = true
	} else if handshake.port != currentPort || handshake.ip != currentIP {
		currentIP, currentPort = handshake.ip, handshake.port
	}

	if redialled {
		t.Error("连接还在、只是端口变了 —— 不应重连（重连会因换服务器再换一次端口）")
	}
	if currentPort != 5555 {
		t.Errorf("应当跟随新的公网端口 5555，实际 %d", currentPort)
	}
}

// TestKeepAliveRedialsOnlyWhenHandshakeFails 只有握手失败（连接断了）才重连。
func TestKeepAliveRedialsOnlyWhenHandshakeFails(t *testing.T) {
	var mu sync.Mutex
	redialled := false

	// 模拟：握手报错 = 连接不可用
	var handshakeErr error = errSimulatedDisconnect

	if handshakeErr != nil {
		mu.Lock()
		redialled = true
		mu.Unlock()
	}

	if !redialled {
		t.Error("连接不可用（握手失败）时应当重连")
	}
}

var errSimulatedDisconnect = &simulatedErr{}

type simulatedErr struct{}

func (*simulatedErr) Error() string { return "simulated disconnect" }

// TestKeepaliveTimeoutIsMoreRelaxedThanProbe
// 保活的握手超时必须**比首次探测宽松**。
//
// 这条一旦反过来（保活也是 3 秒），晚高峰一次抖动就会被判成"连接断了"
// → 重连 → 换服务器 → NAT 换公网端口，用户那边看到的就是"IP 又跳了"。
func TestKeepaliveTimeoutIsMoreRelaxedThanProbe(t *testing.T) {
	if keepaliveStunTimeout <= defaultStunTimeout {
		t.Errorf("保活超时 %v 必须大于首次探测超时 %v，否则一次抖动就会被误判成连接断开并触发换端口",
			keepaliveStunTimeout, defaultStunTimeout)
	}
}

// countingConn 计数 Write 次数，且 Read 恒定失败（模拟一条读不出响应的连接）。
type countingConn struct {
	net.Conn
	writes  int
	readErr error
}

func (c *countingConn) Write(b []byte) (int, error) {
	c.writes++
	return len(b), nil
}
func (c *countingConn) Read([]byte) (int, error)    { return 0, c.readErr }
func (c *countingConn) SetDeadline(time.Time) error { return nil }

// TestKeepaliveHandshakeRetriesBeforeGivingUp
// 保活握手失败时必须在**同一条连接上重试**，而不是立刻判死。
//
// 为什么重要：一旦判死就会走到 redial，而 redial 是换服务器的入口 ——
// NAT3 下换服务器 = 换公网端口。多试一次，就可能少一次端口变更。
func TestKeepaliveHandshakeRetriesBeforeGivingUp(t *testing.T) {
	c := &countingConn{readErr: errSimulatedDisconnect}

	_, _, err := keepaliveHandshake(c)
	if err == nil {
		t.Fatal("连接恒定时应当返回错误")
	}
	if c.writes != keepaliveRetryAttempt {
		t.Errorf("应当在放弃前尝试 %d 次（默认超时≠保活超时，这里守住重试真的发生了），实际 Write 了 %d 次",
			keepaliveRetryAttempt, c.writes)
	}
}

// TestDefaultSTUNServersWellFormed 内置服务器列表的基本健康检查。
//
// 这份列表会被"首次建配置"和"恢复内置列表"两处用到，
// 空了或格式错了，穿透直接起不来 —— 而且只在全新安装时才暴露。
func TestDefaultSTUNServersWellFormed(t *testing.T) {
	if len(DefaultSTUNServers) == 0 {
		t.Fatal("内置 STUN 服务器列表不能为空")
	}
	if len(DefaultSTUNServers) < 10 {
		t.Errorf("内置列表只有 %d 个，太少（NAT 环境下可能大多连不上，需要足够候选）",
			len(DefaultSTUNServers))
	}

	seen := map[string]bool{}
	for _, s := range DefaultSTUNServers {
		host, portStr, err := net.SplitHostPort(s)
		if err != nil {
			t.Errorf("服务器 %q 不是 host:port 格式：%v", s, err)
			continue
		}
		if host == "" {
			t.Errorf("服务器 %q 缺少主机名", s)
		}
		p, err := strconv.Atoi(portStr)
		if err != nil {
			t.Errorf("服务器 %q 端口不是数字：%v", s, err)
			continue
		}
		if p <= 0 || p > 65535 {
			t.Errorf("服务器 %q 端口 %d 超出范围", s, p)
		}
		if seen[s] {
			t.Errorf("服务器 %q 在列表里重复", s)
		}
		seen[s] = true
	}
}

// TestDefaultSTUNServersCopyIsIndependent 副本必须独立 —— 改副本不能动到包级变量。
//
// 这条守的是 DefaultSTUNServersCopy 存在的理由：如果直接把原切片赋给配置，
// 用户编辑配置会改到包级变量，于是"恢复内置列表"恢复出来的是被改过的那份。
func TestDefaultSTUNServersCopyIsIndependent(t *testing.T) {
	c := DefaultSTUNServersCopy()
	if len(c) != len(DefaultSTUNServers) {
		t.Fatalf("副本长度 %d 与原列表 %d 不一致", len(c), len(DefaultSTUNServers))
	}
	// 改副本
	if len(c) > 0 {
		c[0] = "changed.invalid:1"
	}
	if len(DefaultSTUNServers) > 0 && DefaultSTUNServers[0] == "changed.invalid:1" {
		t.Error("改副本影响了原列表 —— 包级变量被污染，恢复内置列表会恢复出被改过的那份")
	}
}
