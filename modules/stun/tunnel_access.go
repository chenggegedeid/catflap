package stun

import (
	"net"
	"strconv"
	"strings"
	"time"

	"github.com/chenggegedeid/catflap/modules/audit"
	"github.com/chenggegedeid/catflap/modules/stun/model"
)

// tunnelAccessAllowed 穿透洞口：这个来源能不能连进来
//
// 和反代站点的门禁是同一套语义（proxy/model.AccessControl.Allows），
// 再加一层**全局封禁**（审计里拉黑的 IP，对穿透同样生效 —— 用户是在
// 审计页看着"这个 IP 在扫我"才封的，不该只拦 HTTP 不拦穿透）。
//
// 同时**记一笔访问**：以前穿透不记访问日志，出了事不知道谁来过。
// 现在每次连接（放行/拒绝）都记来源 IP，审计里查得到。
func tunnelAccessAllowed(req STUNRequest, remote net.Addr) bool {
	ip := addrIP(remote)

	svc := LookupService(req.DeviceID, req.ServiceID)

	// 全局封禁优先：审计里封了的，任何入口都不该放行
	blocked := false
	if ip != "" {
		if _, b := audit.IsBlocked(ip); b {
			blocked = true
		}
	}

	allowed := !blocked
	if allowed && svc != nil {
		allowed = svc.Access.Allows(ip)
	}

	recordTunnelAccess(req, ip, allowed)
	return allowed
}

// addrIP 从对端地址里取出 IP（去掉端口），取不到返回空串
func addrIP(a net.Addr) string {
	if a == nil {
		return ""
	}
	s := a.String()
	if h, _, err := net.SplitHostPort(s); err == nil {
		return h
	}
	return s
}

// recordTunnelAccess 记一笔穿透访问。
//
// 复用 audit 的存储，但穿透不是 HTTP —— Method/Host/Path 这些没有意义，
// 所以用一个 Ref 标出是哪个穿透服务（"s<设备ID>-<服务ID>"，和流量统计同一个键），
// Status 用 200 / 403 表示放行还是被门禁挡下。
func recordTunnelAccess(req STUNRequest, ip string, allowed bool) {
	if ip == "" || !audit.Enabled() {
		return
	}
	status := 200
	if !allowed {
		status = 403
	}
	audit.RecordAccess(audit.Record{
		Time:    time.Now(),
		IP:      ip,
		Status:  status,
		Kind:    "tunnel",
		Ref:     "s" + itoa(uint64(req.DeviceID)) + "-" + itoa(uint64(req.ServiceID)),
		SiteID:  0,
		Method:  "TUNNEL",
		BytesIn: 0,
	})
}

func itoa(v uint64) string {
	return strings.TrimSpace(strconv.FormatUint(v, 10))
}

// LookupService 按 设备ID + 服务ID 找到服务配置（拿它的门禁规则）
func LookupService(deviceID, serviceID uint) *model.Service {
	for i := range Runtime.Config.Services {
		s := &Runtime.Config.Services[i]
		if s.DeviceID == deviceID && s.ID == serviceID {
			return s
		}
	}
	return nil
}
