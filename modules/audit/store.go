package audit

import (
	"encoding/json"
	"fmt"
	"net"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/chenggegedeid/catflap/utils/utilsFile"

	"github.com/sirupsen/logrus"
)

const (
	configPath = "config/audit.json"
	auditDir   = "data/access"
	// ipStatsPath 按 IP 的聚合。这个要长期留着（累计值），不能按天删。
	ipStatsPath = "data/access/ipstats.json"

	defaultKeepDays  = 30
	defaultMaxFileMB = 32
	// maxPathsPerIP 每个 IP 最多记多少条路径。无上限的话被扫一次
	// 就能让这个文件涨到几十 MB。
	maxPathsPerIP = 30
	maxHostsPerIP = 10
	maxUAsPerIP   = 5
)

// ===================== 运行时 =====================

var Runtime = &runtime{
	ipStats:   map[string]*IPStat{},
	blocked:   map[string]BlockEntry{},
	whitelist: nil,
}

type runtime struct {
	mu     sync.RWMutex
	Config Config

	// accum 当前分钟的累加器（写访问明细用）
	accumMu sync.Mutex
	accum   []Record
	accumAt int64

	// ipStats 按 IP 聚合统计。常驻内存 —— 它要按 IP 查（判断是否封禁）
	// 且更新频繁，每次读盘不现实。
	ipStats map[string]*IPStat
	// ipStatsDirty 有变更待落盘
	ipStatsDirty bool

	// blocked 全局封禁表。**常驻内存且无锁读**（热路径上每个请求都要查）
	blocked map[string]BlockEntry
	// whitelist 永不封禁的名单，同上
	whitelist []*net.IPNet
}

// Snapshot 配置快照
func (r *runtime) Snapshot() Config {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return r.Config
}

// Update 改配置并落盘
func (r *runtime) Update(mutator func(cfg *Config) error) error {
	r.mu.Lock()
	if err := mutator(&r.Config); err != nil {
		r.mu.Unlock()
		return err
	}
	snapshot := r.Config
	r.mu.Unlock()

	// 封禁表改了要重建内存索引（它是无锁读的）
	rebuildIndexes(&snapshot)
	return SaveConfig(snapshot)
}

func SaveConfig(cfg Config) error {
	cfg.UpdatedAt = time.Now()
	if cfg.Blocks == nil {
		cfg.Blocks = []BlockEntry{}
	}
	if cfg.Whitelist == nil {
		cfg.Whitelist = []string{}
	}
	if cfg.TrustedProxies == nil {
		cfg.TrustedProxies = []string{}
	}
	if err := utilsFile.EnsurePrivateDir(filepath.Dir(configPath)); err != nil {
		return err
	}
	data, err := json.MarshalIndent(cfg, "", "  ")
	if err != nil {
		return err
	}
	tmp := configPath + ".tmp"
	if err := os.WriteFile(tmp, data, utilsFile.PrivateFileMode); err != nil {
		return err
	}
	return os.Rename(tmp, configPath)
}

func ReadConfig() (Config, error) {
	data, err := os.ReadFile(configPath)
	if err != nil {
		if os.IsNotExist(err) {
			return Config{
				Enabled:            true,
				KeepDays:           defaultKeepDays,
				MaxFileMB:          defaultMaxFileMB,
				AutoBlock:          false, // 默认关：容易误伤
				AutoBlockThreshold: 500,
				AutoBlockMinutes:   60,
				Blocks:             []BlockEntry{},
				Whitelist:          []string{},
				TrustedProxies:     []string{},
				CreatedAt:          time.Now(),
			}, nil
		}
		return Config{}, err
	}
	var cfg Config
	if err := json.Unmarshal(data, &cfg); err != nil {
		return cfg, fmt.Errorf("解析 %s 失败: %w", configPath, err)
	}
	if cfg.KeepDays <= 0 {
		cfg.KeepDays = defaultKeepDays
	}
	if cfg.MaxFileMB <= 0 {
		cfg.MaxFileMB = defaultMaxFileMB
	}
	return cfg, nil
}

// rebuildIndexes 把封禁表与白名单重建进内存索引
func rebuildIndexes(cfg *Config) {
	blocked := make(map[string]BlockEntry, len(cfg.Blocks))
	now := time.Now()
	for _, b := range cfg.Blocks {
		if b.Expired(now) {
			continue // 过期的直接不进索引（但配置里留着，用户能看到历史）
		}
		blocked[b.IP] = b
	}

	var wl []*net.IPNet
	for _, w := range cfg.Whitelist {
		if n := parseCIDROrIP(w); n != nil {
			wl = append(wl, n)
		}
	}

	Runtime.mu.Lock()
	Runtime.blocked = blocked
	Runtime.whitelist = wl
	Runtime.mu.Unlock()
}

// IsBlocked 这个来源是否被封禁。**热路径**（每个请求都查），所以走无锁读。
func IsBlocked(ip string) (BlockEntry, bool) {
	Runtime.mu.RLock()
	defer Runtime.mu.RUnlock()

	// 白名单优先：即使被封禁也放行 —— 开了自动拉黑时这是防误伤的兜底，
	// 别把管理员自己关在外面
	if matchAny(Runtime.whitelist, ip) {
		return BlockEntry{}, false
	}
	b, ok := Runtime.blocked[ip]
	return b, ok
}

// ===================== 写访问记录 =====================

// RecordAccess 记一次访问。
//
// 走内存累加 + 每分钟落盘：被扫的时候一分钟能来几千个请求，
// 每个都写盘会把磁盘打满（而且统计本来也不需要那么细的粒度）。
func RecordAccess(r Record) {
	if !Runtime.isEnabled() {
		return
	}
	if r.Time.IsZero() {
		r.Time = time.Now()
	}

	updateIPStats(r)

	Runtime.accumMu.Lock()
	Runtime.accum = append(Runtime.accum, r)
	Runtime.accumMu.Unlock()
}

func (r *runtime) isEnabled() bool {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return r.Config.Enabled
}

// updateIPStats 更新按 IP 的聚合
func updateIPStats(r Record) {
	Runtime.mu.Lock()
	defer Runtime.mu.Unlock()

	st := Runtime.ipStats[r.IP]
	if st == nil {
		st = &IPStat{IP: r.IP, FirstSeen: r.Time}
		Runtime.ipStats[r.IP] = st
	}
	st.LastSeen = r.Time
	st.Requests++
	st.BytesIn += r.BytesIn
	st.BytesOut += r.BytesOut
	if r.isError() {
		st.Errors++
	}
	st.Paths = addCapped(st.Paths, r.Path, maxPathsPerIP)
	st.Hosts = addCapped(st.Hosts, r.Host, maxHostsPerIP)
	if r.UserAgent != "" {
		st.UserAgents = addCapped(st.UserAgents, r.UserAgent, maxUAsPerIP)
	}
	Runtime.ipStatsDirty = true
}

// addCapped 去重追加，超过上限就不再记（但已记的保留）。
//
// 为什么要上限：一个被扫的 IP 能刷出几万条不同路径，不封顶的话
// ipstats.json 会涨到几十 MB，而且界面上也看不过来。
func addCapped(list []string, v string, max int) []string {
	if v == "" {
		return list
	}
	for _, x := range list {
		if x == v {
			return list
		}
	}
	if len(list) >= max {
		return list
	}
	return append(list, v)
}

// Flush 把累积的访问记录与 IP 统计落盘（每分钟一次、退出时一次）
func Flush() {
	flushRecords()
	flushIPStats()
}

func flushRecords() {
	Runtime.accumMu.Lock()
	recs := Runtime.accum
	Runtime.accum = nil
	Runtime.accumMu.Unlock()

	if len(recs) == 0 {
		return
	}

	// 按天分组写（跨分钟落盘时可能跨天）
	byDay := map[string][]Record{}
	for _, r := range recs {
		day := r.Time.Format("2006-01-02")
		byDay[day] = append(byDay[day], r)
	}

	for day, list := range byDay {
		name := filepath.Join(auditDir, day+".jsonl")

		// 单文件超限：停写并告警。被刷时宁可丢明细也不能把磁盘写满 ——
		// 磁盘满了整个服务都会挂，那损失比"少了一些审计记录"大得多。
		maxBytes := int64(Runtime.Snapshot().MaxFileMB) << 20
		if fi, err := os.Stat(name); err == nil && fi.Size() > maxBytes {
			markBlocked(name, maxBytes)
			continue
		}

		if err := utilsFile.EnsurePrivateDir(auditDir); err != nil {
			logrus.Warnf("[audit] 创建目录失败：%v", err)
			continue
		}
		f, err := os.OpenFile(name, os.O_APPEND|os.O_CREATE|os.O_WRONLY, utilsFile.PrivateFileMode)
		if err != nil {
			logrus.Warnf("[audit] 打开 %s 失败：%v", name, err)
			continue
		}
		for _, r := range list {
			line, err := json.Marshal(r)
			if err != nil {
				continue
			}
			_, _ = f.Write(append(line, '\n'))
		}
		_ = f.Close()
	}
}

func markBlocked(name string, maxBytes int64) {
	if writeBlocked && writeBlockedOn == name {
		return
	}
	writeBlocked = true
	writeBlockedOn = name
	logrus.Warnf("[audit] %s 已达 %d MB 上限，暂停写入访问明细（IP 统计不受影响）",
		name, maxBytes>>20)
}

var (
	writeBlocked   bool
	writeBlockedOn string
)

// flushIPStats 落盘 IP 聚合。**先写临时文件再改名** ——
// 直接覆盖时断电会留下半截 JSON，下次启动解析失败 = 累计统计全丢。
func flushIPStats() {
	Runtime.mu.Lock()
	if !Runtime.ipStatsDirty {
		Runtime.mu.Unlock()
		return
	}
	// 顺便把封禁状态同步进统计（界面上要显示"已封禁"）
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
	out := make([]IPStat, 0, len(Runtime.ipStats))
	for _, st := range Runtime.ipStats {
		out = append(out, *st)
	}
	Runtime.ipStatsDirty = false
	Runtime.mu.Unlock()

	if err := utilsFile.EnsurePrivateDir(filepath.Dir(ipStatsPath)); err != nil {
		return
	}
	data, err := json.MarshalIndent(map[string]any{
		"savedAt": time.Now(),
		"stats":   out,
	}, "", " ")
	if err != nil {
		return
	}
	tmp := ipStatsPath + ".tmp"
	if err := os.WriteFile(tmp, data, utilsFile.PrivateFileMode); err != nil {
		return
	}
	_ = os.Rename(tmp, ipStatsPath)
}

// ===================== 读 =====================

// ListIPStats 按 IP 列表。sortBy: errors / requests / recent
func ListIPStats(sortBy string, onlySuspicious bool, limit int) []IPStat {
	Runtime.mu.RLock()
	out := make([]IPStat, 0, len(Runtime.ipStats))
	for _, st := range Runtime.ipStats {
		if onlySuspicious && !st.Suspicious() {
			continue
		}
		out = append(out, *st)
	}
	Runtime.mu.RUnlock()

	switch sortBy {
	case "requests":
		sort.Slice(out, func(i, j int) bool { return out[i].Requests > out[j].Requests })
	case "recent":
		sort.Slice(out, func(i, j int) bool { return out[i].LastSeen.After(out[j].LastSeen) })
	default: // errors
		sort.Slice(out, func(i, j int) bool {
			if out[i].Errors != out[j].Errors {
				return out[i].Errors > out[j].Errors
			}
			return out[i].Requests > out[j].Requests
		})
	}

	if limit > 0 && len(out) > limit {
		out = out[:limit]
	}
	return out
}

// GetIPStat 查单个 IP
func GetIPStat(ip string) (IPStat, bool) {
	Runtime.mu.RLock()
	defer Runtime.mu.RUnlock()
	st, ok := Runtime.ipStats[ip]
	if !ok {
		return IPStat{}, false
	}
	return *st, true
}

// ReadDay 读某天的访问明细（给"展开看某个 IP 干了什么"用）
func ReadDay(day string, ipFilter string, limit int) ([]Record, error) {
	name := filepath.Join(auditDir, day+".jsonl")
	f, err := os.Open(name)
	if err != nil {
		return nil, err
	}
	defer f.Close()

	var out []Record
	dec := json.NewDecoder(f)
	for {
		var r Record
		if err := dec.Decode(&r); err != nil {
			break
		}
		if ipFilter != "" && r.IP != ipFilter {
			continue
		}
		out = append(out, r)
	}

	// 新的在前
	sort.Slice(out, func(i, j int) bool { return out[i].Time.After(out[j].Time) })
	if limit > 0 && len(out) > limit {
		out = out[:limit]
	}
	return out, nil
}

// Days 有哪天有访问记录
func Days() []string {
	items, err := os.ReadDir(auditDir)
	if err != nil {
		return []string{}
	}
	var days []string
	for _, it := range items {
		name := it.Name()
		if it.IsDir() || !strings.HasSuffix(name, ".jsonl") {
			continue
		}
		days = append(days, strings.TrimSuffix(name, ".jsonl"))
	}
	sort.Sort(sort.Reverse(sort.StringSlice(days)))
	return days
}

// parseCIDROrIP 单个 IP 或 CIDR 都能解析成网段（单个 IP 当 /32 或 /128）
func parseCIDROrIP(s string) *net.IPNet {
	s = strings.TrimSpace(s)
	if s == "" {
		return nil
	}
	if _, cidr, err := net.ParseCIDR(s); err == nil {
		return cidr
	}
	if ip := net.ParseIP(s); ip != nil {
		bits := 32
		if ip.To4() == nil {
			bits = 128
		}
		return &net.IPNet{IP: ip, Mask: net.CIDRMask(bits, bits)}
	}
	return nil
}

func matchAny(nets []*net.IPNet, ipStr string) bool {
	ip := net.ParseIP(ipStr)
	if ip == nil {
		return false
	}
	for _, n := range nets {
		if n.Contains(ip) {
			return true
		}
	}
	return false
}

// ===================== 登录事件 =====================
//
// 存哪里：data/access/login-<日期>.jsonl，和访问明细分开两个文件。
// 分开是因为它们的字段不同、用途不同（一个是"谁访问了服务"，
// 一个是"谁在试着进后台"），混在一个文件里读起来要判断类型。

const loginFilePrefix = "login-"

// RecordLogin 记一次登录尝试
func RecordLogin(ev LoginEvent) {
	if !Runtime.isEnabled() {
		return
	}
	if ev.Time.IsZero() {
		ev.Time = time.Now()
	}

	name := filepath.Join(auditDir, loginFilePrefix+ev.Time.Format("2006-01-02")+".jsonl")

	if err := utilsFile.EnsurePrivateDir(auditDir); err != nil {
		logrus.Warnf("[audit] 创建目录失败：%v", err)
		return
	}
	f, err := os.OpenFile(name, os.O_APPEND|os.O_CREATE|os.O_WRONLY, utilsFile.PrivateFileMode)
	if err != nil {
		logrus.Warnf("[audit] 打开 %s 失败：%v", name, err)
		return
	}
	defer f.Close()

	line, err := json.Marshal(ev)
	if err != nil {
		return
	}
	_, _ = f.Write(append(line, '\n'))
}

// ReadLoginEvents 读某天的登录事件，新的在前。ip 留空表示不限。
func ReadLoginEvents(day string, ip string, limit int) ([]LoginEvent, error) {
	name := filepath.Join(auditDir, loginFilePrefix+day+".jsonl")
	f, err := os.Open(name)
	if err != nil {
		return nil, err
	}
	defer f.Close()

	var out []LoginEvent
	dec := json.NewDecoder(f)
	for {
		var e LoginEvent
		if err := dec.Decode(&e); err != nil {
			break
		}
		if ip != "" && e.IP != ip {
			continue
		}
		out = append(out, e)
	}

	sort.Slice(out, func(i, j int) bool { return out[i].Time.After(out[j].Time) })
	if limit > 0 && len(out) > limit {
		out = out[:limit]
	}
	return out, nil
}

// LoginSummary 某天登录事件的汇总，给界面上"今天有人试了几次"用
type LoginSummary struct {
	Total     int      `json:"total"`
	Failed    int      `json:"failed"`
	Succeeded int      `json:"succeeded"`
	IPs       []string `json:"ips"`
	// Usernames 被试过的用户名（去重）。看到一堆 admin/root 就知道在字典攻击
	Usernames []string `json:"usernames"`
}

// SummarizeLogins 汇总某天的登录事件
func SummarizeLogins(day string) LoginSummary {
	events, err := ReadLoginEvents(day, "", 0)
	if err != nil {
		return LoginSummary{}
	}

	ipSet := map[string]bool{}
	nameSet := map[string]bool{}
	var s LoginSummary
	for _, e := range events {
		s.Total++
		if e.Success {
			s.Succeeded++
		} else {
			s.Failed++
		}
		if e.IP != "" {
			ipSet[e.IP] = true
		}
		if e.Username != "" {
			nameSet[e.Username] = true
		}
	}
	for ip := range ipSet {
		s.IPs = append(s.IPs, ip)
	}
	for n := range nameSet {
		s.Usernames = append(s.Usernames, n)
	}
	sort.Strings(s.IPs)
	sort.Strings(s.Usernames)
	return s
}
