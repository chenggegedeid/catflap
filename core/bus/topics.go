package bus

import (
	"context"
	"fmt"
)

// Topic 事件主题。命名规范：`<发布方模块>.<事件>`，全小写，点分。
//
// 所有主题**必须**在这里声明成常量，不要在业务代码里手写字符串 ——
// 否则「改了发布方忘了改订阅方」这种问题编译器帮不上忙，只能靠运行时发现事件没人收。
type Topic string

// ══════════════════════════════════════════════════════════════════════════
// 一、已启用：有发布方，也有订阅方
// ══════════════════════════════════════════════════════════════════════════
//
// 这两个主题对应 modules/stun/hooks.go 里原有的两组回调，迁移时一一对应。

const (
	// TopicServiceDeleted 某个穿透服务被删除。
	//
	// 发布方：modules/stun（NotifyServiceDeleted）
	// 订阅方：modules/home（门户页要同步删掉对应的服务卡片）
	// 载荷：  ServiceDeleted
	TopicServiceDeleted Topic = "stun.service_deleted"

	// TopicPublicIPChanged STUN 首次探到公网 IP，或家宽 IP 发生了变化。
	//
	// 发布方：modules/stun（notifyPublicIPChanged）
	// 订阅方：modules/ddns（立即推一次解析记录，别等下一个周期）
	// 载荷：  PublicIPChanged
	//
	// 注意：IP 为空表示「还没有」，不是「变了」，此时**不要**发布本事件。
	TopicPublicIPChanged Topic = "stun.public_ip_changed"
)

// ServiceDeleted 载荷：服务被删除。
type ServiceDeleted struct {
	DeviceID  uint `json:"deviceId"`
	ServiceID uint `json:"serviceId"`
}

// PublicIPChanged 载荷：公网 IP 有了或变了。
type PublicIPChanged struct {
	// IP 当前公网 IP。
	IP string `json:"ip"`
	// Previous 上一次的公网 IP；首次探到时为空。
	Previous string `json:"previous,omitempty"`
}

// ══════════════════════════════════════════════════════════════════════════
// 二、已预留：主题名已定，但**目前没有发布方**
// ══════════════════════════════════════════════════════════════════════════
//
// ⚠️ 订阅这些主题现在**不会收到任何事件**。它们是为后续版本先占位，
// 避免 v1.2/v1.3 时因为命名不统一而返工。实现对应功能时请一并补上发布点。

const (
	// TopicTunnelUp 一个洞打通了。载荷：TunnelStateChanged
	// 计划用于：v1.3 的可观测性（记录打通耗时）与通知。
	TopicTunnelUp Topic = "stun.tunnel_up"

	// TopicTunnelDown 一个洞断了。载荷：TunnelStateChanged
	// 计划用于：v1.3 的健康检查与故障转移 —— 断了之后自动切到备用上游。
	TopicTunnelDown Topic = "stun.tunnel_down"

	// TopicCertExpiring 证书即将到期。载荷：CertExpiring
	// 计划用于：v1.3 的通知渠道（到期前 14/7/3 天各提醒一次）。
	TopicCertExpiring Topic = "cert.expiring"

	// TopicCertIssued 证书签发或续期成功。载荷：CertIssued
	TopicCertIssued Topic = "cert.issued"

	// TopicConfigChanged 某个模块的配置被修改。载荷：ConfigChanged
	// 计划用于：v1.2 的配置导入导出后让各模块热加载，以及操作审计。
	TopicConfigChanged Topic = "core.config_changed"

	// TopicServiceExposed 一个新服务被开放到公网。载荷：ServiceExposed
	// 计划用于：v1.3 的安全中心 —— 对「把敏感服务暴露出去」给出提醒。
	TopicServiceExposed Topic = "stun.service_exposed"
)

// TunnelStateChanged 载荷：洞的连通状态变化。
type TunnelStateChanged struct {
	DeviceID  uint `json:"deviceId"`
	ServiceID uint `json:"serviceId"`
	// Address 洞当前的公网地址，形如 "1.2.3.4:56789"。
	Address string `json:"address,omitempty"`
	// Reason 断开原因，仅 TopicTunnelDown 有意义。
	Reason string `json:"reason,omitempty"`
}

// CertExpiring 载荷：证书即将到期。
type CertExpiring struct {
	CertID    uint   `json:"certId"`
	Domain    string `json:"domain"`
	ExpiresAt string `json:"expiresAt"` // RFC3339
	DaysLeft  int    `json:"daysLeft"`
}

// CertIssued 载荷：证书签发/续期成功。
type CertIssued struct {
	CertID  uint   `json:"certId"`
	Domain  string `json:"domain"`
	Renewal bool   `json:"renewal"`
}

// ConfigChanged 载荷：配置变更。
type ConfigChanged struct {
	// Module 变更所属模块名，与 component.Name() 一致。
	Module string `json:"module"`
	// Keys 发生变化的字段名列表；为空表示整体替换。
	Keys []string `json:"keys,omitempty"`
}

// ServiceExposed 载荷：新服务被开放到公网。
type ServiceExposed struct {
	DeviceID  uint   `json:"deviceId"`
	ServiceID uint   `json:"serviceId"`
	Name      string `json:"name"`
	// LocalTarget 内网目标，例如 "192.168.1.10:8080"。注意可能属于敏感信息，
	// 订阅方若要把载荷写进日志或通知，请先脱敏。
	LocalTarget string `json:"localTarget"`
}

// SubscribeTyped 类型安全的订阅：订阅时即校验载荷类型。
//
// 好处是把「载荷类型写错」从**运行时收不到事件**提前到了**启动时报错**。
// 由于 Go 的方法不能带自己的类型参数，需要指定总线时用 SubscribeTypedOn。
func SubscribeTyped[T any](topic Topic, h func(ctx context.Context, p T) error) (func(), error) {
	return SubscribeTypedOn(DefaultBus, topic, h)
}

// SubscribeTypedOn 在指定总线上做类型安全的订阅。
func SubscribeTypedOn[T any](b *Bus, topic Topic, h func(ctx context.Context, p T) error) (func(), error) {
	if h == nil {
		return nil, fmt.Errorf("bus: 主题 %s 的处理器为 nil", topic)
	}

	var zero T
	switch any(zero).(type) {
	case ServiceDeleted, PublicIPChanged, TunnelStateChanged,
		CertExpiring, CertIssued, ConfigChanged, ServiceExposed:
	default:
		return nil, fmt.Errorf("bus: 主题 %s 声明了未登记的载荷类型 %T，"+
			"请先在 core/bus/topics.go 里声明该主题与载荷类型", topic, zero)
	}

	cancel := b.Subscribe(topic, func(ctx context.Context, payload any) error {
		p, ok := payload.(T)
		if !ok {
			return fmt.Errorf("bus: 主题 %s 期望载荷 %T，实际收到 %T", topic, zero, payload)
		}
		return h(ctx, p)
	})
	return cancel, nil
}
