package audit

import (
	"context"
	"encoding/json"
	"net"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"sync/atomic"
	"time"

	"github.com/chenggegedeid/catflap/core"
	"github.com/chenggegedeid/catflap/core/component"

	"github.com/sirupsen/logrus"
)

// Name 组件名
const Name = "audit"

// enabled 审计总开关。热路径（每个请求）上读，所以用 atomic。
var enabled = atomic.Bool{}

type auditComponent struct {
	component.LocalSwitch
}

func (auditComponent) Name() string { return Name }

func (auditComponent) Init(context.Context) error {
	cfg, err := ReadConfig()
	if err != nil {
		return err
	}
	Runtime.mu.Lock()
	Runtime.Config = cfg
	Runtime.mu.Unlock()

	// 封禁表与白名单要进内存索引（热路径无锁读）
	rebuildIndexes(&cfg)

	if err := loadIPStats(); err != nil {
		logrus.Warnf("[audit] 读取 IP 统计失败（从零开始）：%v", err)
	}

	core.OnShutdown(func() {
		Flush()
	})

	enabled.Store(cfg.Enabled)

	go ticker()
	logrus.Infof("[audit] 已启动，封禁表 %d 条，白名单 %d 条", len(cfg.Blocks), len(cfg.Whitelist))
	return nil
}

func init() {
	component.MustRegister(auditComponent{
		LocalSwitch: component.NewLocalSwitch(Name),
	})
}

// ticker 每分钟落盘 + 清理过期封禁 + 清理过期文件 + 自动拉黑判定
func ticker() {
	t := time.NewTicker(time.Minute)
	defer t.Stop()
	for range t.C {
		Flush()
		cleanOldFiles()
		pruneExpiredBlocks()
		runAutoBlock()
	}
}

// loadIPStats 读回按 IP 的统计
func loadIPStats() error {
	data, err := os.ReadFile(ipStatsPath)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return err
	}
	var payload struct {
		Stats []IPStat `json:"stats"`
	}
	if err := json.Unmarshal(data, &payload); err != nil {
		return err
	}

	Runtime.mu.Lock()
	for i := range payload.Stats {
		st := payload.Stats[i]
		// 旧版本落盘时穿透 IP 的 paths/hosts/UA 会是 null（穿透没有这些字段），
		// 读回来就是 nil —— 这里兜底成空数组，下次落盘文件自动修好。
		if st.Paths == nil {
			st.Paths = []string{}
		}
		if st.Hosts == nil {
			st.Hosts = []string{}
		}
		if st.UserAgents == nil {
			st.UserAgents = []string{}
		}
		Runtime.ipStats[st.IP] = &st
	}
	Runtime.mu.Unlock()
	return nil
}

// cleanOldFiles 删过期明细。IP 聚合不删（那是累计值）。
func cleanOldFiles() {
	days := Runtime.Snapshot().KeepDays
	if days <= 0 {
		days = defaultKeepDays
	}
	cutoff := time.Now().AddDate(0, 0, -days)

	for _, d := range Days() {
		t, err := time.Parse("2006-01-02", d)
		if err != nil {
			continue
		}
		if t.Before(cutoff) {
			_ = os.Remove(filepath.Join(auditDir, d+".jsonl"))
		}
	}
}

// pruneExpiredBlocks 把已过期的临时封禁从配置里摘掉
func pruneExpiredBlocks() {
	cfg := Runtime.Snapshot()
	now := time.Now()

	var kept []BlockEntry
	var dropped []string
	for _, b := range cfg.Blocks {
		if b.Expired(now) {
			dropped = append(dropped, b.IP)
			continue
		}
		kept = append(kept, b)
	}
	if len(dropped) == 0 {
		return
	}

	if err := Runtime.Update(func(c *Config) error {
		c.Blocks = kept
		return nil
	}); err != nil {
		logrus.Warnf("[audit] 清理过期封禁失败：%v", err)
		return
	}
	logrus.Infof("[audit] 已解除 %d 个到期封禁：%s", len(dropped), strings.Join(dropped, ", "))
}

// ===================== 封禁操作 =====================

// Block 手动封禁一个 IP（或网段）。
//
// minutes > 0 是临时封禁；0 表示永久。
func Block(ip, reason string, minutes int) error {
	ip = strings.TrimSpace(ip)
	if ip == "" {
		return errEmptyIP
	}
	// 只收单个 IP 或 CIDR：别的形态（域名、通配符）封不了，
	// 直接报错比默默不生效好
	if parseCIDROrIP(ip) == nil {
		return errBadIP
	}

	entry := BlockEntry{
		IP:      ip,
		Reason:  strings.TrimSpace(reason),
		AddedAt: time.Now(),
		Auto:    false,
	}
	if minutes > 0 {
		entry.ExpiresAt = time.Now().Add(time.Duration(minutes) * time.Minute)
	}

	cfg := Runtime.Snapshot()
	if matchAny(parseNets(cfg.Whitelist), ip) {
		return errWhitelisted
	}

	if err := Runtime.Update(func(c *Config) error {
		// 已存在就更新（改成新的原因/时长），别塞两条一样的
		for i := range c.Blocks {
			if c.Blocks[i].IP == ip {
				c.Blocks[i] = entry
				return nil
			}
		}
		c.Blocks = append(c.Blocks, entry)
		return nil
	}); err != nil {
		return err
	}
	// 立刻同步内存里的展示字段：否则界面上要等到下次落盘才显示"已封禁"，
	// 用户点了按钮看不到变化，会以为没生效
	syncBlockedFlags()
	logrus.Infof("[audit] 已封禁 %s（%s）", ip, reason)
	return nil
}

// Unblock 解除封禁
func Unblock(ip string) error {
	defer syncBlockedFlags()
	return Runtime.Update(func(c *Config) error {
		kept := make([]BlockEntry, 0, len(c.Blocks))
		for _, b := range c.Blocks {
			if b.IP != ip {
				kept = append(kept, b)
			}
		}
		c.Blocks = kept
		return nil
	})
}

// ListBlocks 当前生效的封禁
func ListBlocks() []BlockEntry {
	cfg := Runtime.Snapshot()
	now := time.Now()
	out := make([]BlockEntry, 0, len(cfg.Blocks))
	for _, b := range cfg.Blocks {
		if !b.Expired(now) {
			out = append(out, b)
		}
	}
	// 手动在前、自动在后（手动的通常更要紧），其次按加入时间倒序
	sortBlocks(out)
	return out
}

// syncBlockedFlags 把封禁状态刷进内存统计（界面上要显示"已封禁 + 原因"）
func syncBlockedFlags() {
	Runtime.mu.Lock()
	defer Runtime.mu.Unlock()
	for ip, st := range Runtime.ipStats {
		if b, ok := Runtime.blocked[ip]; ok {
			st.Blocked = true
			st.BlockReason = b.Reason
			st.BlockExpiresAt = b.ExpiresAt
		} else {
			st.Blocked = false
			st.BlockReason = ""
			st.BlockExpiresAt = time.Time{}
		}
	}
	Runtime.ipStatsDirty = true
}

// runAutoBlock 自动拉黑：把错误数超阈值的 IP 封一段时间。
//
// **默认关**，且必须配好白名单 —— 它很容易误伤（自己调试时刷出几百个
// 404 就能把自己封了）。
func runAutoBlock() {
	cfg := Runtime.Snapshot()
	if !cfg.AutoBlock {
		return
	}
	threshold := cfg.AutoBlockThreshold
	if threshold <= 0 {
		threshold = 500
	}
	mins := cfg.AutoBlockMinutes

	candidates := ListIPStats("errors", false, 0)
	for _, st := range candidates {
		if !st.Suspicious() || st.Errors < threshold {
			continue
		}
		if st.Blocked {
			continue // 已经封了
		}
		// 白名单兜底：自动拉黑绝不碰白名单里的地址
		if matchAny(parseNets(cfg.Whitelist), st.IP) {
			continue
		}

		entry := BlockEntry{
			IP:      st.IP,
			Reason:  "自动拉黑：" + strconv.FormatInt(st.Errors, 10) + " 次异常访问",
			AddedAt: time.Now(),
			Auto:    true,
		}
		if mins > 0 {
			entry.ExpiresAt = time.Now().Add(time.Duration(mins) * time.Minute)
		}

		if err := Runtime.Update(func(c *Config) error {
			for i := range c.Blocks {
				if c.Blocks[i].IP == st.IP {
					return nil
				}
			}
			c.Blocks = append(c.Blocks, entry)
			return nil
		}); err != nil {
			logrus.Warnf("[audit] 自动拉黑 %s 失败：%v", st.IP, err)
			continue
		}
		logrus.Warnf("[audit] 自动拉黑 %s：%s", st.IP, entry.Reason)
	}
}

// ===================== 小工具 =====================

func parseNets(list []string) []*net.IPNet {
	out := make([]*net.IPNet, 0, len(list))
	for _, s := range list {
		if n := parseCIDROrIP(s); n != nil {
			out = append(out, n)
		}
	}
	return out
}

// sortBlocks 手动封禁在前，其次按加入时间倒序。
// 手动的是用户明确要封的，更重要；自动的可能误伤，排在后面便于复查。
func sortBlocks(list []BlockEntry) {
	sort.Slice(list, func(i, j int) bool {
		if list[i].Auto != list[j].Auto {
			return !list[i].Auto
		}
		return list[i].AddedAt.After(list[j].AddedAt)
	})
}

// Enabled 审计是否开着。热路径上读，所以走 atomic。
func Enabled() bool { return enabled.Load() }

// SetEnabled 开关
func SetEnabled(on bool) { enabled.Store(on) }
