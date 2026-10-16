package audit_api

import (
	"fmt"
	"regexp"

	"strconv"
	"strings"

	"github.com/chenggegedeid/catflap/middleware"
	"github.com/chenggegedeid/catflap/modules/audit"
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

// AuditIPsView 按 IP 的统计列表
func (AuditApi) AuditIPsView(c *gin.Context) {
	sortBy := c.DefaultQuery("sort", "errors")
	onlySuspicious := c.Query("suspicious") == "1"
	limit, _ := strconv.Atoi(c.DefaultQuery("limit", "100"))
	if limit <= 0 {
		limit = 100
	}

	list := audit.ListIPStats(sortBy, onlySuspicious, limit)
	if list == nil {
		list = []audit.IPStat{}
	}
	res.OkWithData(gin.H{"list": list}, c)
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

// syncBlockToServices 把封禁的 IP 写进"它访问过的服务"的门禁规则。
//
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

	list, err := audit.ReadLoginEvents(day, ip, limit)
	if err != nil {
		// 那天没有登录记录，不是错误
		res.OkWithData(gin.H{"list": []audit.LoginEvent{}, "day": day}, c)
		return
	}
	if list == nil {
		list = []audit.LoginEvent{}
	}
	res.OkWithData(gin.H{"list": list, "day": day, "summary": audit.SummarizeLogins(day)}, c)
}
