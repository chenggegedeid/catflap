package audit

import (
	"testing"
	"time"
)

// TestIsBlockedSupportsCIDR 封禁网段要真的能拦住网段里的 IP。
//
// # 这条守的是一个真实 BUG
//
// 以前 Block() 的校验是 parseCIDROrIP（单个 IP **或** CIDR 都收），
// 所以填 "10.0.0.0/8" 能通过校验并写进配置；但 rebuildIndexes 拿
// b.IP 当 map 的 key 存，IsBlocked 又是**精确字符串比对** ——
// 于是查 "10.0.0.5" 时永远查不到那个 key 为 "10.0.0.0/8" 的条目。
//
// 表现极其隐蔽：封禁列表里明明看得到那条记录，界面也不报错，
// 但那个网段照样能访问服务。用户只会觉得"网段功能没用"。
func TestIsBlockedSupportsCIDR(t *testing.T) {
	Runtime.mu.Lock()
	Runtime.blocked = map[string]BlockEntry{}
	Runtime.blockedNets = []blockedNet{}
	Runtime.whitelist = nil
	Runtime.mu.Unlock()

	entry := BlockEntry{IP: "10.0.0.0/8", Reason: "测试网段", AddedAt: time.Now()}
	rebuildIndexes(&Config{Blocks: []BlockEntry{entry}})

	if _, ok := IsBlocked("10.0.0.5"); !ok {
		t.Error("封禁了 10.0.0.0/8，但 10.0.0.5 没被拦住")
	}
	if _, ok := IsBlocked("10.255.255.254"); !ok {
		t.Error("封禁了 10.0.0.0/8，但 10.255.255.254 没被拦住")
	}
	// 网段外不该被误伤
	if _, ok := IsBlocked("192.168.1.1"); ok {
		t.Error("192.168.1.1 不在 10.0.0.0/8 里，不该被拦")
	}
}

// TestIsBlockedIPv6CIDR IPv6 网段同理。
func TestIsBlockedIPv6CIDR(t *testing.T) {
	Runtime.mu.Lock()
	Runtime.blocked = map[string]BlockEntry{}
	Runtime.blockedNets = []blockedNet{}
	Runtime.whitelist = nil
	Runtime.mu.Unlock()

	entry := BlockEntry{IP: "2409:8a60::/32", Reason: "测试 IPv6 网段", AddedAt: time.Now()}
	rebuildIndexes(&Config{Blocks: []BlockEntry{entry}})

	if _, ok := IsBlocked("2409:8a60:1234::1"); !ok {
		t.Error("封禁了 2409:8a60::/32，但网段内的 IPv6 没被拦住")
	}
	if _, ok := IsBlocked("2400:3200::1"); ok {
		t.Error("2400:3200::1 不在该网段里，不该被拦")
	}
}

// TestIsBlockedWhitelistWins 白名单优先于封禁。
//
// 这条必须成立：自动拉黑可能误伤，白名单是管理员防止把自己关在门外的兜底。
func TestIsBlockedWhitelistWins(t *testing.T) {
	Runtime.mu.Lock()
	Runtime.blocked = map[string]BlockEntry{}
	Runtime.blockedNets = []blockedNet{}
	Runtime.whitelist = nil
	Runtime.mu.Unlock()

	cfg := &Config{
		Blocks:    []BlockEntry{{IP: "203.0.113.0/24", Reason: "整段封禁"}},
		Whitelist: []string{"203.0.113.7"},
	}
	rebuildIndexes(cfg)

	if _, ok := IsBlocked("203.0.113.7"); ok {
		t.Error("白名单里的地址不该被封禁拦住")
	}
	if _, ok := IsBlocked("203.0.113.9"); !ok {
		t.Error("同网段但不在白名单里的地址应当被拦")
	}
}

// TestIsBlockedExpiredIgnored 过期条目不生效。
func TestIsBlockedExpiredIgnored(t *testing.T) {
	Runtime.mu.Lock()
	Runtime.blocked = map[string]BlockEntry{}
	Runtime.blockedNets = []blockedNet{}
	Runtime.whitelist = nil
	Runtime.mu.Unlock()

	old := BlockEntry{
		IP:        "198.51.100.0/24",
		Reason:    "过期",
		AddedAt:   time.Now().Add(-48 * time.Hour),
		ExpiresAt: time.Now().Add(-1 * time.Hour),
	}
	rebuildIndexes(&Config{Blocks: []BlockEntry{old}})

	if _, ok := IsBlocked("198.51.100.5"); ok {
		t.Error("过期的封禁条目不该继续拦请求")
	}
}
