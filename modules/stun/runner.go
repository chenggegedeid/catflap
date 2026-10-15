package stun

import (
	"context"
	"github.com/chenggegedeid/catflap/modules/stun/model"
	"github.com/chenggegedeid/catflap/modules/webhook"
)

// STUN 需要的参数
type STUNRequest struct {
	ServiceName   string
	TargetIP      string
	InternalPort  uint16
	Protocol      string
	UseUPnP       bool
	WebhookConfig webhook.WebhookConfig

	// DeviceID / ServiceID 这条洞属于哪个设备的哪个服务。
	//
	// 只用于**流量统计按服务分组**（穿透的流量要算到具体服务头上，
	// 不然流量大屏里只知道总量、不知道是哪个服务在用）。
	// 转发逻辑本身不关心它们，填 0 也能正常跑（只是统计归到"未分组"）。
	DeviceID  uint
	ServiceID uint

	// PreferredLocalPort 打洞希望使用的本机端口；0 表示随机分配。
	//
	// 外部端口是 NAT 按「本机 IP + 本机端口」分配的，本机端口一变外部端口就变。
	// 想让外部端口稳定（这样才能用固定端口对外服务），就得固定本机端口。
	// 端口被占用时 Run 会自动退回随机，不会因此失败。
	PreferredLocalPort uint16

	// Redirect 入口域名跟着外部端口走的配置
	Redirect model.RedirectConfig
	// MCEntry MC Java 版联机的 SRV 记录跟着外部端口走
	MCEntry model.MCEntryConfig

	// TLSTerminate 是否由 Catflap 在洞口终结 TLS（仅 TCP 有效）
	TLSTerminate bool
	// CertID 绑定的证书；0 表示按 SNI / 默认证书自动匹配
	CertID uint
	// BackendHTTPS 拨内网时用 tls.Dial（等价 nginx 的 proxy_pass https://），只在终结 TLS 时成立
	BackendHTTPS bool
}

// 定义STUN 状态类型
type STUNStateType int

const (
	STUNMapped STUNStateType = iota
	STUNAlive
	STUNFailed
	STUNLog
)

// 当前STUN 服务的状态
type STUNState struct {
	State        STUNStateType // 状态
	ExternalIP   string        // 外部 IP
	ExternalPort uint16        // 外部端口
	Log          string        // 日志

	// LocalPort 本次打洞实际用的本机端口。
	//
	// 调度器记住它、下次重打时优先复用：本机端口不变，NAT 给的外部端口就不变，
	// 面板上那个"外部端口"才不会一直跳——否则用户刚拿到的地址过一会儿就失效。
	LocalPort uint16
}

type Runner interface {
	Run(ctx context.Context, req STUNRequest, onState func(STUNState)) error
}

// STUN Runner
type STUNRunner struct {
}

// 创建STUN Runner
func NewSTUNRunner() Runner {
	return STUNRunner{}
}
