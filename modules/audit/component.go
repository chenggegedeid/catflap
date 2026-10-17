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

// Title 见 core/component.Describer。
func (auditComponent) Title() string { return "访问审计" }

// Summary 见 core/component.Describer。
func (auditComponent) Summary() string { return "记录谁访问了站点，自动封禁扫描者" }

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

	// 按天/按小时的聚合。读不到不要紧，读的时候会从明细重算
	LoadDayStats()

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

// loadIPStats 读回按 IP 的统计。
//
// 文件不在时从明细重建，而不是「从零开始」。
//
// 为什么这条重要：ipstats.json 是**缓存在内存里、每分钟才落盘**的。进程被
// kill -9、容器被回收、或者用户清了一次 data/ 里的缓存，文件就没了。
// 早期版本直接返回，结果是审计页上一片空白 —— 而明细明明都在
// （「已记录 IP 321」那个数字来自这里，访问记录却读得出来）。
// 明细是真相，缓存没了就重算，这是派生数据的本分。
func loadIPStats() error {
	data, err := os.ReadFile(ipStatsPath)
	if err != nil {
		if os.IsNotExist(err) {
			rebuildIPStatsFromRecords()
			return nil
		}
		return err
	}
	var payload struct {
		Stats []IPStat `json:"stats"`
	}
	if err := json.Unmarshal(data, &payload); err != nil {
		// 文件坏了也别放弃：明细还在，重建一次比让页面空着强
		logrus.Warnf("[audit] ipstats.json 解析失败，将从访问明细重建：%v", err)
		rebuildIPStatsFromRecords()
		return nil
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

	// 文件在但一条都没有：也走重建（可能是上一次 flush 写到一半被打断）
	if len(payload.Stats) == 0 {
		rebuildIPStatsFromRecords()
	}
	return nil

}

// rebuildIPStatsFromRecords 从访问明细重算按 IP 的聚合。
//
// 读保留期内的全部明细文件。这里可能慢（被扫过的机器一天几十 MB），
// 但只在「缓存丢了」这一种情况下发生，而且是在启动阶段 ——
// 慢一点比页面上什么也看不到强。
func rebuildIPStatsFromRecords() {
	days := Days()
	if len(days) == 0 {
		return
	}

	total := 0
	for _, day := range days {
		recs, err := ReadDay(day, "", 0)
		if err != nil {
			continue
		}
		for _, r := range recs {
			if r.IP == "" {
				continue
			}
			// 直接调 updateIPStats：聚合逻辑只有一份，重建和实时记录
			// 走的必须是同一个函数 —— 抄一遍迟早两边算得不一样
			updateIPStats(r)
			total++
		}
	}
	if total > 0 {
		logrus.Infof("[audit] 按 IP 统计缓存缺失，已从 %d 天的 %d 条明细重建", len(days), total)
	}
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

// BlockAuto 自动拉黑（区别于 Block 的手动）。
//
// 给别的模块用：登录失败达到阈值时，由 auth 调它把来源封掉。
// 走同一份封禁列表的好处是**只有一处能管** —— 审计页上看得见、标了「自动」、
// 也能手动解封。各模块自己存一份封禁表的话，用户封了这里漏了那里。
//
// 白名单是硬兜底：自动拉黑绝不碰白名单里的地址。
// 这条不能省 —— 没有它，别人只要知道管理员的出口 IP，故意触发几次，
// 就能把管理员自己关在门外（一次成本极低的 DoS）。
func BlockAuto(ip, reason string, minutes int) error {
	ip = strings.TrimSpace(ip)
	if ip == "" {
		return errEmptyIP
	}
	if parseCIDROrIP(ip) == nil {
		return errBadIP
	}

	cfg := Runtime.Snapshot()
	if IsWhitelisted(ip) {
		logrus.Debugf("[audit] %s 在白名单里，跳过自动拉黑", ip)
		return errWhitelisted
	}
	_ = cfg

	var entry BlockEntry
	entry.IP = ip
	entry.Reason = strings.TrimSpace(reason)
	entry.AddedAt = time.Now()
	entry.Auto = true
	if minutes > 0 {
		entry.ExpiresAt = time.Now().Add(time.Duration(minutes) * time.Minute)
	}

	if err := Runtime.Update(func(c *Config) error {
		for i := range c.Blocks {
			if c.Blocks[i].IP == ip {
				// 已存在就续期并更新原因，别塞两条一样的
				c.Blocks[i] = entry
				return nil
			}
		}
		c.Blocks = append(c.Blocks, entry)
		return nil
	}); err != nil {
		return err
	}
	syncBlockedFlags()
	logrus.Warnf("[audit] 自动拉黑 %s（%s）", ip, reason)
	return nil
}

// IsWhitelisted 这个 IP 是否在白名单（永不封禁）里。
//
// auth 那边也要用：登录限速的豁免名单就是这一份，
// 免得两处各配各的、用户不知道该填哪个。
func IsWhitelisted(ip string) bool {
	ip = strings.TrimSpace(ip)
	if ip == "" {
		return false
	}
	cfg := Runtime.Snapshot()
	return matchAny(parseNets(cfg.Whitelist), ip)
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
