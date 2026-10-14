package stun_api

import (
	"fmt"
	"github.com/chenggegedeid/catflap/middleware"
	"github.com/chenggegedeid/catflap/modules/stun"
	"github.com/chenggegedeid/catflap/utils/res"
	"strings"

	"github.com/gin-gonic/gin"
)

// StunServersViewResponse 服务器列表 + 当前选中的最优
type StunServersViewResponse struct {
	Servers []string `json:"servers"`
	Best    string   `json:"best"`
}

// GetStunServersView 返回当前配置的 STUN 服务器列表和实测出的最优
func (StunApi) GetStunServersView(c *gin.Context) {
	best := ""
	if stun.Runtime.STUNService != nil {
		best = stun.Runtime.STUNService.BestSTUNServer
	}
	servers := []string{}
	if len(stun.Runtime.Config.StunServerList) > 0 {
		servers = stun.Runtime.Config.StunServerList
	}
	res.OkWithData(StunServersViewResponse{Servers: servers, Best: best}, c)
}

// StunServersResetView 把服务器列表恢复成内置的那一批。
//
// 什么时候会用到：用户为了排错把列表删得只剩一两个，或者加了一堆连不上的，
// 想回到出厂状态。没有这个入口的话，他得手打 17 行 —— 而列表一旦被清空，
// 打洞就彻底起不来，等于把自己锁在外面。
func (StunApi) StunServersResetView(c *gin.Context) {
	list := stun.DefaultSTUNServersCopy()

	stun.Runtime.Config.StunServerList = list
	if err := stun.UpdateConfig(stun.Runtime.Config); err != nil {
		res.FailWithMsg("保存配置失败", c)
		return
	}

	stun.RefreshSTUNService(list)
	best := ""
	if stun.Runtime.STUNService != nil {
		best = stun.Runtime.STUNService.BestSTUNServer
	}
	res.OkWithData(StunServersViewResponse{Servers: list, Best: best}, c)
}

type StunServersUpdateRequest struct {
	Servers []string `json:"servers"`
}

// StunServersUpdateView 保存 STUN 服务器列表，立即重测选优。
// 正在跑的打洞服务不受影响；下一次打洞或保活重连起用新列表。
func (StunApi) StunServersUpdateView(c *gin.Context) {
	cr := middleware.GetBindRequest[StunServersUpdateRequest](c)

	seen := make(map[string]bool)
	list := make([]string, 0, len(cr.Servers))
	for _, raw := range cr.Servers {
		s := strings.TrimSpace(raw)
		if s == "" {
			continue
		}
		norm, err := stun.NormalizeSTUNServer(s)
		if err != nil {
			res.FailWithMsg(fmt.Sprintf("%s：%v", s, err), c)
			return
		}
		if seen[norm] {
			continue
		}
		seen[norm] = true
		list = append(list, norm)
	}
	if len(list) == 0 {
		res.FailWithMsg("至少要保留一个 STUN 服务器", c)
		return
	}

	stun.Runtime.Config.StunServerList = list
	if err := stun.UpdateConfig(stun.Runtime.Config); err != nil {
		res.FailWithMsg("保存配置失败", c)
		return
	}

	// 同步重测一遍，几秒内返回；前端拿到响应时最优已是新列表里的
	stun.RefreshSTUNService(list)
	best := ""
	if stun.Runtime.STUNService != nil {
		best = stun.Runtime.STUNService.BestSTUNServer
	}
	res.OkWithData(StunServersViewResponse{Servers: list, Best: best}, c)
}
