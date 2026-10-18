package audit_api

import (
	"fmt"
	"net"
	"regexp"
	"sort"

	"strconv"
	"strings"
	"sync"

	"github.com/chenggegedeid/catflap/middleware"
	"github.com/chenggegedeid/catflap/modules/audit"
	"github.com/chenggegedeid/catflap/modules/audit/geo"
	"github.com/chenggegedeid/catflap/modules/proxy"
	proxymodel "github.com/chenggegedeid/catflap/modules/proxy/model"
	"github.com/chenggegedeid/catflap/modules/stun"

	"github.com/chenggegedeid/catflap/utils/res"
	"github.com/sirupsen/logrus"

	"github.com/gin-gonic/gin"
)

// AuditApi 访问审计与 IP 管理
type AuditApi struct{}

// ===================== 读 =====================

type ConfigView struct {
	Enabled            bool               `json:"enabled"`
	KeepDays           int                `json:"keepDays"`
	MaxFileMB          int                `json:"maxFileMB"`
	AutoBlock          bool               `json:"autoBlock"`
	AutoBlockThreshold int64              `json:"autoBlockThreshold"`
	AutoBlockMinutes   int                `json:"autoBlockMinutes"`
	Whitelist          []string           `json:"whitelist"`
	TrustedProxies     []string           `json:"trustedProxies"`
	Blocks             []audit.BlockEntry `json:"blocks"`
	Days               []string           `json:"days"`
	SuspiciousCount    int                `json:"suspiciousCount"`
	TotalIPs           int                `json:"totalIPs"`
}

// AuditConfigView 配置 + 当前封禁 + 天数
func (AuditApi) AuditConfigView(c *gin.Context) {
	cfg := audit.Runtime.Snapshot()
	blocks := audit.ListBlocks()
	if blocks == nil {
		blocks = []audit.BlockEntry{}
	}

	// 可疑 IP 的数量：进页面就能看到"有没有事"，不用点进去数
	suspicious := audit.ListIPStats("errors", true, 0)
	all := audit.ListIPStats("errors", false, 0)

	days := audit.Days()
	if days == nil {
		days = []string{}
	}

	res.OkWithData(ConfigView{
		Enabled:            cfg.Enabled,
		KeepDays:           cfg.KeepDays,
		MaxFileMB:          cfg.MaxFileMB,
		AutoBlock:          cfg.AutoBlock,
		AutoBlockThreshold: cfg.AutoBlockThreshold,
		AutoBlockMinutes:   cfg.AutoBlockMinutes,
		Whitelist:          nonNil(cfg.Whitelist),
		TrustedProxies:     nonNil(cfg.TrustedProxies),
		Blocks:             blocks,
		Days:               days,
		SuspiciousCount:    len(suspicious),
		TotalIPs:           len(all),
	}, c)
}

// IPView IP 统计 + 归属地。
//
// 归属地是**查询时现算**的，不存进 ipstats.json：
//   - 库每月更新，存下来就是一份会过期的副本；
//   - 归属地不影响任何判定（封禁、可疑判定都只看 IP 自身），
//     丢了一份假的反而更糟。
//
// 内嵌 IPStat 而不是把字段抄一遍：抄一遍的话以后给 IPStat 加字段，
// 这里会静默漏掉，界面上就是某个数字永远为 0。
type IPView struct {
	audit.IPStat
	// Geo 归属地，例如「江苏 · 南京」「美国」；私有地址是「内网」
	Geo geo.Info `json:"geo"`
	// Attack 这个 IP 最像哪一种攻击（按它扫过的路径判定）
	Attack string `json:"attack"`
}

// AuditIPsView 按 IP 的统计列表。
//
// 支持分页：被扫的时候 IP 数能到几千，一屏塞不下，全塞进一张表既卡又没法看。
// 返回 total 让界面能算页码。
//
// # summary=1：只给数字，不查归属地
//
// 仪表盘上只要「有几个可疑 / 几个攻击 IP」这两个数，不需要每一行的归属地
// 和攻击类型分类。默认路径每行都要查一次归属地库、还要按路径归纳攻击类型，
// 而仪表盘关心的是数字 —— 传 summary=1 就只走计数，省掉整轮行级加工。
func (AuditApi) AuditIPsView(c *gin.Context) {
	sortBy := c.DefaultQuery("sort", "errors")
	onlySuspicious := c.Query("suspicious") == "1"
	keyword := strings.TrimSpace(c.Query("q"))
	// 按哪天看。空 = 累计（跨天累加的那份），有值 = 只算那一天
	day := strings.TrimSpace(c.Query("day"))
	if day != "" && !validDay.MatchString(day) {
		res.FailWithMsg("日期格式不对（应为 YYYY-MM-DD）", c)
		return
	}

	if c.Query("summary") == "1" {
		res.OkWithData(auditSummary(day, sortBy), c)
		return
	}

	limit, _ := strconv.Atoi(c.DefaultQuery("limit", "50"))
	if limit <= 0 {
		limit = 50
	}
	// 上限压住：界面上限是「全部」，但那也得有个头，不然一次拉一万条直接卡死
	if limit > 1000 {
		limit = 1000
	}
	page, _ := strconv.Atoi(c.DefaultQuery("page", "1"))
	if page <= 0 {
		page = 1
	}

	// 先取全量（已经按 sortBy 排好），再做关键词过滤与分页。
	// 为什么不把过滤下推到 ListIPStats：那张表按 IP 索引，按归属地/攻击类型
	// 过滤它做不到 —— 归属地不在数据结构里。
	// 两种来源：累计值（默认）或某一天的明细（传了 day）
	var all []audit.IPStat
	if day != "" {
		var err error
		if all, err = audit.ListIPStatsByDay(day, sortBy, onlySuspicious, 0); err != nil {
			// 那天没有记录不是错误 —— 给空列表，界面显示"这天没有访问"
			all = nil
		}
	} else {
		all = audit.ListIPStats(sortBy, onlySuspicious, 0)
	}

	views := make([]IPView, 0, len(all))
	for _, st := range all {
		v := IPView{IPStat: st, Geo: geo.Lookup(st.IP)}
		v.Attack = attackOfIP(st)
		if keyword != "" && !matchIPView(v, keyword) {
			continue
		}
		views = append(views, v)
	}

	total := len(views)
	start := (page - 1) * limit
	if start > total {
		start = total
	}
	end := start + limit
	if end > total {
		end = total
	}

	res.OkWithData(gin.H{
		"list":  views[start:end],
		"total": total,
		"page":  page,
		"limit": limit,
		"day":   day,
		// 有访问记录的那些日期，给「按日期查看」的下拉用
		"days": audit.Days(),
		// 归属地库没加载成功时界面要把这一列藏起来，而不是显示一列「未知」
		"geoEnabled": geo.Enabled(),
	}, c)
}

// auditSummary 只给计数，不做逐行的归属地 / 攻击类型加工。
//
// 给仪表盘用：它要的是「可疑几个、攻击几个」，不是一张 IP 表。
func auditSummary(day, sortBy string) gin.H {
	var all []audit.IPStat
	if day != "" {
		all, _ = audit.ListIPStatsByDay(day, sortBy, false, 0)
	} else {
		all = audit.ListIPStats(sortBy, false, 0)
	}

	var suspicious, attackers, blocked int
	for i := range all {
		if all[i].Suspicious() {
			suspicious++
		}
		if all[i].Errors > 0 {
			attackers++
		}
		if all[i].Blocked {
			blocked++
		}
	}
	return gin.H{
		"total":      len(all),
		"suspicious": suspicious,
		"attackers":  attackers,
		"blocked":    blocked,
		"day":        day,
	}
}

// attackOfIP 这个 IP 最像哪一类攻击。
//
// 看它扫过的路径：一个 IP 可能同时扫 .env 和 wp-login，按「最严重的那个」
// 归类 —— 界面上一行只能显示一个标签，显示最该关注的那个。
func attackOfIP(st audit.IPStat) string {
	if st.Errors == 0 {
		return ""
	}
	// 按危害从高到低找第一个命中的类别
	best := audit.AttackOther
	for _, p := range st.Paths {
		k := audit.ClassifyAttack(audit.Record{Path: p, Status: 404})
		if k == audit.AttackSecret || k == audit.AttackAdmin || k == audit.AttackInject {
			return k.Label()
		}
		if k == audit.AttackScan {
			best = audit.AttackScan
		}
	}
	if st.Errors >= 50 {
		// 错误多但路径都没特征：多半就是无差别扫描
		return audit.AttackScan.Label()
	}
	return best.Label()
}

// matchIPView 关键词是否命中这个 IP 行。
//
// 支持搜 IP 本身、归属地、主机名、扫过的路径 —— 「哪个 IP 在扫 .env」
// 和「有没有国内的 IP 在打我」都是很自然的问法。
func matchIPView(v IPView, kw string) bool {
	kw = strings.ToLower(kw)
	if strings.Contains(strings.ToLower(v.IP), kw) {
		return true
	}
	if strings.Contains(strings.ToLower(v.Geo.Label), kw) ||
		strings.Contains(strings.ToLower(v.Geo.Country), kw) {
		return true
	}
	for _, p := range v.Paths {
		if strings.Contains(strings.ToLower(p), kw) {
			return true
		}
	}
	for _, h := range v.Hosts {
		if strings.Contains(strings.ToLower(h), kw) {
			return true
		}
	}
	return false
}

// AuditOverviewView 页面顶部那一整块：指标卡 + 趋势 + 攻击类型 + 地区 + 站点。
//
// 合成一个接口而不是拆成五六个：
//   - 它们要**同时**刷新，拆开会出现「指标卡是这一分钟的、趋势图是上一分钟的」
//     这种对不上的画面；
//   - 打开审计页就是一次请求，比分六次快得多。
//
// # 为什么这里要串成一条流水线
//
// 指标卡和趋势都基于明细。以前它们各自调一次 BuildOverview / Trends，
// 各自把当天那个几十 MB 的文件读一遍解析一遍 —— 打开一次页面解析两三次，
// 被扫过的机器上就是好几秒的白屏。
//
// 现在：明细解析有缓存（readDayCached，文件没变直接复用），
// 站点 Top 改读按小时聚合里那份 Sites（不碰明细），
// 地区分布有按 IP 统计版本号的缓存。一次打开页面的磁盘读取降到"文件变了才读"。
func (AuditApi) AuditOverviewView(c *gin.Context) {
	hours, _ := strconv.Atoi(c.DefaultQuery("hours", "24"))
	if hours <= 0 || hours > 168 {
		// 上限一周：再长就没什么意义，而且明细读得慢
		hours = 24
	}

	// 地区分布：按 IP 的累计统计里取（那里已经有所有 IP），按国家聚合
	countries := geoRanking()

	res.OkWithData(gin.H{
		"overview":    audit.BuildOverview(hours),
		"trends":      audit.Trends(hours),
		"attackKinds": audit.AttackKinds(hours),
		"sites":       audit.TopSites(hours, 8),
		"countries":   countries,
		"geoEnabled":  geo.Enabled(),
	}, c)
}

// CountryCount 一个国家/地区的访问量。
type CountryCount struct {
	Name     string `json:"name"`
	Requests int64  `json:"requests"`
	IPs      int64  `json:"ips"`
}

// geoRanking 按国家聚合。
//
// 用按 IP 的累计统计而不是最近 24 小时的明细：明细里一个 IP 会出现几万次，
// 逐条查归属地太浪费；而「哪些国家在访问我」这种问题看累计值更合适。
//
// # 缓存
//
// 这段每次都要对全部 IP 查一次归属地库（纯内存查表，但 IP 上千个时
// 一次也有几毫秒到几十毫秒），并且要对整个统计列表排序 —— 而它在
// **每次打开审计页、每次打开仪表盘**都会跑一遍，结果却几乎不变。
//
// 所以按「IP 集合指纹」缓存：IP 数和企业数一样就复用。
// 指纹用 (IP 数, 请求总数) 两个数拼成 —— 新 IP 出现会改变 IP 数，
// 老 IP 继续刷量会改变请求总数。两者都没变时，排序结果不可能变。
type geoRankKey struct {
	ips      int
	requests int64
}

var (
	geoRankMu    sync.Mutex
	geoRankCache map[geoRankKey][]CountryCount
)

func geoRanking() []CountryCount {
	stats := audit.ListIPStats("requests", false, 0)

	key := geoRankKey{ips: len(stats)}
	for i := range stats {
		key.requests += stats[i].Requests
	}

	geoRankMu.Lock()
	if c, ok := geoRankCache[key]; ok {
		geoRankMu.Unlock()
		return c
	}
	geoRankMu.Unlock()

	agg := map[string]*CountryCount{}
	for _, st := range stats {
		info := geo.Lookup(st.IP)
		name := info.Country
		if name == "" {
			// 内网 / 未知要单独成组，不能混进「未知」里跟国外流量一起算
			name = "内网/未知"
		}
		c := agg[name]
		if c == nil {
			c = &CountryCount{Name: name}
			agg[name] = c
		}
		c.Requests += st.Requests
		c.IPs++
	}

	out := make([]CountryCount, 0, len(agg))
	for _, c := range agg {
		out = append(out, *c)
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].Requests != out[j].Requests {
			return out[i].Requests > out[j].Requests
		}
		return out[i].Name < out[j].Name
	})
	if len(out) > 12 {
		out = out[:12]
	}

	// 只留一份：这个 key 每次数据一变就是新的，留多了纯属吃内存
	geoRankMu.Lock()
	geoRankCache = map[geoRankKey][]CountryCount{key: out}
	geoRankMu.Unlock()
	return out
}

// validDay 日期参数是否合法（YYYY-MM-DD）。
//
// 这个参数会被拼进文件路径（<data>/access/<day>.jsonl），不校验就是目录穿越：
// day=../../xxx 能读到数据目录外面任何 .jsonl 文件。
// 日期是有限集合，用白名单式校验最省事也最严。
var validDay = regexp.MustCompile(`^\d{4}-\d{2}-\d{2}$`)

// AuditRecordsView 某个 IP 在某天的访问明细
func (AuditApi) AuditRecordsView(c *gin.Context) {
	day := strings.TrimSpace(c.Query("day"))
	ip := strings.TrimSpace(c.Query("ip"))
	limit, _ := strconv.Atoi(c.DefaultQuery("limit", "200"))
	if limit <= 0 {
		limit = 200
	}

	if day != "" && !validDay.MatchString(day) {
		res.FailWithMsg("日期格式不对（应为 YYYY-MM-DD）", c)
		return
	}

	if day == "" {
		days := audit.Days()
		if len(days) == 0 {
			res.OkWithData(gin.H{"list": []audit.Record{}}, c)
			return
		}
		day = days[0]
	}

	// 传了 siteId 就是"某个服务的日志模块"要看的：走按服务查询（跨天），
	// 忽略 day/ip —— 那个视角是"最近谁访问过我这个服务"。
	if siteIDStr := strings.TrimSpace(c.Query("siteId")); siteIDStr != "" {
		if siteID, e := strconv.Atoi(siteIDStr); e == nil && siteID > 0 {
			list := audit.ReadBySite(uint(siteID), limit)
			if list == nil {
				list = []audit.Record{}
			}
			res.OkWithData(gin.H{"list": list}, c)
			return
		}
	}

	// 传了 ref 就是内网穿透的日志模块：按 "s<设备ID>-<服务ID>" 取（跨天）
	if ref := strings.TrimSpace(c.Query("ref")); ref != "" {
		list := audit.ReadByRef(ref, limit)
		if list == nil {
			list = []audit.Record{}
		}
		res.OkWithData(gin.H{"list": list}, c)
		return
	}

	list, err := audit.ReadDay(day, ip, limit)
	if err != nil {
		// 那天没有记录不是错误
		res.OkWithData(gin.H{"list": []audit.Record{}}, c)
		return
	}
	if list == nil {
		list = []audit.Record{}
	}
	res.OkWithData(gin.H{"list": list, "day": day}, c)
}

// ===================== 写 =====================

type BlockRequest struct {
	IP string `json:"ip"`
	// Reason 封禁原因（可选，但写清楚以后自己看得懂）
	Reason string `json:"reason"`
	// Minutes 封禁时长（分钟）；0 = 永久
	Minutes int `json:"minutes"`
}

// WhitelistRequest 保存"永不封禁"名单。
//
// 这是**准入豁免**，不是"只准这些 IP 访问"—— 后者已经被移除了
// （站点级访问控制取消，见 CHANGELOG 2.5.0）。
// 含义：列在这里的来源即使触发了自动拉黑规则，也绝不会被封。
type WhitelistRequest struct {
	Whitelist          []string `json:"whitelist"`
	AutoBlock          bool     `json:"autoBlock"`
	AutoBlockThreshold int64    `json:"autoBlockThreshold"`
	AutoBlockMinutes   int      `json:"autoBlockMinutes"`
	TrustedProxies     []string `json:"trustedProxies"`
}

// AuditBlockView 封禁一个 IP（或网段）
//
// 封禁本身是**全局生效**的（proxy 入口会查 audit.IsBlocked，见 handler.go）。
// 除此之外还会把这条 IP **同步进它访问过的那些服务的门禁规则** ——
// 用户是在审计页看着"这个 IP 扫了我的服务"才去封的，封完在对应服务上
// 也该看得到这条规则（不然服务表单里一片空白，看不出它被保护了）。
func (AuditApi) AuditBlockView(c *gin.Context) {
	cr := middleware.GetBindRequest[BlockRequest](c)
	if err := audit.Block(cr.IP, cr.Reason, cr.Minutes); err != nil {
		res.FailWithMsg(err.Error(), c)
		return
	}

	synced := syncBlockToServices(cr.IP)
	if synced > 0 {
		res.OkWithMsg(fmt.Sprintf("已封禁，并同步到 %d 个服务的门禁规则", synced), c)
		return
	}
	res.OkWithMsg("已封禁", c)
}

// AuditWhitelistView 保存"永不封禁"名单。
//
// 前端把名单整份传回来覆盖 —— 白名单是配置项（不是累计事件），
// 整份覆盖最符合直觉：界面上删掉一条就是真的删了。
//
// 顺带保存自动拉黑的设置：它们在同一个表单里，一起存省得两次请求。
func (AuditApi) AuditWhitelistView(c *gin.Context) {
	cr := middleware.GetBindRequest[WhitelistRequest](c)

	cleaned := nonNil(trimAll(cr.Whitelist))
	// 网段要先校验：写错一个字符不报错的话，用户会以为"加了白名单怎么还是被封"
	for _, w := range cleaned {
		if _, _, e1 := net.ParseCIDR(w); e1 != nil && net.ParseIP(w) == nil {
			res.FailWithMsg(fmt.Sprintf("这条不是合法的 IP 或网段：%s", w), c)
			return
		}
	}

	err := audit.Runtime.Update(func(cfg *audit.Config) error {
		cfg.Whitelist = cleaned
		cfg.AutoBlock = cr.AutoBlock
		cfg.AutoBlockThreshold = cr.AutoBlockThreshold
		cfg.AutoBlockMinutes = cr.AutoBlockMinutes
		cfg.TrustedProxies = nonNil(trimAll(cr.TrustedProxies))
		return nil
	})
	if err != nil {
		res.FailWithMsg(err.Error(), c)
		return
	}
	res.OkWithMsg("已保存", c)
}

// 只动两类服务：
//   - 门禁模式为空（不限制）→ 改成黑名单模式并加入这条 IP
//   - 已经是黑名单 → 追加这条 IP（去重）
//
// **不动白名单模式的服务**：那种服务只放行名单内，这个 IP 本来就不在内，
// 再加一条 deny 是重复表达，而且容易让人误以为白名单失效。
//
// 返回实际改动的服务数。
func syncBlockToServices(ip string) int {
	ip = strings.TrimSpace(ip)
	if ip == "" {
		return 0
	}

	// 这个 IP 访问过哪些服务：翻最近几天的审计记录。
	//
	// 两类服务分开收：
	//   · 反代站点 —— 记录里带 SiteID
	//   · 穿透服务 —— **没有 SiteID**（不是 HTTP），身份在 Ref 里，
	//     形如 "s<设备ID>-<服务ID>"（见 modules/stun/tunnel_access.go）。
	//     以前只看 SiteID，于是"只扫过穿透服务"的 IP 封完一个服务都没同步到 ——
	//     用户要的正是"在审计里封了就自动同步到对应的服务"，这条不能漏。
	siteIDs := map[uint]bool{}
	tunnels := map[[2]uint]bool{}
	found := false
	for _, day := range audit.Days() {
		list, err := audit.ReadDay(day, ip, 500)
		if err != nil {
			continue
		}
		for _, r := range list {
			if r.SiteID > 0 {
				siteIDs[r.SiteID] = true
				found = true
				continue
			}
			if d, s, ok := parseTunnelRef(r.Ref); ok {
				tunnels[[2]uint{d, s}] = true
				found = true
			}
		}
		if found {
			break // 最近一天命中就够了，不必翻更早
		}
	}

	changed := 0
	for id := range siteIDs {
		if addDenyRuleToSite(id, ip) {
			changed++
		}
	}
	for k := range tunnels {
		if addDenyRuleToTunnel(k[0], k[1], ip) {
			changed++
		}
	}
	return changed
}

// parseTunnelRef 解析穿透服务的标识 "s<设备ID>-<服务ID>"。
func parseTunnelRef(ref string) (deviceID uint, serviceID uint, ok bool) {
	ref = strings.TrimSpace(ref)
	if len(ref) < 4 || ref[0] != 's' {
		return 0, 0, false
	}
	sep := strings.IndexByte(ref[1:], '-')
	if sep < 0 {
		return 0, 0, false
	}
	d, err1 := strconv.ParseUint(ref[1:1+sep], 10, 32)
	s, err2 := strconv.ParseUint(ref[2+sep:], 10, 32)
	if err1 != nil || err2 != nil {
		return 0, 0, false
	}
	return uint(d), uint(s), true
}

// addDenyRuleToTunnel 给某个穿透服务加一条拒绝规则，返回是否真的改了。
//
// 语义和 addDenyRuleToSite 完全一致（空模式→黑名单；已是黑名单→追加去重；
// 白名单模式不动），只是落点换成穿透的配置。
func addDenyRuleToTunnel(deviceID, serviceID uint, ip string) bool {
	idx := -1
	for i := range stun.Runtime.Config.Devices {
		if stun.Runtime.Config.Devices[i].DeviceID == deviceID {
			idx = i
			break
		}
	}
	if idx < 0 {
		return false
	}
	sidx := -1
	for i := range stun.Runtime.Config.Devices[idx].Services {
		if stun.Runtime.Config.Devices[idx].Services[i].ID == serviceID {
			sidx = i
			break
		}
	}
	if sidx < 0 {
		return false
	}

	svc := &stun.Runtime.Config.Devices[idx].Services[sidx]
	if svc.Access.Mode == "allow" {
		return false
	}
	for _, r := range svc.Access.Rules {
		if strings.TrimSpace(r) == ip {
			return false
		}
	}

	svc.Access.Mode = "deny"
	svc.Access.Rules = append(append([]string{}, svc.Access.Rules...), ip)
	if err := stun.UpdateConfig(stun.Runtime.Config); err != nil {
		logrus.Warnf("[audit] 封禁同步到穿透服务 %s 失败：%v", "s"+strconv.FormatUint(uint64(deviceID), 10)+
			"-"+strconv.FormatUint(uint64(serviceID), 10), err)
		return false
	}
	return true
}

// addDenyRuleToSite 给某个站点加一条拒绝规则，返回是否真的改了
func addDenyRuleToSite(siteID uint, ip string) bool {
	var target *proxymodel.Site
	cfg := proxy.Runtime.Snapshot()
	for i := range cfg.Sites {
		if cfg.Sites[i].ID == siteID {
			target = &cfg.Sites[i]
			break
		}
	}
	if target == nil {
		return false
	}
	// 白名单模式的服务不加：那种服务只放行名单内，这个 IP 本来就不在内，
	// 再加 deny 是重复表达，反而让人误以为白名单失效了。
	if target.Access.Mode == "allow" {
		return false
	}
	// 去重
	for _, r := range target.Access.Rules {
		if strings.TrimSpace(r) == ip {
			return false
		}
	}

	updated := *target
	updated.Access.Mode = "deny"
	updated.Access.Rules = append(append([]string{}, target.Access.Rules...), ip)
	if _, err := proxy.SaveSite(updated); err != nil {
		logrus.Warnf("[audit] 封禁同步到站点 #%d 失败：%v", siteID, err)
		return false
	}
	return true
}

type UnblockRequest struct {
	IP string `json:"ip"`
}

// AuditUnblockView 解除封禁
func (AuditApi) AuditUnblockView(c *gin.Context) {
	cr := middleware.GetBindRequest[UnblockRequest](c)
	if err := audit.Unblock(cr.IP); err != nil {
		res.FailWithMsg("解除失败："+err.Error(), c)
		return
	}
	res.OkWithMsg("已解除", c)
}

type BatchBlockRequest struct {
	IPs     []string `json:"ips"`
	Reason  string   `json:"reason"`
	Minutes int      `json:"minutes"`
}

// AuditBatchBlockView 批量封禁（"一键拉黑所有可疑 IP"用它）
//
// 逐个封并收集失败项，**不是一条失败就全放弃** —— 批量操作里混着
// 一两个白名单地址是常事，不该让整批失败。
func (AuditApi) AuditBatchBlockView(c *gin.Context) {
	cr := middleware.GetBindRequest[BatchBlockRequest](c)
	if len(cr.IPs) == 0 {
		res.FailWithMsg("没有要封禁的地址", c)
		return
	}

	var ok int
	failed := map[string]string{}
	for _, ip := range cr.IPs {
		if err := audit.Block(ip, cr.Reason, cr.Minutes); err != nil {
			failed[ip] = err.Error()
			continue
		}
		ok++
	}

	res.OkWithData(gin.H{
		"ok":     ok,
		"failed": failed,
		"total":  len(cr.IPs),
	}, c)
}

type SettingsRequest struct {
	Enabled            bool     `json:"enabled"`
	KeepDays           int      `json:"keepDays"`
	MaxFileMB          int      `json:"maxFileMB"`
	AutoBlock          bool     `json:"autoBlock"`
	AutoBlockThreshold int64    `json:"autoBlockThreshold"`
	AutoBlockMinutes   int      `json:"autoBlockMinutes"`
	Whitelist          []string `json:"whitelist"`
	TrustedProxies     []string `json:"trustedProxies"`
}

// AuditSettingsView 保存设置
func (AuditApi) AuditSettingsView(c *gin.Context) {
	cr := middleware.GetBindRequest[SettingsRequest](c)

	if cr.KeepDays < 1 || cr.KeepDays > 365 {
		res.FailWithMsg("明细保留天数要在 1 到 365 之间", c)
		return
	}
	if cr.MaxFileMB < 1 || cr.MaxFileMB > 1024 {
		res.FailWithMsg("单天文件上限要在 1 到 1024 MB 之间", c)
		return
	}

	err := audit.Runtime.Update(func(cfg *audit.Config) error {
		cfg.Enabled = cr.Enabled
		cfg.KeepDays = cr.KeepDays
		cfg.MaxFileMB = cr.MaxFileMB
		cfg.AutoBlock = cr.AutoBlock
		cfg.AutoBlockThreshold = cr.AutoBlockThreshold
		cfg.AutoBlockMinutes = cr.AutoBlockMinutes
		cfg.Whitelist = nonNil(trimAll(cr.Whitelist))
		cfg.TrustedProxies = nonNil(trimAll(cr.TrustedProxies))
		return nil
	})
	if err != nil {
		res.FailWithMsg("保存失败："+err.Error(), c)
		return
	}

	audit.SetEnabled(cr.Enabled)
	res.OkWithMsg("已保存", c)
}

func nonNil(in []string) []string {
	if in == nil {
		return []string{}
	}
	return in
}

func trimAll(in []string) []string {
	out := make([]string, 0, len(in))
	for _, s := range in {
		if s = strings.TrimSpace(s); s != "" {
			out = append(out, s)
		}
	}
	return out
}

// ===================== 登录事件 =====================

// AuditLoginsView 某天的登录尝试（新的在前）
//
// 和访问审计分开看：这是"谁在试着进后台"，那个是"谁访问了服务"。
func (AuditApi) AuditLoginsView(c *gin.Context) {
	day := strings.TrimSpace(c.Query("day"))
	ip := strings.TrimSpace(c.Query("ip"))
	limit, _ := strconv.Atoi(c.DefaultQuery("limit", "100"))
	if limit <= 0 {
		limit = 100
	}
	page, _ := strconv.Atoi(c.DefaultQuery("page", "1"))
	if page <= 0 {
		page = 1
	}
	// 同 AuditRecordsView：日期要拼进文件路径，先校验再往下传
	if day != "" && !validDay.MatchString(day) {
		res.FailWithMsg("日期格式不对（应为 YYYY-MM-DD）", c)
		return
	}
	if day == "" {
		// 登录事件是单独的文件（login-<日期>.jsonl），不能用访问记录的日期列表
		days := audit.LoginDays()
		if len(days) == 0 {
			res.OkWithData(gin.H{"list": []audit.LoginEvent{}, "day": ""}, c)
			return
		}
		day = days[0]
	}

	// 先取当天全量再切片。
	// ReadLoginEvents 的 limit 是"取前 N 条"，直接传 limit 的话翻到第 2 页
	// 只会再拿到同一批前 N 条 —— 看起来像翻页没生效。
	all, err := audit.ReadLoginEvents(day, ip, 0)
	if err != nil {
		// 那天没有登录记录，不是错误
		res.OkWithData(gin.H{"list": []audit.LoginEvent{}, "day": day, "days": audit.LoginDays()}, c)
		return
	}
	if all == nil {
		all = []audit.LoginEvent{}
	}

	total := len(all)
	start := (page - 1) * limit
	if start > total {
		start = total
	}
	end := start + limit
	if end > total {
		end = total
	}
	res.OkWithData(gin.H{
		"list":  all[start:end],
		"day":   day,
		"days":  audit.LoginDays(),
		"total": total,
		"page":  page,
		"limit": limit,
		// summary 是**当天**的汇总，不随分页变化 —— 它是"这天总共怎样"的答案
		"summary": audit.SummarizeLogins(day),
	}, c)
}
