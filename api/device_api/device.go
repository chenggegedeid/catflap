package device_api

import (
	"errors"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/chenggegedeid/catflap/core/lanscan"
	"github.com/chenggegedeid/catflap/middleware"
	"github.com/chenggegedeid/catflap/modules/device"
	"github.com/chenggegedeid/catflap/modules/proxy"
	proxymodel "github.com/chenggegedeid/catflap/modules/proxy/model"
	"github.com/chenggegedeid/catflap/modules/stun"
	"github.com/chenggegedeid/catflap/utils/res"

	"github.com/gin-gonic/gin"
	"github.com/sirupsen/logrus"
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
	// Cascade 连带删掉这台设备上的服务。
	//
	// 不带（false）时：如果这台设备上还有服务，直接拒绝并列出它们 ——
	// 让前端弹确认框给用户看清楚"会一起删掉什么"。
	// 带 true 时：设备和服务一起删。
	Cascade bool `json:"cascade"`
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
		// 改 IP 后同步旧规则地址时的**非致命**错误：配置（站点/转发的 backend/target）
		// 已经写进文件了，失败的是"重新监听端口"那一步（端口被别的程序占着）。
		// 那属于端口冲突，页面上的监听状态看得到，不该报成"改 IP 失败"。
		if strings.Contains(err.Error(), "同步旧规则的地址失败") {
			logrus.Warnf("[device] %v", err)
			stun.RefreshDevices()
			res.OkWithData(d, c)
			return
		}
		// IP 已被另一台占着：单独拎出来，给 409 + 明确的指引 ——
		// 用户看到"IP 已存在"不知道该怎么办，要说清"去删旧的那台"。
		if errors.Is(err, device.ErrIPExists) {
			ip := strings.TrimSpace(cr.IP)
			old := device.GetByIP(ip)
			hint := ""
			if old != nil {
				hint = fmt.Sprintf("（已有设备「%s」在用这个 IP）", old.Name)
			}
			res.Fail(http.StatusConflict,
				fmt.Sprintf("IP %s 已被占用%s。请先到「设备」页删掉使用这个 IP 的那台设备，再改这一台的 IP。", ip, hint), c)
			return
		}
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
	used := servicesOnDevice(d.DeviceID, d.IP)

	// 有服务且没带 cascade → 拒绝，并把清单交给前端弹确认框
	if len(used) > 0 && !cr.Cascade {
		res.Fail(http.StatusConflict,
			"这台设备上还有 "+strconv.Itoa(len(used))+" 个服务：\n\n  · "+
				strings.Join(used, "\n  · ")+"\n\n删除设备会**一并删掉**它们，且无法恢复。确认请再点一次删除。", c)
		return
	}

	// cascade：先删服务，再删设备（顺序反了会让服务变成孤儿 ——
	// 设备没了，那些服务还在配置里，但永远匹配不到设备、静默失效）
	var removed []string
	if cr.Cascade && len(used) > 0 {
		if names, err := stun.DeleteServicesOfDevice(d.DeviceID); err != nil {
			res.FailWithMsg("删除穿透服务失败："+err.Error(), c)
			return
		} else {
			removed = append(removed, names...)
		}

		cfg := proxy.Runtime.Snapshot()
		for _, site := range cfg.Sites {
			if site.DeviceID == d.DeviceID || (site.DeviceID == 0 && hostOfAddr(site.Backend) == d.IP) {
				if err := proxy.DeleteSite(site.ID); err != nil {
					// 端口被别的程序占着时 DeleteSite 会报"监听失败"，但**站点数据已经被删掉了**
					// （SaveConfig 在 Apply 之前）。那是端口冲突，不该中断"删设备"这件正事。
					if !isPortBusy(err) {
						res.FailWithMsg("删除站点 #"+strconv.Itoa(int(site.ID))+" 失败："+err.Error(), c)
						return
					}
					logrus.Warnf("[device] 删站点 #%d 后重新监听失败（端口冲突，不影响数据）：%v", site.ID, err)
				}
				removed = append(removed, "站点 "+siteLabel(site))
			}
		}
		for _, fwd := range cfg.Forwards {
			if fwd.DeviceID == d.DeviceID || (fwd.DeviceID == 0 && hostOfAddr(fwd.Target) == d.IP) {
				if err := proxy.DeleteForward(fwd.ID); err != nil {
					if !isPortBusy(err) {
						res.FailWithMsg("删除端口转发 #"+strconv.Itoa(int(fwd.ID))+" 失败："+err.Error(), c)
						return
					}
					logrus.Warnf("[device] 删转发 #%d 后重新监听失败（端口冲突，不影响数据）：%v", fwd.ID, err)
				}
				removed = append(removed, "转发 "+fwdLabel(fwd))
			}
		}
	}

	if err := device.Delete(cr.ID); err != nil {
		res.FailWithMsg(err.Error(), c)
		return
	}
	stun.RefreshDevices()

	if len(removed) > 0 {
		res.OkWithMsg("已删除设备，并连带删除 "+strconv.Itoa(len(removed))+" 个服务："+strings.Join(removed, "、"), c)
		return
	}
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
		// deviceId 优先，IP 兜底：改过设备 IP 后 deviceId 才是可靠的归属依据，
		// 只按 IP 会在"IP 已改但那条规则的 backend 还是旧的"时漏判，让站点变孤儿。
		if s.DeviceID == id || (s.DeviceID == 0 && hostOfAddr(s.Backend) == ip) {
			label := s.Description
			if label == "" && len(s.Hosts) > 0 {
				label = s.Hosts[0]
			}
			out = append(out, "站点："+nameOr(label, "未命名站点"))
		}
	}
	for _, f := range cfg.Forwards {
		if f.DeviceID == id || (f.DeviceID == 0 && hostOfAddr(f.Target) == ip) {
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

func siteLabel(s proxymodel.Site) string {
	if strings.TrimSpace(s.Description) != "" {
		return s.Description
	}
	if len(s.Hosts) > 0 {
		return s.Hosts[0]
	}
	return "#" + strconv.Itoa(int(s.ID))
}

func fwdLabel(f proxymodel.Forward) string {
	if strings.TrimSpace(f.Description) != "" {
		return f.Description
	}
	return "端口 " + strconv.Itoa(f.ListenPort)
}

// isPortBusy 判断错误是不是"端口被占用"（监听失败）。
// 这类错误只影响监听，不影响配置数据 —— 删除/修改操作遇到它可以继续。
func isPortBusy(err error) bool {
	if err == nil {
		return false
	}
	m := err.Error()
	return strings.Contains(m, "已被占用") || strings.Contains(m, "address already in use") ||
		strings.Contains(m, "permission denied")
}
