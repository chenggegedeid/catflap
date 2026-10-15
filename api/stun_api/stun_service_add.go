package stun_api

import (
	"fmt"
	"github.com/chenggegedeid/catflap/middleware"
	"github.com/chenggegedeid/catflap/modules/stun"
	"github.com/chenggegedeid/catflap/modules/stun/model"
	"github.com/chenggegedeid/catflap/modules/webhook"
	"github.com/chenggegedeid/catflap/utils/res"
	"time"

	"github.com/gin-gonic/gin"
)

type StunServiceAddViewRequest struct {
	DeviceID     uint   `json:"deviceId"`     // 设备ID
	Name         string `json:"name"`         // 服务名称,如 "SSH" / "Web管理" / "照片库"
	InternalPort uint16 `json:"internalPort"` // 内网端口,如 22
	Protocol     string `json:"protocol"`     // 传输协议 "TCP"/"UDP" (默认 TCP)
	Https        bool   `json:"https"`        // 是否是 https 服务（影响前端跳转 scheme）

	// UPnP 相关配置
	UseUPnP bool `json:"useUpnp"` // 是否启用 UPnP 自动端口映射 (默认 true)

	// 打洞用的本机端口；0 = 随机。
	// 外部端口是 NAT 照着本机端口分配的，钉住它外部端口才稳定。
	PunchLocalPort uint16 `json:"punchLocalPort"`

	// 对外访问
	Domain       string `json:"domain"`       // 对外域名，如 fw.example.com；留空回落公网 IP
	TLSTerminate bool   `json:"tlsTerminate"` // 由 Catflap 在洞口终结 TLS（仅 TCP）
	CertID       uint   `json:"certId"`       // 绑定证书 ID，0 表示按 SNI 自动匹配
	BackendHTTPS bool   `json:"backendHttps"` // 转发给内网时也用 HTTPS（tls.Dial），仅在 TLSTerminate 时成立

	Enabled     bool   `json:"enabled"`     // 服务是否启用 (默认 true)
	Description string `json:"description"` // 服务描述信息 (可选)

	WebHookConfig webhook.WebhookConfig `json:"webhookconfig"` // Webhook 配置文件

	// ChannelIDs 用哪些**全局通知渠道**。渠道在「通知」页里统一配置，
	// 这里只存 ID —— 同一个群机器人地址不该在每个服务里各存一份。
	ChannelIDs []string             `json:"channelIds"`
	Redirect   model.RedirectConfig `json:"redirect"` // 入口域名跟着外部端口走
	MCEntry    model.MCEntryConfig  `json:"mcEntry"`  // MC Java 版联机的 SRV 记录
}

func (StunApi) StunServiceAddView(c *gin.Context) {
	cr := middleware.GetBindRequest[StunServiceAddViewRequest](c)

	// 先做参数校验再查设备：这条校验和设备无关，放前面既能早点给反馈，
	// 也不会因为"设备不存在"掩盖真正的问题
	//
	// 内网端口不能是 Catflap 自己的后台端口：那等于把管理后台穿透到公网，
	// 任何人都能在洞口上试密码（猜中就完全接管）。
	//
	// 看着像"用户不会这么填"，实际是很容易犯的错 —— 想在外网访问管理页面时，
	// 最顺手的做法就是加个穿透服务指向它，心想"反正还要登录"。
	// 结果是把登录页和 /api/auth/login 一起暴露出去。
	if admin := stun.AdminPort(); admin > 0 && cr.InternalPort == uint16(admin) {
		res.FailWithMsg(fmt.Sprintf(
			"内网端口不能是 Catflap 自己的后台端口 %d——那等于把管理后台穿透到公网，任何人都能来试密码",
			admin), c)
		return
	}

	// 查找目标设备
	deviceIndex := -1
	for i, device := range stun.Runtime.Config.Devices {
		if device.DeviceID == cr.DeviceID {
			deviceIndex = i
			break
		}
	}
	if deviceIndex == -1 {
		res.FailWithMsg("设备不存在", c)
		return
	}

	// 生成新服务ID（取当前最大ID+1）
	var maxID uint = 0
	for _, svc := range stun.Runtime.Config.Devices[deviceIndex].Services {
		if svc.ID > maxID {
			maxID = svc.ID
		}
	}

	// 构建新服务
	domain, tlsTerminate, certID := normalizeTLSFields(cr.Protocol, cr.Domain, cr.TLSTerminate, cr.CertID)
	domain = fillDomainFromCert(domain, tlsTerminate, certID)

	// 没指定本机端口就自动挑一个（并试绑确认没人占着）。
	//
	// 外部端口是 NAT 照着本机端口发的，本机端口每次随机，外部端口就每次都变，
	// 配好的「域名+端口」很快失效。所以这里钉死一个：用户不填我们就挑，
	// 挑中写进配置，这个服务之后一直用它。服务删掉时配置里没这条了，
	// 端口自然回到可分配状态，不需要额外的「释放」动作。
	punchPort := cr.PunchLocalPort
	if punchPort == 0 {
		punchPort = stun.PickPunchLocalPort(
			stun.UsedPunchLocalPorts(stun.Runtime.Config.Devices),
			cr.Protocol,
		)
	}

	newService := model.Service{
		ID:             maxID + 1,
		Name:           cr.Name,
		InternalPort:   cr.InternalPort,
		Protocol:       cr.Protocol,
		Https:          cr.Https,
		UseUPnP:        cr.UseUPnP,
		PunchLocalPort: punchPort,
		Domain:         domain,
		TLSTerminate:   tlsTerminate,
		CertID:         certID,
		BackendHTTPS:   normalizeBackendHTTPS(cr.Protocol, cr.BackendHTTPS, tlsTerminate),
		Enabled:        cr.Enabled,
		Description:    cr.Description,
		WebHookConfig:  cr.WebHookConfig,
		ChannelIDs:     nonNilStrings(cr.ChannelIDs),
		Redirect:       cr.Redirect,
		MCEntry:        normalizeMCEntry(cr.Protocol, cr.MCEntry),
		UpdatedAt:      time.Now(),
	}

	// 添加服务到设备
	stun.Runtime.Config.Devices[deviceIndex].Services = append(
		stun.Runtime.Config.Devices[deviceIndex].Services,
		newService,
	)

	// 持久化配置到文件
	if err := stun.UpdateConfig(stun.Runtime.Config); err != nil {
		res.FailWithMsg("保存配置失败", c)
		return
	}

	// 启动该服务的 STUN 穿透
	// 修复：原版调用已删除的全局函数 stun.StartService，改为调度器实例方法
	// 注意：必须从 global 切片取指针，不能用局部变量 newService（append 后地址可能已变）
	device := &stun.Runtime.Config.Devices[deviceIndex]
	services := device.Services
	stun.Runtime.Scheduler.StartService(device, &services[len(services)-1])

	res.OkWithData(newService, c)
}
