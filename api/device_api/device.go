package device_api

import (
	"github.com/chenggegedeid/catflap/middleware"
	"github.com/chenggegedeid/catflap/modules/device"
	"github.com/chenggegedeid/catflap/utils/res"

	"github.com/gin-gonic/gin"
)

// DeviceApi 设备管理。
//
// 设备是**公共概念**，不属于穿透也不属于反代 —— 所以它有自己一套接口，
// 而不是塞在 stun_api 里。三个功能（穿透 / 反代 / 端口转发）都从这里选机器。
type DeviceApi struct{}

type DeviceSaveRequest struct {
	ID   uint   `json:"id"` // 0 = 新增
	Name string `json:"name"`
	IP   string `json:"ip"`
}

type DeviceDeleteRequest struct {
	ID uint `json:"id"`
}

// DeviceListView 全部设备
func (DeviceApi) DeviceListView(c *gin.Context) {
	res.OkWithData(device.List(), c)
}

// DeviceSaveView 新增或修改一台设备
func (DeviceApi) DeviceSaveView(c *gin.Context) {
	cr := middleware.GetBindRequest[DeviceSaveRequest](c)

	var d device.Device
	var err error
	if cr.ID == 0 {
		d, err = device.Add(cr.Name, cr.IP)
	} else {
		d, err = device.Update(cr.ID, cr.Name, cr.IP)
	}
	if err != nil {
		res.FailWithMsg(err.Error(), c)
		return
	}
	res.OkWithData(d, c)
}

// DeviceDeleteView 删一台设备
func (DeviceApi) DeviceDeleteView(c *gin.Context) {
	cr := middleware.GetBindRequest[DeviceDeleteRequest](c)
	if cr.ID == 0 {
		res.FailWithMsg("id 不能为空", c)
		return
	}
	if err := device.Delete(cr.ID); err != nil {
		res.FailWithMsg(err.Error(), c)
		return
	}
	res.OkWithMsg("已删除", c)
}
