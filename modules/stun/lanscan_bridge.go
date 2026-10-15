package stun

import (
	"strings"

	"github.com/chenggegedeid/catflap/core"
	"github.com/chenggegedeid/catflap/core/lanscan"
)

// ===================== 给 core/lanscan 注入 stun 的业务知识 =====================
//
// 扫描的实现在 core/lanscan（纯网络探测，反代、穿透都能用）。但它需要两条
// 只有 stun 才知道的信息，所以由 stun 这边注入：
//
//   - 本机主 IP：用来把"你正待着的那一段"排在最前
//   - 已添加的设备：扫描结果里标出"已经在列表里"，
//     免得用户照着结果又加一遍（会出现两个同 IP 的设备）
//
// 依赖方向是 stun → core，core 不认识任何业务模块。

func init() {
	lanscan.LocalIPFunc = func() string {
		return Runtime.Network.LocalIP
	}

	lanscan.KnownHostsFunc = func() map[string]string {
		out := map[string]string{}
		for _, d := range Runtime.Config.Devices {
			ip := strings.TrimSpace(d.IP)
			if ip == "" {
				continue
			}
			out[ip] = d.Name
		}
		return out
	}
}

// AdminPort Catflap 后台自己占的端口。
//
// 给 api 层做校验用：穿透服务的内网端口不能是它（否则等于把管理后台
// 穿透到公网），代理站点的后端地址也不能指向它（同一个道理）。
func AdminPort() int { return core.ListenPort() }
