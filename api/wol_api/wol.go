package wol_api

import (
	"fmt"
	"time"

	"github.com/chenggegedeid/catflap/middleware"
	"github.com/chenggegedeid/catflap/modules/wol"
	"github.com/chenggegedeid/catflap/utils/res"

	"github.com/gin-gonic/gin"
	"github.com/sirupsen/logrus"
)

// WolApi WOL 相关接口
type WolApi struct{}

// WolTargetResponse 返回给前端的一台机器
type WolTargetResponse struct {
	ID        string `json:"id"`
	Name      string `json:"name"`
	MAC       string `json:"mac"`
	Broadcast string `json:"broadcast"`
	Port      int    `json:"port"`
	Note      string `json:"note,omitempty"`
	CreatedAt string `json:"createdAt"`
}

// WolListResponse 列表
type WolListResponse struct {
	Targets []WolTargetResponse `json:"targets"`
}

func toResponse(t wol.Target) WolTargetResponse {
	return WolTargetResponse{
		ID:        t.ID,
		Name:      t.Name,
		MAC:       t.MAC,
		Broadcast: t.Broadcast,
		Port:      t.Port,
		Note:      t.Note,
		CreatedAt: t.CreatedAt.Format("2006-01-02 15:04:05"),
	}
}

// WolListView 列出所有机器
func (WolApi) WolListView(c *gin.Context) {
	list := wol.ListTargets()
	out := make([]WolTargetResponse, 0, len(list))
	for _, t := range list {
		out = append(out, toResponse(t))
	}
	res.OkWithData(WolListResponse{Targets: out}, c)
}

// WolSaveRequest 新增/修改一台机器。ID 为空表示新增。
type WolSaveRequest struct {
	ID        string `json:"id"`
	Name      string `json:"name"`
	MAC       string `json:"mac"`
	Broadcast string `json:"broadcast"`
	Port      int    `json:"port"`
	Note      string `json:"note"`
}

func (r WolSaveRequest) validate() error {
	if r.Name == "" {
		return fmt.Errorf("名称不能为空")
	}
	if _, err := wol.ParseMAC(r.MAC); err != nil {
		return err
	}
	if r.Broadcast == "" {
		return fmt.Errorf("广播地址不能为空")
	}
	if r.Port < 0 || r.Port > 65535 {
		return fmt.Errorf("端口要在 0 到 65535 之间（留空或 0 则用 9）")
	}
	return nil
}

// WolAddView 新增一台机器
func (WolApi) WolAddView(c *gin.Context) {
	cr := middleware.GetBindRequest[WolSaveRequest](c)
	if err := cr.validate(); err != nil {
		res.FailWithMsg(err.Error(), c)
		return
	}

	cfg := snapshot()
	cfg.Targets = append(cfg.Targets, wol.Target{
		ID:        fmt.Sprintf("wol-%d", time.Now().UnixNano()),
		Name:      cr.Name,
		MAC:       cr.MAC,
		Broadcast: cr.Broadcast,
		Port:      cr.Port,
		Note:      cr.Note,
		CreatedAt: time.Now(),
	})
	if err := persist(cfg); err != nil {
		res.FailWithMsg("保存失败", c)
		return
	}
	res.OkWithMsg("已添加", c)
}

// WolUpdateView 修改一台机器
func (WolApi) WolUpdateView(c *gin.Context) {
	cr := middleware.GetBindRequest[WolSaveRequest](c)
	if cr.ID == "" {
		res.FailWithMsg("缺少 id", c)
		return
	}
	if err := cr.validate(); err != nil {
		res.FailWithMsg(err.Error(), c)
		return
	}

	cfg := snapshot()
	idx := -1
	for i := range cfg.Targets {
		if cfg.Targets[i].ID == cr.ID {
			idx = i
			break
		}
	}
	if idx == -1 {
		res.FailWithMsg("找不到这台机器", c)
		return
	}

	cfg.Targets[idx].Name = cr.Name
	cfg.Targets[idx].MAC = cr.MAC
	cfg.Targets[idx].Broadcast = cr.Broadcast
	cfg.Targets[idx].Port = cr.Port
	cfg.Targets[idx].Note = cr.Note

	if err := persist(cfg); err != nil {
		res.FailWithMsg("保存失败", c)
		return
	}
	res.OkWithMsg("已保存", c)
}

// WolDeleteRequest 删除
type WolDeleteRequest struct {
	ID string `json:"id"`
}

func (WolApi) WolDeleteView(c *gin.Context) {
	cr := middleware.GetBindRequest[WolDeleteRequest](c)
	cfg := snapshot()
	next := make([]wol.Target, 0, len(cfg.Targets))
	found := false
	for _, t := range cfg.Targets {
		if t.ID == cr.ID {
			found = true
			continue
		}
		next = append(next, t)
	}
	if !found {
		res.FailWithMsg("找不到这台机器", c)
		return
	}
	cfg.Targets = next
	if err := persist(cfg); err != nil {
		res.FailWithMsg("保存失败", c)
		return
	}
	res.OkWithMsg("已删除", c)
}

// WolWakeRequest 唤醒一台机器
type WolWakeRequest struct {
	ID string `json:"id"`
}

// WolWakeView 发送唤醒包。
//
// ⚠️ 返回的「成功」只代表**包发出去了**，不代表目标醒了。
// 目标可能没开 WOL、广播地址填错、或者跨了网段（二层广播过不了路由器）。
// 所以响应里要明确说这句，别让用户以为点完就该亮屏。
func (WolApi) WolWakeView(c *gin.Context) {
	cr := middleware.GetBindRequest[WolWakeRequest](c)
	t, ok := wol.FindTarget(cr.ID)
	if !ok {
		res.FailWithMsg("找不到这台机器", c)
		return
	}
	if err := wol.Wake(t.MAC, t.Broadcast, t.Port); err != nil {
		logrus.Warnf("WOL 唤醒 %s(%s) 失败：%v", t.Name, t.MAC, err)
		res.FailWithMsg(fmt.Sprintf("唤醒包发送失败：%v", err), c)
		return
	}
	logrus.Infof("WOL 已向 %s(%s) 发送唤醒包 → %s", t.Name, t.MAC, t.Broadcast)
	res.OkWithData(gin.H{
		"sent": true,
		"hint": "唤醒包已发出。能否醒来取决于目标机器有没有开 WOL、广播地址对不对；这里无法确认结果。",
	}, c)
}

// snapshot 取一份配置副本
func snapshot() wol.Config {
	list := wol.ListTargets()
	cfg := wol.Config{Targets: list}
	return cfg
}

// persist 写回磁盘并更新运行期
func persist(cfg wol.Config) error {
	if err := wol.SaveConfig(cfg); err != nil {
		return err
	}
	wol.SetConfig(cfg)
	return nil
}
