package stun_api

import (
	"github.com/chenggegedeid/catflap/middleware"
	"github.com/chenggegedeid/catflap/modules/stun"
	"github.com/chenggegedeid/catflap/utils/res"

	"github.com/gin-gonic/gin"
)

// ReorderServicesRequest 某台设备下的服务顺序
type ReorderServicesRequest struct {
	// DeviceID 哪台设备
	DeviceID uint `json:"deviceId"`
	// IDs 服务 ID 按想要的顺序（第一个最前）
	IDs []uint `json:"ids"`
}

// StunServicesReorderView 拖动排序后保存某台设备下的服务顺序
func (StunApi) StunServicesReorderView(c *gin.Context) {
	cr := middleware.GetBindRequest[ReorderServicesRequest](c)
	if cr.DeviceID == 0 {
		res.FailWithMsg("deviceId 不能为空", c)
		return
	}
	if len(cr.IDs) == 0 {
		res.FailWithMsg("ids 不能为空", c)
		return
	}
	if err := stun.ReorderServices(cr.DeviceID, cr.IDs); err != nil {
		res.FailWithMsg(err.Error(), c)
		return
	}
	res.OkWithMsg("已保存", c)
}
