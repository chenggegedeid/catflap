package audit_api

import (
	"strconv"
	"strings"

	"github.com/chenggegedeid/catflap/middleware"
	"github.com/chenggegedeid/catflap/modules/audit"
	"github.com/chenggegedeid/catflap/utils/res"

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

// AuditRecordsView 某个 IP 在某天的访问明细
func (AuditApi) AuditRecordsView(c *gin.Context) {
	day := strings.TrimSpace(c.Query("day"))
	ip := strings.TrimSpace(c.Query("ip"))
	limit, _ := strconv.Atoi(c.DefaultQuery("limit", "200"))
	if limit <= 0 {
		limit = 200
	}

	if day == "" {
		days := audit.Days()
		if len(days) == 0 {
			res.OkWithData(gin.H{"list": []audit.Record{}}, c)
			return
		}
		day = days[0]
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
func (AuditApi) AuditBlockView(c *gin.Context) {
	cr := middleware.GetBindRequest[BlockRequest](c)
	if err := audit.Block(cr.IP, cr.Reason, cr.Minutes); err != nil {
		res.FailWithMsg(err.Error(), c)
		return
	}
	res.OkWithMsg("已封禁", c)
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
