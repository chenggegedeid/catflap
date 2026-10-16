package stun_api

import (
	"github.com/chenggegedeid/catflap/modules/stun"
	"github.com/chenggegedeid/catflap/modules/stun/model"
	"github.com/chenggegedeid/catflap/utils/res"

	"github.com/gin-gonic/gin"
)

type StunConfigViewResponse struct {
	model.Config
	LocalIP       string                `json:"localIP"`
	PublicIP      string                `json:"publicIP"`
	NatRouterList []model.NatRouterInfo `json:"natRouterList"`
	Network       model.NetworkState    `json:"network"`
	// 顶层 Services：扁平化的服务列表，穿透页用它做列表渲染源。
	// 之前页面要"在设备树里翻 services"，那是一种耦合：把设备页的增删穿透到穿透页。
	// 改成顶层字段后，穿透页只读这里，设备树里的 services 只用于派生 deviceId → ip。
	Services []model.Service `json:"services"`
}

// 获取全部的stun配置文件信息
func (StunApi) GetStunConfigView(c *gin.Context) {
	// 扁平化所有 services（不依赖 device 树存在 —— 设备被删后服务也可能还在）
	flat := make([]model.Service, 0)
	for _, d := range stun.Runtime.Config.Devices {
		flat = append(flat, d.Services...)
	}

	data := StunConfigViewResponse{
		Config:        stun.Runtime.Config,
		LocalIP:       stun.Runtime.Network.LocalIP,
		PublicIP:      stun.Runtime.Network.PublicIP,
		NatRouterList: stun.Runtime.Network.NatRouterList,
		Network:       stun.Runtime.Network,
		Services:      flat,
	}

	if stun.Runtime.STUNService != nil && stun.Runtime.STUNService.BestSTUNServer != "" {
		data.BestSTUN = stun.Runtime.STUNService.BestSTUNServer
	}

	res.OkWithData(data, c)
}
