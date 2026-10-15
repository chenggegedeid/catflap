package proxy_api

import (
	"github.com/chenggegedeid/catflap/middleware"
	"github.com/chenggegedeid/catflap/modules/proxy"
	proxymodel "github.com/chenggegedeid/catflap/modules/proxy/model"
	"github.com/chenggegedeid/catflap/utils/res"

	"github.com/gin-gonic/gin"
)

// TunnelSaveRequest 保存 Cloudflare Tunnel 的设置
type TunnelSaveRequest struct {
	Enabled    bool   `json:"enabled"`
	BinaryPath string `json:"binaryPath"`
	// Token 留空表示"不改动"：面板上回显的是掩码，用户没重新填时不会把
	// 原来的 token 冲掉。要清空得显式传一个 clearToken。
	Token      string `json:"token"`
	ClearToken bool   `json:"clearToken"`
	ExtraArgs  string `json:"extraArgs"`
}

// ProxyTunnelSaveView 保存隧道设置，并立刻让运行状态跟上。
//
// token 是这条隧道的全部凭证，日志里绝不打印；响应里也只回 tokenSet。
func (ProxyApi) ProxyTunnelSaveView(c *gin.Context) {
	cr := middleware.GetBindRequest[TunnelSaveRequest](c)

	err := proxy.UpdateTunnel(func(cfg *proxymodel.TunnelConfig) {
		cfg.Enabled = cr.Enabled
		cfg.BinaryPath = cr.BinaryPath
		cfg.ExtraArgs = cr.ExtraArgs
		if cr.ClearToken {
			cfg.Token = ""
			return
		}
		if t := trim(cr.Token); t != "" && !isMaskedToken(t) {
			cfg.Token = t
		}
	})
	if err != nil {
		res.FailWithMsg("保存隧道配置失败："+err.Error(), c)
		return
	}

	// 保存完立刻生效：开就拉起来，关就停掉。用户点保存就该看到状态变，
	// 不用等重启服务。
	proxy.SyncTunnel()
	res.OkWithData(proxy.TunnelState(), c)
}

// ProxyTunnelRestartView 手动重启隧道。
// 改完 CF 那边的隧道配置、或者日志里看到"已退出"时用它。
func (ProxyApi) ProxyTunnelRestartView(c *gin.Context) {
	proxy.StopTunnel()
	if err := proxy.StartTunnel(); err != nil {
		res.FailWithMsg(err.Error(), c)
		return
	}
	res.OkWithData(proxy.TunnelState(), c)
}

// isMaskedToken 面板回显 token 时给的是掩码（一串圆点），用户没改就原样提交回来。
// 这种值不能当成真 token 存进去。
func isMaskedToken(s string) bool {
	for _, r := range s {
		if r != '•' && r != '*' {
			return false
		}
	}
	return true
}

func trim(s string) string {
	// 不想为一个 trim 引 strings：只处理 token/路径首尾的空白，避免用户
	// 从终端复制时带上换行
	for len(s) > 0 && (s[0] == ' ' || s[0] == '\n' || s[0] == '\r' || s[0] == '\t') {
		s = s[1:]
	}
	for len(s) > 0 && (s[len(s)-1] == ' ' || s[len(s)-1] == '\n' || s[len(s)-1] == '\r' || s[len(s)-1] == '\t') {
		s = s[:len(s)-1]
	}
	return s
}
