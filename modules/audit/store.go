package audit

import (
	"bufio"
	"bytes"
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
	/**
	 * ConfigPath 审计配置。
	 *
	 * 导出是为了让备份模块引用它 —— 各模块的配置文件路径应当**只有一个真相**，
	 * 备份那边再抄一份字符串的话，改了这里就会悄悄对不上（今天已经踩过一次）。
	 */
	ConfigPath = "config/audit.json"
	configPath = ConfigPath

	defaultKeepDays  = 30
	defaultMaxFileMB = 32

	// maxPathsPerIP 每个 IP 最多记多少条路径。无上限的话被扫一次
	// 就能让这个文件涨到几十 MB。
	maxPathsPerIP = 30
	maxHostsPerIP = 10
	maxUAsPerIP   = 5
)

// 下面两个是 var 而不是 const：测试要把它们指到临时目录。
//
// 与 core/component.StatePath 同一个理由 —— 否则测试会在源码树里
// 建 data/access/ 并写进真记录。生产代码不要改它们。
var (
	// auditDir 按天存放访问明细的目录
	auditDir = "data/access"
	// ipStatsPath 按 IP 的聚合。这个要长期留着（累计值），不能按天删。
	ipStatsPath = "data/access/ipstats.json"
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

	// blocked 全局封禁表，按**单个 IP** 精确匹配。
	// **常驻内存且无锁读**（热路径上每个请求都要查）。
	blocked map[string]BlockEntry
	// blockedNets 封禁表里那些写成**网段**的条目。
	//
	// 分开存是为了快：绝大部分封禁条目是自动拉黑产生的单个 IP（O(1) 命中），
	// 只有用户手工填的网段才需要挨个 Contains（O(n)，但通常只有几条）。
	// 以前这里只有 blocked 一个 map，把 "10.0.0.0/8" 当普通字符串 key 存进去，
	// 匹配时拿 "10.0.0.5" 去查 → 永远查不到。**填网段会存进去但不生效**。
	blockedNets []blockedNet
	// whitelist 永不封禁的名单，同上
	whitelist []*net.IPNet
	// trustedNets 可信反代的网段。判「要不要信 X-Forwarded-For」用。
	//
	// 存解析后的网段而不是每次现解析：ResolveClientIP 在**每个请求**上都要调，
	// 而 parseCIDROrIP 要做字符串切分 + 地址解析，热路径上不该重复做。
	// 跟上面两位的待遇一致 —— 配置变更时（rebuildIndexes）重建一次。
	trustedNets []*net.IPNet
}

// blockedNet 一条网段封禁
type blockedNet struct {
	net   *net.IPNet
	entry BlockEntry
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
	var nets []blockedNet
	now := time.Now()
	for _, b := range cfg.Blocks {
		if b.Expired(now) {
			continue // 过期的直接不进索引（但配置里留着，用户能看到历史）
		}
		// 写成网段的（带 /）走 Contains 匹配，其余当单个 IP 精确匹配。
		// 注意不能对所有条目都用 Contains：那样 "1.2.3.4" 会被当成
		// 1.2.3.4/32，语义不变但白白多一次遍历；而真正的问题在下面 ——
		// 网段必须走 Contains，否则封网段等于白封。
		if strings.Contains(b.IP, "/") {
			if n := parseCIDROrIP(b.IP); n != nil {
				nets = append(nets, blockedNet{net: n, entry: b})
				continue
			}
		}
		blocked[b.IP] = b
	}

	var wl []*net.IPNet
	for _, w := range cfg.Whitelist {
		if n := parseCIDROrIP(w); n != nil {
			wl = append(wl, n)
		}
	}

	// 可信反代：配了才会去 X-Forwarded-For 里找真实来源（走 Cloudflare 时用）
	var tp []*net.IPNet
	for _, t := range cfg.TrustedProxies {
		if n := parseCIDROrIP(t); n != nil {
			tp = append(tp, n)
		}
	}

	Runtime.mu.Lock()
	Runtime.blocked = blocked
	Runtime.blockedNets = nets
	Runtime.whitelist = wl
	Runtime.trustedNets = tp
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

	// 单个 IP 的封禁：绝大多数情况走这条路（自动拉黑都是单 IP），O(1)
	if b, ok := Runtime.blocked[ip]; ok {
		return b, true
	}

	// 网段封禁：只在上面没命中时才挨个比，通常就几条
	if parsed := net.ParseIP(ip); parsed != nil {
		for _, bn := range Runtime.blockedNets {
			if bn.net.Contains(parsed) {
				return bn.entry, true
			}
		}
	}

	return BlockEntry{}, false
}

// ===================== 写访问记录 =====================

// RecordAccess 记一次访问。
//
// 走内存累加 + 每分钟落盘：被扫的时候一分钟能来几千个请求，
// 每个都写盘会把磁盘打满（而且统计本来也不需要那么细的粒度）。
func RecordAccess(r Record) {
	recordAccess(r, r.BytesIn, r.BytesOut)
}

// RecordAccessProgress 结算一次访问：**途中的字节已经上报过了**时用这个。
//
// # 为什么要有第二个入口
//
// 审计页「来源流量」（每个 IP 的累计进出字节）原来只在请求结束时更新。
// 普通网页几十毫秒结束看不出来，但一个传十几分钟的文件，这十几分钟里
// 那个 IP 的流量**纹丝不动** —— 用户会以为审计没统计到。
//
// 现在传输途中就按 traffic 同样的阈值上报（见 proxy 的 statusRecorder），
// 请求结束时用这里结算。
//
// # 三处数据的口径必须分开
//
//   - 明细（data/access/<日期>.jsonl）：一条记录 = 一次完整请求，写 r 的
//     **完整**字节。如果写剩余值，明细里每条记录就是残缺的。
//   - IPStat（来源 IP 累计值）：途中加过一部分，结算时只能补**剩余**，
//     否则翻倍。这就是为什么字节要单独传进来。
//   - DayStat（按天按小时聚合）：途中不上报它（趋势/指标卡用的是请求数和
//     错误数，字节不参与展示），所以结算时加**完整**值即可。
func RecordAccessProgress(r Record, reportedIn, reportedOut int64) {
	restIn := r.BytesIn - reportedIn
	restOut := r.BytesOut - reportedOut
	if restIn < 0 {
		restIn = 0
	}
	if restOut < 0 {
		restOut = 0
	}
	recordAccess(r, restIn, restOut)
}

// recordAccess 内部实现。addIn/addOut 是**加进 IPStat** 的字节数。
func recordAccess(r Record, addIn, addOut int64) {
	if !Runtime.isEnabled() {
		return
	}
	if r.Time.IsZero() {
		r.Time = time.Now()
	}

	updateIPStatsWith(r, addIn, addOut)

	// 同时累加按天/按小时的聚合。两个维度在同一处更新，
	// 分开写迟早会出现「明细里有、趋势图里没有」这种对不上的情况。
	// 这里用 r 的完整值：DayStat 的字节在途中不上报（见上面注释）。
	AddToDayStat(r)

	Runtime.accumMu.Lock()
	Runtime.accum = append(Runtime.accum, r)
	Runtime.accumMu.Unlock()
}

// AddTrafficProgress 传输**途中**给来源 IP 累加字节。
//
// 与 traffic.RecordPartial 是同一个时机、同一套阈值，由 proxy 层同时调用。
//
// 刻意不更新的三项：
//   - Requests：一次下载是一个请求，不能因为分段上报变成几十个
//   - LastSeen：它表示"这个 IP 最后一次发起请求"，途中的续传不算新请求
//   - 明细 / DayStat：明细要等请求结束才有完整的一条记录
func AddTrafficProgress(ip string, bytesIn, bytesOut int64) {
	if !Runtime.isEnabled() || ip == "" {
		return
	}
	if bytesIn <= 0 && bytesOut <= 0 {
		return
	}

	Runtime.mu.Lock()
	defer Runtime.mu.Unlock()

	st := Runtime.ipStats[ip]
	if st == nil {
		st = &IPStat{IP: ip, FirstSeen: time.Now()}
		Runtime.ipStats[ip] = st
	}
	st.BytesIn += bytesIn
	st.BytesOut += bytesOut
	// 兜底成空数组：这条路径也可能建出新 IPStat，不兜住的话穿透 IP
	// 落盘成 null，前端取 .length 会崩（见 updateIPStats 里的注释）
	if st.Paths == nil {
		st.Paths = []string{}
	}
	if st.Hosts == nil {
		st.Hosts = []string{}
	}
	if st.UserAgents == nil {
		st.UserAgents = []string{}
	}
	Runtime.ipStatsDirty = true
}

func (r *runtime) isEnabled() bool {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return r.Config.Enabled
}

// updateIPStats 更新按 IP 的聚合
func updateIPStats(r Record) {
	updateIPStatsWith(r, r.BytesIn, r.BytesOut)
}

// updateIPStatsWith 更新按 IP 的聚合，字节部分用 addIn/addOut 而不是 Record 里的值。
//
// 为什么要把字节单独传：加了「传输途中上报」之后，请求结束时 Record 里的
// BytesIn/BytesOut 是这次请求的**完整**流量，但其中一部分已经在途中加进 IPStat 了。
// 结算时只能补**剩余**，否则来源 IP 的流量会翻倍。
// 见 RecordAccessProgress 的注释。
func updateIPStatsWith(r Record, addIn, addOut int64) {
	Runtime.mu.Lock()
	defer Runtime.mu.Unlock()

	st := Runtime.ipStats[r.IP]
	if st == nil {
		st = &IPStat{IP: r.IP, FirstSeen: r.Time}
		Runtime.ipStats[r.IP] = st
	}
	st.LastSeen = r.Time
	st.Requests++
	st.BytesIn += addIn
	st.BytesOut += addOut
	if r.isError() {
		st.Errors++
	}
	st.Paths = addCapped(st.Paths, r.Path, maxPathsPerIP)
	st.Hosts = addCapped(st.Hosts, r.Host, maxHostsPerIP)
	if r.UserAgent != "" {
		st.UserAgents = addCapped(st.UserAgents, r.UserAgent, maxUAsPerIP)
	}
	// **兜底成空数组，绝不能是 nil**：穿透访问（kind=tunnel）没有 path/host/UA，
	// 第一条就是穿透的话这三个字段全是 nil，落盘成 null ——
	// 前端取 .length 直接崩（实测：审计页点纯穿透 IP 必崩）。
	if st.Paths == nil {
		st.Paths = []string{}
	}
	if st.Hosts == nil {
		st.Hosts = []string{}
	}
	if st.UserAgents == nil {
		st.UserAgents = []string{}
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
	SaveDayStats()
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
		// 刚追加了记录：把这天已解析的缓存丢掉。
		//
		// 不丢也不会错（下次读会发现 mtime/大小变了而重读），但那样要等
		// stat 一次才发现；这里直接清掉，「刚被扫完马上去看审计」就是新数据。
		invalidateDayCache(day)
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

	sortIPStats(out, sortBy)

	if limit > 0 && len(out) > limit {
		out = out[:limit]
	}
	return out
}

// ListIPStatsByDay 从**某一天的明细**聚合出按 IP 的统计。
//
// 和 ListIPStats 的区别：那个读的是累计值（ipstats.json，跨天累计），
// 这个只读指定的那一天 —— 界面上的「按日期查看」要的是后者。
//
// 明细是真相，所以这里现算，不缓存：某天的数据不会变了，
// 缓存一份反而要考虑失效问题。
func ListIPStatsByDay(day string, sortBy string, onlySuspicious bool, limit int) ([]IPStat, error) {
	recs, err := ReadDay(day, "", 0)
	if err != nil {
		return nil, err
	}

	acc := make(map[string]*IPStat, len(recs))
	for _, r := range recs {
		if r.IP == "" {
			continue
		}
		st := acc[r.IP]
		if st == nil {
			st = &IPStat{IP: r.IP, FirstSeen: r.Time}
			acc[r.IP] = st
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
	}

	out := make([]IPStat, 0, len(acc))
	for _, st := range acc {
		// 穿透访问没有 path/host/UA，一条都没记上时这三个是 nil ——
		// 落盘成 null 前端取 .length 会崩（这个坑以前踩过）
		if st.Paths == nil {
			st.Paths = []string{}
		}
		if st.Hosts == nil {
			st.Hosts = []string{}
		}
		if st.UserAgents == nil {
			st.UserAgents = []string{}
		}
		if onlySuspicious && !st.Suspicious() {
			continue
		}
		out = append(out, *st)
	}

	sortIPStats(out, sortBy)

	if limit > 0 && len(out) > limit {
		out = out[:limit]
	}
	return out, nil
}

// sortIPStats 排序规则：累计和按天两种来源共用同一套，
// 否则同一个「按异常数」在两种视图下顺序不一样，看着像错了。
func sortIPStats(out []IPStat, sortBy string) {
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

// ReadDay 读某天的访问明细（给"展开看某个 IP 干了什么"用）。
//
// # 逐行解析，坏行跳过
//
// 以前这里是一个 json.Decoder 循环，遇到解析错误就 break。看起来没问题
// （文件是自己写的，怎么会坏？），但实测踩到了：只要**某一行**的字段
// 格式对不上（比如 time 少了时区，`2026-10-17T11:31:03.98` 而不是
// `...+08:00`），decoder 就从那一行开始停住，**后面所有记录静默丢失**。
//
// 后果是「明细明明有、审计页只显示一半」，而且没有任何报错 ——
// 这种问题几乎不可能从现象倒推原因。
//
// 现在按行扫，单行失败就记一笔日志然后继续。一行坏数据不该让一整天的
// 记录都读不出来。
func ReadDay(day string, ipFilter string, limit int) ([]Record, error) {
	recs, err := readDayCached(day)
	if err != nil {
		return nil, err
	}

	// 过滤与分页在副本上做，不动缓存里那份 —— 调用方拿到的切片
	// 不能和缓存共享底层数组，否则某个调用方一排序就把缓存改了
	if ipFilter == "" && limit <= 0 {
		out := make([]Record, len(recs))
		copy(out, recs)
		return out, nil
	}

	out := make([]Record, 0, len(recs))
	for _, r := range recs {
		if ipFilter != "" && r.IP != ipFilter {
			continue
		}
		out = append(out, r)
		if limit > 0 && len(out) >= limit {
			break // 已经按时间倒序，够了就停
		}
	}
	return out, nil
}

// dayCacheEntry 某天明细的解析结果缓存。
//
// # 为什么必须缓存
//
// 审计页一打开要算四样东西：指标卡（唯一 IP、攻击 IP）、趋势、攻击类型、
// 站点 Top。四样都从「最近 24 小时的明细」算，而明细是按天一个大文件
// （被扫过的机器一天几十 MB）。不缓存的话，打开一次页面就要把同一个文件
// 完整读四遍、逐行 JSON 解析四遍 —— 实测单次 overview 请求里
// 「站点 Top」一项就把 3 万多条记录读了一遍，页面自然转圈。
//
// # 为什么按 mtime+size 校验而不是按时间过期
//
// 当天的文件是**边写边变大**的（每分钟 flush 一次）。固定 TTL 会出现
// 「刚写进去的记录半分钟内查不到」；而按 TTL 过期又保证不了新鲜。
// 比对 mtime 和大小是最小代价的「变了就重读」：
// 文件没动就直接返回，动了就重新解析一次。历史日期永远不会再动，
// 于是它们的缓存是永久有效的（直到进程退出）。
type dayCacheEntry struct {
	recs  []Record
	mtime time.Time
	size  int64
}

var (
	dayCacheMu sync.Mutex
	dayCache   = map[string]*dayCacheEntry{}
)

// maxCachedDays 缓存最多留几天。
//
// 保留期默认 30 天，全留着也就几十 MB 的解析结果 —— 但如果有人把保留期
// 调到 365 天、又挨个翻日历，无上限就会吃光内存。超了按「最久没用的」淘汰。
const maxCachedDays = 40

// readDayCached 读某天明细（带缓存）。返回的是**缓存里那份切片本身**，
// 调用方只能读、不能排序或改元素；要改必须先自己 copy。
func readDayCached(day string) ([]Record, error) {
	name := filepath.Join(auditDir, day+".jsonl")
	fi, statErr := os.Stat(name)

	dayCacheMu.Lock()
	if e, ok := dayCache[day]; ok {
		// 文件不存在时也算命中：缓存记录的就是「这天没文件」，
		// 免得每次请求都对一个不存在的路径反复 stat 失败
		if statErr != nil || (e.size == fi.Size() && e.mtime.Equal(fi.ModTime())) {
			recs := e.recs
			dayCacheMu.Unlock()
			if statErr != nil {
				return nil, statErr
			}
			return recs, nil
		}
	}
	dayCacheMu.Unlock()

	if statErr != nil {
		return nil, statErr
	}

	recs, err := parseDayFile(name, day)
	if err != nil {
		return nil, err
	}

	dayCacheMu.Lock()
	if len(dayCache) >= maxCachedDays {
		evictDayCacheLocked()
	}
	dayCache[day] = &dayCacheEntry{recs: recs, mtime: fi.ModTime(), size: fi.Size()}
	dayCacheMu.Unlock()
	return recs, nil
}

// evictDayCacheLocked 淘汰一半缓存。调用方必须持有 dayCacheMu。
//
// 淘汰策略是「留最近的两天 + 一半较新的」：审计页和 dashboard 绝大多数
// 请求只碰今天，把今天那天的缓存淘汰掉，下一次请求又得重读整个文件。
func evictDayCacheLocked() {
	days := make([]string, 0, len(dayCache))
	for d := range dayCache {
		days = append(days, d)
	}
	// 日期字符串按字典序就是时间序，倒序即新的在前
	sort.Sort(sort.Reverse(sort.StringSlice(days)))
	keep := maxCachedDays / 2
	for i, d := range days {
		if i < keep {
			continue
		}
		delete(dayCache, d)
	}
}

// invalidateDayCache 让某天的缓存失效。
//
// flush 刚把新记录写进文件后调用：不调也不会错（下一次读会因 mtime 变化
// 而重读），但这里主动清一次能让「刚写完就查」走同一条路径，少一次解析。
func invalidateDayCache(day string) {
	dayCacheMu.Lock()
	delete(dayCache, day)
	dayCacheMu.Unlock()
}

// parseDayFile 真正的逐行解析（带缓存前的实现）。
func parseDayFile(name, day string) ([]Record, error) {
	f, err := os.Open(name)
	if err != nil {
		return nil, err
	}
	defer f.Close()

	var out []Record
	sc := bufio.NewScanner(f)
	// 单条记录带着 UA、路径，正常几百字节；给到 1MB 足够，
	// 超过这个基本是文件损坏而不是正常数据
	sc.Buffer(make([]byte, 0, 64*1024), 1<<20)

	lineNo, badLines := 0, 0
	var firstErr error
	for sc.Scan() {
		lineNo++
		line := bytes.TrimSpace(sc.Bytes())
		if len(line) == 0 {
			continue
		}
		var r Record
		if err := json.Unmarshal(line, &r); err != nil {
			badLines++
			if firstErr == nil {
				firstErr = fmt.Errorf("第 %d 行：%w", lineNo, err)
			}
			continue
		}
		out = append(out, r)
	}

	if badLines > 0 {
		logrus.Warnf("[audit] %s 有 %d 行无法解析（已跳过，其余照常读出），第一条错误：%v",
			day, badLines, firstErr)
	}

	// 新的在前。
	//
	// 缓存的就是**排好序的完整一天**：ReadDay 的 limit 语义是「取最新的 N 条」，
	// 所以只要按时间倒序排好，limit 就退化成取前缀，不需要每次重排。
	sort.Slice(out, func(i, j int) bool { return out[i].Time.After(out[j].Time) })
	return out, nil
}

// Days 有哪天有访问记录
// Days 有访问记录的那些日期（新的在前）。
//
// **必须排除登录事件文件**：登录记录也存在这个目录里，只是文件名带
// login- 前缀（见 loginFilePrefix）。以前没排除，结果 days[0] 会是
// "login-2026-10-16" —— 于是"不看某一天、直接查最近"的请求读到的
// 是登录事件，不是访问记录（审计页默认视图和服务日志都受影响）。
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
		day := strings.TrimSuffix(name, ".jsonl")
		if strings.HasPrefix(day, loginFilePrefix) {
			continue
		}
		days = append(days, day)
	}
	sort.Sort(sort.Reverse(sort.StringSlice(days)))
	return days
}

// LoginDays 有登录事件的那些日期（新的在前，已去掉 login- 前缀）。
// 和 Days() 分开：两者存在同一个目录，用途和字段都不同。
func LoginDays() []string {
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
		day := strings.TrimSuffix(name, ".jsonl")
		if !strings.HasPrefix(day, loginFilePrefix) {
			continue
		}
		days = append(days, strings.TrimPrefix(day, loginFilePrefix))
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

// ReadBySite 某个服务的访问记录（新的在前，最多 limit 条）。
//
// **给"每个服务的日志模块"用**：反代站点卡片上点「日志」，要看的是
// "谁访问过我这个服务" —— 和 Audit 页那个按 IP 展开的视角正好反过来。
// 数据本来就有（Record.SiteID 一直写着），只是之前没有按这个维度取的接口。
//
// 跨天查：只往前翻 maxSiteScanDays 天，够看了（再早的访问对排障没意义），
// 也不会把整个审计目录翻一遍。
func ReadBySite(siteID uint, limit int) []Record {
	if siteID == 0 || limit <= 0 {
		return nil
	}
	if limit > 500 {
		limit = 500
	}
	days := Days()
	if len(days) > maxSiteScanDays {
		days = days[:maxSiteScanDays]
	}

	var out []Record
	for _, day := range days {
		list, err := ReadDay(day, "", 0) // 不限条数，按 siteID 过滤后再截断
		if err != nil {
			continue
		}
		for _, r := range list {
			if r.SiteID == siteID {
				out = append(out, r)
			}
		}
		if len(out) >= limit {
			break
		}
	}
	// 新的在前
	sort.Slice(out, func(i, j int) bool { return out[i].Time.After(out[j].Time) })
	if len(out) > limit {
		out = out[:limit]
	}
	return out
}

// 只往前翻多少天找某个服务的记录
const maxSiteScanDays = 7

// ReadByRef 按引用键取记录（新的在前，最多 limit 条）。
//
// **给内网穿透用**：穿透访问记录的 Ref 是 "s<设备ID>-<服务ID>"（和流量统计
// 同一个键）。反代用 SiteID，穿透没有站点 ID 这个概念，就靠 Ref 认服务。
// 两边最终都是"这个服务最近谁访问过"，共用同一个弹窗展示。
func ReadByRef(ref string, limit int) []Record {
	ref = strings.TrimSpace(ref)
	if ref == "" || limit <= 0 {
		return nil
	}
	if limit > 500 {
		limit = 500
	}
	days := Days()
	if len(days) > maxSiteScanDays {
		days = days[:maxSiteScanDays]
	}

	var out []Record
	for _, day := range days {
		list, err := ReadDay(day, "", 0)
		if err != nil {
			continue
		}
		for _, r := range list {
			if r.Ref == ref {
				out = append(out, r)
			}
		}
		if len(out) >= limit {
			break
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Time.After(out[j].Time) })
	if len(out) > limit {
		out = out[:limit]
	}
	return out
}
