package device_api

import (
	"strconv"
	"strings"
	"time"

	"github.com/chenggegedeid/catflap/core/lanscan"
	"github.com/chenggegedeid/catflap/middleware"
	"github.com/chenggegedeid/catflap/modules/device"
	"github.com/chenggegedeid/catflap/modules/proxy"
	"github.com/chenggegedeid/catflap/modules/stun"
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
	// 设备表变了，穿透那边的设备树（内存视图）要跟着刷新 ——
	// 不然刚加的设备在穿透页看不到，加服务会说"设备不存在"
	stun.RefreshDevices()
	res.OkWithData(d, c)
}

// DeviceDeleteView 删一台设备
func (DeviceApi) DeviceDeleteView(c *gin.Context) {
	cr := middleware.GetBindRequest[DeviceDeleteRequest](c)
	if cr.ID == 0 {
		res.FailWithMsg("id 不能为空", c)
		return
	}
	// 删之前先看有没有服务挂着：设备删了那些服务会静默失效
	// （配置还在、但不再运行），用户只会看到"不工作了"却看不出原因
	d, ok := device.Get(cr.ID)
	if !ok {
		res.FailWithMsg("设备不存在", c)
		return
	}
	if used := servicesOnDevice(d.DeviceID, d.IP); len(used) > 0 {
		res.FailWithMsg(
			"这台设备上还有 "+strconv.Itoa(len(used))+" 个服务，先删掉它们再删设备："+
				strings.Join(used, "、"), c)
		return
	}

	if err := device.Delete(cr.ID); err != nil {
		res.FailWithMsg(err.Error(), c)
		return
	}
	stun.RefreshDevices()
	res.OkWithMsg("已删除", c)
}

// servicesOnDevice 这台设备上挂了哪些服务（给删除前检查用）。
//
// **在 api 层做**：device 模块不认识服务（那是各业务模块的事），
// 而 proxy 和 stun 也不互相依赖。这层能看到两边，所以由它来数。
//
// 为什么要检查：穿透服务是按 deviceId 关联的，设备一删，那些服务
// 就成了孤儿 —— 配置里还在、但 hydrate 时匹配不到设备，
// 于是**静默地不再运行**。用户只会看到"服务不工作了"，看不出原因。
// 这种静默失效比报错难查得多，所以宁可不让删。
func servicesOnDevice(id uint, ip string) []string {
	var out []string

	// 穿透服务：按 deviceId
	for _, s := range stun.Runtime.Config.Services {
		if s.DeviceID == id {
			out = append(out, "穿透："+nameOr(s.Name, "未命名服务"))
		}
	}

	// 反代站点 / 端口转发：按 IP
	cfg := proxy.Runtime.Snapshot()
	for _, s := range cfg.Sites {
		if hostOfAddr(s.Backend) == ip {
			label := s.Description
			if label == "" && len(s.Hosts) > 0 {
				label = s.Hosts[0]
			}
			out = append(out, "站点："+nameOr(label, "未命名站点"))
		}
	}
	for _, f := range cfg.Forwards {
		if hostOfAddr(f.Target) == ip {
			out = append(out, "转发："+nameOr(f.Description, "端口 "+itoa(f.ListenPort)))
		}
	}
	return out
}

func nameOr(s, fallback string) string {
	if strings.TrimSpace(s) == "" {
		return fallback
	}
	return s
}

func hostOfAddr(addr string) string {
	addr = strings.TrimSpace(addr)
	if i := strings.LastIndex(addr, ":"); i > 0 {
		return addr[:i]
	}
	return addr
}

func itoa(n int) string {
	return strconv.Itoa(n)
}

// ===================== 扫单台机器的端口 =====================

type ScanHostRequest struct {
	IP string `json:"ip"`
}

// DeviceScanHostView 探一台机器上开了哪些端口。
//
// 给服务表单用：选好设备之后点「扫描」，列出这台机器的开放端口，
// 点一下就填进端口框 —— 不用去记端口号，也不用扫整个网段（那太慢）。
func (DeviceApi) DeviceScanHostView(c *gin.Context) {
	cr := middleware.GetBindRequest[ScanHostRequest](c)

	ip := strings.TrimSpace(cr.IP)
	if ip == "" {
		res.FailWithMsg("先选一台设备", c)
		return
	}

	// 超时兜底：机器关机时拨号会等满超时，不能把请求一直挂着。
	// 扫一台最多 3 秒，超时就返回已探到的（可能是空）。
	done := make(chan []lanscan.LanPort, 1)
	go func() {
		done <- lanscan.ScanHostPorts(ip, 700*time.Millisecond)
	}()

	select {
	case ports := <-done:
		if ports == nil {
			ports = []lanscan.LanPort{}
		}
		res.OkWithData(ports, c)
	case <-time.After(3 * time.Second):
		// 超时不报错，返回空列表让界面提示"没探到"
		res.OkWithData([]lanscan.LanPort{}, c)
	}
}

type ReorderRequest struct {
	// IDs 设备 ID 按想要的顺序排列（第一个最前）
	IDs []uint `json:"ids"`
}

// DeviceReorderView 拖动排序后保存设备顺序
func (DeviceApi) DeviceReorderView(c *gin.Context) {
	cr := middleware.GetBindRequest[ReorderRequest](c)
	if len(cr.IDs) == 0 {
		res.FailWithMsg("ids 不能为空", c)
		return
	}
	if err := device.Reorder(cr.IDs); err != nil {
		res.FailWithMsg(err.Error(), c)
		return
	}
	res.OkWithMsg("已保存", c)
}
