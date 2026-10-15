package notify_api

import (
	"strconv"
	"strings"
	"time"

	"github.com/chenggegedeid/catflap/middleware"
	"github.com/chenggegedeid/catflap/modules/notify"
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
	}, c)
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
		res.FailWithMsg("发送失败："+err.Error(), c)
		return
	}
	res.OkWithMsg("已发送，请查看是否收到", c)
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
