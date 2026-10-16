package notify_api

import (
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/chenggegedeid/catflap/middleware"
	"github.com/chenggegedeid/catflap/modules/notify"
	"github.com/chenggegedeid/catflap/modules/stun"
	"github.com/chenggegedeid/catflap/utils/res"

	"github.com/gin-gonic/gin"
)

// NotifyApi 通知中心
type NotifyApi struct{}

// ===================== 读 =====================

type channelView struct {
	notify.Channel
	// Config 密钥类字段已掩码，真实值不回显（渠道里有 token，页面上不该飘）
	Config map[string]string `json:"config"`
	// UsedByRules 哪些规则在引用它，删除前给用户看
	UsedByRules []string `json:"usedByRules"`
}

type ConfigViewResponse struct {
	Channels []channelView      `json:"channels"`
	Rules    []notify.Rule      `json:"rules"`
	Events   []notify.EventMeta `json:"events"`
	// Migrated 是否已从旧 webhook 迁移过
	Migrated bool `json:"migrated"`
	// CooldownSeconds 同一事件的最小发送间隔
	CooldownSeconds int `json:"cooldownSeconds"`
	// LegacyWebhookFallback 回退开关状态
	LegacyWebhookFallback bool `json:"legacyWebhookFallback"`
	// DefaultChannelIDs 新建服务时**默认用**的渠道列表 —— 通知模块的全局默认。
	// 服务表单把它作为"新服务默认 channelIds"，免得每个新建都要去选一遍。
	// 默认取所有启用的渠道（让用户去通知页"启用"哪些，新服务就用哪些） ——
	// 比专门存一个 default 配置简单，且和"通知页看到的可用渠道"同步。
	DefaultChannelIDs []string `json:"defaultChannelIds"`
	// ServiceRefs 哪些穿透服务在用哪些渠道（通知页要展示，避免"我配了却没生效"）。
	ServiceRefs []ServiceRefView `json:"serviceRefs"`
}

// ServiceRefView 一个穿透服务对通知渠道的引用（通知页里聚合展示用）
type ServiceRefView struct {
	DeviceID    uint     `json:"deviceId"`
	ServiceID   uint     `json:"serviceId"`
	ServiceName string   `json:"serviceName"`
	ChannelIDs  []string `json:"channelIds"`
}

// NotifyConfigView 通知中心页要的全部数据
func (NotifyApi) NotifyConfigView(c *gin.Context) {
	cfg := notify.Runtime.Snapshot()

	channels := make([]channelView, 0, len(cfg.Channels))
	for _, ch := range cfg.Channels {
		// 注意：这几个切片必须给**空切片**而不是 nil。
		// Go 把 nil 切片序列化成 null，前端拿到 null 再取 .length 就是
		// TypeError —— 整个页面白屏，而且控制台只有那么一句，
		// 看不出跟哪个字段有关。这条坑在 home 模块也踩过一次。
		used := notify.Runtime.ChannelUsedByRules(ch.ID)
		if used == nil {
			used = []string{}
		}
		channels = append(channels, channelView{
			Channel:     ch,
			Config:      maskSecret(ch.Type, ch.Config),
			UsedByRules: used,
		})
	}

	rules := cfg.Rules
	if rules == nil {
		rules = []notify.Rule{}
	}
	events := notify.AllEvents()
	if events == nil {
		events = []notify.EventMeta{}
	}

	res.OkWithData(ConfigViewResponse{
		Channels:              channels,
		Rules:                 rules,
		Events:                events,
		Migrated:              notify.IsMigrated(),
		CooldownSeconds:       cfg.CooldownSeconds,
		LegacyWebhookFallback: cfg.LegacyWebhookFallback,
		DefaultChannelIDs:     enabledChannelIDs(cfg.Channels),
		ServiceRefs:           toServiceRefViews(stun.ServiceChannelRefs()),
	}, c)
}

// toServiceRefViews 透传 stun 的引用结构到 api 层的视图结构（两者形状一致但不同包）
func toServiceRefViews(in []stun.ServiceChannelRef) []ServiceRefView {
	out := make([]ServiceRefView, 0, len(in))
	for _, r := range in {
		out = append(out, ServiceRefView{
			DeviceID:    r.DeviceID,
			ServiceID:   r.ServiceID,
			ServiceName: r.ServiceName,
			ChannelIDs:  r.ChannelIDs,
		})
	}
	return out
}

// enabledChannelIDs 收集所有启用的渠道 ID（按配置顺序）。
//
// 作为"通知模块默认渠道"：服务新建时不传 channelIds 就用这个。
// 跟"通知页里看到的可用渠道"一致 —— 通知页关了某个渠道，新服务
// 自然就不会用上它。
func enabledChannelIDs(chs []notify.Channel) []string {
	out := make([]string, 0, len(chs))
	for _, ch := range chs {
		if ch.Enabled {
			out = append(out, ch.ID)
		}
	}
	return out
}

// secretKeys 各渠道里属于密钥的键。读接口要掩码，写接口要"留空=不改动"。
var secretKeys = map[notify.ChannelType][]string{
	notify.ChannelWebhook:    {"url"},
	notify.ChannelDingTalk:   {"url"},
	notify.ChannelWeCom:      {"url"},
	notify.ChannelFeishu:     {"url"},
	notify.ChannelBark:       {"key"},
	notify.ChannelServerChan: {"key"},
	notify.ChannelPushPlus:   {"token"},
}

const mask = "••••••••"

func maskSecret(t notify.ChannelType, in map[string]string) map[string]string {
	out := make(map[string]string, len(in))
	for k, v := range in {
		out[k] = v
	}
	for _, k := range secretKeys[t] {
		if out[k] != "" {
			out[k] = mask
		}
	}
	return out
}

// ===================== 渠道：增 / 改 / 删 / 测 =====================

type ChannelSaveRequest struct {
	ID      string             `json:"id"`
	Name    string             `json:"name"`
	Type    notify.ChannelType `json:"type"`
	Enabled bool               `json:"enabled"`
	Config  map[string]string  `json:"config"`
}

// NotifyChannelSaveView 新增或更新一个渠道
func (NotifyApi) NotifyChannelSaveView(c *gin.Context) {
	cr := middleware.GetBindRequest[ChannelSaveRequest](c)
	name := strings.TrimSpace(cr.Name)
	if name == "" {
		res.FailWithMsg("渠道名称不能为空", c)
		return
	}
	if cr.Type == "" {
		res.FailWithMsg("渠道类型不能为空", c)
		return
	}

	newID := ""
	err := notify.Runtime.Update(func(cfg *notify.Config) error {
		var existing *notify.Channel
		for i := range cfg.Channels {
			if cfg.Channels[i].ID == cr.ID && cr.ID != "" {
				existing = &cfg.Channels[i]
				break
			}
		}

		if existing == nil {
			newID = newChannelID(cr.Type, name, cfg.Channels)
			cfg.Channels = append(cfg.Channels, notify.Channel{
				ID:        newID,
				Name:      name,
				Type:      cr.Type,
				Enabled:   cr.Enabled,
				Config:    cleanConfig(cr.Type, cr.Config, nil),
				CreatedAt: time.Now(),
			})
			return nil
		}

		existing.Name = name
		existing.Type = cr.Type
		existing.Enabled = cr.Enabled
		// 掩码值不覆盖原值：用户没重新填就保持原样
		existing.Config = cleanConfig(cr.Type, cr.Config, existing.Config)
		existing.UpdatedAt = time.Now()
		return nil
	})
	if err != nil {
		res.FailWithMsg("保存失败："+err.Error(), c)
		return
	}
	res.OkWithData(gin.H{"id": newID}, c)
}

// cleanConfig 处理提交上来的配置。
//
// 掩码（面板回显的那串圆点）和空串都表示"不改动"，沿用 old 里的值。
// 不这么处理的话，用户只想改个名字，保存后 token 就变成一串圆点了。
func cleanConfig(t notify.ChannelType, in, old map[string]string) map[string]string {
	out := map[string]string{}
	for k, v := range in {
		v = strings.TrimSpace(v)
		if isMask(v) || v == "" {
			if old != nil {
				if ov, ok := old[k]; ok {
					out[k] = ov
				}
			}
			continue
		}
		out[k] = strings.TrimSpace(v)
	}
	// 老值里还有、这次没提交的键也保留（表单只改一部分时）
	if old != nil {
		for k, v := range old {
			if _, ok := out[k]; !ok {
				if _, submitted := in[k]; !submitted {
					out[k] = v
				}
			}
		}
	}
	_ = t
	return out
}

func isMask(s string) bool {
	if s == "" {
		return false
	}
	for _, r := range s {
		if r != '•' && r != '*' {
			return false
		}
	}
	return true
}

func newChannelID(t notify.ChannelType, name string, existing []notify.Channel) string {
	base := "ch-" + string(t)
	for i := 1; ; i++ {
		candidate := base + "-" + strconv.Itoa(i)
		dup := false
		for _, ch := range existing {
			if ch.ID == candidate {
				dup = true
				break
			}
		}
		if !dup {
			return candidate
		}
	}
}

type ChannelDeleteRequest struct {
	ID string `json:"id"`
}

// NotifyChannelDeleteView 删渠道（同时从所有规则里摘掉引用）
func (NotifyApi) NotifyChannelDeleteView(c *gin.Context) {
	cr := middleware.GetBindRequest[ChannelDeleteRequest](c)
	if cr.ID == "" {
		res.FailWithMsg("id 不能为空", c)
		return
	}

	err := notify.Runtime.Update(func(cfg *notify.Config) error {
		kept := make([]notify.Channel, 0, len(cfg.Channels))
		for _, ch := range cfg.Channels {
			if ch.ID != cr.ID {
				kept = append(kept, ch)
			}
		}
		cfg.Channels = kept
		// 规则里的引用一起清掉，否则会留下指向不存在渠道的悬空 ID
		for i := range cfg.Rules {
			ids := make([]string, 0, len(cfg.Rules[i].ChannelIDs))
			for _, id := range cfg.Rules[i].ChannelIDs {
				if id != cr.ID {
					ids = append(ids, id)
				}
			}
			cfg.Rules[i].ChannelIDs = ids
		}
		// **穿透服务的 channelIds 也要一起清**：服务里也引用了渠道 ID，
		// 不清的话会留下指向不存在渠道的悬空 ID，发通知时 nil 报错。
		// （渠道在"通知"页全局 —— 服务只是引用它，删渠道 = 通知配置没了，
		// 服务不应该再继续用这个 ID。）
		// 失败要往上抛：不然界面上渠道已经没了，服务里还留着悬空 ID，
		// 用户看到的是"删干净了"，实际两边不一致且没有任何提示。
		if err := stun.CleanupDeletedChannel(cr.ID); err != nil {
			return fmt.Errorf("清理服务里的渠道引用失败：%w", err)
		}
		return nil
	})
	if err != nil {
		res.FailWithMsg("删除失败："+err.Error(), c)
		return
	}
	res.OkWithMsg("已删除", c)
}

type ChannelTestRequest struct {
	ID string `json:"id"`
	// 带配置直接测（还没保存时也能测通不通）
	Type   notify.ChannelType `json:"type"`
	Config map[string]string  `json:"config"`
}

// NotifyServiceRefsView 通知页要的"哪些服务在用哪些渠道"。
// （channelRefs 是 stun 侧从每个服务的 channelIds 聚出来的）
func (NotifyApi) NotifyServiceRefsView(c *gin.Context) {
	res.OkWithData(stun.ServiceChannelRefs(), c)
}

// NotifyChannelTestView 测试发送。同步等结果 —— 用户点了就该立刻知道通不通。
func (NotifyApi) NotifyChannelTestView(c *gin.Context) {
	cr := middleware.GetBindRequest[ChannelTestRequest](c)

	var ch notify.Channel
	if cr.ID != "" {
		found, ok := notify.Runtime.FindChannel(cr.ID)
		if !ok {
			res.FailWithMsg("渠道不存在", c)
			return
		}
		ch = found
	} else {
		ch = notify.Channel{
			ID:     "test",
			Name:   "测试",
			Type:   cr.Type,
			Config: cleanConfig(cr.Type, cr.Config, nil),
		}
	}

	if err := notify.TestChannel(ch); err != nil {
		// 不能把 err.Error() 原样回给界面：渠道凭证（token / sendkey）
		// 大多拼在 URL 上，Go 的 http 报错会把整条 URL 带出来。
		// notify.ScrubChannelSecrets 抹掉这个渠道自己配过的所有值。
		res.FailWithMsg("发送失败："+notify.ScrubChannelSecrets(err.Error(), ch), c)
		return
	}
	res.OkWithMsg("已发送，请查看是否收到", c)
}

// ServiceTestRequest 按服务自己的配置试发一条通知。
//
// 界面上「发一条测试通知」按钮用它：把服务选的渠道和写的模板原样送来，
// 服务端走真实事件那条路径渲染 + 发送，逐渠道回结果。
type ServiceTestRequest struct {
	ChannelIDs []string          `json:"channelIds"`
	Template   string            `json:"template"`
	Fields     map[string]string `json:"fields"`
}

// NotifyServiceTestView 试发通知，逐渠道返回成功/失败
func (NotifyApi) NotifyServiceTestView(c *gin.Context) {
	cr := middleware.GetBindRequest[ServiceTestRequest](c)
	if len(cr.ChannelIDs) == 0 {
		res.FailWithMsg("还没有选通知渠道，先在上面勾一个", c)
		return
	}
	results := notify.TestServiceNotify(cr.ChannelIDs, cr.Template, cr.Fields)

	ok := 0
	for _, r := range results {
		if r.OK {
			ok++
		}
	}
	if ok == 0 && len(results) > 0 {
		res.FailWithMsg("全部发送失败："+results[0].Error, c)
		return
	}
	res.OkWithData(map[string]any{
		"results": results,
		"sent":    ok,
		"total":   len(results),
	}, c)
}

// ===================== 规则 =====================

type RuleSaveRequest struct {
	ID         string           `json:"id"`
	Name       string           `json:"name"`
	Event      notify.EventType `json:"event"`
	ChannelIDs []string         `json:"channelIds"`
	Enabled    bool             `json:"enabled"`
}

// NotifyRuleSaveView 新增或更新一条规则
func (NotifyApi) NotifyRuleSaveView(c *gin.Context) {
	cr := middleware.GetBindRequest[RuleSaveRequest](c)
	if cr.Event == "" {
		res.FailWithMsg("要选一个事件", c)
		return
	}
	if len(cr.ChannelIDs) == 0 {
		res.FailWithMsg("至少选一个渠道，否则这条规则不会发任何东西", c)
		return
	}

	err := notify.Runtime.Update(func(cfg *notify.Config) error {
		for i := range cfg.Rules {
			if cfg.Rules[i].ID != cr.ID || cr.ID == "" {
				continue
			}
			cfg.Rules[i].Name = strings.TrimSpace(cr.Name)
			cfg.Rules[i].Event = cr.Event
			cfg.Rules[i].ChannelIDs = cr.ChannelIDs
			cfg.Rules[i].Enabled = cr.Enabled
			cfg.Rules[i].UpdatedAt = time.Now()
			return nil
		}
		cfg.Rules = append(cfg.Rules, notify.Rule{
			ID:         newRuleID(cfg.Rules),
			Name:       defaultRuleName(cr.Name, cr.Event),
			Event:      cr.Event,
			ChannelIDs: cr.ChannelIDs,
			Enabled:    cr.Enabled,
			CreatedAt:  time.Now(),
		})
		return nil
	})
	if err != nil {
		res.FailWithMsg("保存失败："+err.Error(), c)
		return
	}
	res.OkWithMsg("已保存", c)
}

func defaultRuleName(name string, ev notify.EventType) string {
	if n := strings.TrimSpace(name); n != "" {
		return n
	}
	for _, m := range notify.AllEvents() {
		if m.Type == ev {
			return m.Title
		}
	}
	return string(ev)
}

func newRuleID(existing []notify.Rule) string {
	for i := 1; ; i++ {
		candidate := "rule-" + strconv.Itoa(i)
		dup := false
		for _, r := range existing {
			if r.ID == candidate {
				dup = true
				break
			}
		}
		if !dup {
			return candidate
		}
	}
}

type RuleDeleteRequest struct {
	ID string `json:"id"`
}

// NotifyRuleDeleteView 删规则
func (NotifyApi) NotifyRuleDeleteView(c *gin.Context) {
	cr := middleware.GetBindRequest[RuleDeleteRequest](c)
	err := notify.Runtime.Update(func(cfg *notify.Config) error {
		kept := make([]notify.Rule, 0, len(cfg.Rules))
		for _, r := range cfg.Rules {
			if r.ID != cr.ID {
				kept = append(kept, r)
			}
		}
		cfg.Rules = kept
		return nil
	})
	if err != nil {
		res.FailWithMsg("删除失败："+err.Error(), c)
		return
	}
	res.OkWithMsg("已删除", c)
}
